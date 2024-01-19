package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/slarwise/yamlls/pkg/ast"
	"github.com/slarwise/yamlls/pkg/lsp"
	"github.com/slarwise/yamlls/pkg/messages"
	"github.com/slarwise/yamlls/pkg/schemas"
	"go.lsp.dev/protocol"
	"gopkg.in/yaml.v3"
)

func main() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		slog.Error("Failed to locate user's cache directory", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(path.Join(cacheDir, "yamlls"), 0755); err != nil {
		slog.Error("Failed to create `yamlls` dir in cache directory", "cache_dir", cacheDir, "error", err)
		os.Exit(1)
	}
	logpath := path.Join(cacheDir, "yamlls", "log")
	logfile, err := os.Create(logpath)
	if err != nil {
		slog.Error("Failed to create log output file", "error", err)
		os.Exit(1)
	}
	defer logfile.Close()
	logger := slog.New(slog.NewJSONHandler(logfile, nil))
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic", "recovered", r)
		}
	}()

	schemasDir := path.Join(cacheDir, "yamlls", "schemas")
	if err := os.MkdirAll(schemasDir, 0755); err != nil {
		logger.Error("Failed to create `yamlls/schemas` dir in cache directory", "cache_dir", cacheDir, "error", err)
		os.Exit(1)
	}
	schemaStore, err := schemas.NewSchemaStore(logger, schemasDir, "https://raw.githubusercontent.com")
	if err != nil {
		logger.Error("Failed to create schema store", "error", err)
		os.Exit(1)
	}

	m := lsp.NewMux(logger, os.Stdin, os.Stdout)

	fileURIToContents := map[string]string{}

	m.HandleMethod("initialize", func(params json.RawMessage) (any, error) {
		var initializeParams messages.InitializeParams
		if err = json.Unmarshal(params, &initializeParams); err != nil {
			return nil, err
		}
		logger.Info("Received initialize request", "params", initializeParams)

		result := messages.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				TextDocumentSync:   messages.TextDocumentSyncKindFull,
				CompletionProvider: &protocol.CompletionOptions{TriggerCharacters: []string{":"}},
				HoverProvider:      true,
				CodeActionProvider: true,
				ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
					Commands: []string{"Open"},
				},
			},
			ServerInfo: &messages.ServerInfo{
				Name: "yamlls",
			},
		}
		return result, nil
	})

	m.HandleNotification("initialized", func(params json.RawMessage) error {
		logger.Info("Receivied initialized notification", "params", params)
		return nil
	})

	m.HandleMethod("shutdown", func(params json.RawMessage) (any, error) {
		logger.Info("Received shutdown request")
		return nil, nil
	})

	exitChannel := make(chan int, 1)
	m.HandleNotification("exit", func(params json.RawMessage) error {
		logger.Info("Received exit notification")
		exitChannel <- 1
		return nil
	})

	documentUpdates := make(chan messages.TextDocumentItem, 10)
	go func() {
		for doc := range documentUpdates {
			fileURIToContents[doc.URI] = doc.Text
			logger.Info("In channel goroutine", "fileURIToContents", fileURIToContents)
			diagnostics := []messages.Diagnostic{}
			m.Notify(messages.PublishDiagnosticsMethod, messages.PublishDiagnosticsParams{
				URI:         doc.URI,
				Version:     &doc.Version,
				Diagnostics: diagnostics,
			})
		}
	}()

	m.HandleNotification(messages.DidOpenTextDocumentNotification, func(rawParams json.RawMessage) error {
		logger.Info("Received didOpenTextDocument notification")
		var params messages.DidOpenTextDocumentParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return err
		}
		documentUpdates <- params.TextDocument
		return nil
	})

	m.HandleNotification(messages.DidChangeTextDocumentNotification, func(rawParams json.RawMessage) error {
		logger.Info("Received didChangeTextDocument notification")
		var params messages.DidChangeTextDocumentParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return err
		}

		documentUpdates <- messages.TextDocumentItem{
			URI:     params.TextDocument.URI,
			Version: params.TextDocument.Version,
			Text:    params.ContentChanges[0].Text,
		}

		return nil
	})

	m.HandleMethod("textDocument/hover", func(rawParams json.RawMessage) (any, error) {
		logger.Info("Received textDocument/hover request")
		var params messages.HoverParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		text := fileURIToContents[params.TextDocument.URI]
		kind, apiVersion, found := schemas.GetKindApiVersion([]byte(text))
		if !found {
			logger.Error("Failed to get kind and apiVersion", "text", string(text))
			return nil, errors.New("Not found")
		}
		yamlPath, err := ast.GetPathAtPosition(params.Position.Line+1, params.Position.Character+1, text)
		if err != nil {
			logger.Error("Failed to get path at position", "line", params.Position.Line, "column", params.Position.Character)
			return nil, errors.New("Not found")
		}
		description, found := schemaStore.GetDescriptionFromKindApiVersion(kind, apiVersion, yamlPath)
		if !found {
			logger.Error("Failed to get description", "kind", kind, "apiVersion", apiVersion, "yamlPath", yamlPath)
			return nil, errors.New("Not found")
		}
		return messages.HoverResult{
			Contents: messages.MarkupContent{
				Kind:  "markdown",
				Value: description,
			},
		}, nil
	})

	m.HandleMethod("textDocument/completion", func(rawParams json.RawMessage) (any, error) {
		logger.Info("Received textDocument/completion request")
		var params messages.CompletionParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		text := fileURIToContents[params.TextDocument.URI]
		textBeforeCursor := strings.Split(text, "\n")
		textBeforeCursor = textBeforeCursor[:params.Position.Line]
		kind, apiVersion, found := schemas.GetKindApiVersion([]byte(strings.Join(textBeforeCursor, "\n")))
		if !found {
			logger.Error("Failed to get kind and apiVersion")
			return nil, errors.New("Not found")
		}
		// TODO: This fails when there is a syntax error, which it will be
		// when you haven't finished writing the field name. Perhaps get the
		// node with one less indent?
		yamlPath, err := ast.GetPathAtPosition(params.Position.Line+1, params.Position.Character+1, text)
		if err != nil {
			logger.Error("Failed to get path at position", "line", params.Position.Line, "column", params.Position.Character)
			return nil, errors.New("Not found")
		}
		parentPath := schemas.GetPathToParent(yamlPath)
		logger.Info("Computed parent path", "parent_path", parentPath)
		properties, found := schemaStore.GetPropertiesFromKindApiVersion(kind, apiVersion, parentPath)
		if !found {
			logger.Error("Failed to get properties", "yaml_path", yamlPath)
			return nil, errors.New("Not found")
		}
		result := messages.CompletionResult{}
		for _, p := range properties {
			result = append(result, messages.CompletionItem{
				Label: p,
				Documentation: messages.MarkupContent{
					Kind:  "markdown",
					Value: "TODO: Description",
				},
			})
		}
		return result, nil
	})

	m.HandleMethod(protocol.MethodTextDocumentCodeAction, func(rawParams json.RawMessage) (any, error) {
		logger.Info(fmt.Sprintf("Received %s request", protocol.MethodTextDocumentCodeAction))
		var params protocol.CodeActionParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		text := fileURIToContents[string(params.TextDocument.URI)]
		textBeforeCursor := strings.Split(text, "\n")
		logger.Info("Code action", "start_range", params.Range.Start, "lines_in_document", len(textBeforeCursor), "filename", params.TextDocument.URI, "map", fileURIToContents)
		textBeforeCursor = textBeforeCursor[:params.Range.Start.Line]
		kind, apiVersion, found := schemas.GetKindApiVersion([]byte(strings.Join(textBeforeCursor, "\n")))
		if !found {
			logger.Error("Failed to get kind and apiVersion")
			return nil, errors.New("Not found")
		}
		schemaURL := getExternalDocumentation(kind, apiVersion)
		if schemaURL == "" {
			return nil, err
		}
		viewerUrl := "https://json-schema.app/view/" + url.PathEscape("#") + "?url=" + url.QueryEscape(schemaURL)
		response := []protocol.CodeAction{
			{
				Title: "Open external documentation",
				Command: &protocol.Command{
					Title:     "Open external documentation",
					Command:   "external-docs",
					Arguments: []interface{}{viewerUrl},
				},
			},
		}
		return response, nil
	})

	m.HandleMethod(protocol.MethodWorkspaceExecuteCommand, func(rawParams json.RawMessage) (any, error) {
		logger.Info(fmt.Sprintf("Received %s request", protocol.MethodWorkspaceExecuteCommand))
		var params protocol.ExecuteCommandParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		logger.Info("Received command", "command", params.Command, "args", params.Arguments)
		switch params.Command {
		case "external-docs":
			viewerURL := params.Arguments[0].(string)
			// TODO: Use showDocument instead
			// Currently not in a Helix release, it was added on Jan 17
			// https://github.com/helix-editor/helix/pull/8865
			// showDocumentParams := protocol.ShowDocumentParams{
			// 	URI:       uri.New(viewerURL),
			// 	External:  true,
			// 	TakeFocus: true,
			// }
			// m.Request("window/showDocument", showDocumentParams)
			if err = exec.Command("open", viewerURL).Run(); err != nil {
				logger.Error("Failed to execute command", "error", err)
			}
		default:
			return "", fmt.Errorf("Command not found %s", params.Command)
		}
		return "", nil
	})

	logger.Info("Handler set up", "log_path", logpath)

	go func() {
		if err := m.Process(); err != nil {
			logger.Error("Processing stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-exitChannel
	logger.Info("Server exited")
	os.Exit(1)
}

func ptr[T any](v T) *T {
	return &v
}

func getKindAndApiVersion(text string) (string, string, bool) {
	parsed := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(text), &parsed)
	if err != nil {
		return "", "", false
	}
	kind, found := parsed["kind"]
	if !found {
		return "", "", false
	}
	apiVersion, found := parsed["apiVersion"]
	if !found {
		return "", "", false
	}
	return kind.(string), apiVersion.(string), true
}

func getKind(text string) []messages.Diagnostic {
	parsed := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(text), &parsed)
	if err != nil {
		return nil
	}
	kind, found := parsed["kind"]
	if !found {
		return nil
	}

	d := messages.Diagnostic{
		Range: messages.Range{
			Start: messages.NewPosition(0, 0),
			End:   messages.NewPosition(0, 0),
		},
		Severity: ptr(messages.DiagnosticSeverityInformation),
		Message:  fmt.Sprintf("The current kind for the document is %s", kind),
	}
	diagnostics := []messages.Diagnostic{}
	return append(diagnostics, d)
}

func getDocs(text string) []messages.Diagnostic {
	parsed := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(text), &parsed)
	if err != nil {
		return nil
	}
	kind, found := parsed["kind"]
	if !found {
		return nil
	}
	apiVersion, found := parsed["apiVersion"]
	if !found {
		return nil
	}
	schemaURL := getExternalDocumentation(kind.(string), apiVersion.(string))
	if schemaURL == "" {
		return nil
	}
	viewerUrl := "https://json-schema.app/view/" + url.PathEscape("#") + "?url=" + url.QueryEscape(schemaURL)
	d := messages.Diagnostic{
		Range: messages.Range{
			Start: messages.NewPosition(0, 0),
			End:   messages.NewPosition(1, 0),
		},
		Severity: ptr(messages.DiagnosticSeverityHint),
		Message:  fmt.Sprintf("Docs: %s", viewerUrl),
	}
	return []messages.Diagnostic{d}
}

func getExternalDocumentation(kind string, apiVersion string) string {
	// I think CRD's have dots in the apiVersion
	if strings.Contains(apiVersion, ".") {
		// CRD
		split := strings.Split(apiVersion, "/")
		if len(split) != 2 {
			return ""
		}
		host := split[0]
		version := split[1]
		url := fmt.Sprintf("https://github.com/datreeio/CRDs-catalog/main/%s/%s_%s.json", host, kind, version)
		return url
	} else {
		// Built-in
		url := fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/%s-%s.json", strings.ToLower(kind), strings.ReplaceAll(apiVersion, "/", "-"))
		return url
	}
}

func getCurrentWord(position messages.Position, text string) string {
	wordPattern := regexp.MustCompile("[a-zA-Z]")
	lines := strings.Split(text, "\n")
	line := lines[position.Line]
	char := position.Character
	leftChar := char
	if !wordPattern.Match([]byte{line[char]}) {
		return ""
	}
	for {
		if leftChar == 0 {
			break
		}
		if !wordPattern.Match([]byte{line[leftChar-1]}) {
			break
		}
		leftChar -= 1
	}
	rightChar := char
	for {
		if rightChar == len(line)-1 {
			break
		}
		if !wordPattern.Match([]byte{line[rightChar+1]}) {
			break
		}
		rightChar += 1
	}
	return line[leftChar : rightChar+1]
}
