package cachedhttp

import "testing"

func TestUrlToFilename(t *testing.T) {
	url := "https://github.com/user/repo/file.json"
	expected := "https:!!github.com!user!repo!file.json"
	actual := urlToFilename(url)
	if actual != expected {
		t.Fatalf("Expected %s, got %s", expected, actual)
	}
}

func TestFilenameToUrl(t *testing.T) {
	url := "https:!!github.com!user!repo!file.json"
	expected := "https://github.com/user/repo/file.json"
	actual := filenameToUrl(url)
	if actual != expected {
		t.Fatalf("Expected %s, got %s", expected, actual)
	}
}
