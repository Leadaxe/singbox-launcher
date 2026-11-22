package dialogs

// rule_cond_refs.go — поле наборов (вид `refs` таблицы условий) в окне
// правила: ссылки URL или файл по строке (SPEC 187), «Add file…», подсказка
// SPEC 188 §6 (набор может держать порты и процессы; «или» с другими
// условиями — группа правил).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/srstag"
)

const (
	ruleRefsPlaceholderText = "URLs or files (one per line), .srs or .json\ne.g. https://raw.githubusercontent.com/.../file.srs"
	ruleRefsHintText        = "A rule set may also contain ports and processes. For “or” with other conditions, use a Rule group."
	ruleRefsProjectURL      = "https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box"
)

// refsField — поле ссылок на наборы.
type refsField struct {
	entry *widget.Entry
	box   *fyne.Container
	add   *widget.Button
	help  *widget.Button
}

func newRefsField(ctx *ruleFormCtx, label string, initial []string, onChange func()) *refsField {
	f := &refsField{}
	f.entry = widget.NewMultiLineEntry()
	f.entry.SetPlaceHolder(locale.T(ruleRefsPlaceholderText))
	f.entry.Wrapping = fyne.TextWrapWord
	f.entry.SetMinRowsVisible(3)
	f.entry.SetText(strings.Join(initial, "\n"))
	f.entry.OnChanged = func(string) {
		if onChange != nil {
			onChange()
		}
	}
	// SPEC 187: набор с диска — строка file://… в том же поле.
	f.add = widget.NewButton(locale.T("Add file…"), func() { f.pickFile(ctx) })
	f.help = widget.NewButton("?", func() {
		msg := widget.NewLabel(locale.T("We recommend looking for suitable rule-set files in the project:"))
		msg.Wrapping = fyne.TextWrapWord
		open := widget.NewButton(locale.T("Open"), func() { _ = platform.OpenURL(ruleRefsProjectURL) })
		dialog.ShowCustom(locale.T("SRS rule-sets"), locale.T("Close"), container.NewVBox(msg, open), ctx.window())
	})
	title := widget.NewLabel(label)
	title.Truncation = fyne.TextTruncateEllipsis
	hint := widget.NewLabel(locale.T(ruleRefsHintText))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance
	f.box = container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(layout.NewSpacer(), f.add, f.help), title),
		f.entry,
		hint,
	)
	return f
}

func (f *refsField) object() fyne.CanvasObject { return f.box }

// values — строки поля; strict — каждая приведена к ссылке и проверена
// (файл существует и это набор), иначе как набраны.
func (f *refsField) values(strict bool) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, line := range ParseLines(f.entry.Text, false) {
		ref := line
		if strict {
			r, err := normalizeRuleSetRef(line)
			if err != nil {
				return nil, err
			}
			ref = r
		}
		if seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out, nil
}

func (f *refsField) setEnabled(on bool) {
	if on {
		f.entry.Enable()
		f.add.Enable()
	} else {
		f.entry.Disable()
		f.add.Disable()
	}
}

func (f *refsField) pickFile(ctx *ruleFormCtx) {
	win := ctx.window()
	appendFile := func(path string) {
		ref, err := srstag.FileRefFromPath(path)
		if err != nil {
			dialog.ShowError(ruleSetFileError(path, err), win)
			return
		}
		text := strings.TrimRight(f.entry.Text, "\n")
		if text != "" {
			text += "\n"
		}
		f.entry.SetText(text + ref)
	}
	path, ok, err := platform.PickOpenFile(locale.T("Choose a rule-set file"), []string{"srs", "json"})
	if err == platform.ErrNativeDialogUnavailable {
		// Linux без zenity/kdialog — встроенный диалог Fyne.
		fd := dialog.NewFileOpen(func(rc fyne.URIReadCloser, ferr error) {
			if ferr != nil || rc == nil {
				return
			}
			picked := rc.URI().Path()
			_ = rc.Close()
			appendFile(picked)
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".srs", ".json"}))
		fd.Show()
		return
	}
	if err != nil || !ok {
		return
	}
	appendFile(path)
}

// ruleRefsHaveFile — среди ссылок есть набор-файл (SPEC 187: режим всегда
// local, выбор режима заперт).
func ruleRefsHaveFile(refs []string) bool {
	for _, r := range refs {
		r = strings.TrimSpace(r)
		if srstag.IsFileRef(r) || srstag.IsAbsPath(r) {
			return true
		}
	}
	return false
}

// normalizeRuleSetRef приводит строку поля наборов к ссылке (SPEC 187):
// абсолютный путь становится file://-ссылкой, а файл по file://-ссылке
// проверяется — он должен существовать и быть набором своего формата. URL
// возвращается как есть.
func normalizeRuleSetRef(line string) (string, error) {
	ref := line
	if srstag.IsAbsPath(line) {
		r, err := srstag.FileRefFromPath(line)
		if err != nil {
			return "", ruleSetFileError(line, err)
		}
		ref = r
	}
	if !srstag.IsFileRef(ref) {
		return ref, nil
	}
	p, err := srstag.PathFromFileRef(ref)
	if err != nil {
		return "", ruleSetFileError(line, err)
	}
	path := filepath.FromSlash(p)
	if info, statErr := os.Stat(path); statErr != nil || info.IsDir() {
		return "", fmt.Errorf("%s: %s", locale.T("Rule-set file not found"), path)
	}
	if err := services.ValidateRuleSetFile(path, srstag.FormatFromRef(ref)); err != nil {
		return "", fmt.Errorf("%s: %s", locale.T("Not a rule-set file (.srs or .json expected)"), path)
	}
	return ref, nil
}

// ruleSetFileError — понятная ошибка пути набора.
func ruleSetFileError(path string, err error) error {
	if errors.Is(err, srstag.ErrNetworkPath) {
		return fmt.Errorf("%s: %s", locale.T("Network paths are not supported for rule sets"), path)
	}
	return fmt.Errorf("%s: %s", locale.T("Rule-set file path is not valid"), path)
}
