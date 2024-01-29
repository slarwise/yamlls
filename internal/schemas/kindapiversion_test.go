package schemas

import "testing"

func TestIsCRDFile(t *testing.T) {
	tests := map[string]struct {
		filename string
		match    bool
	}{
		"match": {
			filename: "monitoring.coreos.com/alertmanager_v1.json",
			match:    true,
		},
		"top-level-file": {
			filename: "README.md",
			match:    false,
		},
		"not-a-crd": {
			filename: "some-dir/test.json",
			match:    false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			match := isCRDFile(test.filename)
			if match != test.match {
				t.Fatalf("Expected %t, got %t", test.match, match)
			}
		})
	}
}

func TestKindApiVersionURLs(t *testing.T) {
	URLs, err := kindApiVersionURLs()
	if err != nil {
		t.Fatalf("Failed to get urls: %s", err)
	}
	t.Log(URLs)
}
