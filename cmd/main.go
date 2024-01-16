package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/slarwise/yamlls/pkg/lsp"
	"github.com/slarwise/yamlls/pkg/messages"
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
	}
	defer logfile.Close()
	log := slog.New(slog.NewJSONHandler(logfile, nil))
	defer func() {
		if r := recover(); r != nil {
			log.Error("panic", "recovered", r)
		}
	}()

	if err := os.MkdirAll(path.Join(cacheDir, "yamlls", "schemas"), 0755); err != nil {
		slog.Error("Failed to create `yamlls/schemas` dir in cache directory", "cache_dir", cacheDir, "error", err)
	}

	m := lsp.NewMux(log, os.Stdin, os.Stdout)

	fileURIToContents := map[string]string{}

	m.HandleMethod("initialize", func(params json.RawMessage) (any, error) {
		var initializeParams messages.InitializeParams
		if err = json.Unmarshal(params, &initializeParams); err != nil {
			return nil, err
		}
		log.Info("Received initialize request", "params", initializeParams)

		result := messages.InitializeResult{
			Capabilities: messages.ServerCapabilities{
				TextDocumentSync: messages.TextDocumentSyncKindFull,
				HoverProvider:    true,
			},
			ServerInfo: &messages.ServerInfo{
				Name: "yamlls",
			},
		}
		return result, nil
	})

	m.HandleNotification("initialized", func(params json.RawMessage) error {
		log.Info("Receivied initialized notification", "params", params)
		return nil
	})

	m.HandleMethod("shutdown", func(params json.RawMessage) (any, error) {
		log.Info("Received shutdown request")
		return nil, nil
	})

	exitChannel := make(chan int, 1)
	m.HandleNotification("exit", func(params json.RawMessage) error {
		log.Info("Received exit notification")
		exitChannel <- 1
		return nil
	})

	documentUpdates := make(chan messages.TextDocumentItem, 10)
	go func() {
		for doc := range documentUpdates {
			fileURIToContents[doc.URI] = doc.Text
			log.Info("In channel goroutine", "fileURIToContents", fileURIToContents)
			diagnostics := []messages.Diagnostic{}
			// diagnostics = append(diagnostics, getKind(doc.Text)...)
			diagnostics = append(diagnostics, getDocs(doc.Text)...)
			m.Notify(messages.PublishDiagnosticsMethod, messages.PublishDiagnosticsParams{
				URI:         doc.URI,
				Version:     &doc.Version,
				Diagnostics: diagnostics,
			})
		}
	}()

	m.HandleNotification(messages.DidOpenTextDocumentNotification, func(rawParams json.RawMessage) error {
		log.Info("Received didOpenTextDocument notification")
		var params messages.DidOpenTextDocumentParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return err
		}
		documentUpdates <- params.TextDocument
		return nil
	})

	m.HandleNotification(messages.DidChangeTextDocumentNotification, func(rawParams json.RawMessage) error {
		log.Info("Received didChangeTextDocument notification")
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
		log.Info("Received textDocument/hover request")
		var params messages.HoverParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, err
		}
		text := fileURIToContents[params.TextDocument.URI]
		kind, apiVersion, found := getKindAndApiVersion(text)
		if !found {
			return nil, errors.New("Not found")
		}
		schemaUrl := getExternalDocumentation(kind, apiVersion)
		if schemaUrl == "" {
			return nil, errors.New("Not found")
		}
		resp, err := http.Get(schemaUrl)
		if err != nil || resp.StatusCode != 200 {
			return nil, errors.New("Not found")
		}
		defer resp.Body.Close()
		decoded := make(map[string]interface{})
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			return nil, errors.New("Not found")
		}
		word := getCurrentWord(params.Position, text)
		properties := decoded["properties"].(map[string]interface{})
		object, found := properties[word]
		if !found {
			return nil, errors.New("Not found")
		}
		description := object.(map[string]interface{})["description"]
		return messages.HoverResult{
			Contents: messages.MarkupContent{
				Kind:  "markdown",
				Value: description.(string),
			},
		}, nil
	})

	log.Info("Handler set up", "log_path", logpath)

	go func() {
		if err := m.Process(); err != nil {
			log.Error("Processing stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-exitChannel
	log.Info("Server exited")
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
	url := getExternalDocumentation(kind.(string), apiVersion.(string))
	if url == "" {
		return nil
	}
	d := messages.Diagnostic{
		Range: messages.Range{
			Start: messages.NewPosition(0, 0),
			End:   messages.NewPosition(1, 0),
		},
		Severity: ptr(messages.DiagnosticSeverityHint),
		Message:  fmt.Sprintf("Docs: %s", url),
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
