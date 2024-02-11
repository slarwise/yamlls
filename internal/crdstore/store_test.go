package crdstore

import "testing"

var fileTreeResponse = `{
    "sha": "586facb829549bff7151567dd9a0d0e34cd8227a",
    "url": "https://api.github.com/repos/datreeio/CRDs-catalog/git/trees/586facb829549bff7151567dd9a0d0e34cd8227a",
    "tree": [
	    {
	        "type": "tree",
	        "sha": "56ac24e5fa46fbb4dda9a33edfc3615d70288baa",
	        "url": "https://api.github.com/repos/datreeio/CRDs-catalog/git/trees/56ac24e5fa46fbb4dda9a33edfc3615d70288baa"
	    },
	    {
	        "path": ".github/workflows",
	        "mode": "040000",
	        "type": "tree",
	        "sha": "922d7edd74ce6084ee78e72632401c00f04d42b9",
	        "url": "https://api.github.com/repos/datreeio/CRDs-catalog/git/trees/922d7edd74ce6084ee78e72632401c00f04d42b9"
	    },
	    {
	        "path": ".github/workflows/linter.yaml",
	        "mode": "100644",
	        "type": "blob",
	        "sha": "869d5e73bb965d5bf5d679b45aa784069228fb15",
	        "size": 402,
	        "url": "https://api.github.com/repos/datreeio/CRDs-catalog/git/blobs/869d5e73bb965d5bf5d679b45aa784069228fb15"
	    },
	    {
	        "path": "acid.zalan.do",
	        "mode": "040000",
	        "type": "tree",
	        "sha": "f2c8d3c1f02de1fb39927e376e3df833bc09230a",
	        "url": "https://api.github.com/repos/datreeio/CRDs-catalog/git/trees/f2c8d3c1f02de1fb39927e376e3df833bc09230a"
	    },
	    {
	        "path": "acid.zalan.do/operatorconfiguration_v1.json",
	        "mode": "100644",
	        "type": "blob",
	        "sha": "2f2051520259a196b35039a0282a76b4677646ab",
	        "size": 26299,
	        "url": "https://api.github.com/repos/datreeio/CRDs-catalog/git/blobs/2f2051520259a196b35039a0282a76b4677646ab"
	    }
    ]
}`

func TestParseFileTreeResponse(t *testing.T) {
	gvks, err := parseFileTreeResponse([]byte(fileTreeResponse))
	if err != nil {
		t.Fatalf("Got unexpected error: %s", err)
	}
	if len(gvks) != 1 {
		t.Fatalf("Expected to find 1 resource, got %d", len(gvks))
	}
}

func TestGetGroupVersionKindFromFilename(t *testing.T) {
	tests := map[string]struct {
		filename string
		group    string
		version  string
		kind     string
	}{
		"match": {
			filename: "acid.zalan.do/operatorconfiguration_v1.json",
			group:    "acid.zalan.do",
			version:  "v1",
			kind:     "operatorconfiguration",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			group, version, kind := getGroupVersionKindFromFilename(test.filename)
			t.Log(group, version, kind)
			if group != test.group {
				t.Fatalf("Expected %s, got %s", test.group, group)
			}
			if version != test.version {
				t.Fatalf("Expected %s, got %s", test.version, version)
			}
			if kind != test.kind {
				t.Fatalf("Expected %s, got %s", test.kind, kind)
			}
		})
	}
}

func TestBuildSchemaURL(t *testing.T) {
	group := "monitoring.coreos.com"
	version := "v1"
	kind := "alertmanager"
	expected := "https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/monitoring.coreos.com/alertmanager_v1.json"
	actual := buildSchemaURL(group, version, kind)
	if actual != expected {
		t.Fatalf("Expected %s, got %s", expected, actual)
	}
}
