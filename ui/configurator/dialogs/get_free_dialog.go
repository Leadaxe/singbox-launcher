// Package dialogs — диалоги визарда конфигурации.
//
// Файл get_free_dialog.go: «Бесплатные серверы сообщества». LxBox-стиль —
// клик по серверу подставляет URL в поле ввода SourceURLEntry, ничего не
// сохраняет в state.json и не мутирует модель. Дальше пользователь сам
// нажимает «Add» как при обычной ручной вставке URL.
//
// Это сознательное упрощение vs предыдущей реализации, которая через
// presenter.LoadState заменяла модель целиком — был баг утечки записи
// state.json в обход Save (Get Free → Cancel визарда → подписки потеряны).
//
// Источник списка: bin/get_free.json (community-managed, лежит в репе);
// при наличии сети fetch'им свежую копию с pinned-ref'а текущей сборки.
// Если нет ни кэша, ни сети — диалог покажет ошибку без эффектов.
package dialogs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"image/color"

	"singbox-launcher/internal/constants"
	internaldialogs "singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// scrollbarGutterWidth — right padding under scrollable content так чтобы
// нативная полоска прокрутки не наезжала на кнопки/текст. Должно совпадать
// со значением в `ui/configurator/tabs/scroll_gutter.go` (тот же визуальный
// инвариант). Дублируем потому что это другой пакет; константа маленькая
// и выделять отдельный shared-utility ради 10px пока не оправдано.
const scrollbarGutterWidth = 10

// getFreeData — плоская LxBox-style схема: текст благодарности + ссылка
// + список URL'ов подписок. Никакого parser_config / outbounds / правил —
// клик по URL подставляет его в поле, дальнейшую обработку (парс подписки,
// генерация outbounds) делает обычный pipeline через Add → Update.
type getFreeData struct {
	Credit struct {
		Text string `json:"text"`
		Link string `json:"link"`
	} `json:"credit"`
	Sources []string `json:"sources"`
}

const (
	getFreeFileName    = "get_free.json"
	getFreeDownloadTTL = 24 * time.Hour
)

// fetchOrLoadGetFree пытается обновить bin/get_free.json с pinned-ref'а
// сборки (best-effort; ошибки не фатальны), затем читает локальный файл.
// Если ни кэша, ни свежескачанной копии нет — возвращает ошибку.
func fetchOrLoadGetFree(execDir string) (*getFreeData, error) {
	binDir := filepath.Join(execDir, "bin")
	target := filepath.Join(binDir, getFreeFileName)

	stale := true
	if st, err := os.Stat(target); err == nil {
		stale = time.Since(st.ModTime()) > getFreeDownloadTTL
	}
	if stale {
		_ = downloadGetFree(target)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", target, err)
	}
	var d getFreeData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("parse %s: %w", target, err)
	}
	return &d, nil
}

// downloadGetFree — best-effort загрузка свежей копии с GitHub.
// Не возвращаем ошибку если сеть недоступна: locally-cached копии хватит.
func downloadGetFree(target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	branch := constants.GetMyBranch()
	src := fmt.Sprintf("https://raw.githubusercontent.com/Leadaxe/singbox-launcher/%s/bin/%s", branch, getFreeFileName)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(src)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	tmp := target + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.ReadFrom(resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp) // best-effort cleanup of partial write
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// ShowGetFreeVPNDialog — открыть picker community-источников.
//
// Семантика (LxBox-style): клик «Insert» подставляет URL в
// `guiState.SourceURLEntry` и закрывает диалог. Пользователь сам нажимает
// «Add», чтобы зарегистрировать URL как источник в `parser_config.proxies`.
//
// **Не** мутирует WizardModel и **не** пишет state.json.
func ShowGetFreeVPNDialog(presenter *wizardpresentation.WizardPresenter) {
	guiState := presenter.GUIState()
	if guiState == nil || guiState.Window == nil {
		return
	}

	loading := dialog.NewInformation(
		locale.T("wizard.get_free.loading_title"),
		locale.T("wizard.get_free.loading_msg"),
		guiState.Window,
	)
	loading.Show()

	go func() {
		ac := presenter.Controller()
		if ac == nil || ac.FileService == nil {
			fyne.Do(func() {
				loading.Hide()
				dialog.ShowError(fmt.Errorf("file service unavailable"), guiState.Window)
			})
			return
		}
		data, err := fetchOrLoadGetFree(ac.FileService.ExecDir)
		fyne.Do(func() {
			loading.Hide()
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.get_free.error_load"), err), guiState.Window)
				return
			}
			renderGetFreeDialog(presenter, guiState, data)
		})
	}()
}

// renderGetFreeDialog строит UI: thank-you text + список URL'ов с
// per-row кнопкой Insert. Должен вызываться из main thread.
func renderGetFreeDialog(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	data *getFreeData,
) {
	_ = presenter // зарезервирован под будущий audit-лог; mутаций модели здесь нет

	intro := widget.NewLabel(locale.T("wizard.get_free.thanks_label"))
	intro.Wrapping = fyne.TextWrapWord

	// Текст благодарности + кликабельная ссылка на одной горизонтальной
	// линии. Используем Border (left=label, center=hyperlink), чтобы
	// hyperlink занимал всю оставшуюся ширину и при нехватке усекался
	// многоточием — иначе HBox без layout-constraint'а рос бы за край
	// диалога. Truncation включена через `Truncation = TruncateEllipsis`.
	var credit fyne.CanvasObject = widget.NewLabel(data.Credit.Text)
	if data.Credit.Link != "" {
		if u, err := url.Parse(data.Credit.Link); err == nil {
			thanksLabel := widget.NewLabel(data.Credit.Text)
			linkHyperlink := widget.NewHyperlink(data.Credit.Link, u)
			linkHyperlink.Truncation = fyne.TextTruncateEllipsis
			credit = container.NewBorder(nil, nil, thanksLabel, nil, linkHyperlink)
		}
	}

	separator := canvas.NewRectangle(color.RGBA{R: 80, G: 80, B: 80, A: 255})
	separator.SetMinSize(fyne.NewSize(0, 1))

	var d dialog.Dialog
	rows := make([]fyne.CanvasObject, 0, len(data.Sources)+1)

	if len(data.Sources) == 0 {
		empty := widget.NewLabel(locale.T("wizard.get_free.no_sources"))
		empty.Wrapping = fyne.TextWrapWord
		rows = append(rows, empty)
	} else {
		for i, src := range data.Sources {
			if src == "" {
				continue
			}
			rows = append(rows, buildSourceRow(i+1, src, guiState, &d))
		}
	}

	listVBox := container.NewVBox(rows...)

	// Right scroll gutter — иначе native scrollbar наезжает на кнопки
	// Insert (тот же приём, что в tabs/source_tab.go::urlURIGutter).
	gutter := canvas.NewRectangle(color.Transparent)
	gutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	listWithGutter := container.NewBorder(nil, nil, nil, gutter, listVBox)

	header := container.NewVBox(intro, credit, separator)
	content := container.NewBorder(header, nil, nil, nil, container.NewVScroll(listWithGutter))

	d = internaldialogs.NewCustom(
		locale.T("wizard.get_free.dialog_title"),
		content,
		nil,
		locale.T("wizard.get_free.button_close"),
		guiState.Window,
	)
	d.Resize(fyne.NewSize(680, 480))
	d.Show()
}

// buildSourceRow — одна линия: [номер] [URL ........] [Insert].
// URL не кликабельный (просто label с ellipsis-усечением); единственный
// тап-target — кнопка Insert. Это упрощает scan'абельность: глаз идёт по
// номерам слева, action-кнопкам справа.
func buildSourceRow(idx int, source string, guiState *wizardpresentation.GUIState, dlg *dialog.Dialog) fyne.CanvasObject {
	insert := widget.NewButton(locale.T("wizard.get_free.button_insert"), func() {
		if guiState.SourceURLEntry == nil {
			return
		}
		guiState.SourceURLsProgrammatic = true
		guiState.SourceURLEntry.SetText(source)
		guiState.SourceURLsProgrammatic = false
		if dlg != nil && *dlg != nil {
			(*dlg).Hide()
		}
	})
	insert.Importance = widget.HighImportance

	header := widget.NewLabelWithStyle(
		fmt.Sprintf("%d.", idx),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	urlLabel := widget.NewLabel(source)
	urlLabel.Truncation = fyne.TextTruncateEllipsis

	// Border: header слева, кнопка справа, URL заполняет середину
	// (truncation сработает при недостатке ширины — без переноса строки).
	return container.NewBorder(nil, nil, header, insert, urlLabel)
}
