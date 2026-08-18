package toolchain

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	archiveSchemaVersion  = 1
	defaultUpdateInterval = 24 * time.Hour
	defaultNightlyAPIURL  = "https://api.github.com/repos/yt-dlp/yt-dlp-nightly-builds/releases/latest"
)

// Paths contains the app-owned executables used by the download pipeline.
type Paths struct {
	YTDLP   string
	FFmpeg  string
	FFprobe string
	Deno    string
}

// Versions identifies the active app-owned toolchain.
type Versions struct {
	Baseline string `json:"baseline"`
	YTDLP    string `json:"ytDlp"`
}

// Config makes Manager testable without changing production behavior.
type Config struct {
	RootDir          string
	BootstrapArchive string
	GOOS             string
	GOARCH           string
	HTTPClient       *http.Client
	Now              func() time.Time
	UpdateInterval   time.Duration
	NightlyAPIURL    string
	Logf             func(format string, args ...any)
}

type archiveFile struct {
	SHA256 string `json:"sha256"`
}

type archiveManifest struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Version       string                 `json:"version"`
	GOOS          string                 `json:"goos"`
	GOARCH        string                 `json:"goarch"`
	Tools         map[string]string      `json:"tools"`
	Files         map[string]archiveFile `json:"files"`
}

type managerState struct {
	ActiveYTDLP          string    `json:"activeYtDlp,omitempty"`
	ActiveYTDLPVersion   string    `json:"activeYtDlpVersion,omitempty"`
	PreviousYTDLP        string    `json:"previousYtDlp,omitempty"`
	PreviousYTDLPVersion string    `json:"previousYtDlpVersion,omitempty"`
	LastUpdateCheck      time.Time `json:"lastUpdateCheck,omitempty"`
}

// Manager installs, verifies, updates, and rolls back the app-owned tools.
type Manager struct {
	mu               sync.Mutex
	rootDir          string
	bootstrapArchive string
	goos             string
	goarch           string
	client           *http.Client
	now              func() time.Time
	updateInterval   time.Duration
	nightlyAPIURL    string
	logf             func(format string, args ...any)
	directPaths      *Paths
	cachedPaths      Paths
	cachedVersions   Versions
	cacheValid       bool
}

// NewManager creates a toolchain manager. Empty configuration fields receive
// production defaults. Production never discovers tools through PATH.
func NewManager(cfg Config) (*Manager, error) {
	goos := cfg.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := cfg.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	rootDir := cfg.RootDir
	if rootDir == "" {
		rootDir = os.Getenv("YT_DOWNLOADER_TOOLCHAIN_ROOT")
		if rootDir == "" {
			configDir, err := os.UserConfigDir()
			if err != nil {
				return nil, fmt.Errorf("locate application support directory: %w", err)
			}
			rootDir = filepath.Join(configDir, "YT Downloader", "tools")
		}
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	interval := cfg.UpdateInterval
	if interval <= 0 {
		interval = defaultUpdateInterval
	}
	apiURL := cfg.NightlyAPIURL
	if apiURL == "" {
		apiURL = defaultNightlyAPIURL
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	archivePath := cfg.BootstrapArchive
	if archivePath == "" {
		archivePath = discoverBootstrapArchive(goos)
	}

	m := &Manager{
		rootDir:          rootDir,
		bootstrapArchive: archivePath,
		goos:             goos,
		goarch:           goarch,
		client:           client,
		now:              now,
		updateInterval:   interval,
		nightlyAPIURL:    apiURL,
		logf:             logf,
	}

	if direct, ok := developmentPathsFromEnvironment(); ok {
		m.directPaths = &direct
	}
	return m, nil
}

// RootDir returns the application-owned tool directory.
func (m *Manager) RootDir() string {
	return m.rootDir
}

// Ensure installs and verifies the baseline toolchain.
func (m *Manager) Ensure(ctx context.Context) (Paths, Versions, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cacheValid {
		return m.cachedPaths, m.cachedVersions, nil
	}

	if m.directPaths != nil {
		if err := verifyExecutables(ctx, *m.directPaths); err != nil {
			return Paths{}, Versions{}, fmt.Errorf("verify development tool paths: %w", err)
		}
		version, err := executableVersion(ctx, m.directPaths.YTDLP)
		if err != nil {
			return Paths{}, Versions{}, err
		}
		m.cachedPaths = *m.directPaths
		m.cachedVersions = Versions{Baseline: "development", YTDLP: version}
		m.cacheValid = true
		return m.cachedPaths, m.cachedVersions, nil
	}

	if err := os.MkdirAll(m.rootDir, 0755); err != nil {
		return Paths{}, Versions{}, fmt.Errorf("create tool directory: %w", err)
	}

	manifest, err := m.loadInstalledManifest()
	if err != nil || m.verifyInstalledFiles(manifest) != nil {
		if m.bootstrapArchive == "" {
			return Paths{}, Versions{}, errors.New("self-contained toolchain is missing; reinstall the application")
		}
		m.logf("Installing baseline toolchain from %s", m.bootstrapArchive)
		if err := m.installBootstrap(); err != nil {
			return Paths{}, Versions{}, fmt.Errorf("install baseline toolchain: %w", err)
		}
		manifest, err = m.loadInstalledManifest()
		if err != nil {
			return Paths{}, Versions{}, err
		}
	}

	paths, versions, err := m.pathsLocked(manifest)
	if err != nil {
		return Paths{}, Versions{}, err
	}
	if err := verifyExecutables(ctx, paths); err != nil {
		return Paths{}, Versions{}, fmt.Errorf("verify installed toolchain: %w", err)
	}
	if versions.YTDLP == "" {
		versions.YTDLP, err = executableVersion(ctx, paths.YTDLP)
		if err != nil {
			return Paths{}, Versions{}, err
		}
	}
	m.cachedPaths = paths
	m.cachedVersions = versions
	m.cacheValid = true
	return paths, versions, nil
}

// CheckForYTDLPUpdate updates yt-dlp from the official nightly channel.
// Updates are checksum verified and activated only after execution succeeds.
func (m *Manager) CheckForYTDLPUpdate(ctx context.Context, force bool) (bool, Versions, error) {
	if m.directPaths != nil {
		paths, versions, err := m.Ensure(ctx)
		_ = paths
		return false, versions, err
	}
	if _, _, err := m.Ensure(ctx); err != nil {
		return false, Versions{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.loadState()
	if err != nil {
		return false, Versions{}, err
	}
	if !force && !state.LastUpdateCheck.IsZero() && m.now().Sub(state.LastUpdateCheck) < m.updateInterval {
		return false, m.cachedVersions, nil
	}

	release, err := m.fetchNightlyRelease(ctx)
	state.LastUpdateCheck = m.now().UTC()
	if err != nil {
		_ = m.saveState(state)
		return false, Versions{}, err
	}
	if state.ActiveYTDLPVersion == release.TagName || m.cachedVersions.YTDLP == release.TagName {
		if err := m.saveState(state); err != nil {
			return false, Versions{}, err
		}
		return false, m.cachedVersions, nil
	}

	assetName := nightlyAssetName(m.goos, m.goarch)
	assetURL, sumsURL, err := releaseURLs(release, assetName)
	if err != nil {
		return false, Versions{}, err
	}
	expected, err := m.fetchExpectedChecksum(ctx, sumsURL, assetName)
	if err != nil {
		return false, Versions{}, err
	}

	updatesDir := filepath.Join(m.rootDir, "updates")
	if err := os.MkdirAll(updatesDir, 0755); err != nil {
		return false, Versions{}, fmt.Errorf("create update directory: %w", err)
	}
	tmp, err := os.CreateTemp(updatesDir, ".yt-dlp-update-*")
	if err != nil {
		return false, Versions{}, fmt.Errorf("create update file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := m.downloadToFile(ctx, assetURL, tmp, expected); err != nil {
		_ = tmp.Close()
		return false, Versions{}, err
	}
	if err := tmp.Close(); err != nil {
		return false, Versions{}, fmt.Errorf("close update file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return false, Versions{}, fmt.Errorf("make update executable: %w", err)
	}
	version, err := executableVersion(ctx, tmpPath)
	if err != nil {
		return false, Versions{}, fmt.Errorf("verify yt-dlp update: %w", err)
	}

	ext := filepath.Ext(assetName)
	finalName := "yt-dlp-" + safeVersion(release.TagName) + ext
	finalPath := filepath.Join(updatesDir, finalName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return false, Versions{}, fmt.Errorf("activate yt-dlp update: %w", err)
	}

	manifest, err := m.loadInstalledManifest()
	if err != nil {
		return false, Versions{}, err
	}
	currentPaths, currentVersions, err := m.pathsLocked(manifest)
	if err != nil {
		return false, Versions{}, err
	}
	state.PreviousYTDLP = relativeToRoot(m.rootDir, currentPaths.YTDLP)
	state.PreviousYTDLPVersion = currentVersions.YTDLP
	state.ActiveYTDLP = relativeToRoot(m.rootDir, finalPath)
	state.ActiveYTDLPVersion = version
	if err := m.saveState(state); err != nil {
		return false, Versions{}, err
	}
	m.logf("Activated yt-dlp nightly %s", version)
	m.cachedPaths = Paths{YTDLP: finalPath, FFmpeg: currentPaths.FFmpeg, FFprobe: currentPaths.FFprobe, Deno: currentPaths.Deno}
	m.cachedVersions = Versions{Baseline: manifest.Version, YTDLP: version}
	m.cacheValid = true
	return true, m.cachedVersions, nil
}

// RollbackYTDLP activates the previous verified yt-dlp version.
func (m *Manager) RollbackYTDLP(ctx context.Context) (bool, error) {
	if m.directPaths != nil {
		return false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.loadState()
	if err != nil {
		return false, err
	}
	if state.PreviousYTDLP == "" {
		return false, nil
	}
	previousPath := filepath.Join(m.rootDir, filepath.FromSlash(state.PreviousYTDLP))
	if _, err := executableVersion(ctx, previousPath); err != nil {
		return false, fmt.Errorf("verify previous yt-dlp: %w", err)
	}
	state.ActiveYTDLP, state.PreviousYTDLP = state.PreviousYTDLP, state.ActiveYTDLP
	state.ActiveYTDLPVersion, state.PreviousYTDLPVersion = state.PreviousYTDLPVersion, state.ActiveYTDLPVersion
	if err := m.saveState(state); err != nil {
		return false, err
	}
	m.cacheValid = false
	m.logf("Rolled yt-dlp back to %s", state.ActiveYTDLPVersion)
	return true, nil
}

func (m *Manager) installBootstrap() error {
	zr, err := zip.OpenReader(m.bootstrapArchive)
	if err != nil {
		return fmt.Errorf("open toolchain archive: %w", err)
	}
	defer zr.Close()

	manifest, err := readArchiveManifest(&zr.Reader)
	if err != nil {
		return err
	}
	if manifest.SchemaVersion != archiveSchemaVersion {
		return fmt.Errorf("unsupported toolchain schema %d", manifest.SchemaVersion)
	}
	if manifest.GOOS != m.goos || manifest.GOARCH != m.goarch {
		return fmt.Errorf("toolchain is for %s/%s, need %s/%s", manifest.GOOS, manifest.GOARCH, m.goos, m.goarch)
	}
	if err := validateManifest(manifest); err != nil {
		return err
	}

	staging, err := os.MkdirTemp(m.rootDir, ".install-*")
	if err != nil {
		return fmt.Errorf("create installation staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	for name, spec := range manifest.Files {
		entry := findZipEntry(&zr.Reader, name)
		if entry == nil {
			return fmt.Errorf("toolchain archive is missing %s", name)
		}
		if err := extractVerifiedEntry(entry, filepath.Join(staging, name), spec.SHA256); err != nil {
			return err
		}
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), append(manifestBytes, '\n'), 0644); err != nil {
		return fmt.Errorf("write installed manifest: %w", err)
	}

	current := filepath.Join(m.rootDir, "current")
	backup := filepath.Join(m.rootDir, ".previous-bootstrap")
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(current); err == nil {
		if err := os.Rename(current, backup); err != nil {
			return fmt.Errorf("stage previous toolchain: %w", err)
		}
	}
	if err := os.Rename(staging, current); err != nil {
		_ = os.Rename(backup, current)
		return fmt.Errorf("activate baseline toolchain: %w", err)
	}
	_ = os.RemoveAll(backup)
	return nil
}

func (m *Manager) pathsLocked(manifest archiveManifest) (Paths, Versions, error) {
	paths := Paths{
		YTDLP:   filepath.Join(m.rootDir, "current", filepath.FromSlash(manifest.Tools["yt-dlp"])),
		FFmpeg:  filepath.Join(m.rootDir, "current", filepath.FromSlash(manifest.Tools["ffmpeg"])),
		FFprobe: filepath.Join(m.rootDir, "current", filepath.FromSlash(manifest.Tools["ffprobe"])),
		Deno:    filepath.Join(m.rootDir, "current", filepath.FromSlash(manifest.Tools["deno"])),
	}
	versions := Versions{Baseline: manifest.Version}
	state, err := m.loadState()
	if err != nil {
		return Paths{}, Versions{}, err
	}
	if state.ActiveYTDLP != "" {
		candidate := filepath.Join(m.rootDir, filepath.FromSlash(state.ActiveYTDLP))
		if pathInside(m.rootDir, candidate) {
			if _, err := os.Stat(candidate); err == nil {
				paths.YTDLP = candidate
				versions.YTDLP = state.ActiveYTDLPVersion
			}
		}
	}
	return paths, versions, nil
}

func (m *Manager) loadInstalledManifest() (archiveManifest, error) {
	data, err := os.ReadFile(filepath.Join(m.rootDir, "current", "manifest.json"))
	if err != nil {
		return archiveManifest{}, err
	}
	var manifest archiveManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return archiveManifest{}, fmt.Errorf("parse installed toolchain manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return archiveManifest{}, err
	}
	return manifest, nil
}

func (m *Manager) verifyInstalledFiles(manifest archiveManifest) error {
	if manifest.GOOS != m.goos || manifest.GOARCH != m.goarch {
		return errors.New("installed toolchain platform does not match")
	}
	for name, spec := range manifest.Files {
		path := filepath.Join(m.rootDir, "current", filepath.FromSlash(name))
		if !pathInside(filepath.Join(m.rootDir, "current"), path) {
			return fmt.Errorf("invalid installed tool path %q", name)
		}
		actual, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if !strings.EqualFold(actual, spec.SHA256) {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	return nil
}

func (m *Manager) loadState() (managerState, error) {
	data, err := os.ReadFile(filepath.Join(m.rootDir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return managerState{}, nil
	}
	if err != nil {
		return managerState{}, fmt.Errorf("read toolchain state: %w", err)
	}
	var state managerState
	if err := json.Unmarshal(data, &state); err != nil {
		return managerState{}, fmt.Errorf("parse toolchain state: %w", err)
	}
	return state, nil
}

func (m *Manager) saveState(state managerState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(m.rootDir, ".state-*")
	if err != nil {
		return fmt.Errorf("create toolchain state: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filepath.Join(m.rootDir, "state.json")); err != nil {
		return fmt.Errorf("activate toolchain state: %w", err)
	}
	return nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (m *Manager) fetchNightlyRelease(ctx context.Context) (githubRelease, error) {
	var release githubRelease
	if err := m.getJSON(ctx, m.nightlyAPIURL, &release); err != nil {
		return release, fmt.Errorf("check yt-dlp nightly release: %w", err)
	}
	if release.TagName == "" {
		return release, errors.New("yt-dlp nightly release has no tag")
	}
	return release, nil
}

func (m *Manager) fetchExpectedChecksum(ctx context.Context, sumsURL, assetName string) (string, error) {
	req, err := newRequest(ctx, sumsURL)
	if err != nil {
		return "", err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download yt-dlp checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download yt-dlp checksums: %s", resp.Status)
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 2<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == assetName {
			if len(fields[0]) != 64 {
				return "", errors.New("invalid yt-dlp checksum")
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksum for %s not found", assetName)
}

func (m *Manager) downloadToFile(ctx context.Context, url string, dst *os.File, expected string) error {
	req, err := newRequest(ctx, url)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("download yt-dlp update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download yt-dlp update: %s", resp.Status)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, h), resp.Body); err != nil {
		return fmt.Errorf("download yt-dlp update: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return errors.New("yt-dlp update checksum mismatch")
	}
	return nil
}

func (m *Manager) getJSON(ctx context.Context, url string, dst any) error {
	req, err := newRequest(ctx, url)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(dst)
}

func discoverBootstrapArchive(goos string) string {
	if path := os.Getenv("YT_DOWNLOADER_TOOLCHAIN_ARCHIVE"); path != "" {
		return path
	}
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	if goos == "darwin" {
		contents := filepath.Dir(filepath.Dir(executable))
		candidate := filepath.Join(contents, "Resources", "toolchain.zip")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		return ""
	}
	candidate := filepath.Join(filepath.Dir(executable), "toolchain.zip")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func developmentPathsFromEnvironment() (Paths, bool) {
	paths := Paths{
		YTDLP:   os.Getenv("YT_DOWNLOADER_YTDLP"),
		FFmpeg:  os.Getenv("YT_DOWNLOADER_FFMPEG"),
		FFprobe: os.Getenv("YT_DOWNLOADER_FFPROBE"),
		Deno:    os.Getenv("YT_DOWNLOADER_DENO"),
	}
	return paths, paths.YTDLP != "" && paths.FFmpeg != "" && paths.FFprobe != "" && paths.Deno != ""
}

func verifyExecutables(ctx context.Context, paths Paths) error {
	tools := []struct {
		name string
		path string
		arg  string
	}{
		{"yt-dlp", paths.YTDLP, "--version"},
		{"ffmpeg", paths.FFmpeg, "-version"},
		{"ffprobe", paths.FFprobe, "-version"},
		{"deno", paths.Deno, "--version"},
	}
	for _, tool := range tools {
		if tool.path == "" || !filepath.IsAbs(tool.path) {
			return fmt.Errorf("%s path is not absolute", tool.name)
		}
		if _, err := executableVersion(ctx, tool.path, tool.arg); err != nil {
			return fmt.Errorf("%s: %w", tool.name, err)
		}
	}
	return nil
}

func executableVersion(parent context.Context, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if len(args) == 0 {
		args = []string{"--version"}
	}
	output, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %s %s: %w: %s", filepath.Base(path), strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	line := strings.TrimSpace(string(output))
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	if line == "" {
		return "unknown", nil
	}
	return line, nil
}

func readArchiveManifest(zr *zip.Reader) (archiveManifest, error) {
	entry := findZipEntry(zr, "manifest.json")
	if entry == nil {
		return archiveManifest{}, errors.New("toolchain archive has no manifest")
	}
	r, err := entry.Open()
	if err != nil {
		return archiveManifest{}, err
	}
	defer r.Close()
	var manifest archiveManifest
	if err := json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&manifest); err != nil {
		return archiveManifest{}, fmt.Errorf("parse toolchain archive manifest: %w", err)
	}
	return manifest, nil
}

func validateManifest(manifest archiveManifest) error {
	if manifest.Version == "" || manifest.GOOS == "" || manifest.GOARCH == "" {
		return errors.New("toolchain manifest is incomplete")
	}
	for _, logical := range []string{"yt-dlp", "ffmpeg", "ffprobe", "deno"} {
		name := manifest.Tools[logical]
		if name == "" || filepath.Base(name) != name {
			return fmt.Errorf("toolchain manifest has invalid %s path", logical)
		}
		spec, ok := manifest.Files[name]
		if !ok || len(spec.SHA256) != 64 {
			return fmt.Errorf("toolchain manifest has invalid %s checksum", logical)
		}
	}
	for name, spec := range manifest.Files {
		if filepath.Base(name) != name || len(spec.SHA256) != 64 {
			return fmt.Errorf("invalid toolchain file %q", name)
		}
	}
	return nil
}

func findZipEntry(zr *zip.Reader, name string) *zip.File {
	for _, entry := range zr.File {
		if entry.Name == name {
			return entry
		}
	}
	return nil
}

func extractVerifiedEntry(entry *zip.File, destination, expected string) error {
	if entry.UncompressedSize64 > 1<<30 {
		return fmt.Errorf("toolchain file %s is too large", entry.Name)
	}
	r, err := entry.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(r, 1<<30))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s", entry.Name)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func nightlyAssetName(goos, goarch string) string {
	switch goos {
	case "darwin":
		return "yt-dlp_macos"
	case "windows":
		if goarch == "arm64" {
			return "yt-dlp_arm64.exe"
		}
		return "yt-dlp.exe"
	case "linux":
		if goarch == "arm64" {
			return "yt-dlp_linux_aarch64"
		}
		return "yt-dlp_linux"
	default:
		return "yt-dlp"
	}
}

func releaseURLs(release githubRelease, assetName string) (string, string, error) {
	var assetURL, sumsURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			assetURL = asset.BrowserDownloadURL
		case "SHA2-256SUMS":
			sumsURL = asset.BrowserDownloadURL
		}
	}
	if assetURL == "" || sumsURL == "" {
		return "", "", fmt.Errorf("yt-dlp nightly release is missing %s or checksums", assetName)
	}
	return assetURL, sumsURL, nil
}

func newRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "YT-Downloader/1")
	req.Header.Set("Accept", "application/vnd.github+json")
	return req, nil
}

func pathInside(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func relativeToRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

func safeVersion(version string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, version)
}
