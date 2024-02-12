package cachedhttp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

type CachedHttpClient struct {
	cacheDir      string
	inMemoryCache map[string][]byte
}

func NewCachedHttpClient(cacheDir string) (CachedHttpClient, error) {
	cache := map[string][]byte{}
	cachedFiles, err := os.ReadDir(cacheDir)
	if err != nil {
		return CachedHttpClient{}, fmt.Errorf("Failed to read files in cache dir %s: %s", cacheDir, err)
	}
	for _, f := range cachedFiles {
		response, err := os.ReadFile(path.Join(cacheDir, f.Name()))
		if err != nil {
			return CachedHttpClient{}, fmt.Errorf("Failed to read file %s: %s", f.Name(), err)
		}
		url := filenameToUrl(f.Name())
		cache[url] = response
	}
	return CachedHttpClient{
		cacheDir:      cacheDir,
		inMemoryCache: cache,
	}, nil
}

func (c *CachedHttpClient) GetBody(url string) ([]byte, error) {
	cachedResponse, found := c.inMemoryCache[url]
	if found {
		return cachedResponse, nil
	}
	resp, err := http.Get(url)
	if err != nil {
		return []byte{}, fmt.Errorf("Failed to call the internet: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return []byte{}, fmt.Errorf("Got non-200 status code: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("Failed to read body: %s", err)
	}
	c.inMemoryCache[url] = body
	filename := urlToFilename(url)
	if err := os.WriteFile(path.Join(c.cacheDir, filename), body, 0644); err != nil {
		return []byte{}, fmt.Errorf("Failed to cache response to filesystem: %s", err)
	}
	return body, nil
}

func urlToFilename(url string) string {
	return strings.ReplaceAll(url, "/", "!")
}

func filenameToUrl(filename string) string {
	return strings.ReplaceAll(filename, "!", "/")
}
