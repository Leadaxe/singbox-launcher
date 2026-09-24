package template

import (
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/paths"
)

// TestResolveTemplate — правило выбора шаблона SPEC 135 §3.3 для всех мест
// чтения: свежий поставляемый побеждает скачанный прошлой версией, иначе
// Data → App, иначе путь скачивания в Data.
func TestResolveTemplate(t *testing.T) {
	const version = "v0.9.0"
	cases := []struct {
		name       string
		portable   bool   // App == Data
		noApp      bool   // Layout.App пуст
		app        bool   // App/bin/wizard_template.json
		appMarker  string // App/bin/wizard_template.version; "" = нет файла
		data       bool   // Data/bin/wizard_template.json
		stamp      string // last_template_launcher_version; "" = нет
		wantSource string
		wantInApp  bool // путь ведёт в App (иначе в Data)
		wantCur    bool
	}{
		{name: "1: shipped for this version, nothing downloaded", app: true, appMarker: version,
			wantSource: TemplateSourceApp, wantInApp: true, wantCur: true},
		{name: "1: shipped for this version beats a download of the previous one", app: true, appMarker: version, data: true, stamp: "v0.8.9",
			wantSource: TemplateSourceApp, wantInApp: true, wantCur: true},
		{name: "1: shipped for this version beats a download without a stamp", app: true, appMarker: version, data: true,
			wantSource: TemplateSourceApp, wantInApp: true, wantCur: true},
		{name: "2: download stamped by this version beats the shipped one", app: true, appMarker: version, data: true, stamp: version,
			wantSource: TemplateSourceData, wantCur: true},
		{name: "2: shipped for another version loses to the download", app: true, appMarker: "v0.8.9", data: true, stamp: "v0.8.9",
			wantSource: TemplateSourceData},
		{name: "2: shipped without a marker loses to the download", app: true, data: true,
			wantSource: TemplateSourceData},
		{name: "3: only the shipped one, marker of another version", app: true, appMarker: "v0.8.9",
			wantSource: TemplateSourceApp, wantInApp: true},
		{name: "4: nothing anywhere — download target in Data",
			wantSource: ""},
		{name: "4: marker without a template", appMarker: version,
			wantSource: "", wantCur: true},
		{name: "no App in the layout", noApp: true, data: true,
			wantSource: TemplateSourceData},
		{name: "portable, stamp older — the one file", portable: true, app: true, appMarker: version, stamp: "v0.8.9",
			wantSource: TemplateSourceApp, wantCur: true},
		{name: "portable, stamp current — the one file", portable: true, app: true, appMarker: version, stamp: version,
			wantSource: TemplateSourceData, wantCur: true},
	}
	prev := constants.AppVersion
	constants.AppVersion = version
	t.Cleanup(func() { constants.AppVersion = prev })

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			appRoot := t.TempDir()
			dataRoot := t.TempDir()
			if tc.portable {
				dataRoot = appRoot
			}
			l := paths.Layout{App: paths.AppDir(appRoot), Data: paths.DataDir(dataRoot)}
			if tc.noApp {
				l.App = ""
			}
			write := func(root, name, body string) {
				t.Helper()
				p := filepath.Join(root, constants.BinDirName, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.app {
				write(appRoot, constants.WizardTemplateFileName, `{"from":"app"}`)
			}
			if tc.appMarker != "" {
				write(appRoot, constants.WizardTemplateVersionFileName, tc.appMarker+"\n")
			}
			if tc.data {
				write(dataRoot, constants.WizardTemplateFileName, `{"from":"data"}`)
			}
			if tc.stamp != "" {
				write(dataRoot, "settings.json", `{"lang":"en","last_template_launcher_version":"`+tc.stamp+`"}`)
			}

			got := ResolveTemplate(l)
			if got.Source != tc.wantSource {
				t.Errorf("source = %q, want %q", got.Source, tc.wantSource)
			}
			if got.ShippedCurrent != tc.wantCur {
				t.Errorf("ShippedCurrent = %v, want %v", got.ShippedCurrent, tc.wantCur)
			}
			wantPath := filepath.Join(dataRoot, constants.BinDirName, constants.WizardTemplateFileName)
			if tc.wantInApp {
				wantPath = filepath.Join(appRoot, constants.BinDirName, constants.WizardTemplateFileName)
			}
			if got.Path != wantPath {
				t.Errorf("path = %q, want %q", got.Path, wantPath)
			}
		})
	}
}
