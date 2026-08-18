package toolchain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindDevelopmentArchiveFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "build", "toolchains", "toolchain-darwin-arm64.zip")
	if err := os.MkdirAll(filepath.Dir(archive), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("archive"), 0644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	foundRoot, foundArchive := findDevelopmentArchiveFrom(nested, filepath.Base(archive))
	if foundRoot != root || foundArchive != archive {
		t.Fatalf("unexpected development archive result root=%q archive=%q", foundRoot, foundArchive)
	}
}

func TestFindDevelopmentArchiveReportsMissingArchive(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	foundRoot, foundArchive := findDevelopmentArchiveFrom(root, "toolchain-darwin-arm64.zip")
	if foundRoot != root || foundArchive != "" {
		t.Fatalf("unexpected missing archive result root=%q archive=%q", foundRoot, foundArchive)
	}
}

func TestDevelopmentArchiveName(t *testing.T) {
	name, err := developmentArchiveName("windows", "amd64")
	if err != nil || name != "toolchain-windows-amd64.zip" {
		t.Fatalf("unexpected Windows archive name=%q error=%v", name, err)
	}
	if _, err := developmentArchiveName("linux", "amd64"); err == nil {
		t.Fatal("unsupported platform was accepted")
	}
}
