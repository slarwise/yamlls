package parser

import (
	"slices"
	"testing"
)

func TestGetGroupKindVersion(t *testing.T) {
	tests := map[string]struct {
		group   string
		version string
		kind    string
		text    string
	}{
		"kubernetes": {
			group:   "",
			version: "v1",
			kind:    "Service",
			text:    "kind: Service\napiVersion: v1",
		},
		"CRD": {
			group:   "kustomize.config.k8s.io",
			version: "v1beta1",
			kind:    "Kustomization",
			text:    "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization",
		},
		"no kind": {
			group:   "kustomize.config.k8s.io",
			version: "v1beta1",
			kind:    "",
			text:    "apiVersion: kustomize.config.k8s.io/v1beta1",
		},
		"no apiVersion": {
			group:   "",
			version: "",
			kind:    "Kustomization",
			text:    "kind: Kustomization",
		},
		"empty": {
			group:   "",
			version: "",
			kind:    "",
			text:    "",
		},
		"not yaml": {
			group:   "",
			version: "",
			kind:    "",
			text:    "Hello\nWorld",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			group, version, kind := GetGroupVersionKind(test.text)
			if group != test.group {
				t.Fatalf("Expected `%s`, got `%s`", test.group, group)
			}
			if version != test.version {
				t.Fatalf("Expected `%s`, got `%s`", test.version, version)
			}
			if kind != test.kind {
				t.Fatalf("Expected `%s`, got `%s`", test.kind, kind)
			}
		})
	}
}

func TestSplitIntoYamlDocuments(t *testing.T) {
	tests := map[string]struct {
		text     string
		expected []string
	}{
		"one-document": {
			text:     "a",
			expected: []string{"a"},
		},
		"two-documents": {
			text:     "a\n---\nb",
			expected: []string{"a\n", "b"},
		},
		"prefix": {
			text:     "---\na\n---\nb",
			expected: []string{"a\n", "b"},
		},
		"suffix": {
			text:     "a\n---\nb\n---",
			expected: []string{"a\n", "b\n"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := SplitIntoYamlDocuments(test.text)
			if !slices.Equal(actual, test.expected) {
				t.Fatalf("Expected %v, got %v", test.expected, actual)
			}
		})
	}
}
