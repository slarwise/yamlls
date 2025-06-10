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

func TestFileMatch(t *testing.T) {
	tests := map[string]struct {
		fileMatch   string
		filename    string
		shouldMatch bool
	}{
		"simple-match": {
			fileMatch:   ".prettierrc",
			filename:    "/Users/tonyzaret/projects/thejoker/.prettierrc",
			shouldMatch: true,
		},
		"simple-no-match": {
			fileMatch:   "/Users/tonyzaret/projects/thejoker/.prettierrc",
			filename:    ".vimrc",
			shouldMatch: false,
		},
		"double-star-match": {
			fileMatch:   "**/.dependabot/config.yml",
			filename:    "/usr/collengreen/home/tvismyfriend/.dependabot/config.yml",
			shouldMatch: true,
		},
		"double-star-no-match": {
			fileMatch:   "**/.dependabot/config.yml",
			filename:    "/usr/collengreen/home/tvismyfriend/and-it-has-been/config.yml",
			shouldMatch: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := checkFileMatch(test.fileMatch, test.filename)
			if actual != test.shouldMatch {
				t.Fatalf("expected %v, got %v", test.shouldMatch, actual)
			}
		})
	}
}
