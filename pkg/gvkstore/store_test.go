package gvkstore

import "testing"

func TestGetKubernetesGVKs(t *testing.T) {
	gvks, err := getKubernetesGVKs()
	if err != nil {
		t.Fatalf("Expected no error, got: %s", err)
	}
	// Verify that one looks alright
	found := false
	for _, gvk := range gvks {
		if gvk.Kind == "Deployment" {
			found = true
			if gvk.Group != "apps" {
				t.Fatalf("Expected group to be apps, got %s: ", gvk.Group)
			}
			if gvk.Version != "v1" {
				t.Fatalf("Expected version to be v1, got %s: ", gvk.Version)
			}
		}
	}
	if !found {
		t.Fatal("Expected to find Deployment")
	}
}

func TestGetCRDGVKs(t *testing.T) {
	gvks, err := getCRDGKVs()
	if err != nil {
		t.Fatalf("Expected no error, got: %s", err)
	}
	// Verify that one looks alright
	found := false
	for _, gvk := range gvks {
		if gvk.Kind == "ServiceMonitor" {
			found = true
			if gvk.Group != "apps" {
				t.Fatalf("Expected group to be apps, got %s: ", gvk.Group)
			}
			if gvk.Version != "v1" {
				t.Fatalf("Expected version to be v1, got %s: ", gvk.Version)
			}
		}
	}
	if !found {
		t.Fatal("Expected to find Deployment")
	}
}
