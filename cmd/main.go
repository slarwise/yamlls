package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/slarwise/yamlls/internal/lsp"
	"github.com/slarwise/yamlls/internal/parser"
	"github.com/slarwise/yamlls/internal/schemas"

	"github.com/goccy/go-yaml"
	"github.com/xeipuuv/gojsonschema"
	"go.lsp.dev/protocol"
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
	schemaStore, err := schemas.NewSchemaStore(logger, schemasDir)
	if err != nil {
		logger.Error("Failed to create schema store", "error", err)
		os.Exit(1)
	}

	m := lsp.NewMux(logger, os.Stdin, os.Stdout)

	filenameToContents := map[string]string{}

	m.HandleMethod("initialize", func(params json.RawMessage) (any, error) {
		var initializeParams protocol.InitializeParams
		if err = json.Unmarshal(params, &initializeParams); err != nil {
			return nil, err
		}
		logger.Info("Received initialize request", "params", initializeParams)

		result := protocol.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				TextDocumentSync:   protocol.TextDocumentSyncKindFull,
				HoverProvider:      true,
				CodeActionProvider: true,
				ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
					Commands: []string{"external-docs"},
				},
			},
			ServerInfo: &protocol.ServerInfo{
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

	documentUpdates := make(chan protocol.TextDocumentItem, 10)
	go func() {
		for doc := range documentUpdates {
			filenameToContents[doc.URI.Filename()] = doc.Text
			logger.Info("In channel goroutine", "fileURIToContents", filenameToContents)
			diagnostics := []protocol.Diagnostic{}
			validYamlDiagnostics := isValidYaml(doc.Text)
			diagnostics = append(diagnostics, validYamlDiagnostics...)
			if len(validYamlDiagnostics) == 0 {
				diagnostics = append(diagnostics, validateAgainstSchema(schemaStore, doc.URI.Filename(), doc.Text)...)
			}
			m.Notify(protocol.MethodTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
				URI:         doc.URI,
				Version:     uint32(doc.Version),
				Diagnostics: diagnostics,
			})
		}
	}()

	m.HandleNotification(protocol.MethodTextDocumentDidOpen, func(rawParams json.RawMessage) error {
		logger.Info("Received TextDocument/didOpen notification")
		var params protocol.DidOpenTextDocumentParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return err
		}
		documentUpdates <- params.TextDocument
		return nil
	})

	m.HandleNotification(protocol.MethodTextDocumentDidChange, func(rawParams json.RawMessage) error {
		logger.Info("Received textDocument/didChange notification")
		var params protocol.DidChangeTextDocumentParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return err
		}

		documentUpdates <- protocol.TextDocumentItem{
			URI:     params.TextDocument.URI,
			Version: params.TextDocument.Version,
			Text:    params.ContentChanges[0].Text,
		}

		return nil
	})

	m.HandleMethod("textDocument/hover", func(rawParams json.RawMessage) (any, error) {
		logger.Info("Received textDocument/hover request")
		var params protocol.HoverParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		text := filenameToContents[params.TextDocument.URI.Filename()]
		schema, found := resolveSchema(schemaStore, params.TextDocument.URI.Filename(), text)
		if !found {
			logger.Error("Could not find schema", "filename", params.TextDocument.URI.Filename())
			return nil, errors.New("Not found")
		}
		yamlPath, err := parser.GetPathAtPosition(params.Position.Line, params.Position.Character, text)
		if err != nil {
			logger.Error("Failed to get path at position", "line", params.Position.Line, "column", params.Position.Character)
			return nil, errors.New("Not found")
		}
		description, found := parser.GetDescription(yamlPath, schema)
		if !found {
			logger.Error("Failed to get description", "yamlPath", yamlPath)
			return nil, errors.New("Not found")
		}
		return protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  "markdown",
				Value: description,
			},
		}, nil
	})

	m.HandleMethod("textDocument/completion", func(rawParams json.RawMessage) (any, error) {
		logger.Info("Received textDocument/completion request")
		var params protocol.CompletionParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		text := filenameToContents[params.TextDocument.URI.Filename()]
		schema, found := resolveSchema(schemaStore, params.TextDocument.URI.Filename(), text)
		if !found {
			logger.Error("Could not find schema", "filename", params.TextDocument.URI.Filename())
			return nil, errors.New("Not found")
		}
		// TODO: This fails when there is a syntax error, which it will be
		// when you haven't finished writing the field name. Perhaps get the
		// node with one less indent?
		yamlPath, err := parser.GetPathAtPosition(params.Position.Line, params.Position.Character, text)
		if err != nil {
			logger.Error("Failed to get path at position", "line", params.Position.Line, "column", params.Position.Character)
			return nil, errors.New("Not found")
		}
		parentPath := parser.GetPathToParent(yamlPath)
		logger.Info("Computed parent path", "parent_path", parentPath)
		properties, found := parser.GetProperties(parentPath, schema)
		if !found {
			logger.Error("Failed to get properties", "yaml_path", yamlPath)
			return nil, errors.New("Not found")
		}
		result := protocol.CompletionList{}
		for _, p := range properties {
			result.Items = append(result.Items, protocol.CompletionItem{
				Label: p,
				Documentation: protocol.MarkupContent{
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
		text := filenameToContents[params.TextDocument.URI.Filename()]
		schemaURL, found := resolveSchemaURL(schemaStore, params.TextDocument.URI.Filename(), text)
		if !found {
			logger.Error("Could not find schema", "filename", params.TextDocument.URI.Filename())
			return nil, errors.New("Not found")
		}
		viewerURL := schemas.DocsViewerURL(schemaURL)
		response := []protocol.CodeAction{
			{
				Title: "Open documentation",
				Command: &protocol.Command{
					Title:     "Open documentation",
					Command:   "external-docs",
					Arguments: []interface{}{viewerURL},
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
			if len(params.Arguments) != 1 {
				logger.Info("Must provide 1 argument to external-docs, the viewerURL")
				return "", fmt.Errorf("Must provide 1 argument to external-docs, the viewerURL")
			}
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

func resolveSchema(store schemas.SchemaStore, filename string, text string) ([]byte, bool) {
	kind, apiVersion := parser.GetKindApiVersion(text)
	if kind != "" && apiVersion != "" {
		schema, found := store.SchemaFromKindApiVersion(kind, apiVersion)
		if found {
			return schema, true
		}
	}
	schema, err := store.SchemaFromFilePath(filename)
	if err != nil {
		return []byte{}, false
	}
	return schema, true
}

func resolveSchemaURL(store schemas.SchemaStore, filename string, text string) (string, bool) {
	kind, apiVersion := parser.GetKindApiVersion(text)
	if kind != "" && apiVersion != "" {
		url, err := store.SchemaURLFromKindApiVersion(kind, apiVersion)
		if err == nil {
			return url, true
		}
	}
	url, err := store.SchemaURLFromFilePath(filename)
	if err != nil {
		return "", false
	}
	return url, true
}

func validateAgainstSchema(store schemas.SchemaStore, filename string, text string) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}
	schema, found := resolveSchema(store, filename, text)
	if !found {
		store.Logger.Error("Could not resolve schema")
		return diagnostics
	}
	jsonText, err := yaml.YAMLToJSON([]byte(text))
	if err != nil {
		store.Logger.Error("Failed to convert yaml to json")
		return diagnostics
	}
	schemaLoader := gojsonschema.NewBytesLoader(schema)
	documentLoader := gojsonschema.NewBytesLoader(jsonText)
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		store.Logger.Error("Failed to validate against schema", "error", err)
		return diagnostics
	}
	if result.Valid() {
		store.Logger.Info("No errors found when validating")
		return diagnostics
	}
	for _, e := range result.Errors() {
		store.Logger.Info("context", "context", e.Context(), "details", e.Details(), "field", e.Field(), "type", e.Type())
		d := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      0,
					Character: 0,
				},
				End: protocol.Position{
					Line:      1,
					Character: 0,
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "yamlls",
			Message:  e.Description(),
		}
		diagnostics = append(diagnostics, d)
	}
	return diagnostics
}

func isValidYaml(text string) []protocol.Diagnostic {
	ds := []protocol.Diagnostic{}
	var output interface{}
	lines := strings.Split(text, "\n")
	if err := yaml.Unmarshal([]byte(text), &output); err != nil {
		d := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      0,
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(len(lines) - 1),
					Character: uint32(len(lines[len(lines)-1])),
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "yamlls",
			Message:  "Invalid yaml",
		}
		ds = append(ds, d)
	}
	return ds
}
