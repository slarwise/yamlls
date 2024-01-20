package parser

import (
	"testing"
)

func TestGetKindApiVersion(t *testing.T) {
	tests := map[string]struct {
		kind       string
		apiVersion string
		text       string
	}{
		"kubernetes": {
			kind:       "Service",
			apiVersion: "v1",
			text:       "kind: Service\napiVersion: v1",
		},
		"CRD": {
			kind:       "Kustomization",
			apiVersion: "kustomize.config.k8s.io/v1beta1",
			text:       "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization",
		},
		"no kind": {
			kind:       "",
			apiVersion: "kustomize.config.k8s.io/v1beta1",
			text:       "apiVersion: kustomize.config.k8s.io/v1beta1",
		},
		"no apiVersion": {
			kind:       "Kustomization",
			apiVersion: "",
			text:       "kind: Kustomization",
		},
		"empty": {
			kind:       "",
			apiVersion: "",
			text:       "",
		},
		"not yaml": {
			kind:       "",
			apiVersion: "",
			text:       "Hello\nWorld",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kind, apiVersion := GetKindApiVersion(test.text)
			if kind != test.kind {
				t.Fatalf("Expected `%s`, got `%s`", test.kind, kind)
			}
			if apiVersion != test.apiVersion {
				t.Fatalf("Expected `%s`, got `%s`", test.apiVersion, apiVersion)
			}
		})
	}
}
