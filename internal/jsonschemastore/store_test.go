package jsonschemastore

import "testing"

func TestParseIndexResponse(t *testing.T) {
	indexResponse := `{
	"schemas": [
		{
			"name": "1Password SSH Agent Config",
			"url": "https://developer.1password.com/schema/ssh-agent-config.json",
			"fileMatch": [
				"**/1password/ssh/agent.toml"
			]
		},
		{
			"name": "Application Accelerator",
			"url": "https://json.schemastore.org/accelerator.json",
			"fileMatch": [
				"accelerator.yaml"
			]
		}
	]
}`
	index, err := parseIndexResponse([]byte(indexResponse))
	if err != nil {
		t.Fatalf("Did not expect an error, got %s", err)
	}
	if len(index) != 2 {
		t.Fatalf("Expected index to have 2 elements, got %d", len(index))
	}
	expectedURL := "https://json.schemastore.org/accelerator.json"
	if index[1].URL != expectedURL {
		t.Fatalf("Expected %s, got %s", expectedURL, index[1].URL)
	}
}

func TestMatchFilePattern(t *testing.T) {
	tests := map[string]struct {
		pattern  string
		filename string
		match    bool
	}{
		"basename-match": {
			pattern:  "kustomization.yaml",
			filename: "kustomization.yaml",
			match:    true,
		},
		"basename-no-match": {
			pattern:  "kustomization.yaml",
			filename: "kustomization.yml",
			match:    false,
		},
		"full-path-match": {
			pattern:  "**/.github/workflows/*.yaml",
			filename: "/home/.github/workflows/build.yaml",
			match:    true,
		},
		"full-path-no-match": {
			pattern:  "**/.github/workflows/*.yaml",
			filename: ".github/workflws/build.yaml",
			match:    false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			match := matchFilePattern(test.pattern, test.filename)
			if match != test.match {
				t.Fatalf("Expected %t, got %t", test.match, match)
			}
		})
	}
}
