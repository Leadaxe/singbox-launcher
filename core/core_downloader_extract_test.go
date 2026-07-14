package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/internal/platform"
)

// Naive outbound in purego core builds needs libcronet.* next to the sing-box
// binary (SPEC 044). The extractors must pull the companion library out of the
// release archive alongside the binary — historically they extracted only the
// binary and silently dropped everything else.

func writeTestZip(t *testing.T, dir string, entries map[string]string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "core.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return archivePath
}

func writeTestTarGz(t *testing.T, dir string, entries map[string]string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "core.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}
	defer f.Close()
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)
	for name, content := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return archivePath
}

func TestExtractZip_BinaryAndCronetLib(t *testing.T) {
	dir := t.TempDir()
	singboxName := platform.GetExecutableNames()
	archivePath := writeTestZip(t, dir, map[string]string{
		"sing-box-1.14.0-lx.4-windows-amd64/" + singboxName: "binary-bytes",
		"sing-box-1.14.0-lx.4-windows-amd64/libcronet.dll":  "cronet-bytes",
		"sing-box-1.14.0-lx.4-windows-amd64/LICENSE":        "license",
		"sing-box-1.14.0-lx.4-windows-amd64/README.md":      "readme",
	})

	destDir := t.TempDir()
	ac := &AppController{}
	binaryPath, companions, err := ac.extractZip(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if filepath.Base(binaryPath) != singboxName {
		t.Errorf("binaryPath = %q, want base %q", binaryPath, singboxName)
	}
	if len(companions) != 1 || filepath.Base(companions[0]) != "libcronet.dll" {
		t.Fatalf("companions = %v, want exactly [libcronet.dll]", companions)
	}
	data, err := os.ReadFile(companions[0])
	if err != nil || string(data) != "cronet-bytes" {
		t.Errorf("companion content = %q, err = %v, want %q", data, err, "cronet-bytes")
	}
	// LICENSE/README must NOT be extracted.
	if _, err := os.Stat(filepath.Join(destDir, "LICENSE")); !os.IsNotExist(err) {
		t.Errorf("LICENSE was extracted, want skipped")
	}
}

func TestExtractZip_NoCompanion(t *testing.T) {
	dir := t.TempDir()
	singboxName := platform.GetExecutableNames()
	archivePath := writeTestZip(t, dir, map[string]string{
		"sing-box-1.14.0-lx.3-windows-amd64/" + singboxName: "binary-bytes",
	})

	ac := &AppController{}
	binaryPath, companions, err := ac.extractZip(archivePath, t.TempDir())
	if err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if binaryPath == "" {
		t.Error("binaryPath is empty")
	}
	if len(companions) != 0 {
		t.Errorf("companions = %v, want none", companions)
	}
}

func TestExtractZip_NoBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := writeTestZip(t, dir, map[string]string{
		"sing-box-1.14.0-lx.4-windows-amd64/libcronet.dll": "cronet-bytes",
	})

	ac := &AppController{}
	if _, _, err := ac.extractZip(archivePath, t.TempDir()); err == nil {
		t.Fatal("extractZip succeeded on an archive without the binary, want error")
	}
}

func TestExtractTarGz_BinaryAndCronetLib(t *testing.T) {
	dir := t.TempDir()
	singboxName := platform.GetExecutableNames()
	archivePath := writeTestTarGz(t, dir, map[string]string{
		"sing-box-1.14.0-lx.4-linux-amd64/" + singboxName: "binary-bytes",
		"sing-box-1.14.0-lx.4-linux-amd64/libcronet.so":   "cronet-bytes",
		"sing-box-1.14.0-lx.4-linux-amd64/LICENSE":        "license",
	})

	destDir := t.TempDir()
	ac := &AppController{}
	binaryPath, companions, err := ac.extractTarGz(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	if filepath.Base(binaryPath) != singboxName {
		t.Errorf("binaryPath = %q, want base %q", binaryPath, singboxName)
	}
	if len(companions) != 1 || filepath.Base(companions[0]) != "libcronet.so" {
		t.Fatalf("companions = %v, want exactly [libcronet.so]", companions)
	}
	if _, err := os.Stat(filepath.Join(destDir, "LICENSE")); !os.IsNotExist(err) {
		t.Errorf("LICENSE was extracted, want skipped")
	}
}

func TestIsCompanionLib(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"sing-box-1.14.0-lx.4-windows-amd64/libcronet.dll", true},
		{"sing-box-1.14.0-lx.4-darwin-arm64/libcronet.dylib", true},
		{"sing-box-1.14.0-lx.4-linux-amd64/libcronet.so", true},
		{"libcronet.dll", true},
		{"sing-box-1.14.0-lx.4-windows-amd64/sing-box.exe", false},
		{"sing-box-1.14.0-lx.4-windows-amd64/LICENSE", false},
		{"some/dir/libcronet", false},
	}
	for _, c := range cases {
		if got := isCompanionLib(c.name); got != c.want {
			t.Errorf("isCompanionLib(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
