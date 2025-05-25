package parser

import "testing"

func TestPathsToPositions(t *testing.T) {
	doc := []byte(`name: arvid
status: chillin'
cat:
  name: strimma
  nice: true`)
	result, err := PathsToPositions(doc)
	if err != nil {
		t.Logf("unexpected error: %v", err)
	}
	for path, position := range result {
		t.Log(path, position)
	}
}
