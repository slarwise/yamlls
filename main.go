package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/slarwise/yamlls/pkg/schema"

	"github.com/goccy/go-yaml"
	"go.lsp.dev/protocol"
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
		panic("TODO: start the language server")
	} else {
		subCommand, args := os.Args[1], os.Args[2:]
		switch subCommand {
		case "schemas":
			if err := listSchemas(); err != nil {
				return fmt.Errorf("list schemas: %s", err)
			}
		case "docs":
			if len(args) == 0 {
				return fmt.Errorf("must provide `kind`, e.g. yamlls list-schemas deployment")
			}
			kind := args[0]
			if err := showDocs(kind); err != nil {
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
			panic(fmt.Sprintf("TODO: Validate file `%s` against its schema", file))
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
			baseName := gvk.Kind + "-"
			if gvk.Group != "" {
				group := strings.Split(gvk.Group, ".")[0]
				baseName += group + "-"
			}
			baseName += gvk.Version + ".json"
			schemaUrl := fmt.Sprintf("%s/%s", NATIVE_SCHEMAS_BASE_URL, strings.ToLower(baseName))
			schema, err := httpGet(schemaUrl)
			if err != nil {
				return fmt.Errorf("get schema: %s", err)
			}
			filename := filepath.Join(DB_DIR, baseName)
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
				baseName := d.Kind + "-"
				split := strings.Split(d.ApiVersion, "/")
				if len(split) != 2 {
					return fmt.Errorf("expected apiVersion to have exactly one `/`, got %s", d.ApiVersion)
				}
				group, version := split[0], split[1]
				baseName += group + "-" + version + ".json"
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

func showDocs(kind string) error {
	ids, err := schemaIds()
	if err != nil {
		return fmt.Errorf("get schema ids: %s", err)
	}
	kind = strings.ToLower(kind)
	var matches []string
	for _, id := range ids {
		currentKind := strings.Split(id, "-")[0]
		if strings.ToLower(currentKind) == kind {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 0:
		return fmt.Errorf("no schema found matching kind `%s`", kind)
	case 1:
		fmt.Println(matches[0])
	default:
		found := false
		for _, id := range matches {
			gvk := parseSchemaId(id)
			// Favor native schemas. These have no group name or one without a dot
			if gvk.group == "" || strings.Count(gvk.group, ".") == 0 {
				fmt.Println(id)
				found = true
				break
			}
		}
		if !found {
			fmt.Println(matches[0])
		}
	}
	panic("TODO: Create and print the html docs")
}

type GVK struct{ group, version, kind string }

func parseSchemaId(id string) GVK {
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

// func languageServer() {
// 	logpath := filepath.Join(yamllsCacheDir, "log")
// 	logfile, err := os.Create(logpath)
// 	if err != nil {
// 		slog.Error("Failed to create log output file", "error", err)
// 		os.Exit(1)
// 	}

// 	kubernetesStore, err := schema.NewKubernetesStore()
// 	if err != nil {
// 		slog.Error("create kubernetes store", "err", err)
// 		os.Exit(1)
// 	}

// 	defer logfile.Close()
// 	logger = slog.New(slog.NewJSONHandler(logfile, nil))
// 	defer func() {
// 		if r := recover(); r != nil {
// 			logger.Error("panic", "recovered", r)
// 		}
// 	}()

// 	m := lsp.NewMux(logger, os.Stdin, os.Stdout)

// 	filenameToContents := map[string]string{}

// 	m.HandleMethod("initialize", func(params json.RawMessage) (any, error) {
// 		var initializeParams protocol.InitializeParams
// 		if err = json.Unmarshal(params, &initializeParams); err != nil {
// 			return nil, err
// 		}
// 		logger.Info("Received initialize request", "params", initializeParams)
// 		// TODO: Support filenameOverrides

// 		result := protocol.InitializeResult{
// 			Capabilities: protocol.ServerCapabilities{
// 				TextDocumentSync:   protocol.TextDocumentSyncKindFull,
// 				HoverProvider:      true,
// 				CodeActionProvider: true,
// 				ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
// 					Commands: []string{"open-docs"},
// 				},
// 			},
// 			ServerInfo: &protocol.ServerInfo{Name: "yamlls"},
// 		}
// 		return result, nil
// 	})

// 	m.HandleNotification("initialized", func(params json.RawMessage) error {
// 		logger.Info("Receivied initialized notification", "params", params)
// 		return nil
// 	})

// 	m.HandleMethod("shutdown", func(params json.RawMessage) (any, error) {
// 		logger.Info("Received shutdown request")
// 		return nil, nil
// 	})

// 	exitChannel := make(chan int, 1)
// 	m.HandleNotification("exit", func(params json.RawMessage) error {
// 		logger.Info("Received exit notification")
// 		exitChannel <- 1
// 		return nil
// 	})

// 	documentUpdates := make(chan protocol.TextDocumentItem, 10)
// 	go func() {
// 		for doc := range documentUpdates {
// 			filenameToContents[doc.URI.Filename()] = doc.Text
// 			diagnostics, err := validateFile(doc.Text, kubernetesStore)
// 			if err != nil {
// 				logger.Error("validate file", "err", err)
// 			}
// 			m.Notify(protocol.MethodTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
// 				URI:         doc.URI,
// 				Version:     uint32(doc.Version),
// 				Diagnostics: diagnostics,
// 			})
// 		}
// 	}()

// 	m.HandleNotification(protocol.MethodTextDocumentDidOpen, func(rawParams json.RawMessage) error {
// 		logger.Info("Received TextDocument/didOpen notification")
// 		var params protocol.DidOpenTextDocumentParams
// 		if err := json.Unmarshal(rawParams, &params); err != nil {
// 			return err
// 		}
// 		documentUpdates <- params.TextDocument
// 		return nil
// 	})

// 	m.HandleNotification(protocol.MethodTextDocumentDidChange, func(rawParams json.RawMessage) error {
// 		logger.Info("Received textDocument/didChange notification")
// 		var params protocol.DidChangeTextDocumentParams
// 		if err := json.Unmarshal(rawParams, &params); err != nil {
// 			return err
// 		}
// 		documentUpdates <- protocol.TextDocumentItem{URI: params.TextDocument.URI, Version: params.TextDocument.Version, Text: params.ContentChanges[0].Text}
// 		return nil
// 	})

// 	m.HandleMethod("textDocument/hover", func(rawParams json.RawMessage) (any, error) {
// 		logger.Info("Received textDocument/hover request")
// 		var params protocol.HoverParams
// 		if err := json.Unmarshal(rawParams, &params); err != nil {
// 			return nil, err
// 		}
// 		contents := filenameToContents[params.TextDocument.URI.Filename()]
// 		documentation, err := kubernetesStore.DocumentationAtCursor(contents, int(params.Position.Line), int(params.Position.Character))
// 		if err != nil {
// 			logger.Error("failed to get description", "line", params.Position.Line, "char", params.Position.Character, "err", err)
// 			return nil, nil
// 		} else if documentation.Description == "" {
// 			return nil, nil
// 		}

// 		return protocol.Hover{
// 			Contents: protocol.MarkupContent{
// 				Kind:  protocol.PlainText,
// 				Value: documentation.Description,
// 			},
// 		}, nil
// 	})

// 	m.HandleMethod("textDocument/completion", func(rawParams json.RawMessage) (any, error) {
// 		logger.Info("Receiver textDocument/completion request, not supported")
// 		return nil, nil
// 	})

// 	m.HandleMethod(protocol.MethodTextDocumentCodeAction, func(rawParams json.RawMessage) (any, error) {
// 		logger.Info("Received textDocument/codeAction request, not supported")
// 		return nil, nil
// 	})

// 	m.HandleMethod(protocol.MethodTextDocumentCodeAction, func(rawParams json.RawMessage) (any, error) {
// 		logger.Info(fmt.Sprintf("Received %s request", protocol.MethodTextDocumentCodeAction))
// 		var params protocol.CodeActionParams
// 		if err := json.Unmarshal(rawParams, &params); err != nil {
// 			return nil, err
// 		}
// 		contents := filenameToContents[params.TextDocument.URI.Filename()]
// 		documentation, found := kubernetesStore.HtmlDocumentation(contents, int(params.Range.Start.Line), int(params.Range.Start.Character))
// 		if !found {
// 			return "", errors.New("no schema found")
// 		}
// 		filename := filepath.Join(cacheDir, "docs.html")
// 		if err := os.WriteFile(filename, []byte(documentation), 0755); err != nil {
// 			slog.Error("write html documentation to file", "err", err, "file", filename)
// 			return "", errors.New("failed to write docs to file")
// 		}
// 		htmlDocsUri := "file://" + filename
// 		response := []protocol.CodeAction{
// 			{
// 				Title: "Open documentation",
// 				Command: &protocol.Command{
// 					Title:     "Open documentation",
// 					Command:   "open-docs",
// 					Arguments: []any{htmlDocsUri},
// 				},
// 			},
// 		}
// 		return response, nil
// 	})

// 	m.HandleMethod(protocol.MethodWorkspaceExecuteCommand, func(rawParams json.RawMessage) (any, error) {
// 		logger.Info(fmt.Sprintf("Received %s request", protocol.MethodWorkspaceExecuteCommand))
// 		var params protocol.ExecuteCommandParams
// 		if err := json.Unmarshal(rawParams, &params); err != nil {
// 			return nil, err
// 		}
// 		logger.Info("Received command", "command", params.Command, "args", params.Arguments)
// 		switch params.Command {
// 		case "open-docs":
// 			if len(params.Arguments) != 1 {
// 				return "", fmt.Errorf("Must provide 1 argument to open-docs, the uri")
// 			}
// 			viewerURL := params.Arguments[0].(string)
// 			uri := uri.URI(viewerURL)
// 			showDocumentParams := protocol.ShowDocumentParams{
// 				URI:       uri,
// 				External:  true,
// 				TakeFocus: true,
// 			}
// 			m.Request("window/showDocument", showDocumentParams)
// 		default:
// 			return "", fmt.Errorf("Command not found %s", params.Command)
// 		}
// 		return "", nil
// 	})

// 	logger.Info("Handler set up", "log_path", logpath)

// 	go func() {
// 		if err := m.Process(); err != nil {
// 			logger.Error("Processing stopped", "error", err)
// 			os.Exit(1)
// 		}
// 	}()

// 	<-exitChannel
// 	logger.Info("Server exited")
// 	os.Exit(1)
// }

func validateFile(contents string, store schema.KubernetesStore) ([]protocol.Diagnostic, error) {
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
