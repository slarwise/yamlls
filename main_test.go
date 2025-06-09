package main

import (
	"testing"

	"github.com/slarwise/yamlls/pkg/schema2"
	"go.lsp.dev/protocol"
)

// TODO: Not really a reason to test this since it just wraps
// schema2.ValidateFile. Merge these tests with schema_test
func TestValidateFile(t *testing.T) {
	store, err := schema2.NewStore()
	if err != nil {
		t.Fatalf("unexepcted error: %v", err)
	}
	tests := map[string]struct {
		contents    string
		diagnostics []protocol.Diagnostic
		filename    string
	}{
		"valid": {
			contents: `apiVersion: apps/v1
kind: Deployment
`,
			diagnostics: nil,
		},
		"invalid-yaml": {
			contents: `apiVersion: v1
metadata: {
`,
			diagnostics: []protocol.Diagnostic{
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 0, Character: 0},
						End:   protocol.Position{Line: 2, Character: 0},
					},
					Severity: protocol.DiagnosticSeverityError,
				},
			},
		},
		"invalid-according-to-schema": {
			contents: `apiVersion: v1
kind: Service
spec:
  asdf: hej
`,
			diagnostics: []protocol.Diagnostic{
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 3, Character: 2},
						End:   protocol.Position{Line: 3, Character: 6},
					},
					Severity: protocol.DiagnosticSeverityError,
				},
			},
		},
		"two-documents": {
			contents: `apiVersion: v1
kind: Namespace
metadata:
  name: mynamespace
---
apiVersion: v1
kind: Service
spec:
  asdf: hej
`,
			diagnostics: []protocol.Diagnostic{
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 8, Character: 2},
						End:   protocol.Position{Line: 8, Character: 6},
					},
					Severity: protocol.DiagnosticSeverityError,
				},
			},
		},
		"error-in-array": {
			contents: `apiVersion: v1
kind: Service
metadata:
  name: hej
spec:
  ports:
    - por: 8080
      name: asdf
    - port: 3000
      nam: hej
`,
			diagnostics: []protocol.Diagnostic{
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 6, Character: 4},
						End:   protocol.Position{Line: 6, Character: 5},
					},
					Severity: protocol.DiagnosticSeverityError,
				},
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 6, Character: 6},
						End:   protocol.Position{Line: 6, Character: 9},
					},
					Severity: protocol.DiagnosticSeverityError,
				},
				{
					Range: protocol.Range{
						Start: protocol.Position{Line: 9, Character: 6},
						End:   protocol.Position{Line: 9, Character: 9},
					},
					Severity: protocol.DiagnosticSeverityError,
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			diagnostics, err := validateFile(test.contents, test.filename, store)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(diagnostics) != len(test.diagnostics) {
				t.Fatalf("Expected %d diagnostics, got %d", len(test.diagnostics), len(diagnostics))
			}
			for i, d := range diagnostics {
				expected := test.diagnostics[i]
				if d.Range != expected.Range {
					t.Fatalf("expected range to be `%v`, got `%v`", expected.Range, d.Range)
				}
			}
		})
	}
}
