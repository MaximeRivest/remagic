package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/maximerivest/remagic/cli/internal/registry"
	"github.com/maximerivest/remagic/cli/internal/webconfig"
)

// appManifest is external.manifest.json. AppLoad reads name/application/…;
// remagic additionally reads id/version/description for publishing (unknown
// keys are ignored by AppLoad's Qt JSON parser).
type appManifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Application string `json:"application"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// cmdPublish makes releasing an app as easy as `npm publish`: validate the
// folder, zip it, create a GitHub release (through the gh CLI, which carries
// the auth), and emit the catalog entry — written in place when -catalog-dir
// points at a remagic checkout. Merging the catalog change stays a human act:
// these apps run as root on people's tablets.
func cmdPublish(dir, version, catalogDir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "external.manifest.json"))
	if err != nil {
		die("not an app folder: %v", err)
	}
	var m appManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		die("external.manifest.json: %v", err)
	}
	if m.Name == "" || m.Application == "" {
		die(`external.manifest.json needs at least "name" and "application"`)
	}
	if m.ID == "" {
		abs, _ := filepath.Abs(dir)
		m.ID = filepath.Base(abs)
	}
	if version == "" {
		version = m.Version
	}
	if version == "" {
		die(`no version: add "version" to external.manifest.json or pass -version`)
	}

	step("validating %s %s", m.ID, version)
	if !strings.HasPrefix(m.Application, "/") {
		if _, err := os.Stat(filepath.Join(dir, m.Application)); err != nil {
			die("manifest points at %q but it is not in the folder", m.Application)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "icon.png")); err != nil {
		warn("no icon.png — the launcher will show a blank tile")
	}
	// Never ship live settings: the schema's env file and the default
	// settings.env carry API keys.
	exclude := map[string]bool{"settings.env": true}
	if raw, err := os.ReadFile(filepath.Join(dir, "settings.schema.json")); err == nil {
		s, err := webconfig.ParseSchema(m.ID, "", raw)
		if err != nil {
			die("%v", err)
		}
		exclude[s.Env] = true
		ok("settings.schema.json (%d fields) — %s excluded from the zip", len(s.Fields), s.Env)
	}

	zipName := fmt.Sprintf("%s-%s.zip", m.ID, version)
	zipPath := filepath.Join(os.TempDir(), zipName)
	sum, files, err := buildZip(dir, zipPath, exclude)
	if err != nil {
		die("%v", err)
	}
	defer os.Remove(zipPath)
	ok("%s: %d files, sha256 %s…", zipName, files, sum[:12])

	step("creating GitHub release v%s", version)
	repo, err := ghOut(dir, "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		die("gh repo view: %v (publish needs the gh CLI, authenticated, in a repo with a GitHub remote)", err)
	}
	notes := m.Description
	if notes == "" {
		notes = m.Name + " " + version
	}
	if out, err := ghOut(dir, "release", "create", "v"+version, zipPath,
		"--title", m.Name+" "+version, "--notes", notes); err != nil {
		die("gh release create: %v: %s", err, out)
	}
	url := fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", repo, version, zipName)
	ok("released: %s", url)

	entry := registry.App{
		ID: m.ID, Name: m.Name, Description: m.Description,
		Version: version, URL: url, SHA256: sum,
	}
	if catalogDir != "" {
		path := filepath.Join(catalogDir, "catalog.json")
		if err := upsertCatalog(path, entry); err != nil {
			die("%v", err)
		}
		ok("catalog entry written to %s — review, commit, push.", path)
	} else {
		step("catalog entry (add via PR to the remagic repo, or -catalog-dir):")
		j, _ := json.MarshalIndent(entry, "  ", "  ")
		fmt.Printf("  %s\n", j)
	}
}

// buildZip zips dir's contents (paths relative to dir) skipping excluded and
// junk files; returns the archive's sha256 and file count.
func buildZip(dir, dest string, exclude map[string]bool) (string, int, error) {
	f, err := os.Create(dest)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	zw := zip.NewWriter(io.MultiWriter(f, h))
	count := 0
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		base := filepath.Base(rel)
		if exclude[rel] || exclude[base] || strings.HasPrefix(rel, ".git") ||
			strings.HasSuffix(base, ".zip") || base == ".DS_Store" {
			return nil
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		if _, err := io.Copy(w, src); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	if err := zw.Close(); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), count, nil
}

func ghOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// upsertCatalog replaces (by id) or appends the entry in catalog.json.
func upsertCatalog(path string, entry registry.App) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cat registry.Catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	found := false
	for i := range cat.Apps {
		if cat.Apps[i].ID == entry.ID {
			cat.Apps[i] = entry
			found = true
		}
	}
	if !found {
		cat.Apps = append(cat.Apps, entry)
	}
	out, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}
