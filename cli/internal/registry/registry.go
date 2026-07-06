// Package registry reads the remagic app catalog — one JSON file consumed by
// both this CLI and (phase 2) the on-device store app.
package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const DefaultCatalogURL = "https://raw.githubusercontent.com/maximerivest/remagic/main/catalog.json"

type App struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	// URL of a zip whose contents are the app folder AppLoad expects
	// (external.manifest.json at the root). Empty = not yet published.
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
}

type Catalog struct {
	Apps []App `json:"apps"`
}

func Fetch(url string) (*Catalog, error) {
	return FetchWith(&http.Client{Timeout: 20 * time.Second}, url)
}

// FetchWith lets on-device callers supply a client with their own CA roots.
func FetchWith(client *http.Client, url string) (*Catalog, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch catalog: %s from %s", resp.Status, url)
	}
	var c Catalog
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	return &c, nil
}

func (c *Catalog) Find(id string) *App {
	for i := range c.Apps {
		if c.Apps[i].ID == id {
			return &c.Apps[i]
		}
	}
	return nil
}

// Download fetches the app zip to a temp file, verifying the checksum when
// the catalog pins one. Returns the temp path; caller removes it.
func (a *App) Download() (string, error) {
	return a.DownloadWith(&http.Client{Timeout: 5 * time.Minute})
}

func (a *App) DownloadWith(client *http.Client) (string, error) {
	if a.URL == "" {
		return "", fmt.Errorf("%s has no published download yet", a.ID)
	}
	resp, err := client.Get(a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download %s: %s", a.URL, resp.Status)
	}
	f, err := os.CreateTemp("", "remagic-app-*.zip")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	if a.SHA256 != "" && hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		os.Remove(f.Name())
		return "", fmt.Errorf("%s: checksum mismatch (corrupted download or tampered catalog)", a.ID)
	}
	return f.Name(), nil
}
