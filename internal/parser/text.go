package parser

import (
	"regexp"
	"strings"
)

var groupAndVersionPattern = regexp.MustCompile(`^apiVersion:\s+([^/]*/){0,1}(.+)$`)
var kindPattern = regexp.MustCompile(`^kind:\s+(.+)$`)

func GetGroupVersionKind(text string) (string, string, string) {
	lines := strings.Split(text, "\n")
	group := ""
	version := ""
	kind := ""
	for _, l := range lines {
		groupAndVersionMatch := groupAndVersionPattern.FindStringSubmatch(l)
		if len(groupAndVersionMatch) == 3 {
			group = groupAndVersionMatch[1]
			group = strings.TrimSuffix(group, "/")
			version = groupAndVersionMatch[2]
		}
		kindMatch := kindPattern.FindStringSubmatch(l)
		if len(kindMatch) == 2 {
			kind = kindMatch[1]
		}
	}
	group = strings.Trim(group, `"`)
	version = strings.Trim(version, `"`)
	kind = strings.Trim(kind, `"`)
	return group, version, kind
}

func SplitIntoYamlDocuments(text string) []string {
	text = strings.TrimPrefix(text, "---\n")
	text = strings.TrimSuffix(text, "---\n")
	text = strings.TrimSuffix(text, "---")
	return strings.Split(text, "---\n")
}
