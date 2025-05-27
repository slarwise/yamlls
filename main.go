package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/slarwise/yamlls/internal/lsp"
	"github.com/slarwise/yamlls/internal/parser"
	"github.com/slarwise/yamlls/internal/schemas"
	parser2 "github.com/slarwise/yamlls/pkg/parser"
	schemas2 "github.com/slarwise/yamlls/pkg/schemas"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
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

	schemasDir := path.Join(cacheDir, "yamlls", "schemas")
	if err := os.MkdirAll(schemasDir, 0755); err != nil {
		logger.Error("Failed to create `yamlls/schemas` dir in cache directory", "cache_dir", cacheDir, "error", err)
		os.Exit(1)
	}
	schemaStore, err := schemas.NewSchemaStore(schemasDir, logger)
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
		switch v := initializeParams.InitializationOptions.(type) {
		case map[string]interface{}:
			if overrides, found := v["filenameOverrides"]; found {
				overrides, ok := overrides.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("filenameOverrides must be a an object with strings as keys and strings as values")
				}
				parsedOverrides := map[string]string{}
				for key, val := range overrides {
					if val, ok := val.(string); ok {
						parsedOverrides[key] = val
					} else {
						if !ok {
							return nil, fmt.Errorf("filenameOverrides must be a an object with strings as keys and strings as values")
						}
					}
				}
				if ok {
					schemaStore.AddFilenameOverrides(parsedOverrides)
					logger.Info("Added filename overrides")
				}
			}
		}

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
			diagnostics := validateFile(doc.Text)
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
		yamlDocuments := parser.SplitIntoYamlDocuments(text)
		currentDocument := ""
		lineOffset := 0
		for _, d := range yamlDocuments {
			documentLines := len(strings.Split(d, "\n"))
			if int(params.Position.Line) <= lineOffset+documentLines {
				currentDocument = d
				break
			}
			lineOffset += documentLines
		}
		if currentDocument == "" {
			logger.Error("Failed to find corresponding yaml document from position", "positionLine", params.Position.Line, "nDocuments", len(yamlDocuments))
			return nil, errors.New("Not found")
		}
		schema, err := schemaStore.GetSchema(params.TextDocument.URI.Filename(), currentDocument)
		if err != nil {
			logger.Error("Could not find schema", "filename", params.TextDocument.URI.Filename(), "error", err)
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
		schema, err := schemaStore.GetSchema(params.TextDocument.URI.Filename(), text)
		if err != nil {
			logger.Error("Could not find schema", "filename", params.TextDocument.URI.Filename(), "error", err)
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
		yamlDocuments := parser.SplitIntoYamlDocuments(text)
		currentDocument := ""
		lineOffset := 0
		var documentStart, documentEnd int
		for _, d := range yamlDocuments {
			documentLines := len(strings.Split(d, "\n"))
			if int(params.Range.Start.Line) <= lineOffset+documentLines-1 {
				currentDocument = d
				documentStart = lineOffset
				documentEnd = lineOffset + documentLines - 1
				break
			}
			lineOffset += documentLines - 1
		}
		if currentDocument == "" {
			logger.Error("Failed to find corresponding yaml document from position", "positionLine", params.Range.Start.Line, "nDocuments", len(yamlDocuments))
			return nil, errors.New("Not found")
		}
		logger.Info("Current document", "current", currentDocument)
		schemaURL, err := schemaStore.GetSchemaURL(params.TextDocument.URI.Filename(), currentDocument)
		if err != nil {
			logger.Error("Could not find schema URL", "filename", params.TextDocument.URI.Filename(), "error", err)
			return nil, errors.New("Not found")
		}
		viewerURL := schemas.DocsViewerURL(schemaURL)

		response := []protocol.CodeAction{
			{
				Title: "Open documentation",
				Command: &protocol.Command{
					Title:     "Open documentation",
					Command:   "external-docs",
					Arguments: []any{viewerURL},
				},
			},
			{
				Title: "Fill document",
				Command: &protocol.Command{
					Title:     "Fill document",
					Command:   "fill-document",
					Arguments: []any{params.TextDocument.URI, currentDocument, false, params.Range.Start.Line, params.Range.Start.Character, documentStart, documentEnd},
				},
			},
			{
				Title: "Fill document with required fields",
				Command: &protocol.Command{
					Title:     "Fill document",
					Command:   "fill-document",
					Arguments: []any{params.TextDocument.URI, currentDocument, true, params.Range.Start.Line, params.Range.Start.Character, documentStart, documentEnd},
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
			showDocumentParams := protocol.ShowDocumentParams{
				URI:       uri.URI(viewerURL),
				External:  true,
				TakeFocus: true,
			}
			m.Request("window/showDocument", showDocumentParams)
		case "fill-document":
			// if len(params.Arguments) != 7 {
			// 	logger.Info("Expected 7 arguments to fill-document")
			// 	return "", fmt.Errorf("Expected 7 arguments to fill-document")
			// }
			// uri_ := params.Arguments[0].(string)
			// uri := protocol.DocumentURI(uri_)
			// currentDocument := params.Arguments[1].(string)
			// requiredOnly := params.Arguments[2].(bool)
			// line := uint32(params.Arguments[3].(float64))
			// column := uint32(params.Arguments[4].(float64))
			// documentStart := params.Arguments[5].(float64)
			// documentEnd := params.Arguments[6].(float64)
			// yamlPath, err := parser.GetPathAtPosition(line, column, currentDocument)
			// if err != nil {
			// 	logger.Error("Failed to get path at position", "line", line, "column", column)
			// 	return nil, errors.New("Not found")
			// }
			// schema, err := schemaStore.GetSchema(uri.Filename(), currentDocument)
			// if err != nil {
			// 	return nil, fmt.Errorf("get schema to fill document: %v", err)
			// }
			// subSchema, found := parser.GetSubSchema(yamlPath, schema)
			// if !found {
			// 	return nil, fmt.Errorf("could not find schema on path: %v", err)
			// }
			// fullDoc, err := fillDocument(subSchema, requiredOnly)
			// if err != nil {
			// 	return nil, fmt.Errorf("fill document: %v", err)
			// }
			// // TODO: We only want to touch the node that we are filling, not anything else in
			// // the document. It works pretty well to update the whole thing but it might change
			// // the order of the keys in other parts of the document. So insert only at the current
			// // line.
			// jsonPath := strings.TrimPrefix(yamlPath, "$.")
			// logger.Info("path", "json", jsonPath, "yaml", yamlPath, "current", currentDocument, "full", fullDoc)
			// currentDocumentJson, err := yaml.YAMLToJSON([]byte(currentDocument))
			// if err != nil {
			// 	return nil, fmt.Errorf("convert current document to json: %v", err)
			// }
			// logger.Info("path", "json", jsonPath, "yaml", yamlPath, "current", currentDocument, "full", fullDoc, "currentJson", currentDocumentJson)
			// updatedDoc, err := sjson.SetBytes([]byte(currentDocumentJson), jsonPath, fullDoc)
			// if err != nil {
			// 	return nil, fmt.Errorf("update current document: %v", err)
			// }
			// resultBytes, err := yaml.JSONToYAML(updatedDoc)
			// if err != nil {
			// 	return nil, fmt.Errorf("convert updated document to yaml: %v", err)
			// }
			// var resultYaml map[string]any
			// _ = yaml.Unmarshal(resultBytes, &resultYaml)
			// wellFormattedBytes, _ := yaml.MarshalWithOptions(resultYaml, yaml.IndentSequence(true))
			// applyParams := protocol.ApplyWorkspaceEditParams{
			// 	Edit: protocol.WorkspaceEdit{
			// 		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			// 			protocol.DocumentURI(uri): {
			// 				{
			// 					Range: protocol.Range{
			// 						Start: protocol.Position{Line: uint32(documentStart), Character: 0},
			// 						End:   protocol.Position{Line: uint32(documentEnd), Character: 0},
			// 					},
			// 					NewText: string(wellFormattedBytes),
			// 				},
			// 			},
			// 		},
			// 	},
			// }
			// m.Request("workspace/applyEdit", applyParams)
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

func validateFile(contents string) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}
	yamlDocs := parser.SplitIntoYamlDocuments(contents)
	lineOffset := 0
	for _, doc := range yamlDocs {
		lines := strings.FieldsFunc(doc, func(r rune) bool { return r == '\n' })
		linesCount := len(lines)
		startLine := lineOffset
		endLine := startLine + linesCount - 1
		logger.Info("lines", "startLine", startLine, "endLine", endLine)
		lineOffset += linesCount + 1 // Account for the --- line between documents
		if !parser2.DocumentIsValid([]byte(doc)) {
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
		kind, apiVersion, err := parser2.GetKindAndApiVersion([]byte(doc))
		if err != nil {
			logger.Error("should not get an error from GetKindAndApiVersion with valid yaml", "err", err)
		}
		var schemaUrl string
		var found bool
		if kind != "" && apiVersion != "" {
			schemaUrl, found = schemas2.GetKubernetesSchemaUrl(kind, apiVersion)
			if !found {
				continue
			}
		} else if kind != "" && apiVersion == "" {
			apiVersions := schemas2.GetApiVersions(kind)
			switch len(apiVersions) {
			case 0:
				continue
			case 1:
				schemaUrl, found = schemas2.GetKubernetesSchemaUrl(kind, apiVersion)
				if !found {
					continue
				}
			case 2:
				logger.Error("ambiguous apiVersions not supported")
			}
		} else {
			continue
		}
		schema, err := schemas2.LoadSchema(schemaUrl)
		if err != nil {
			logger.Error("get schema", "err", err)
			continue
		}
		errors, err := schemas2.ValidateYaml(schema, []byte(doc))
		if err != nil {
			logger.Error("validate against schema", "err", err)
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
	return diagnostics
}
