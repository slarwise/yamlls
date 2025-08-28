package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	yamlparser "github.com/goccy/go-yaml/parser"
	"github.com/tidwall/gjson"
	"github.com/xeipuuv/gojsonschema"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

var (
	CACHE_DIR string
	DB_DIR    string
	logger    *slog.Logger
)

func init() {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		fatal("locate user's cache directory: %s", err)
	}
	CACHE_DIR = filepath.Join(userCacheDir, "yamlls")
	DB_DIR = filepath.Join(CACHE_DIR, "db")
	if err := os.MkdirAll(DB_DIR, 0755); err != nil {
		fatal("create db dir for storing schemas %s: %s", DB_DIR, err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	if err := run(); err != nil {
		fatal(err.Error())
	}
}

func run() error {
	if len(os.Args) < 2 {
		if err := runLanguageServer(); err != nil {
			return fmt.Errorf("run language server: %s", err)
		}
	} else {
		subCommand, args := os.Args[1], os.Args[2:]
		switch subCommand {
		case "schemas":
			if err := listSchemas(); err != nil {
				return fmt.Errorf("list schemas: %s", err)
			}
		case "docs":
			if len(args) == 0 {
				return fmt.Errorf("must provide `basename`, e.g. `yamlls docs Deployment-apps-v1.json`. Get the basename from `yamlls schemas`.")
			}
			basename := args[0]
			if err := showDocs(basename); err != nil {
				return fmt.Errorf("get docs: %s", err)
			}
		case "fill":
			if len(args) == 0 {
				return fmt.Errorf("must provide the id of the schema to fill")
			}
			id := args[0]
			panic(fmt.Sprintf("TODO: Fill the schema with id `%s`", id))
		case "validate":
			if len(args) == 0 {
				return fmt.Errorf("must provide the filename to validate")
			}
			file := args[0]
			bytes, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read `%s`: %s", file, err)
			}
			errors := validateFile(string(bytes))
			for _, e := range errors {
				fmt.Printf("%s:%d:%s\n", file, e.Range.Start.Line, e.Message)
			}
		case "refresh":
			if err := refreshDatabase(); err != nil {
				return fmt.Errorf("refresh database: %s", err)
			}
		default:
			panic("TODO: Handle unknown subcommand")
		}
	}
	return nil
}

const (
	NATIVE_SCHEMAS_BASE_URL = "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/refs/heads/master/master-standalone-strict"
	CUSTOM_SCHEMAS_BASE_URL = "https://raw.githubusercontent.com/datreeio/CRDs-catalog/refs/heads/main"
)

func refreshDatabase() error {
	if err := os.RemoveAll(DB_DIR); err != nil {
		return fmt.Errorf("remove `%s`: %s", DB_DIR, err)
	}
	if err := os.MkdirAll(DB_DIR, 0755); err != nil {
		return fmt.Errorf("create `%s`: %s", DB_DIR, err)
	}

	{
		nativeDefinitionsUrl := fmt.Sprintf("%s/_definitions.json", NATIVE_SCHEMAS_BASE_URL)
		body, err := httpGet(nativeDefinitionsUrl)
		if err != nil {
			return fmt.Errorf("get native definitions: %s", err)
		}
		var definitions struct {
			Definitions map[string]struct {
				GroupVersionKind []struct {
					Group   string `json:"group"`
					Kind    string `json:"kind"`
					Version string `json:"version"`
				} `json:"x-kubernetes-group-version-kind"`
			} `json:"definitions"`
		}
		if err := json.Unmarshal(body, &definitions); err != nil {
			return fmt.Errorf("unmarshal native definitions: %s", err)
		}
		i := 0
		for id, definition := range definitions.Definitions {
			i++
			fmt.Fprintf(os.Stderr, "%-3d/%d\r", i, len(definitions.Definitions))
			if strings.Contains(id, "apimachinery") || strings.Contains(id, "apiextensions") || strings.Contains(id, "apiserverinternal") || len(definition.GroupVersionKind) != 1 {
				continue
			}
			gvk := definition.GroupVersionKind[0]
			group := gvk.Group
			groupFirstPart := strings.Split(gvk.Group, ".")[0]
			schemaId := gvkToSchemaId(group, gvk.Version, gvk.Kind)
			// NOTE: We want the group in schema id to be the full group, e.g. `networking.k8s.io`
			//       But the group in the filename in the git repo is just `networking`
			baseName := strings.Replace(schemaId, group, groupFirstPart, 1) + ".json"
			schemaUrl := fmt.Sprintf("%s/%s", NATIVE_SCHEMAS_BASE_URL, strings.ToLower(baseName))
			schema, err := httpGet(schemaUrl)
			if err != nil {
				return fmt.Errorf("get schema: %s", err)
			}
			filename := filepath.Join(DB_DIR, schemaId+".json")
			if err := os.WriteFile(filename, schema, 0644); err != nil {
				return fmt.Errorf("write schema to %s: %s", filename, err)
			}
		}
	}

	{
		customDefinitionsUrl := fmt.Sprintf("%s/index.yaml", CUSTOM_SCHEMAS_BASE_URL)
		body, err := httpGet(customDefinitionsUrl)
		if err != nil {
			return fmt.Errorf("get index for custom definitions: %s", err)
		}
		var index map[string][]struct {
			ApiVersion string `yaml:"apiVersion"`
			Kind       string `yaml:"kind"`
			Filename   string `yaml:"filename"`
		}
		if err := yaml.Unmarshal(body, &index); err != nil {
			return fmt.Errorf("unmarshal custom definitions index: %s", err)
		}
		i := 0
		for _, definitions := range index {
			i++
			fmt.Fprintf(os.Stderr, "%-3d/%d\r", i, len(index))
			for _, d := range definitions {
				if strings.Contains(d.Kind, "/") {
					fmt.Fprintf(os.Stderr, "kind `%s` contains a `/`, it shouldn't\n", d.Kind)
					continue
				}
				schemaUrl := fmt.Sprintf("%s/%s", CUSTOM_SCHEMAS_BASE_URL, d.Filename)
				body, err := httpGet(schemaUrl)
				if err != nil {
					return fmt.Errorf("get schema: %s", err)
				}
				split := strings.Split(d.ApiVersion, "/")
				if len(split) != 2 {
					return fmt.Errorf("expected apiVersion to have exactly one `/`, got %s", d.ApiVersion)
				}
				group, version := split[0], split[1]
				schemaId := gvkToSchemaId(group, version, d.Kind)
				baseName := schemaId + ".json"
				filename := filepath.Join(DB_DIR, baseName)
				if err := os.WriteFile(filename, body, 0644); err != nil {
					return fmt.Errorf("write schema to %s: %s", filename, err)
				}
			}
		}
	}
	return nil
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %s", url, err)
	}
	defer func() error {
		if err := resp.Body.Close(); err != nil {
			return fmt.Errorf("close body: %s", err)
		}
		return nil
	}()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("get %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %s", err)
	}
	return body, nil
}

func listSchemas() error {
	ids, err := schemaIds()
	if err != nil {
		return fmt.Errorf("get schema ids: %s", err)
	}
	for _, id := range ids {
		fmt.Println(id)
	}
	return nil
}

func schemaIds() ([]string, error) {
	files, err := os.ReadDir(DB_DIR)
	if err != nil {
		return nil, fmt.Errorf("read db %s: %s", DB_DIR, err)
	}
	var ids []string
	for _, f := range files {
		if f.IsDir() {
			return nil, fmt.Errorf("expected all files in %s to be files, got a dir: %s", DB_DIR, f.Name())
		}
		ids = append(ids, f.Name())
	}
	return ids, nil
}

func showDocs(basename string) error {
	filename := filepath.Join(DB_DIR, basename)
	schema, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read schema %s: %s", filename, err)
	}
	docs, err := docs(schema)
	if err != nil {
		return fmt.Errorf("create docs for %s: %s", filename, err)
	}
	fmt.Print(htmlDocs(docs, ""))
	return nil
}

type GVK struct{ group, version, kind string }

func schemaIdToGvk(id string) GVK {
	id = strings.TrimSuffix(id, ".json")
	split := strings.Split(id, "-")
	gvk := GVK{kind: split[0]}
	if len(split) == 2 {
		gvk.version = split[1]
	} else {
		gvk.group = split[1]
		gvk.version = split[2]
	}
	return gvk
}

func gvkToSchemaId(group, version, kind string) string {
	id := kind + "-"
	if group != "" {
		id += group + "-"
	}
	id += version
	return id
}

type Schema struct {
	Type        Type              `json:"type"`
	Description string            `json:"description"`
	Properties  map[string]Schema `json:"properties"`
	Items       *Schema           `json:"items"`
	AnyOf       []Schema          `json:"anyOf"`
	AllOf       []Schema          `json:"allOf"`
	OneOf       []Schema          `json:"oneOf"`
	Const       string            `json:"const"`
	Enum        []string          `json:"enum"`
	Ref         string            `json:"$ref"`
	Required    []string          `json:"required"`
}

type Type struct {
	One  string
	Many []string
}

func (t *Type) UnmarshalJSON(b []byte) error {
	var val any
	if err := json.Unmarshal(b, &val); err != nil {
		return err
	}
	switch concreteVal := val.(type) {
	case string:
		t.One = concreteVal
	case []any:
		for _, e := range concreteVal {
			if s, ok := e.(string); ok {
				t.Many = append(t.Many, s)
			} else {
				return fmt.Errorf("expected string, got %v", e)
			}
		}
	default:
		return fmt.Errorf("expected string or array of strings, got %v", concreteVal)
	}
	return nil
}

type SchemaProperty struct {
	Path, Description, Type string
	Required                bool
}

func docs(schema []byte) ([]SchemaProperty, error) {
	var schemaParsed Schema
	if err := json.Unmarshal(schema, &schemaParsed); err != nil {
		return nil, fmt.Errorf("parse schema: %s", err)
	}
	properties := docs2(".", schemaParsed, schema)
	return properties, nil
}

func docs2(path string, s Schema, root []byte) []SchemaProperty {
	docs := []SchemaProperty{{Path: path, Description: s.Description, Type: typeString(s)}}
	for prop /* webdev moment */, schema := range s.Properties {
		subPath := path + "." + prop
		if path == "." {
			subPath = path + prop
		}
		subDocs := docs2(subPath, schema, root)
		if slices.Contains(s.Required, prop) {
			subDocs[0].Required = true
		}
		docs = append(docs, subDocs...)
	}
	if s.Items != nil {
		docs = append(docs, docs2(path+"[]", *s.Items, root)...)
	}
	for i, schema := range s.AnyOf {
		docs = append(docs, docs2(fmt.Sprintf("%s?%d", path, i), schema, root)...)
	}
	for i, schema := range s.OneOf {
		docs = append(docs, docs2(fmt.Sprintf("%s?%d", path, i), schema, root)...)
	}
	if len(s.AllOf) > 0 {
		var subDocs []SchemaProperty
		for _, schema := range s.AllOf {
			subDocs = append(subDocs, docs2(path, schema, root)...)
		}
		subDocs = slices.DeleteFunc(subDocs, func(s SchemaProperty) bool {
			return s.Path == path
		})
		docs = append(docs, subDocs...)
	}
	if s.Ref != "" {
		// NOTE: We expect all references to be part of the same file
		ref := strings.Split(s.Ref, "#")[1]
		refPath := strings.ReplaceAll(ref[1:], "/", ".")
		res := gjson.GetBytes(root, refPath)
		if !res.Exists() {
			panicf("could not find the reference at path %s in the root schema %s", refPath, root)
		}
		var refSchema Schema
		if err := json.Unmarshal([]byte(res.Raw), &refSchema); err != nil {
			panicf("expected ref to point to a valid schema: %s", err)
		}
		docs = docs2(path, refSchema, root)
	}
	return docs
}

func typeString(s Schema) string {
	if s.Const != "" {
		return "const"
	} else if len(s.Enum) > 0 {
		return "enum"
	} else if len(s.AnyOf) > 0 {
		return "anyOf"
	} else if len(s.OneOf) > 0 {
		return "oneOf"
	} else if len(s.AllOf) > 0 {
		return "allOf"
	}
	if s.Type.One != "" {
		return s.Type.One
	} else if len(s.Type.Many) > 0 {
		return fmt.Sprintf("%s", strings.Join(s.Type.Many, ", "))
	}
	return ""
}

func panicf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

func htmlDocs(docs []SchemaProperty, highlightProperty string) string {
	output := strings.Builder{}
	fmt.Fprint(&output, `<!DOCTYPE html>
<html>
<head>
  <title>Documentation</title>
  <style>
    body {background-color: #3f3f3f; color: #DCDCCC; font-size: 18px; }
    code {font-size: 80%;}
    code.required {color: #E0CF9F;}
    span.path {color: #DCA3A3; }
  </style>
</head>
`)
	fmt.Fprintln(&output, "<body>")

	for _, property := range docs {
		fmt.Fprintln(&output, "  <p>")

		requiredClass := ""
		if property.Required {
			requiredClass = ` class="required"`
		}
		fmt.Fprintf(&output, `    <span class="path" id="%s">%s</span> <code%s>[%s]</code>`, property.Path, property.Path, requiredClass, property.Type)

		fmt.Fprintln(&output)
		if property.Description != "" {
			fmt.Fprint(&output, "    <br>\n")
			fmt.Fprintf(&output, "    %s\n", property.Description)
		}
		fmt.Fprintln(&output, "  </p>")
	}

	if highlightProperty != "" {
		fmt.Fprintf(&output, `  <script>window.location.hash = "%s"</script>`, highlightProperty)
		fmt.Fprintln(&output, "")
	}

	fmt.Fprintln(&output, "</body>")
	fmt.Fprintln(&output, "</html>")
	return output.String()
}

func validateFile(contents string) []ValidationError {
	lines := strings.FieldsFunc(contents, func(r rune) bool { return r == '\n' })
	positions := getDocumentPositions(contents)
	var errors []ValidationError
	for _, docPos := range positions {
		documentString := strings.Join(lines[docPos.Start:docPos.End], "\n")

		gvk, ok := extractGvkFromDocument([]byte(documentString))
		if !ok {
			errors = append(errors, ValidationError{
				Range: Range{
					Start: Position{
						Line: docPos.Start,
						Char: 0,
					},
					End: Position{
						Line: docPos.End,
						Char: 0,
					},
				},
				Message: "invalid yaml",
				Type:    "invalid_yaml",
			})
			continue
		}

		if gvk.kind == "" || gvk.version == "" {
			fmt.Fprintf(os.Stderr, "no kind and group found for document %s\n", documentString)
			continue
		}

		schemaId := gvkToSchemaId(gvk.group, gvk.version, gvk.kind)
		schemaBytes, err := os.ReadFile(filepath.Join(DB_DIR, schemaId+".json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "load schema `%s: %s\n`", schemaId, err)
		}
		schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)

		jsonDocument, err := yaml.YAMLToJSON([]byte(documentString))
		if err != nil {
			fmt.Fprintf(os.Stderr, "convert yaml to json: %s\n", err)
			continue
		}
		documentLoader := gojsonschema.NewBytesLoader(jsonDocument)

		res, err := gojsonschema.Validate(schemaLoader, documentLoader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "schema and/or document is invalid: %s\n", err)
			continue
		}

		paths := yamlDocumentPaths([]byte(documentString))
		for _, e := range res.Errors() {
			field := e.Field() // The root here is (root)
			if e.Type() == "additional_property_not_allowed" {
				field = field + "." + e.Details()["property"].(string)
			}
			field = "." + field
			if field == ".(root)" {
				field = "."
			}
			range_, found := paths[field]
			if !found {
				// expected path `.(root)` to exist in the document. Available paths: map[.apiVersion:{{1 0} {1 10}} .kind:{{0 0} {0 4}} .metadata:{{2 0} {2 8}} .metadata.name:{{3 2} {3 6}}]. Error type: required\n
				panic(fmt.Sprintf("expected path `%s` to exist in the document. Available paths: %v. Error type: %s", field, paths, e.Type()))
			}
			errors = append(errors, ValidationError{
				Range:   newRange(docPos.Start+range_.Start.Line, range_.Start.Char, docPos.Start+range_.End.Line, range_.End.Char),
				Message: e.Description(),
				Type:    e.Type(), // I've got life!
			})
		}
	}
	return errors
}

type ValidationError struct {
	Range   Range
	Message string
	Type    string
}

type Range struct{ Start, End Position } // zero-based, the start character is inclusive and the end character is exclusive
type Position struct{ Line, Char int }   // zero-based

func newRange(startLine, startChar, endLine, endChar int) Range {
	return Range{
		Start: Position{Line: startLine, Char: startChar},
		End:   Position{Line: endLine, Char: endChar},
	}
}

type lineRange struct{ Start, End int } // [Start, End), 0-indexed

func getDocumentPositions(file string) []lineRange {
	var ranges []lineRange
	startLine := 0
	lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
	for i, line := range lines {
		if line == "---" {
			ranges = append(ranges, lineRange{
				Start: startLine,
				End:   i,
			})
			startLine = i + 1
		} else if i == len(lines)-1 {
			ranges = append(ranges, lineRange{
				Start: startLine,
				End:   i + 1,
			})
		}
	}
	return ranges
}

func yamlDocumentPaths(doc []byte) Paths {
	astFile, err := yamlparser.ParseBytes([]byte(doc), 0)
	if err != nil {
		panic(fmt.Sprintf("expected a valid yaml document: %v", err))
	}
	if len(astFile.Docs) != 1 {
		panic(fmt.Sprintf("expected 1 document, got %d", len(astFile.Docs)))
	}
	paths := Paths{}
	ast.Walk(&paths, astFile.Docs[0])
	return paths
}

type Paths map[string]Range

var (
	arrayPattern          = regexp.MustCompile(`\[(\d+)\]`)
	endingIndex           = regexp.MustCompile(`\.(\d+)$`)
	endingIndexInBrackets = regexp.MustCompile(`\[(\d+)\]$`)
)

func (p Paths) Visit(node ast.Node) ast.Visitor {
	if node.Type() == ast.MappingValueType || node.Type() == ast.DocumentType {
		return p
	}
	path := strings.TrimPrefix(node.GetPath(), "$")
	if path == "" {
		path = "."
	}
	path = arrayPattern.ReplaceAllString(path, ".$1")
	if node.Type() == ast.MappingType && endingIndex.MatchString(path) {
		// The path looks like spec.ports[1] here
		// This is the `:` in the first element in an object array
		// Not sure why it's only on the first one
		// Use the parent path to compute the column
		parent := endingIndex.ReplaceAllString(path, "")
		var char int
		for existingPath, pos := range p {
			if existingPath == parent {
				char = pos.Start.Char + 2 // NOTE: Assuming that lists are indented here
			}
		}
		t := node.GetToken()
		p[path] = Range{
			Start: Position{
				Line: t.Position.Line - 1,
				Char: char,
			},
			End: Position{
				Line: t.Position.Line - 1,
				Char: char + 1,
			},
		}
		return p
	}
	// Turn spec.ports[0].port into spec.ports.0.port
	// path = arrayPattern.ReplaceAllString(path, ".$1")
	if _, found := p[path]; found {
		// Store the path to the key only, not the value
		// Assumes that the key is always visited first, couldn't find a way to distinguish
		// key nodes and value nodes
		return p
	}
	t := node.GetToken()
	if path == "." {
		p[path] = Range{}
		return p
	}
	p[path] = Range{
		Start: Position{
			Line: t.Position.Line - 1,
			Char: t.Position.Column - 1,
		},
		End: Position{
			Line: t.Position.Line - 1,
			Char: t.Position.Column + len(t.Value) - 1,
		},
	}
	return p
}

var exitChannel chan (int)
var documentUpdates chan (protocol.TextDocumentItem)
var filenameToContents map[string]string
var m *Mux

func runLanguageServer() error {
	logpath := filepath.Join(CACHE_DIR, "log.json")
	logfile, err := os.Create(logpath)
	if err != nil {
		return fmt.Errorf("create log file: %s", err)
	}
	defer logfile.Close()
	logger = slog.New(slog.NewJSONHandler(logfile, nil))
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic", "recovered", r)
		}
	}()

	m = NewMux(logger, os.Stdin, os.Stdout)

	exitChannel = make(chan int, 1)
	documentUpdates = make(chan protocol.TextDocumentItem, 10)
	filenameToContents = map[string]string{}

	m.HandleMethod(protocol.MethodInitialize, lspInitialize)
	m.HandleNotification(protocol.MethodInitialized, lspInitialized)
	m.HandleMethod(protocol.MethodShutdown, lspShutdown)
	m.HandleNotification(protocol.MethodExit, lspExit)
	m.HandleNotification(protocol.MethodTextDocumentDidOpen, lspTextDocumentDidOpen)
	m.HandleNotification(protocol.MethodTextDocumentDidChange, lspTextDocumentDidChange)
	m.HandleMethod(protocol.MethodTextDocumentHover, lspTextDocumentHover)
	m.HandleMethod(protocol.MethodTextDocumentCompletion, lspTextDocumentCompletion)
	m.HandleMethod(protocol.MethodTextDocumentCodeAction, lspMethodTextDocumentCodeAction)
	m.HandleMethod(protocol.MethodWorkspaceExecuteCommand, lspMethodWorkspaceExecuteCommand)

	go func() {
		for doc := range documentUpdates {
			filenameToContents[doc.URI.Filename()] = doc.Text
			errors := validateFile(doc.Text)
			var diagnostics []protocol.Diagnostic
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
			m.Notify(protocol.MethodTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
				URI:         doc.URI,
				Version:     uint32(doc.Version),
				Diagnostics: diagnostics,
			})
		}
	}()

	logger.Info("Handler set up", "log_path", logpath)

	go func() {
		if err := m.Process(); err != nil {
			logger.Error("Processing stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-exitChannel

	logger.Info("Server exited")

	return nil
}

func lspInitialize(params json.RawMessage) (any, error) {
	var initializeParams protocol.InitializeParams
	if err := json.Unmarshal(params, &initializeParams); err != nil {
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
}

func lspInitialized(params json.RawMessage) error {
	logger.Info("Receivied initialized notification")
	return nil
}

func lspShutdown(params json.RawMessage) (any, error) {
	logger.Info("Received shutdown request")
	return nil, nil
}

func lspExit(params json.RawMessage) error {
	logger.Info("Received exit notification")
	exitChannel <- 1
	return nil
}

func lspTextDocumentDidOpen(rawParams json.RawMessage) error {
	logger.Info("Received TextDocument/didOpen notification")
	var params protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return err
	}
	documentUpdates <- params.TextDocument
	return nil
}

func lspTextDocumentDidChange(rawParams json.RawMessage) error {
	logger.Info("Received textDocument/didChange notification")
	var params protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return err
	}
	documentUpdates <- protocol.TextDocumentItem{URI: params.TextDocument.URI, Version: params.TextDocument.Version, Text: params.ContentChanges[0].Text}
	return nil
}

func lspTextDocumentHover(rawParams json.RawMessage) (any, error) {
	logger.Info("Received textDocument/hover request")
	var params protocol.HoverParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, err
	}
	contents := filenameToContents[params.TextDocument.URI.Filename()]

	documentPositions := getDocumentPositions(contents)
	var currentDocument string
	var lineInDocument int
	for _, r := range documentPositions {
		if r.Start <= int(params.Position.Line) && int(params.Position.Line) < r.End {
			lines := strings.FieldsFunc(contents, func(r rune) bool { return r == '\n' })
			currentDocument = strings.Join(lines[r.Start:r.End], "\n")
			lineInDocument = int(params.Position.Line) - r.Start
		}
	}
	if currentDocument == "" {
		return nil, nil
	}
	paths := yamlDocumentPaths([]byte(currentDocument))
	pathAtCursor, found := pathAtCursor(paths, lineInDocument, int(params.Position.Character))
	if !found {
		return nil, nil
	}

	gvk, ok := extractGvkFromDocument([]byte(currentDocument))
	if !ok {
		return nil, errors.New("no kind and apiVersion found")
	}
	schemaId := gvkToSchemaId(gvk.group, gvk.version, gvk.kind)
	schema, err := os.ReadFile(filepath.Join(DB_DIR, schemaId+".json"))
	if err != nil {
		return nil, fmt.Errorf("no schema found for %s", schemaId)
	}

	docs, err := docs(schema)
	if err != nil {
		return nil, fmt.Errorf("create docs: %s", err)
	}

	// Turn spec.ports.0.name into spec.ports[].name
	pathAtCursor = arrayPath.ReplaceAllString(pathAtCursor, "[]")
	for _, property := range docs {
		if property.Path == pathAtCursor {
			if property.Description == "" {
				break
			}
			return protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.PlainText,
					Value: property.Description,
				},
			}, nil
		}
	}
	return nil, nil
}

func lspTextDocumentCompletion(rawParams json.RawMessage) (any, error) {
	logger.Info("Receiver textDocument/completion request, not supported")
	return nil, nil
}

var arrayPath = regexp.MustCompile(`\.\d+`)

func lspMethodTextDocumentCodeAction(rawParams json.RawMessage) (any, error) {
	logger.Info(fmt.Sprintf("Received %s request", protocol.MethodTextDocumentCodeAction))
	var params protocol.CodeActionParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, err
	}
	contents := filenameToContents[params.TextDocument.URI.Filename()]

	documentPositions := getDocumentPositions(contents)
	var currentDocument string
	var lineInDocument int
	for _, r := range documentPositions {
		if r.Start <= int(params.Range.Start.Line) && int(params.Range.Start.Line) < r.End {
			lines := strings.FieldsFunc(contents, func(r rune) bool { return r == '\n' })
			currentDocument = strings.Join(lines[r.Start:r.End], "\n")
			lineInDocument = int(params.Range.Start.Line) - r.Start
		}
	}
	if currentDocument == "" {
		return nil, nil
	}
	paths := yamlDocumentPaths([]byte(currentDocument))
	pathAtCursor, found := pathAtCursor(paths, lineInDocument, int(params.Range.Start.Character))
	if found {
		// Turn spec.ports.0.name into spec.ports[].name
		// TODO: pathAtCursor should return a good path
		pathAtCursor = arrayPath.ReplaceAllString(pathAtCursor, "[]")
	}

	gvk, ok := extractGvkFromDocument([]byte(currentDocument))
	if !ok {
		return nil, errors.New("no kind and apiVersion found")
	}
	schemaId := gvkToSchemaId(gvk.group, gvk.version, gvk.kind)
	schema, err := os.ReadFile(filepath.Join(DB_DIR, schemaId+".json"))
	if err != nil {
		return nil, fmt.Errorf("no schema found for %s", schemaId)
	}

	docs, err := docs(schema)
	if err != nil {
		return nil, fmt.Errorf("create docs: %s", err)
	}
	html := htmlDocs(docs, pathAtCursor)

	filename := filepath.Join(CACHE_DIR, "docs.html")
	if err := os.WriteFile(filename, []byte(html), 0755); err != nil {
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
}

func lspMethodWorkspaceExecuteCommand(rawParams json.RawMessage) (any, error) {
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
}

// Return false if yaml is invalid
func extractGvkFromDocument(docBytes []byte) (GVK, bool) {
	var document map[string]any
	if err := yaml.Unmarshal(docBytes, &document); err != nil {
		return GVK{}, false
	}
	var gvk GVK
	if kind_, ok := document["kind"]; ok {
		if kind, ok := kind_.(string); ok {
			gvk.kind = kind
		}
	}
	if apiVersion_, ok := document["apiVersion"]; ok {
		if apiVersion, ok := apiVersion_.(string); ok {
			split := strings.Split(apiVersion, "/")
			switch len(split) {
			case 1:
				gvk.version = split[0]
			case 2:
				gvk.group = split[0]
				gvk.version = split[1]
			}
		}
	}
	return gvk, true
}

func pathAtCursor(paths Paths, line, char int) (string, bool) {
	for path, r := range paths {
		if r.Start.Line == line && r.Start.Char <= char && char < r.End.Char {
			return path, true
		}
	}
	return "", false
}

const protocolVersion = "2.0"

type Message interface {
	IsJSONRPC() bool
}

type Request struct {
	ProtocolVersion string           `json:"jsonrpc"`
	ID              *json.RawMessage `json:"id"`
	Method          string           `json:"method"`
	Params          json.RawMessage  `json:"params"`
}

func (r Request) IsJSONRPC() bool {
	return r.ProtocolVersion == protocolVersion
}

type OutgoingRequest struct {
	ProtocolVersion string `json:"jsonrpc"`
	ID              string `json:"id"`
	Method          string `json:"method"`
	Params          any    `json:"params"`
}

func (r OutgoingRequest) IsJSONRPC() bool {
	return r.ProtocolVersion == protocolVersion
}

func (r Request) IsNotification() bool {
	return r.ID == nil
}

type Response struct {
	ProtocolVersion string           `json:"jsonrpc"`
	ID              *json.RawMessage `json:"id"`
	Result          any              `json:"result"`
	Error           *Error           `json:"error"`
}

func (r Response) IsJSONRPC() bool {
	return r.ProtocolVersion == protocolVersion
}

func NewResponse(id *json.RawMessage, result any) Response {
	return Response{
		ProtocolVersion: protocolVersion,
		ID:              id,
		Result:          result,
		Error:           nil,
	}
}

func NewResponseError(id *json.RawMessage, err error) Response {
	return Response{
		ProtocolVersion: protocolVersion,
		ID:              id,
		Result:          nil,
		Error:           newError(err),
	}
}

type Error struct {
	// Code is a Number that indicates the error type that occurred.
	Code int64 `json:"code"`
	// Message of the error.
	// The message SHOULD be limited to a concise single sentence.
	Message string `json:"message"`
	// A Primitive or Structured value that contains additional information about the error.
	// This may be omitted.
	// The value of this member is defined by the Server (e.g. detailed error information, nested errors etc.).
	Data any `json:"data"`
}

func (e *Error) Error() string {
	return e.Message
}

func newError(err error) *Error {
	return &Error{
		Code:    0,
		Message: err.Error(),
		Data:    nil,
	}
}

var (
	ErrParseError                 *Error = &Error{Code: -32700, Message: "Parse error"}
	ErrInvalidRequest             *Error = &Error{Code: -32600, Message: "Invalid Request"}
	ErrMethodNotFound             *Error = &Error{Code: -32601, Message: "Method not found"}
	ErrInvalidParams              *Error = &Error{Code: -32602, Message: "Invalid params"}
	ErrInternal                   *Error = &Error{Code: -32603, Message: "Internal error"}
	ErrServerNotInitialized       *Error = &Error{Code: -32002, Message: "Server not initialized"}
	ErrInvalidContentLengthHeader        = errors.New("missing or invalid Content-Length header")
)

type Notification struct {
	ProtocolVersion string `json:"jsonrpc"`
	Method          string `json:"method"`
	Params          any    `json:"params"`
}

func (n Notification) IsJSONRPC() bool {
	return n.ProtocolVersion == protocolVersion
}

func Read(r *bufio.Reader) (Request, error) {
	req := Request{}
	header, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		return req, err
	}
	contentLength, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	if err != nil {
		return req, ErrInvalidRequest
	}
	err = json.NewDecoder(io.LimitReader(r, contentLength)).Decode(&req)
	if err != nil {
		return req, nil
	}
	if !req.IsJSONRPC() {
		return req, ErrInvalidRequest
	}
	return req, nil
}

func Write(w *bufio.Writer, msg Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = w.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)))
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	if err != nil {
		return err
	}
	err = w.Flush()
	return err
}

func NewMux(log *slog.Logger, r io.Reader, w io.Writer) *Mux {
	return &Mux{
		reader:               bufio.NewReader(r),
		concurrencyLimit:     4,
		methodHandlers:       map[string]MethodHandler{},
		notificationHandlers: map[string]NotificationHandler{},
		writer:               bufio.NewWriter(w),
		writeLock:            &sync.Mutex{},
		Log:                  log,
		error: func(err error) {
			return
		},
	}
}

type Mux struct {
	initialized          bool
	reader               *bufio.Reader
	concurrencyLimit     int64
	methodHandlers       map[string]MethodHandler
	notificationHandlers map[string]NotificationHandler
	writer               *bufio.Writer
	writeLock            *sync.Mutex
	Log                  *slog.Logger
	error                func(err error)
}

type MethodHandler func(params json.RawMessage) (any, error)
type NotificationHandler func(params json.RawMessage) error

func (m *Mux) HandleMethod(name string, method MethodHandler) {
	m.methodHandlers[name] = method
}

func (m *Mux) HandleNotification(name string, notification NotificationHandler) {
	m.notificationHandlers[name] = notification
}

func (m *Mux) Notify(method string, params any) error {
	n := Notification{
		ProtocolVersion: protocolVersion,
		Method:          method,
		Params:          params,
	}
	return m.write(n)
}

func (m *Mux) Request(method string, params any) error {
	r := OutgoingRequest{
		ProtocolVersion: protocolVersion,
		ID:              "1",
		Method:          method,
		Params:          params,
	}
	return m.write(r)
}

func (m *Mux) write(msg Message) error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()
	return Write(m.writer, msg)
}

func (m *Mux) Process() error {
	for {
		req, err := Read(m.reader)
		if err != nil {
			return err
		}
		if req.IsNotification() {
			if req.Method != "exit" {
				m.Log.Warn("Dropping notification sent before initialization", slog.Any("req", req))
				continue
			}
			m.handleMessage(req)
			continue
		}
		if req.Method != "initialize" {
			m.Log.Warn("The client sent a method before initialization", slog.Any("req", req))
			if err = m.write(NewResponseError(req.ID, ErrServerNotInitialized)); err != nil {
				return err
			}
			continue
		}
		m.handleMessage(req)
		break
	}
	m.Log.Info("Initialization complete")

	sem := make(chan struct{}, m.concurrencyLimit)
	for {
		sem <- struct{}{}
		req, err := Read(m.reader)
		if err != nil {
			return err
		}
		go func(req Request) {
			m.handleMessage(req)
			<-sem
		}(req)
	}
}

func (m *Mux) handleMessage(req Request) {
	if req.IsNotification() {
		m.handleNotification(req)
		return
	}
	m.handleRequestResponse(req)
}

func (m *Mux) handleNotification(req Request) {
	log := m.Log.With(slog.String("method", req.Method))
	nh, ok := m.notificationHandlers[req.Method]
	if !ok {
		log.Warn("No notification handler found")
		return
	}
	if err := nh(req.Params); err != nil && m.error != nil {
		log.Error("Failed to handle notification", slog.Any("error", err))
		m.error(err)
	}
}

func (m *Mux) handleRequestResponse(req Request) {
	log := m.Log.With(slog.Any("id", req.ID), slog.String("method", req.Method))
	if req.Method == "" {
		// The method for window/showDocument is empty, nothing to handle on that response
		return
	}
	mh, ok := m.methodHandlers[req.Method]
	if !ok {
		log.Error("No method handler found")
		if err := m.write(NewResponseError(req.ID, ErrMethodNotFound)); err != nil {
			log.Error("Failed to respond", slog.Any("error", err))
			m.error(fmt.Errorf("Failed to respond: %w", err))
		}
		return
	}
	var res Response
	result, err := mh(req.Params)
	if err != nil {
		log.Error("Failed to handle", slog.Any("error", err))
		res = NewResponseError(req.ID, err)
	} else {
		res = NewResponse(req.ID, result)
	}
	if err = m.write(res); err != nil {
		log.Error("Failed to respond", slog.Any("error", err))
		m.error(fmt.Errorf("Failed to response: %w", err))
	}
}
