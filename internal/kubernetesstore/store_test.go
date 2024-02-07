package kubernetesstore

import (
	"testing"
)

func TestBuildSchemaURL(t *testing.T) {
	tests := map[string]struct {
		group, version, kind string
		expected             string
	}{
		"no-group": {
			group:    "",
			version:  "v1",
			kind:     "Pod",
			expected: "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/pod-v1.json",
		},
		"with-group": {
			group:    "apps",
			version:  "v1",
			kind:     "Deployment",
			expected: "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/master-standalone-strict/deployment-apps-v1.json",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := buildSchemaURL(test.group, test.version, test.kind)
			if actual != test.expected {
				t.Fatalf("Expected %s, got %s", test.expected, actual)
			}
		})
	}
}

func TestIsKnownGroupVersionKind(t *testing.T) {
	index := []GroupVersionKind{
		{
			Group:   "",
			Version: "v1",
			Kind:    "Pod",
		},
		{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		},
	}
	tests := map[string]struct {
		group, version, kind string
		expected             bool
	}{
		"known-no-group": {
			group:    "",
			version:  "v1",
			kind:     "Pod",
			expected: true,
		},
		"known-with-group": {
			group:    "apps",
			version:  "v1",
			kind:     "Deployment",
			expected: true,
		},
		"unknown": {
			group:    "apps",
			version:  "v2",
			kind:     "Deployment",
			expected: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := isKnownGroupVersionKind(index, test.group, test.version, test.kind)
			if actual != test.expected {
				t.Fatalf("Expected %t, got %t", test.expected, actual)
			}
		})
	}
}

func TestParseIndex(t *testing.T) {
	indexResponse := `{
  "definitions": {
    "io.k8s.api.apps.v1.Deployment": {
      "description": "Deployment enables declarative updates for Pods and ReplicaSets.",
      "properties": {
        "apiVersion": {
          "description": "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
          "type": "string"
        },
        "kind": {
          "description": "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
          "type": "string",
          "enum": [
            "Deployment"
          ]
        },
        "metadata": {
          "$ref": "#/definitions/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta",
          "description": "Standard object's metadata. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata"
        },
        "spec": {
          "$ref": "#/definitions/io.k8s.api.apps.v1.DeploymentSpec",
          "description": "Specification of the desired behavior of the Deployment."
        },
        "status": {
          "$ref": "#/definitions/io.k8s.api.apps.v1.DeploymentStatus",
          "description": "Most recently observed status of the Deployment."
        }
      },
      "type": "object",
      "x-kubernetes-group-version-kind": [
        {
          "group": "apps",
          "kind": "Deployment",
          "version": "v1"
        }
      ],
      "additionalProperties": false
    }
  }
}`
	index, err := parseIndexResponse([]byte(indexResponse))
	if err != nil {
		t.Fatalf("Got unexpected error when parsing the index response: %s", err)
	}
	if len(index) != 1 {
		t.Fatalf("Expected 1 item in index, got %d", len(index))
	}
	if index[0].Group != "apps" || index[0].Version != "v1" || index[0].Kind != "Deployment" {
		t.Fatalf("Expected index to have apps/v1 Deployment, got %v", index[0])
	}
}
