// Package applocal installs catalog apps when the code is running ON the
// tablet itself (the store app): download, checksum, unzip to a staging dir
// on the same filesystem, rename entries over. Renames replace directory
// entries without touching running inodes, and files not in the bundle
// (settings env) survive.
package applocal

import (
	"archive/zip"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maximerivest/remagic/cli/internal/registry"
)

const (
	ApploadDir = "/home/root/xovi/exthome/appload"
	stagingDir = "/home/root/.remagic-stage"
)

// The tablet's BusyBox has no CA bundle to speak of; ship one so catalog and
// release downloads are actually verified. Refreshed whenever the store is
// rebuilt (it's the build machine's bundle).
//
//go:embed cacert.pem
var caBundle []byte

// HTTPClient verifies TLS against the system pool when present, falling back
// to the embedded bundle.
func HTTPClient() *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(caBundle)
	return &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
			Proxy:           http.ProxyFromEnvironment,
		},
	}
}

// Installed lists app folders (those carrying an external.manifest.json).
func Installed() map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(ApploadDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(ApploadDir, e.Name(), "external.manifest.json")); err == nil {
			out[e.Name()] = true
		}
	}
	return out
}

// Install downloads and installs one catalog app, reporting progress lines.
func Install(app *registry.App, progress func(string)) error {
	if app.URL == "" {
		return fmt.Errorf("%s has no published download yet", app.ID)
	}
	progress("downloading " + app.Version + "…")
	zipPath, err := app.DownloadWith(HTTPClient())
	if err != nil {
		return err
	}
	defer os.Remove(zipPath)

	progress("unpacking…")
	if err := os.RemoveAll(stagingDir); err != nil {
		return err
	}
	if err := unzipTo(zipPath, stagingDir); err != nil {
		return err
	}

	progress("installing…")
	target := filepath.Join(ApploadDir, app.ID)
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		dst := filepath.Join(target, e.Name())
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(stagingDir, e.Name()), dst); err != nil {
			return err
		}
	}
	os.RemoveAll(stagingDir)
	return nil
}

func unzipTo(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("zip escapes the app folder: %s", f.Name)
		}
		path := filepath.Join(dest, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return err
		}
		src.Close()
		dst.Close()
	}
	return nil
}
