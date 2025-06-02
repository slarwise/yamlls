package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/slarwise/yamlls/internal/lsp"
	"github.com/slarwise/yamlls/pkg/parser"
	"github.com/slarwise/yamlls/pkg/schemas"

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
			diagnostics, err := validateFile(doc.Text)
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
		var params protocol.HoverParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		contents := filenameToContents[params.TextDocument.URI.Filename()]
		description, err := getDescription(contents, int(params.Position.Line), int(params.Position.Character))
		if err != nil {
			return nil, fmt.Errorf("get description: %v", err)
		}

		return protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.PlainText,
				Value: description,
			},
		}, nil
	})

	m.HandleMethod("textDocument/completion", func(rawParams json.RawMessage) (any, error) { panic("TODO: Support completion") })

	m.HandleMethod(protocol.MethodTextDocumentCodeAction, func(rawParams json.RawMessage) (any, error) { panic("TODO: Support code actions") })

	m.HandleMethod(protocol.MethodWorkspaceExecuteCommand, func(rawParams json.RawMessage) (any, error) { panic("TODO: Support execute command") })

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

func validateFile(contents string) ([]protocol.Diagnostic, error) {
	diagnostics := []protocol.Diagnostic{}
	yamlDocs := parser.SplitIntoDocuments(contents)
	lineOffset := 0
	for _, doc := range yamlDocs {
		lines := strings.FieldsFunc(doc, func(r rune) bool { return r == '\n' })
		linesCount := len(lines)
		startLine := lineOffset
		endLine := startLine + linesCount - 1
		lineOffset += linesCount + 1 // Account for the --- line between documents
		if !parser.DocumentIsValid([]byte(doc)) {
			diagnostics = append(diagnostics, protocol.Diagnostic{
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(startLine), Character: 0},
					End:   protocol.Position{Line: uint32(endLine), Character: uint32(len(lines[linesCount-1]))},
				},
				Severity: protocol.DiagnosticSeverityError,
				Source:   "yamlls",
				Message:  "invalid yaml",
			})
			continue
		}
		// TODO: Support getting schema from filename
		schema, found, err := getSchema(doc)
		if err != nil {
			return nil, fmt.Errorf("get schema: %v", err)
		}
		if !found {
			continue
		}
		errors, err := schemas.ValidateYaml(schema, []byte(doc))
		if err != nil {
			return nil, fmt.Errorf("validate against schema: %v", err)
		}
		for _, e := range errors {
			diagnostics = append(diagnostics, protocol.Diagnostic{
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(startLine + e.Line),
						Character: uint32(e.StartCol),
					},
					End: protocol.Position{
						Line:      uint32(startLine + e.Line),
						Character: uint32(e.EndCol),
					},
				},
				Severity: protocol.DiagnosticSeverityError,
				Source:   "yamlls",
				Message:  e.Description,
			})
		}
	}
	return diagnostics, nil
}

func getSchema(doc string) (map[string]any, bool, error) {
	// TODO: Support getting schema from filename
	kind, apiVersion, err := parser.GetKindAndApiVersion([]byte(doc))
	if err != nil {
		return nil, false, fmt.Errorf("get kind and apiVersion: %v", err)
	}
	var schemaUrl string
	var found bool
	if kind != "" && apiVersion != "" {
		schemaUrl, found = schemas.GetKubernetesSchemaUrl(kind, apiVersion)
		if !found {
			return nil, false, nil
		}
	} else if kind != "" && apiVersion == "" {
		apiVersions := schemas.GetApiVersions(kind)
		switch len(apiVersions) {
		case 0:
			return nil, false, nil
		case 1:
			schemaUrl, found = schemas.GetKubernetesSchemaUrl(kind, apiVersions[0])
			if !found {
				panic("got a suggested apiVersion but then couldn't find the url for it")
			}
		case 2:
			panic("ambiguous apiVersions not supported")
		}
	} else {
		return nil, false, fmt.Errorf("no kind found in document")
	}
	schema, err := schemas.LoadSchema(schemaUrl)
	if err != nil {
		return nil, false, fmt.Errorf("get schema: %v", err)
	}
	return schema, true, nil
}

func getDescription(contents string, line, char int) (string, error) {
	lineOffset := 0
	var currentDoc string
	var lineInDoc int
	for _, doc := range parser.SplitIntoDocuments(contents) {
		lines := strings.FieldsFunc(doc, func(r rune) bool { return r == '\n' })
		linesCount := len(lines)
		startLine := lineOffset
		endLine := startLine + linesCount - 1
		lineOffset += linesCount + 1 // Account for the --- line between documents
		if startLine <= line && line <= endLine {
			currentDoc = doc
		}
		lineInDoc = line - startLine
	}
	if currentDoc == "" {
		panic("could not find document from where hover was called")
	}
	schema, found, err := getSchema(currentDoc)
	if err != nil {
		return "", fmt.Errorf("get schema: %v", err)
	}
	if !found {
		return "", fmt.Errorf("not found")
	}
	path, err := parser.PathAtPosition([]byte(currentDoc), lineInDoc, int(char))
	if err != nil {
		return "", fmt.Errorf("get path at position: %v", err)
	}
	if path == "" {
		return "", fmt.Errorf("path not found")
	}
	bytes, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("marshal schema: %v", err)
	}
	description := schemas.GetDescription(bytes, path)
	if description == "" {
		return "", fmt.Errorf("not found")
	}
	return description, nil
}
