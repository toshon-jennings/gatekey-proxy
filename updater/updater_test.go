package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.2", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.2", "v1.2.3", -1},
	}
	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCheckAndStageVerifiedRelease(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	archive := makeArchive(t, []byte("new gatekey binary"))
	archiveHash := sha256.Sum256(archive)
	archiveHashHex := hex.EncodeToString(archiveHash[:])
	const archiveName = "gatekey-proxy_v1.1.0_darwin_arm64.tar.gz"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			json.NewEncoder(w).Encode(release{
				TagName: "v1.1.0",
				HTMLURL: "https://github.com/toshon-jennings/gatekey-proxy/releases/tag/v1.1.0",
				Assets: []releaseAsset{
					{Name: archiveName, BrowserDownloadURL: server.URL + "/archive", Digest: "sha256:" + archiveHashHex},
					{Name: "checksums.txt", BrowserDownloadURL: server.URL + "/checksums"},
				},
			})
		case "/checksums":
			fmt.Fprintf(w, "%s  %s\n", archiveHashHex, archiveName)
		case "/archive":
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	allowTestServer := func(parsed *url.URL) bool { return strings.HasPrefix(parsed.String(), server.URL) }
	manager := newManager("v1.0.0", "darwin", "arm64", server.URL+"/latest", server.Client(), allowTestServer)
	status, err := manager.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.UpdateAvailable || status.LatestVersion != "v1.1.0" {
		t.Fatalf("Check() status = %#v", status)
	}

	status, err = manager.Stage(context.Background())
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if status.State != "staged" || status.StagedVersion != "v1.1.0" {
		t.Fatalf("Stage() status = %#v", status)
	}
	manifest, err := loadStagedManifest()
	if err != nil {
		t.Fatalf("loadStagedManifest() error = %v", err)
	}
	if manifest.Version != "v1.1.0" {
		t.Fatalf("staged version = %q", manifest.Version)
	}
	dir, _ := updateDir()
	got, err := os.ReadFile(filepath.Join(dir, "gatekey-proxy"))
	if err != nil {
		t.Fatalf("ReadFile(staged binary) error = %v", err)
	}
	if string(got) != "new gatekey binary" {
		t.Fatalf("staged binary = %q", got)
	}
}

func TestStageRejectsChecksumMismatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	archive := makeArchive(t, []byte("new gatekey binary"))
	const archiveName = "gatekey-proxy_v1.1.0_linux_amd64.tar.gz"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			json.NewEncoder(w).Encode(release{
				TagName: "v1.1.0",
				HTMLURL: "https://github.com/toshon-jennings/gatekey-proxy/releases/tag/v1.1.0",
				Assets: []releaseAsset{
					{Name: archiveName, BrowserDownloadURL: server.URL + "/archive"},
					{Name: "checksums.txt", BrowserDownloadURL: server.URL + "/checksums"},
				},
			})
		case "/checksums":
			fmt.Fprintf(w, "%064d  %s\n", 0, archiveName)
		case "/archive":
			w.Write(archive)
		}
	}))
	defer server.Close()

	allowTestServer := func(parsed *url.URL) bool { return strings.HasPrefix(parsed.String(), server.URL) }
	manager := newManager("v1.0.0", "linux", "amd64", server.URL+"/latest", server.Client(), allowTestServer)
	if _, err := manager.Stage(context.Background()); err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("Stage() error = %v, want checksum mismatch", err)
	}
}

func TestApplyStagedAtReplacesExecutable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installDir := t.TempDir()
	executablePath := filepath.Join(installDir, "gatekey-proxy")
	if err := os.WriteFile(executablePath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	dir, err := updateDir()
	if err != nil {
		t.Fatal(err)
	}
	newBinary := []byte("new")
	if err := os.WriteFile(filepath.Join(dir, "gatekey-proxy"), newBinary, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(newBinary)
	if err := saveStagedManifest(stagedManifest{Version: "v1.1.0", BinarySHA256: hex.EncodeToString(hash[:])}); err != nil {
		t.Fatal(err)
	}

	applied, err := applyStagedAt("v1.0.0", executablePath)
	if err != nil {
		t.Fatalf("applyStagedAt() error = %v", err)
	}
	if !applied {
		t.Fatal("applyStagedAt() applied = false")
	}
	got, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("executable = %q, want new", got)
	}
}

func TestGitHubDownloadURLAllowlist(t *testing.T) {
	allowed := []string{
		"https://github.com/owner/repo/releases/download/v1/file",
		"https://release-assets.githubusercontent.com/file",
	}
	blocked := []string{
		"http://github.com/file",
		"https://github.com.evil.example/file",
		"https://example.com/file",
	}
	for _, raw := range allowed {
		parsed, _ := url.Parse(raw)
		if !isGitHubDownloadURL(parsed) {
			t.Fatalf("isGitHubDownloadURL(%q) = false", raw)
		}
	}
	for _, raw := range blocked {
		parsed, _ := url.Parse(raw)
		if isGitHubDownloadURL(parsed) {
			t.Fatalf("isGitHubDownloadURL(%q) = true", raw)
		}
	}
}

func makeArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "gatekey-proxy", Mode: 0755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
