package kustomization

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

type kustomization struct {
	Resources []string `yaml:"resources"`
}

func getResources(text string) ([]string, error) {
	var kustomization kustomization
	if err := yaml.Unmarshal([]byte(text), &kustomization); err != nil {
		return []string{}, fmt.Errorf("Could not parse resources in kustomization file: %s", err)
	}
	return kustomization.Resources, nil
}

func filterDirEntries(files []fs.DirEntry) []string {
	var filteredFiles []string
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !slices.Contains([]string{".yaml", ".yml"}, path.Ext(f.Name())) {
			continue
		}
		if f.Name() == "kustomization.yaml" {
			continue
		}
		filteredFiles = append(filteredFiles, f.Name())
	}
	return filteredFiles
}

func FilesNotIncluded(dir string, text string) ([]string, error) {
	resources, err := getResources(text)
	if err != nil {
		return []string{}, err
	}
	var trimmedResources []string
	for _, r := range resources {
		trimmedResources = append(trimmedResources, path.Base(r))
	}
	resources = trimmedResources
	filesInCurrentDir, err := os.ReadDir(dir)
	if err != nil {
		return []string{}, fmt.Errorf("Could not read files in current kustomization dir: %s", err)
	}
	candidates := filterDirEntries(filesInCurrentDir)
	filesNotIncluded := []string{}
	for _, c := range candidates {
		if !slices.Contains(resources, c) {
			filesNotIncluded = append(filesNotIncluded, c)
		}
	}
	return filesNotIncluded, nil
}

func GetResourcesLine(text string) int {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "resources") {
			return i
		}
	}
	return -1
}
