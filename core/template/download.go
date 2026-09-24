// Файл download.go содержит единственный механизм установки шаблона
// конфигурации (bin/wizard_template.json) на диск.
//
// До появления этого файла скачивание жило только в UI вкладки Local
// (ui/core_dashboard_tab.go, кнопка Download template). Мастер, открытый
// другим входом — Remote → Configure у машины — при отсутствии файла сразу
// показывал «Download failed. See the log», хотя ни одной попытки скачивания
// не предпринималось. Механизм вынесен сюда, чтобы у обоих входов он был
// один и тот же, а не два расходящихся копипаста.
//
// Пакет намеренно не зависит от core: core сам импортирует core/template,
// поэтому HTTP-геттер (core.GetURLBytes) ВНЕДРЯЕТСЯ вызывающим — это же
// делает функцию тестируемой без сети.
package template

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// DownloadTimeout — таймаут одной попытки скачивания шаблона.
// Тот же, что исторически стоял на кнопке вкладки Local.
const DownloadTimeout = 30 * time.Second

// URLFetcher получает тело по URL. Сигнатура повторяет core.GetURLBytes
// (тело, HTTP-статус, ошибка отправки/чтения), чтобы вкладка Local могла
// передать метод контроллера как есть.
type URLFetcher func(ctx context.Context, url string, timeout time.Duration) ([]byte, int, error)

// DownloadTemplate скачивает шаблон и кладёт его в <DataDir>/bin.
//
// Возвращает путь установленного файла. Ошибка пригодна для показа
// пользователю: она всегда содержит конкретную причину, а не «см. лог» —
// именно этого не хватало диалогу в Мастере.
//
// Установленный шаблон не трогается, пока новый не скачан целиком и не
// разобран: тело проверяется ParseTemplateData, а на место старого файла
// встаёт переименованием временного. Любой провал — сеть, HTTP-статус,
// заглушка вместо JSON, запись — оставляет прежний файл как был.
//
// СЕТЕВАЯ функция: вызывать только вне UI-потока, мутации виджетов после —
// через fyne.Do.
func DownloadTemplate(ctx context.Context, d paths.DataDir, fetch URLFetcher) (string, error) {
	if fetch == nil {
		return "", fmt.Errorf("template download: no URL fetcher provided")
	}
	url := GetTemplateURL()
	binDir := d.Bin()
	target := platform.GetWizardTemplatePath(d)

	debuglog.InfoLog("template: downloading %s → %s", url, target)

	data, status, err := fetch(ctx, url, DownloadTimeout)
	if err != nil {
		debuglog.ErrorLog("template: download request failed: %v", err)
		return "", fmt.Errorf("%s: %w", locale.T("Config template download failed"), err)
	}
	if status != http.StatusOK {
		debuglog.ErrorLog("template: download returned HTTP %d", status)
		return "", fmt.Errorf("%s: HTTP %d", locale.T("Config template download failed"), status)
	}
	if len(data) == 0 {
		debuglog.ErrorLog("template: download returned an empty body")
		return "", fmt.Errorf("%s: %s", locale.T("Config template download failed"), locale.T("server returned an empty file"))
	}
	if _, err := ParseTemplateData(data); err != nil {
		debuglog.ErrorLog("template: downloaded body is not a usable template: %v", err)
		return "", fmt.Errorf("%s: %w", locale.T("Config template download failed"), err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		debuglog.ErrorLog("template: mkdir %s failed: %v", binDir, err)
		return "", fmt.Errorf("%s: %w", locale.T("Config template download failed"), err)
	}
	if err := replaceFileAtomically(target, data); err != nil {
		debuglog.ErrorLog("template: write %s failed: %v", target, err)
		return "", fmt.Errorf("%s: %w", locale.T("Config template download failed"), err)
	}
	// Pin install: помечаем, какая версия лаунчера поставила шаблон, чтобы
	// следующий апгрейд знал, что его надо обновить (SPEC 046).
	// Best effort — провал отметки не отменяет уже записанный файл.
	if err := locale.MarkTemplateInstalled(binDir, constants.AppVersion); err != nil {
		debuglog.WarnLog("template: failed to record install version: %v", err)
	}
	debuglog.InfoLog("template: installed %s (%d bytes)", target, len(data))
	return target, nil
}

// replaceFileAtomically пишет data во временный файл рядом с target и
// переименовывает его поверх: на месте target всегда лежит либо прежний файл
// целиком, либо новый целиком — обрыв записи не оставляет обрезка.
//
// Имя временного файла уникально: фоновое обновление после апгрейда и кнопка
// Download (или Мастер) могут качать одновременно, и общий временный файл
// один писатель переименовал бы из-под другого.
func replaceFileAtomically(target string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".*.download")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		// CreateTemp создаёт 0600; шаблон всегда лежал с 0644.
		writeErr = os.Chmod(tmp, 0o644)
	}
	if writeErr == nil {
		writeErr = os.Rename(tmp, target)
	}
	if writeErr != nil {
		_ = os.Remove(tmp) // best-effort: прежний target остался нетронутым
		return writeErr
	}
	return nil
}

// EnsureTemplate загружает шаблон, а если файла нет или он нечитаем —
// СНАЧАЛА пытается его скачать и только потом сдаётся.
//
// Второй результат — true, если скачивание действительно понадобилось: у
// вкладки Local по нему обновляется статус-строка.
//
// СЕТЕВАЯ функция: вызывать только вне UI-потока.
func EnsureTemplate(ctx context.Context, l paths.Layout, fetch URLFetcher) (*TemplateData, bool, error) {
	data, err := LoadTemplateData(l)
	if err == nil {
		return data, false, nil
	}
	debuglog.InfoLog("template: load failed (%v) — attempting download before giving up", err)

	if _, dlErr := DownloadTemplate(ctx, l.Data, fetch); dlErr != nil {
		return nil, true, dlErr
	}
	data, err = LoadTemplateData(l)
	if err != nil {
		// Скачали, но прочитать всё равно не смогли — причина именно в
		// содержимом, и в диалог должна уехать она, а не «см. лог».
		debuglog.ErrorLog("template: still unreadable after a successful download: %v", err)
		return nil, true, fmt.Errorf("%s: %w", locale.T("Config template failed to load"), err)
	}
	return data, true, nil
}
