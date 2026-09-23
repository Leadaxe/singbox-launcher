package main

import (
	"go/token"
	"testing"
)

func scanTest(t *testing.T, relPath, src string) []Finding {
	t.Helper()
	fs, err := ScanSource(token.NewFileSet(), relPath, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestConversionBypass(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "direct field",
			src: `package demo
import "singbox-launcher/internal/paths"
func f(layout paths.Layout) {
	_ = paths.DataDir(layout.App)
}
`,
			want: 1,
		},
		{
			name: "string conversion of field",
			src: `package demo
import "singbox-launcher/internal/paths"
func f(l paths.Layout) {
	_ = paths.DataDir(string(l.App))
}
`,
			want: 1,
		},
		{
			name: "variable named appDir",
			src: `package demo
import "singbox-launcher/internal/paths"
func f(appDir paths.AppDir) {
	_ = paths.LogDir(appDir)
}
`,
			want: 1,
		},
		{
			name: "join with string conversion nested",
			src: `package demo
import (
	"path/filepath"
	"singbox-launcher/internal/paths"
)
func f(layout paths.Layout) {
	_ = paths.DataDir(filepath.Join(string(layout.App), "cache"))
}
`,
			want: 1,
		},
		{
			name: "negative: tempdir",
			src: `package demo
import (
	"singbox-launcher/internal/paths"
	"testing"
)
func f(t *testing.T) {
	_ = paths.DataDir(t.TempDir())
}
`,
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := scanTest(t, "core/demo.go", c.src)
			if len(fs) != c.want {
				t.Errorf("findings = %d, want %d (%+v)", len(fs), c.want, fs)
			}
		})
	}
}

func TestDirectWriteBypass(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "WriteFile to AppDir field",
			src: `package demo
import (
	"os"
	"path/filepath"
	"singbox-launcher/internal/paths"
)
func f(l paths.Layout) {
	_ = os.WriteFile(filepath.Join(string(l.App), "x"), nil, 0o600)
}
`,
			want: 1,
		},
		{
			name: "Rename second arg AppDir",
			src: `package demo
import (
	"os"
	"path/filepath"
	"singbox-launcher/internal/paths"
)
func f(layout paths.Layout, src string) {
	_ = os.Rename(src, filepath.Join(string(layout.App), "y"))
}
`,
			want: 1,
		},
		{
			name: "MkdirAll from appDir var",
			src: `package demo
import "os"
func f(appDir string) {
	_ = os.MkdirAll(appDir, 0o755)
}
`,
			want: 1,
		},
		{
			name: "negative: writes under Data.Bin()",
			src: `package demo
import (
	"os"
	"path/filepath"
	"singbox-launcher/internal/paths"
)
func f(layout paths.Layout) {
	_ = os.WriteFile(filepath.Join(layout.Data.Bin(), "x"), nil, 0o600)
}
`,
			want: 0,
		},
		{
			name: "negative: application word is not App token",
			src: `package demo
import "os"
func f(application string) {
	_ = os.WriteFile(application, nil, 0o600)
}
`,
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := scanTest(t, "core/demo.go", c.src)
			if len(fs) != c.want {
				t.Errorf("findings = %d, want %d (%+v)", len(fs), c.want, fs)
			}
		})
	}
}

func TestAllowlist(t *testing.T) {
	src := `package platform
import (
	"os"
	"singbox-launcher/internal/paths"
)
func f(appDir paths.AppDir) {
	_ = os.WriteFile(string(appDir), nil, 0o600)
}
`
	if fs := scanTest(t, "internal/platform/glstate.go", src); len(fs) != 0 {
		t.Errorf("allowlisted file must be skipped, got %+v", fs)
	}
	if fs := scanTest(t, "internal/platform/other.go", src); len(fs) == 0 {
		t.Errorf("non-allowlisted file in same package must still be scanned")
	}
	if fs := scanTest(t, "core/demo_test.go", src); len(fs) != 0 {
		t.Errorf("_test.go files must be skipped, got %+v", fs)
	}
	if fs := scanTest(t, "tools/other/demo.go", src); len(fs) != 0 {
		t.Errorf("tools/ directory must be skipped, got %+v", fs)
	}
}

func TestContainsAppToken(t *testing.T) {
	positive := []string{"layout.App", "string(layout.App)", "appDir", "AppDir(x)", "l.App"}
	for _, s := range positive {
		if !containsAppToken(s) {
			t.Errorf("containsAppToken(%q) = false, want true", s)
		}
	}
	negative := []string{"application", "t.TempDir()", "layout.Data.Bin()", "happy"}
	for _, s := range negative {
		if containsAppToken(s) {
			t.Errorf("containsAppToken(%q) = true, want false", s)
		}
	}
}
