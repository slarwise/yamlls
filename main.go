package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/slarwise/yamlls/internal/kustomization"
	"github.com/slarwise/yamlls/internal/lsp"
	"github.com/slarwise/yamlls/internal/parser"
	"github.com/slarwise/yamlls/internal/schemas"

	"github.com/goccy/go-yaml"
	"github.com/xeipuuv/gojsonschema"
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
			diagnostics := []protocol.Diagnostic{}
			validYamlDiagnostics := isValidYaml(doc.Text)
			diagnostics = append(diagnostics, validYamlDiagnostics...)
			if len(validYamlDiagnostics) == 0 {
				yamlDocuments := parser.SplitIntoYamlDocuments(doc.Text)
				lineOffset := 0
				for _, d := range yamlDocuments {
					schema, err := schemaStore.GetSchema(doc.URI.Filename(), d)
					if err != nil {
						logger.Error("Could not find schema", "filename", doc.URI.Filename(), "error", err)
					} else {
						validateDiagnostics, err := validateAgainstSchema(schema, d, uint32(lineOffset))
						if err != nil {
							logger.Error("Could not validate against schema", "error", err)
						} else {
							diagnostics = append(diagnostics, validateDiagnostics...)
						}
					}
					lineOffset += len(strings.Split(d, "\n"))
				}
			}
			if path.Base(doc.URI.Filename()) == "kustomization.yaml" {
				diagnostics = append(diagnostics, kustomizationForgottenResources(doc.URI.Filename(), doc.Text)...)
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
		for _, d := range yamlDocuments {
			documentLines := len(strings.Split(d, "\n"))
			if int(params.Range.Start.Line) <= lineOffset+documentLines-1 {
				currentDocument = d
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
					Arguments: []any{params.TextDocument.URI, currentDocument, false, params.Range.Start.Line, params.Range.Start.Character},
				},
			},
			{
				Title: "Fill document with required fields",
				Command: &protocol.Command{
					Title:     "Fill document",
					Command:   "fill-document",
					Arguments: []any{params.TextDocument.URI, currentDocument, true, params.Range.Start.Line, params.Range.Start.Character},
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
			if len(params.Arguments) != 5 {
				logger.Info("Expected 5 arguments to fill-document")
				return "", fmt.Errorf("Expected 5 arguments to fill-document")
			}
			uri_ := params.Arguments[0].(string)
			uri := protocol.DocumentURI(uri_)
			currentDocument := params.Arguments[1].(string)
			requiredOnly := params.Arguments[2].(bool)
			line := uint32(params.Arguments[3].(float64))
			column := uint32(params.Arguments[4].(float64))
			yamlPath, err := parser.GetPathAtPosition(line, column, currentDocument)
			if err != nil {
				logger.Error("Failed to get path at position", "line", line, "column", column)
				return nil, errors.New("Not found")
			}
			indentLevel := strings.Count(yamlPath, ".") - 1
			schema, err := schemaStore.GetSchema(uri.Filename(), currentDocument)
			if err != nil {
				return nil, fmt.Errorf("get schema to fill document: %v", err)
			}
			subSchema, found := parser.GetSubSchema(yamlPath, schema)
			if !found {
				return nil, fmt.Errorf("could not find schema on path: %v", err)
			}
			logger.Info("subSchema", "subSchema", subSchema)
			fullDoc, err := fillDocument(subSchema, requiredOnly)
			if err != nil {
				return nil, fmt.Errorf("fill document: %v", err)
			}
			logger.Info("fullDoc", "fullDoc", fullDoc)
			resultBytes, err := yaml.Marshal(fullDoc)
			if err != nil {
				return nil, fmt.Errorf("marshal filled document: %v", err)
			}
			var indented string
			for _, line := range strings.FieldsFunc(string(resultBytes), func(r rune) bool { return r == '\n' }) {
				indented += strings.Repeat(" ", 2*indentLevel+2) + line + "\n"
			}
			applyParams := protocol.ApplyWorkspaceEditParams{
				Edit: protocol.WorkspaceEdit{
					Changes: map[protocol.DocumentURI][]protocol.TextEdit{
						protocol.DocumentURI(uri): {
							{
								Range: protocol.Range{
									Start: protocol.Position{Line: line + 1, Character: 0},
									End:   protocol.Position{Line: line + 1, Character: 0},
								},
								NewText: indented,
							},
						},
					},
				},
			}
			m.Request("workspace/applyEdit", applyParams)
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

var errorPathToParserPath = regexp.MustCompile(`\.(\d+)`)
var trailingIndex = regexp.MustCompile(`\[\d+\]$`)

func validateAgainstSchema(schema []byte, text string, lineOffset uint32) ([]protocol.Diagnostic, error) {
	diagnostics := []protocol.Diagnostic{}
	jsonText, err := yaml.YAMLToJSON([]byte(text))
	if err != nil {
		return diagnostics, fmt.Errorf("Failed to convert yaml to json: %s", err)
	}
	schemaLoader := gojsonschema.NewBytesLoader(schema)
	documentLoader := gojsonschema.NewBytesLoader(jsonText)
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return diagnostics, fmt.Errorf("Failed to validate against schema: %s", err)
	}
	if result.Valid() {
		return diagnostics, nil
	}
	for _, e := range result.Errors() {
		path := fmt.Sprintf("$.%s", e.Field())
		details := e.Details()
		path = errorPathToParserPath.ReplaceAllString(path, "[$1]")
		property, found := details["property"]
		if found && e.Type() != "required" {
			path = fmt.Sprintf("%s.%s", path, property)
		}
		path = trailingIndex.ReplaceAllString(path, "")
		line, startColumn, endColumn, err := parser.GetPositionForPath(path, text)
		if err != nil {
			logger.Error("Failed to get position for path", "path", path)
		}
		d := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      line + lineOffset,
					Character: startColumn,
				},
				End: protocol.Position{
					Line:      line + lineOffset,
					Character: endColumn,
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "yamlls",
			Message:  e.Description(),
		}
		diagnostics = append(diagnostics, d)
	}
	return diagnostics, nil
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

func kustomizationForgottenResources(filename string, text string) []protocol.Diagnostic {
	forgottenFiles, err := kustomization.FilesNotIncluded(path.Dir(filename), text)
	forgottenFilesString := strings.Join(forgottenFiles, ", ")
	if err != nil {
		return []protocol.Diagnostic{}
	}
	if len(forgottenFiles) == 0 {
		return []protocol.Diagnostic{}
	}
	line := kustomization.GetResourcesLine(text)
	d := protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(line),
				Character: 0,
			},
			End: protocol.Position{
				Line:      uint32(line),
				Character: uint32(len("resources:")),
			},
		},
		Severity: protocol.DiagnosticSeverityHint,
		Source:   "yamlls",
		Message:  fmt.Sprintf("Resources not included: %s", forgottenFilesString),
	}
	return []protocol.Diagnostic{d}
}

func mustMarshalYaml(y any) string {
	bytes, err := yaml.Marshal(y)
	if err != nil {
		panic(fmt.Sprintf("marshal yaml: %v", err))
	}
	return string(bytes)
}

func fillDocument(schema []byte, requiredOnly bool) (any, error) {
	var jsonSchema map[string]any
	if err := json.Unmarshal(schema, &jsonSchema); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %v", err)
	}
	logger.Info("jsonSchema", "jsonSchema", jsonSchema)
	var fullDoc any
	fullDoc, err := parseSchema(jsonSchema, requiredOnly)
	if err != nil {
		return nil, fmt.Errorf("create example for schema: %v", err)
	}
	fmt.Print(mustMarshalYaml(fullDoc))
	return fullDoc, nil
}

func parseSchema(schema map[string]any, requiredOnly bool) (any, error) {
	type_, found := schema["type"]
	if found {
		switch type_ := type_.(type) {
		case string:
			result, err := parseByType(type_, schema, requiredOnly)
			if err != nil {
				return nil, err
			}
			return result, nil
		case []any:
			var singleType string
			for _, v := range type_ {
				v := v.(string)
				if v == "null" {
					continue
				}
				singleType = v
				break
			}
			if singleType == "" {
				return nil, fmt.Errorf("expected at least one type to be not null when type is an array, got %v", type_)
			}
			result, err := parseByType(singleType, schema, requiredOnly)
			if err != nil {
				return nil, err
			}
			return result, nil
		default:
			return nil, fmt.Errorf("expected type to be a string or an array of strings, got %T", type_)
		}
	}

	const_, found := schema["const"]
	if found {
		return const_, nil
	}

	enum, found := schema["enum"]
	if found {
		return enum.([]any)[0], nil
	}

	oneOf, found := schema["oneOf"]
	if found {
		first := oneOf.([]any)[0]
		result, err := parseSchema(first.(map[string]any), requiredOnly)
		if err != nil {
			logger.Error("parse oneOf", "oneOf", oneOf, "error", err)
			return nil, fmt.Errorf("parse oneOf: %v", err)
		}
		return result, nil
	}

	anyOf, found := schema["anyOf"]
	if found {
		first := anyOf.([]any)[0]
		result, err := parseSchema(first.(map[string]any), requiredOnly)
		if err != nil {
			logger.Error("parse anyOf", "anyOf", anyOf, "error", err)
			return nil, fmt.Errorf("parse anyOf: %v", err)
		}
		return result, nil
	}

	_, found = schema["x-kubernetes-preserve-unknown-fields"]
	if found {
		return map[string]any{}, nil
	}

	return nil, fmt.Errorf("expected schema to have type, enum, const, oneOf, anyOf, x-kubernetes-preserve-unknown-fields set, got %v", schema)
}

func parseByType(type_ string, schema map[string]any, requiredOnly bool) (any, error) {
	switch type_ {
	case "string":
		return "", nil
	case "integer":
		return 0, nil
	case "object":
		properties, found := schema["properties"]
		logger.Info("object", "properties", properties)
		if !found {
			return map[string]any{}, nil
			// return nil, errors.New("expected a schema of type object to have properties")
		}
		result := map[string]any{}
		var required []string
		if requiredOnly {
			required__, found := schema["required"]
			if !found {
				return result, nil
			}
			required_ := required__.([]any)
			for _, r := range required_ {
				required = append(required, r.(string))
			}
		}
		for k, v := range properties.(map[string]any) {
			if requiredOnly && !slices.Contains(required, k) {
				continue
			}
			logger.Info("key", "k", k)
			subResult, err := parseSchema(v.(map[string]any), requiredOnly)
			if err != nil {
				return nil, err
			}
			result[k] = subResult
		}
		return result, nil
	case "boolean":
		return false, nil
	case "array":
		items, found := schema["items"]
		if !found {
			return nil, errors.New("expected a schema of type array to have items")
		}
		subResult, err := parseSchema(items.(map[string]any), requiredOnly)
		if err != nil {
			return nil, err
		}
		return []any{subResult}, nil
	default:
		panic(fmt.Sprintf("type `%v` not implemented", type_))
	}
}
