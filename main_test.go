package main

import (
	"testing"
)

func TestValidateFile(t *testing.T) {
	tests := map[string]struct {
		contents string
		errors   []ValidationError
	}{
		"one-document/valid": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: myingress
spec:
  rules:
    - host: example.com
      http:
        paths:
          - path: /
            pathType: ImplementationSpecific
            backend:
              service:
                name: myapp
`,
			errors: nil,
		},
		"one-document/invalid-yaml": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
spec: [
`,
			errors: []ValidationError{
				{
					Range:    newRange(0, 0, 3, 0),
					Type:     "invalid_yaml",
					Severity: SEVERITY_ERROR,
				},
			},
		},
		"one-document/no-schema-found": {
			contents: `kind: Ingress
apiVersion: does-not-exist
`,
			errors: []ValidationError{
				{
					Range:    newRange(0, 0, 0, 0),
					Type:     "no_schema_found",
					Severity: SEVERITY_WARN,
				},
			},
		},
		"two-documents/valid": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: myingress
spec:
  rules:
    - host: example.com
      http:
        paths:
          - path: /
            pathType: ImplementationSpecific
            backend:
              service:
                name: myapp
---
kind: Service
apiVersion: v1
metadata:
  name: myapp
spec:
  ports:
    - port: 8080
      name: http
`,
			errors: nil,
		},
		"two-documents/invalid-yaml": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
metadata:
  name: myingress
spec:
  rules:
    - host: example.com
      http:
        paths:
          - path: /
            pathType: ImplementationSpecific
            backend:
              service:
                name: myapp
---
kind: [
apiVersion: v1
metadata:
  name: myapp
spec:
  ports:
    - port: 8080
      name: http
`,
			errors: []ValidationError{
				{
					Range:    newRange(15, 0, 23, 0),
					Type:     "invalid_yaml",
					Severity: SEVERITY_ERROR,
				},
			},
		},
		"two-document/no-schema-found": {
			contents: `kind: Ingress
apiVersion: does-not-exist
---
kind: Service
apiVersion: v1
metadata:
  name: myapp
spec:
  ports:
    - port: 8080
      name: http
`,
			errors: []ValidationError{
				{
					Range:    newRange(0, 0, 0, 0),
					Type:     "no_schema_found",
					Severity: SEVERITY_WARN,
				},
			},
		},
		"one-document/no-kind-and-apiVersion": {
			contents: `hej: du
`,
			errors: nil,
		},
		"one-document/additional-property": {
			contents: `kind: Ingress
apiVersion: networking.k8s.io/v1
hej: du
`,
			errors: []ValidationError{
				{
					Range:    newRange(2, 0, 2, 3),
					Type:     "additional_property_not_allowed",
					Severity: SEVERITY_ERROR,
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			errors, fail := validateFile(test.contents)
			if fail != VALIDATION_FAILURE_REASON_NOT_A_FAILURE {
				t.Fatalf("expected validation to work, got %s", fail)
			}
			if len(errors) != len(test.errors) {
				t.Fatalf("expected %d errors, got %v", len(test.errors), errors)
			}
			for i := range errors {
				expectedError := test.errors[i]
				if errors[i].Type != expectedError.Type {
					t.Fatalf("expected type `%s`, got `%s`", expectedError.Type, errors[i].Type)
				}
				if errors[i].Range != expectedError.Range {
					t.Fatalf("expected range %v, got %v", expectedError.Range, errors[i].Range)
				}
				if errors[i].Severity != expectedError.Severity {
					t.Fatalf("expected severity %v, got %v", expectedError.Severity, errors[i].Severity)
				}
			}
		})
	}
}
