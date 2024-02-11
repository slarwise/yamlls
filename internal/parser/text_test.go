package parser

import (
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
