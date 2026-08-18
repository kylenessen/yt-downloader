package toolchain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FindDevelopmentArchive locates the pinned archive created by the repository
// build script. It is called only when Wails reports a development build.
func FindDevelopmentArchive(goos, goarch string) (string, error) {
	archiveName, err := developmentArchiveName(goos, goarch)
	if err != nil {
		return "", err
	}

	starts := make([]string, 0, 2)
	if workingDir, err := os.Getwd(); err == nil {
		starts = append(starts, workingDir)
	}
	if executable, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(executable))
	}

	var projectRoot string
	for _, start := range starts {
		root, archive := findDevelopmentArchiveFrom(start, archiveName)
		if archive != "" {
			return archive, nil
		}
		if projectRoot == "" && root != "" {
			projectRoot = root
		}
	}
	if projectRoot != "" {
		return "", fmt.Errorf("development toolchain is missing; run scripts/build-toolchain.sh %s-%s build/toolchains/%s", goos, goarch, archiveName)
	}
	return "", errors.New("could not locate the YT Downloader project root for the development toolchain")
}

func developmentArchiveName(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "darwin/arm64", "darwin/amd64", "windows/amd64":
		return fmt.Sprintf("toolchain-%s-%s.zip", goos, goarch), nil
	default:
		return "", fmt.Errorf("development toolchain is not available for %s/%s", goos, goarch)
	}
}

func findDevelopmentArchiveFrom(start, archiveName string) (string, string) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", ""
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "wails.json")); err == nil {
			archive := filepath.Join(current, "build", "toolchains", archiveName)
			if info, err := os.Stat(archive); err == nil && !info.IsDir() {
				return current, archive
			}
			return current, ""
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", ""
		}
		current = parent
	}
}
