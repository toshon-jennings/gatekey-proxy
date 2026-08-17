package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toshon-jennings/gatekey-proxy/buildinfo"
	"github.com/toshon-jennings/gatekey-proxy/config"
)

const (
	githubAPIURL      = "https://api.github.com/repos/" + buildinfo.Repository + "/releases/latest"
	maxReleaseBody    = 2 << 20
	maxChecksumBody   = 1 << 20
	maxArchiveBody    = 100 << 20
	maxBinarySize     = 100 << 20
	defaultCheckEvery = 24 * time.Hour
)

type Status struct {
	CurrentVersion   string     `json:"currentVersion"`
	LatestVersion    string     `json:"latestVersion,omitempty"`
	UpdateAvailable  bool       `json:"updateAvailable"`
	InstallSupported bool       `json:"installSupported"`
	AutoCheck        bool       `json:"autoCheck"`
	AutoInstall      bool       `json:"autoInstall"`
	State            string     `json:"state"`
	Message          string     `json:"message"`
	CheckedAt        *time.Time `json:"checkedAt,omitempty"`
	ReleaseURL       string     `json:"releaseUrl,omitempty"`
	StagedVersion    string     `json:"stagedVersion,omitempty"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type release struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

type stagedManifest struct {
	Version      string `json:"version"`
	BinarySHA256 string `json:"binarySha256"`
}

type Manager struct {
	mu          sync.RWMutex
	operationMu sync.Mutex
	current     string
	goos        string
	goarch      string
	apiURL      string
	client      *http.Client
	allowURL    func(*url.URL) bool
	preferences config.UpdatePreferences
	status      Status
	release     *release
}

func New(current string) *Manager {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many update download redirects")
			}
			if !isGitHubDownloadURL(req.URL) {
				return fmt.Errorf("refusing update redirect to %s", req.URL.Hostname())
			}
			return nil
		},
	}
	return newManager(current, runtime.GOOS, runtime.GOARCH, githubAPIURL, client, isGitHubDownloadURL)
}

func newManager(current, goos, goarch, apiURL string, client *http.Client, allowURL func(*url.URL) bool) *Manager {
	preferences, err := config.LoadUpdatePreferences()
	if err != nil {
		preferences = config.DefaultUpdatePreferences()
	}
	m := &Manager{
		current:     current,
		goos:        goos,
		goarch:      goarch,
		apiURL:      apiURL,
		client:      client,
		allowURL:    allowURL,
		preferences: preferences,
	}
	m.status = Status{
		CurrentVersion:   current,
		InstallSupported: installSupported(current, goos),
		AutoCheck:        preferences.AutoCheck,
		AutoInstall:      preferences.AutoInstall,
		State:            "idle",
		Message:          "Updates have not been checked yet.",
	}
	manifest, stageErr := loadStagedManifest()
	if stageErr == nil {
		m.status.State = "staged"
		m.status.StagedVersion = manifest.Version
		m.status.Message = fmt.Sprintf("%s is ready and will be applied the next time Gatekey starts.", manifest.Version)
	}
	if stageErr != nil && !os.IsNotExist(stageErr) {
		m.status.State = "error"
		m.status.Message = stageErr.Error()
	}
	return m
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) SetPreferences(preferences config.UpdatePreferences) (Status, error) {
	if !preferences.AutoCheck {
		preferences.AutoInstall = false
	}
	if err := config.SaveUpdatePreferences(preferences); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.preferences = preferences
	m.status.AutoCheck = preferences.AutoCheck
	m.status.AutoInstall = preferences.AutoInstall
	status := m.status
	m.mu.Unlock()

	if preferences.AutoCheck {
		go m.autoCycle(context.Background())
	}
	return status, nil
}

func (m *Manager) Check(ctx context.Context) (Status, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	return m.checkLocked(ctx)
}

func (m *Manager) checkLocked(ctx context.Context) (Status, error) {
	m.setOperationState("checking", "Checking GitHub Releases…")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.apiURL, nil)
	if err != nil {
		return m.fail(err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "gatekey-proxy/"+m.current)

	resp, err := m.client.Do(req)
	if err != nil {
		return m.fail(fmt.Errorf("update check failed: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return m.fail(errors.New("no published Gatekey release is available yet"))
	}
	if resp.StatusCode != http.StatusOK {
		return m.fail(fmt.Errorf("GitHub update check returned %s", resp.Status))
	}

	var latest release
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBody))
	if err := decoder.Decode(&latest); err != nil {
		return m.fail(fmt.Errorf("invalid GitHub release response: %w", err))
	}
	if _, err := parseVersion(latest.TagName); err != nil {
		return m.fail(fmt.Errorf("latest release has an invalid version: %w", err))
	}
	if latest.HTMLURL != "" {
		releaseURL, err := url.Parse(latest.HTMLURL)
		if err != nil || releaseURL.Scheme != "https" || releaseURL.Hostname() != "github.com" {
			return m.fail(errors.New("latest release has an invalid GitHub URL"))
		}
	}

	available := false
	if _, err := parseVersion(m.current); err == nil {
		available = compareVersions(latest.TagName, m.current) > 0
	}

	m.mu.Lock()
	m.release = &latest
	m.status.LatestVersion = latest.TagName
	m.status.ReleaseURL = latest.HTMLURL
	checkedAt := time.Now().UTC()
	m.status.CheckedAt = &checkedAt
	m.status.UpdateAvailable = available
	switch {
	case m.status.StagedVersion == latest.TagName:
		m.status.State = "staged"
		m.status.Message = fmt.Sprintf("%s is ready and will be applied the next time Gatekey starts.", latest.TagName)
	case !m.status.InstallSupported:
		m.status.State = "development"
		m.status.Message = "Development builds can check releases but cannot replace themselves."
	case available:
		m.status.State = "available"
		m.status.Message = fmt.Sprintf("%s is available.", latest.TagName)
	default:
		m.status.State = "current"
		m.status.Message = "Gatekey is up to date."
	}
	status := m.status
	m.mu.Unlock()
	return status, nil
}

func (m *Manager) Stage(ctx context.Context) (Status, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	m.mu.RLock()
	latest := m.release
	supported := m.status.InstallSupported
	m.mu.RUnlock()
	if !supported {
		return m.fail(errors.New("automatic installation is only available in release builds on macOS and Linux"))
	}
	if latest == nil {
		if _, err := m.checkLocked(ctx); err != nil {
			return m.Status(), err
		}
		m.mu.RLock()
		latest = m.release
		m.mu.RUnlock()
	}
	if latest == nil || compareVersions(latest.TagName, m.current) <= 0 {
		return m.Status(), errors.New("no newer release is available")
	}
	m.mu.RLock()
	alreadyStaged := m.status.StagedVersion == latest.TagName
	m.mu.RUnlock()
	if alreadyStaged {
		return m.Status(), nil
	}

	m.setOperationState("downloading", fmt.Sprintf("Downloading %s…", latest.TagName))
	archiveName := fmt.Sprintf("gatekey-proxy_%s_%s_%s.tar.gz", latest.TagName, m.goos, m.goarch)
	archiveAsset, ok := findAsset(latest.Assets, archiveName)
	if !ok {
		return m.fail(fmt.Errorf("release %s has no asset for %s/%s", latest.TagName, m.goos, m.goarch))
	}
	checksumAsset, ok := findAsset(latest.Assets, "checksums.txt")
	if !ok {
		return m.fail(errors.New("release has no checksums.txt asset"))
	}

	checksums, err := m.download(ctx, checksumAsset.BrowserDownloadURL, maxChecksumBody)
	if err != nil {
		return m.fail(fmt.Errorf("failed to download checksums: %w", err))
	}
	wantHash, err := checksumFor(checksums, archiveName)
	if err != nil {
		return m.fail(err)
	}
	archive, err := m.download(ctx, archiveAsset.BrowserDownloadURL, maxArchiveBody)
	if err != nil {
		return m.fail(fmt.Errorf("failed to download update: %w", err))
	}
	gotHash := sha256.Sum256(archive)
	gotHashHex := hex.EncodeToString(gotHash[:])
	if gotHashHex != wantHash {
		return m.fail(errors.New("downloaded update did not match checksums.txt"))
	}
	if archiveAsset.Digest != "" && archiveAsset.Digest != "sha256:"+gotHashHex {
		return m.fail(errors.New("downloaded update did not match the GitHub asset digest"))
	}

	binaryPath, binaryHash, err := extractAndStage(archive)
	if err != nil {
		return m.fail(err)
	}
	if err := saveStagedManifest(stagedManifest{Version: latest.TagName, BinarySHA256: binaryHash}); err != nil {
		os.Remove(binaryPath)
		return m.fail(err)
	}

	m.mu.Lock()
	m.status.State = "staged"
	m.status.StagedVersion = latest.TagName
	m.status.Message = fmt.Sprintf("%s is ready and will be applied the next time Gatekey starts.", latest.TagName)
	status := m.status
	m.mu.Unlock()
	return status, nil
}

func (m *Manager) RunAutomaticChecks(ctx context.Context) {
	m.autoCycle(ctx)
	ticker := time.NewTicker(defaultCheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.autoCycle(ctx)
		}
	}
}

func (m *Manager) autoCycle(ctx context.Context) {
	m.mu.RLock()
	preferences := m.preferences
	m.mu.RUnlock()
	if !preferences.AutoCheck {
		return
	}
	status, err := m.Check(ctx)
	if err != nil || !preferences.AutoInstall || !status.UpdateAvailable || !status.InstallSupported {
		return
	}
	m.Stage(ctx)
}

func (m *Manager) setOperationState(state, message string) {
	m.mu.Lock()
	m.status.State = state
	m.status.Message = message
	m.mu.Unlock()
}

func (m *Manager) fail(err error) (Status, error) {
	m.mu.Lock()
	m.status.State = "error"
	m.status.Message = err.Error()
	status := m.status
	m.mu.Unlock()
	return status, err
}

func (m *Manager) download(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !m.allowURL(parsed) {
		return nil, errors.New("release asset URL is not an allowed GitHub HTTPS URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gatekey-proxy/"+m.current)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("download exceeded the allowed size")
	}
	return data, nil
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func checksumFor(data []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			hash := strings.ToLower(fields[0])
			if len(hash) != sha256.Size*2 {
				break
			}
			if _, err := hex.DecodeString(hash); err != nil {
				break
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no valid checksum for %s", filename)
}

func extractAndStage(archive []byte) (string, string, error) {
	dir, err := updateDir()
	if err != nil {
		return "", "", err
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", "", fmt.Errorf("invalid update archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", fmt.Errorf("invalid update archive: %w", err)
		}
		if header.Name != "gatekey-proxy" || !header.FileInfo().Mode().IsRegular() {
			continue
		}
		if header.Size < 1 || header.Size > maxBinarySize {
			return "", "", errors.New("update binary has an invalid size")
		}

		temp, err := os.CreateTemp(dir, "gatekey-proxy-stage-*")
		if err != nil {
			return "", "", fmt.Errorf("failed to create staged update: %w", err)
		}
		tempPath := temp.Name()
		cleanup := func() {
			temp.Close()
			os.Remove(tempPath)
		}
		hash := sha256.New()
		written, err := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(tarReader, maxBinarySize+1))
		if err != nil || written != header.Size {
			cleanup()
			return "", "", errors.New("failed to extract the update binary")
		}
		if err := temp.Chmod(0700); err != nil {
			cleanup()
			return "", "", fmt.Errorf("failed to secure staged update: %w", err)
		}
		if err := temp.Sync(); err != nil {
			cleanup()
			return "", "", fmt.Errorf("failed to sync staged update: %w", err)
		}
		if err := temp.Close(); err != nil {
			os.Remove(tempPath)
			return "", "", fmt.Errorf("failed to close staged update: %w", err)
		}
		finalPath := filepath.Join(dir, "gatekey-proxy")
		if err := os.Rename(tempPath, finalPath); err != nil {
			os.Remove(tempPath)
			return "", "", fmt.Errorf("failed to stage update: %w", err)
		}
		return finalPath, hex.EncodeToString(hash.Sum(nil)), nil
	}
	return "", "", errors.New("update archive did not contain gatekey-proxy")
}

func isGitHubDownloadURL(parsed *url.URL) bool {
	if parsed == nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

func installSupported(current, goos string) bool {
	if _, err := parseVersion(current); err != nil {
		return false
	}
	return goos == "darwin" || goos == "linux"
}

func parseVersion(value string) ([3]int, error) {
	var version [3]int
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "v")
	if strings.ContainsAny(trimmed, "+-") {
		return version, fmt.Errorf("version %q must be a stable semantic version", value)
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return version, fmt.Errorf("version %q must use major.minor.patch", value)
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version, fmt.Errorf("version %q is invalid", value)
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version, fmt.Errorf("version %q is invalid", value)
		}
		version[i] = n
	}
	return version, nil
}

func compareVersions(a, b string) int {
	left, leftErr := parseVersion(a)
	right, rightErr := parseVersion(b)
	if leftErr != nil || rightErr != nil {
		return 0
	}
	for i := range left {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	return 0
}

func updateDir() (string, error) {
	configDir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(configDir, "update")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create update directory: %w", err)
	}
	return dir, nil
}

func manifestPath() (string, error) {
	dir, err := updateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "staged.json"), nil
}

func loadStagedManifest() (stagedManifest, error) {
	path, err := manifestPath()
	if err != nil {
		return stagedManifest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return stagedManifest{}, err
	}
	var manifest stagedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return stagedManifest{}, fmt.Errorf("failed to parse staged update: %w", err)
	}
	if _, err := parseVersion(manifest.Version); err != nil {
		return stagedManifest{}, fmt.Errorf("staged update has an invalid version: %w", err)
	}
	if len(manifest.BinarySHA256) != sha256.Size*2 {
		return stagedManifest{}, errors.New("staged update has an invalid checksum")
	}
	return manifest, nil
}

func saveStagedManifest(manifest stagedManifest) error {
	path, err := manifestPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "staged-*.json")
	if err != nil {
		return fmt.Errorf("failed to create staged update manifest: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to save staged update manifest: %w", err)
	}
	return nil
}
