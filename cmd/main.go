package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"

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
		slog.Error("Failed to create `yamlls` dir in cache directory", "cache_dir", cacheDir)
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
			diagnostics := []messages.Diagnostic{}
			diagnostics = append(diagnostics, getKind(doc.Text)...)
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
