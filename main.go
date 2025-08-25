package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/slarwise/yamlls/internal/lsp"
	"github.com/slarwise/yamlls/pkg/schema2"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

var logger *slog.Logger

func main() {
	s := schema2.Schema{}
	bytes := []byte(`{
    "type": "object",
    "description": "the thing",
    "properties": {
    	"hej": { "description": "the hej", "type": "string" }
    }
}`)
	if err := json.Unmarshal(bytes, &s); err != nil {
		panic(err)
	}
	for _, p := range schema2.Docs2(s, bytes) {
		fmt.Printf("%+v\n", p)
	}
}

func main2() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		slog.Error("Failed to locate user's cache directory", "error", err)
		os.Exit(1)
	}
	yamllsCacheDir := filepath.Join(cacheDir, "yamlls")
	if err := os.MkdirAll(yamllsCacheDir, 0755); err != nil {
		slog.Error("Failed to create `yamlls` dir in cache directory", "cache_dir", cacheDir, "error", err)
		os.Exit(1)
	}
	logpath := filepath.Join(yamllsCacheDir, "log")
	logfile, err := os.Create(logpath)
	if err != nil {
		slog.Error("Failed to create log output file", "error", err)
		os.Exit(1)
	}

	kubernetesStore, err := schema2.NewKubernetesStore()
	if err != nil {
		slog.Error("create kubernetes store", "err", err)
		os.Exit(1)
	}

	defer logfile.Close()
	logger = slog.New(slog.NewJSONHandler(logfile, nil))
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic", "recovered", r)
		}
	}()

	m := lsp.NewMux(logger, os.Stdin, os.Stdout)

	filenameToContents := map[string]string{}

	m.HandleMethod("initialize", func(params json.RawMessage) (any, error) {
		var initializeParams protocol.InitializeParams
		if err = json.Unmarshal(params, &initializeParams); err != nil {
			return nil, err
		}
		logger.Info("Received initialize request", "params", initializeParams)
		// TODO: Support filenameOverrides

		result := protocol.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				TextDocumentSync:   protocol.TextDocumentSyncKindFull,
				HoverProvider:      true,
				CodeActionProvider: true,
				ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
					Commands: []string{"open-docs"},
				},
			},
			ServerInfo: &protocol.ServerInfo{Name: "yamlls"},
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
			diagnostics, err := validateFile(doc.Text, kubernetesStore)
			if err != nil {
				logger.Error("validate file", "err", err)
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
		documentUpdates <- protocol.TextDocumentItem{URI: params.TextDocument.URI, Version: params.TextDocument.Version, Text: params.ContentChanges[0].Text}
		return nil
	})

	m.HandleMethod("textDocument/hover", func(rawParams json.RawMessage) (any, error) {
		logger.Info("Received textDocument/hover request")
		var params protocol.HoverParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		contents := filenameToContents[params.TextDocument.URI.Filename()]
		documentation, err := kubernetesStore.DocumentationAtCursor(contents, int(params.Position.Line), int(params.Position.Character))
		if err != nil {
			logger.Error("failed to get description", "line", params.Position.Line, "char", params.Position.Character, "err", err)
			return nil, nil
		} else if documentation.Description == "" {
			return nil, nil
		}

		return protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.PlainText,
				Value: documentation.Description,
			},
		}, nil
	})

	m.HandleMethod("textDocument/completion", func(rawParams json.RawMessage) (any, error) {
		logger.Info("Receiver textDocument/completion request, not supported")
		return nil, nil
	})

	m.HandleMethod(protocol.MethodTextDocumentCodeAction, func(rawParams json.RawMessage) (any, error) {
		logger.Info("Received textDocument/codeAction request, not supported")
		return nil, nil
	})

	m.HandleMethod(protocol.MethodTextDocumentCodeAction, func(rawParams json.RawMessage) (any, error) {
		logger.Info(fmt.Sprintf("Received %s request", protocol.MethodTextDocumentCodeAction))
		var params protocol.CodeActionParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		contents := filenameToContents[params.TextDocument.URI.Filename()]
		documentation, found := kubernetesStore.HtmlDocumentation(contents, int(params.Range.Start.Line), int(params.Range.Start.Character))
		if !found {
			return "", errors.New("no schema found")
		}
		filename := filepath.Join(cacheDir, "docs.html")
		if err := os.WriteFile(filename, []byte(documentation), 0755); err != nil {
			slog.Error("write html documentation to file", "err", err, "file", filename)
			return "", errors.New("failed to write docs to file")
		}
		htmlDocsUri := "file://" + filename
		response := []protocol.CodeAction{
			{
				Title: "Open documentation",
				Command: &protocol.Command{
					Title:     "Open documentation",
					Command:   "open-docs",
					Arguments: []any{htmlDocsUri},
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
		case "open-docs":
			if len(params.Arguments) != 1 {
				return "", fmt.Errorf("Must provide 1 argument to open-docs, the uri")
			}
			viewerURL := params.Arguments[0].(string)
			uri := uri.URI(viewerURL)
			showDocumentParams := protocol.ShowDocumentParams{
				URI:       uri,
				External:  true,
				TakeFocus: true,
			}
			m.Request("window/showDocument", showDocumentParams)
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

func validateFile(contents string, store schema2.KubernetesStore) ([]protocol.Diagnostic, error) {
	errors := store.ValidateFile(contents)
	diagnostics := []protocol.Diagnostic{}
	for _, e := range errors {
		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(e.Range.Start.Line),
					Character: uint32(e.Range.Start.Char),
				},
				End: protocol.Position{
					Line:      uint32(e.Range.End.Line),
					Character: uint32(e.Range.End.Char),
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "yamlls",
			Message:  e.Message,
		})
	}
	return diagnostics, nil
}
