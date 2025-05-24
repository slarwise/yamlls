package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/slarwise/yamlls/internal/lsp"
	"github.com/slarwise/yamlls/pkg/schema2"

	"go.lsp.dev/protocol"
)

var logger *slog.Logger

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
					Commands: []string{"external-docs"},
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
		description, err := getDescription(contents, int(params.Position.Line), int(params.Position.Character), kubernetesStore)
		if err != nil {
			logger.Error("failed to get description", "line", params.Position.Line, "char", params.Position.Character, "err", err)
			return nil, nil
		} else if description == "" {
			return nil, nil
		}

		return protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.PlainText,
				Value: description,
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

func validateFile(contents string, store schema2.Store) ([]protocol.Diagnostic, error) {
	errors := schema2.ValidateFile(contents, store)
	diagnostics := []protocol.Diagnostic{}
	for _, e := range errors {
		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(e.Position.LineStart),
					Character: uint32(e.Position.CharStart),
				},
				End: protocol.Position{
					Line:      uint32(e.Position.LineEnd),
					Character: uint32(e.Position.CharEnd),
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "yamlls",
			Message:  e.Message,
		})
	}
	return diagnostics, nil
}

var arrayPath = regexp.MustCompile(`\.\d+`)

func getDescription(contents string, line, char int, store schema2.Store) (string, error) {
	ranges := schema2.GetDocumentPositions(contents)
	var maybeValidDocument string
	for _, r := range ranges {
		if r.Start <= line && line < r.End {
			lines := strings.FieldsFunc(contents, func(r rune) bool { return r == '\n' })
			maybeValidDocument = strings.Join(lines[r.Start:r.End], "\n")
			line = line - r.Start
		}
	}
	document, valid := schema2.NewYamlDocument(maybeValidDocument)
	if !valid {
		return "", fmt.Errorf("current yaml document is invalid")
	}
	paths := document.Paths()
	path, found := paths.AtCursor(line, char)
	if !found {
		// Happens if the cursor is not on a property
		return "", fmt.Errorf("No yaml path found at position %d:%d. Paths: %v", line, char, paths)
	}
	schema, schemaFound := store.Get(string(document))
	if !schemaFound {
		return "", fmt.Errorf("no schema found for current document")
	}
	// Turn spec.ports.0.name into spec.ports[].name
	path = arrayPath.ReplaceAllString(path, "[]")
	pathFound := false
	var description string
	documentation := schema.Docs()
	for _, property := range documentation {
		if property.Path == path {
			description = property.Description
			pathFound = true
		}
	}
	if !pathFound {
		return "", fmt.Errorf("could not find path %s in documentation", path)
	}
	return description, nil
}
