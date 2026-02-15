package main

import (
	"slices"
	"testing"
)

func TestDocumentsInFile(t *testing.T) {
	tests := map[string]struct {
		file      string
		documents []DocumentPosition
	}{
		"empty": {
			file:      "",
			documents: []DocumentPosition{},
		},
		"one": {
			file: `hej: du
`,
			documents: []DocumentPosition{
				{
					document: `hej: du
`,
					start: 0,
					end:   1,
				},
			},
		},
		"one-with-separator": {
			file: `hej: du
---`,
			documents: []DocumentPosition{
				{
					document: `hej: du
`,
					start: 0,
					end:   1,
				},
			},
		},
		"two": {
			file: `hej: du
---
hej: hej
du: du
`,
			documents: []DocumentPosition{
				{
					document: `hej: du
`,
					start: 0,
					end:   1,
				},
				{
					document: `hej: hej
du: du
`,
					start: 2,
					end:   4,
				},
			},
		},
		"two-with-separator": {
			file: `hej: du
---
hej: hej
du: du
---
`,
			documents: []DocumentPosition{
				{
					document: `hej: du
`,
					start: 0,
					end:   1,
				},
				{
					document: `hej: hej
du: du
`,
					start: 2,
					end:   4,
				},
			},
		},
		"leading-separator": {
			file: `---
hej: du
`,
			documents: []DocumentPosition{
				{
					document: `hej: du
`,
					start: 1,
					end:   2,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			documents := documentsInFile(test.file)
			if !slices.Equal(documents, test.documents) {
				t.Fatalf("expected `%v`, got `%v`", test.documents, documents)
			}
		})
	}
}

func TestDocumentAtPosition(t *testing.T) {
	tests := map[string]struct {
		file           string
		line           int
		document       string
		lineInDocument int
		found          bool
	}{
		"empty": {
			file:  "",
			found: false,
		},
		"one": {
			file: `kind: Service
spec:
  ports: []
`,
			line: 0,
			document: `kind: Service
spec:
  ports: []
`,
			lineInDocument: 0,
			found:          true,
		},
		"one-at-end": {
			file: `kind: Service
spec:
  ports: []
`,
			line: 2,
			document: `kind: Service
spec:
  ports: []
`,
			lineInDocument: 2,
			found:          true,
		},
		"two-first": {
			file: `kind: Service
spec:
  ports: []
---
kind: Deployment
spec:
  replicas: 2
`,
			line: 1,
			document: `kind: Service
spec:
  ports: []
`,
			lineInDocument: 1,
			found:          true,
		},
		"two-second": {
			file: `kind: Service
spec:
  ports: []
---
kind: Deployment
spec:
  replicas: 2
`,
			line: 5,
			document: `kind: Deployment
spec:
  replicas: 2
`,
			lineInDocument: 1,
			found:          true,
		},
		"between": {
			file: `kind: Service
spec:
  ports: []
---
kind: Deployment
spec:
  replicas: 2
`,
			line:  3,
			found: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			document, lineInDocument, found := documentAtPosition(test.file, test.line)
			if test.found && !found {
				t.Fatalf("expected a document to be found")
			} else if !test.found && found {
				t.Fatalf("did not expect a document to be found")
			}

			if document != test.document {
				t.Fatalf("expected document to be `%v`, got `%v`", test.document, document)
			}
			if lineInDocument != test.lineInDocument {
				t.Fatalf("expected line in document to be `%v`, got `%v`", test.lineInDocument, lineInDocument)
			}
		})
	}
}
