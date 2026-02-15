package main

import (
	"fmt"
	"strings"
)

type DocumentPosition struct {
	document   string
	start, end int
}

func documentsInFile(file string) []DocumentPosition {
	documents := []DocumentPosition{}
	docBuilder := strings.Builder{}
	currentStart := 0
	lines := strings.FieldsFunc(file, func(r rune) bool { return r == '\n' })
	for i, line := range lines {
		if line == "---" {
			if docBuilder.Len() > 0 {
				documents = append(documents, DocumentPosition{
					document: docBuilder.String(),
					start:    currentStart,
					end:      i,
				})
			}
			docBuilder.Reset()
			currentStart = i + 1
		} else {
			fmt.Fprintf(&docBuilder, "%s\n", line)
		}
	}
	if docBuilder.Len() > 0 {
		documents = append(documents, DocumentPosition{
			document: docBuilder.String(),
			start:    currentStart,
			end:      len(lines),
		})
	}
	return documents
}

func documentAtPosition(file string, line int) (string, int, bool) {
	documents := documentsInFile(file)
	for _, doc := range documents {
		if doc.start <= line && line < doc.end {
			return doc.document, line - doc.start, true
		}
	}
	return "", 0, false
}
