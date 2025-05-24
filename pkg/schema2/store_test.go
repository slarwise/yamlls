package schema2

import "testing"

func TestFindKindAndApiVersion(t *testing.T) {
	tests := map[string]struct {
		contents   string
		kind       string
		apiVersion string
	}{
		"found": {
			contents: `apiVersion: v1
kind: Service
`,
			kind:       "Service",
			apiVersion: "v1",
		},
		"not-found": {
			contents: `hej: v1
`,
			kind:       "",
			apiVersion: "",
		},
		"double-quoted": {
			contents: `kind: "Service"
apiVersion: "v1"
`,
			kind:       "Service",
			apiVersion: "v1",
		},
		"single-quoted": {
			contents: `kind: 'Service'
apiVersion: 'v1'
`,
			kind:       "Service",
			apiVersion: "v1",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kind, apiVersion := findKindAndApiVersion(test.contents)
			if kind != test.kind {
				t.Fatalf("expected kind to be `%s`, got `%s`", test.kind, kind)
			}
			if apiVersion != test.apiVersion {
				t.Fatalf("expected apiVersion to be `%s`, got `%s`", test.apiVersion, apiVersion)
			}
		})
	}
}
