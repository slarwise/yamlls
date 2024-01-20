package parser

import (
	"regexp"
	"strings"
)

var kindPattern = regexp.MustCompile(`^kind:\s+(.+)$`)
var apiVersionPattern = regexp.MustCompile(`^apiVersion:\s+(.+)$`)

func GetKindApiVersion(text string) (string, string) {
	lines := strings.Split(text, "\n")
	kind := ""
	apiVersion := ""
	for _, l := range lines {
		kindMatch := kindPattern.FindStringSubmatch(l)
		if len(kindMatch) == 2 {
			kind = kindMatch[1]
		}
		apiVersionMatch := apiVersionPattern.FindStringSubmatch(l)
		if len(apiVersionMatch) == 2 {
			apiVersion = apiVersionMatch[1]
		}
	}
	return kind, apiVersion
}
