package template

import (
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// Источник выбранного шаблона (TemplateResolution.Source).
const (
	TemplateSourceApp  = "app"  // <App>/bin/wizard_template.json — поставляемый
	TemplateSourceData = "data" // <Data>/bin/wizard_template.json — скачанный
)

// TemplateResolution — какой файл шаблона читать.
type TemplateResolution struct {
	// Path — файл для чтения. Нет ни одного → путь в Data (куда скачается),
	// Source пуст.
	Path string
	// Source — TemplateSourceApp / TemplateSourceData; "" — шаблона нет.
	Source string
	// ShippedCurrent — маркер App/bin/wizard_template.version равен AppVersion:
	// поставляемый шаблон положен установщиком под эту версию лаунчера.
	ShippedCurrent bool
}

// ResolveTemplate — единственное правило выбора шаблона для всех мест чтения
// (SPEC 135 §3.3). A — App/bin/wizard_template.json, D — Data/bin/…:
//
//  1. A есть, его маркер == AppVersion и (D нет, или штамп
//     LastTemplateLauncherVersion пуст, или штамп < AppVersion) → A:
//     после обновления лаунчера свежий поставляемый шаблон побеждает
//     скачанный прошлой версией;
//  2. иначе D есть → D;
//  3. иначе A есть → A;
//  4. иначе → путь D (цель скачивания), Source "".
//
// В portable/legacy App == Data, и правило вырождается в прежнее «читать
// bin/wizard_template.json». Запись (скачивание) — только в Data.
func ResolveTemplate(l paths.Layout) TemplateResolution {
	dataPath := platform.GetWizardTemplatePath(l.Data)
	appPath := ""
	if l.App != "" {
		appPath = platform.GetShippedTemplatePath(l.App)
	}
	appOK := appPath != "" && templateFileExists(appPath)
	dataOK := templateFileExists(dataPath)

	shippedCurrent := false
	if l.App != "" {
		if marker, ok := ReadTemplateMarker(l.App.Bin()); ok && marker == constants.AppVersion {
			shippedCurrent = true
		}
	}

	if appOK && shippedCurrent {
		stampOld := !dataOK
		if !stampOld {
			stamp := strings.TrimSpace(locale.LoadSettings(l.Data.Bin()).LastTemplateLauncherVersion)
			stampOld = stamp == "" || constants.CompareVersions(stamp, constants.AppVersion) < 0
		}
		if stampOld {
			return TemplateResolution{Path: appPath, Source: TemplateSourceApp, ShippedCurrent: true}
		}
	}
	if dataOK {
		return TemplateResolution{Path: dataPath, Source: TemplateSourceData, ShippedCurrent: shippedCurrent}
	}
	if appOK {
		return TemplateResolution{Path: appPath, Source: TemplateSourceApp, ShippedCurrent: shippedCurrent}
	}
	return TemplateResolution{Path: dataPath, ShippedCurrent: shippedCurrent}
}

// ReadTemplateMarker читает <binDir>/wizard_template.version (версия
// лаунчера, под которую установщик положил шаблон). ok=false — файла нет или
// он не читается.
func ReadTemplateMarker(binDir string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(binDir, constants.WizardTemplateVersionFileName))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(raw)), true
}

// templateFileExists — файл есть и это не каталог.
func templateFileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
