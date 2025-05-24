package main

import (
	"testing"

	"github.com/slarwise/yamlls/pkg/schema2"
	"go.lsp.dev/protocol"
)

func TestValidateFile(t *testing.T) {
	store, err := schema2.NewKubernetesStore()
	if err != nil {
		t.Fatalf("unexepcted error: %v", err)
	}
	tests := map[string]struct {
		contents    string
		diagnostics []protocol.Diagnostic
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
			diagnostics, err := validateFile(test.contents, store)
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

func TestGetDescription(t *testing.T) {
	store, err := schema2.NewKubernetesStore()
	if err != nil {
		t.Fatalf("unexepcted error: %v", err)
	}
	tests := map[string]struct {
		contents    string
		line, char  int
		description string
	}{
		"top-level": {
			contents: `apiVersion: v1
kind: Service
spec: {}
`,
			line:        2,
			char:        0,
			description: "ServiceSpec describes the attributes that a user creates on a service.",
		},
		"second-level": {
			contents: `apiVersion: v1
kind: Service
spec:
  ports: {}
`,
			line:        3,
			char:        2,
			description: "The list of ports that are exposed by this service. More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies",
		},
		"two-docs": {
			contents: `apiVersion: v1
kind: Service
spec: {}
---
apiVersion: v1
kind: Service
spec:
  ports: {}
`,
			line:        5,
			char:        2,
			description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
		},
		"array": {
			contents: `apiVersion: v1
kind: Service
spec:
  ports:
    - name: hej
`,
			line:        4,
			char:        6,
			description: "The name of this port within the service. This must be a DNS_LABEL. All ports within a ServiceSpec must have unique names. When considering the endpoints for a Service, this must match the 'name' field in the EndpointPort. Optional if only one ServicePort is defined on this service.",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			description, err := getDescription(test.contents, test.line, test.char, store)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if description != test.description {
				t.Fatalf("expected `%s`, got `%s`", test.description, description)
			}
		})
	}
}
