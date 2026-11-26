// File chain_banner.go — баннер «цепочка не собрана» на вкладке ядра (SPEC 189).
//
// Решение владельца 26.11.2026 (как в LxBox): цепочка, которую сборка не
// выпустила в config.json (кольцо, конфликт, локальный порт, позиция в
// никуда…), заметна не только строкой отчёта сборки, а подсказкой в главном
// окне: заголовок, причина из реестра, кнопка перехода в редактор этой
// цепочки. Баннер держится, пока цепочка не исправлена или не удалена;
// «Скрыть» убирает его до следующего изменения конфига.
//
// Источник правды — свой state.json (включённые цепочки) и собранный
// config.json (есть ли там outbound цепочки): так баннер верен и после
// перезапуска, когда отчёта сборки ещё нет. Причина — из отчёта сборки, если
// он есть.
package ui

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/configurator"
)

// chainBannerMax — сколько цепочек показывается отдельными строками; остальные
// — одной строкой «и ещё N».
const chainBannerMax = 3

// chainBanner — контейнер баннера и подпись скрытого конфига.
type chainBanner struct {
	ac  *core.AppController
	box *fyne.Container
	// dismissed — подпись config.json (время изменения и размер), при которой
	// пользователь нажал «Скрыть»; другой конфиг — баннер снова виден.
	dismissed string
}

func newChainBanner(ac *core.AppController) *chainBanner {
	b := &chainBanner{ac: ac, box: container.NewVBox()}
	b.box.Hide()
	return b
}

// brokenChain — включённая цепочка, которой нет в собранном конфиге.
type brokenChain struct {
	Tag    string
	Reason string
}

// configSignature — подпись config.json; пусто — файла нет.
func configSignature(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return st.ModTime().UTC().Format("20060102T150405.000000000") + "/" + strconv.FormatInt(st.Size(), 10)
}

// brokenChains — включённые цепочки своего состояния, которых нет среди
// outbounds собранного config.json, с причиной из отчёта сборки.
func brokenChains(ac *core.AppController) []brokenChain {
	if ac == nil || ac.FileService == nil {
		return nil
	}
	st, err := state.Load(platform.GetWizardStatePath(ac.FileService.Layout.Data))
	if err != nil || st == nil {
		return nil
	}
	var chains []string
	for _, src := range st.Sources {
		if src.Kind == state.SourceKindChain && src.Enabled {
			if tag := strings.TrimSpace(src.NodeTagOrLabel()); tag != "" {
				chains = append(chains, tag)
			}
		}
	}
	if len(chains) == 0 {
		return nil
	}
	raw, err := os.ReadFile(ac.FileService.ConfigPath)
	if err != nil {
		return nil // конфига ещё нет — судить не о чем
	}
	var cfg struct {
		Outbounds []struct {
			Tag string `json:"tag"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(jsonc.ToJSON(raw), &cfg) != nil {
		return nil
	}
	built := make(map[string]bool, len(cfg.Outbounds))
	for _, ob := range cfg.Outbounds {
		built[ob.Tag] = true
	}
	var out []brokenChain
	for _, tag := range chains {
		if !built[tag] {
			out = append(out, brokenChain{Tag: tag, Reason: chainBuildReason(tag)})
		}
	}
	return out
}

// chainBuildReason — текст причины из отчёта последней сборки: запись правила 7
// (Params["chain"]) или цепочки, не ставшей узлом (Subject); пусто — отчёта нет.
func chainBuildReason(tag string) string {
	entries, _, _ := config.BuildReport()
	for _, e := range entries {
		match := e.Params["chain"] == tag
		if e.Kind == config.BuildReportChainFailed && e.Subject == tag {
			match = true
		}
		if !match {
			continue
		}
		if e.Code != "" {
			if reg, err := registry.Get(); err == nil {
				if _, text, ok := reg.WarningText(e.Code, locale.GetLang(), e.Params); ok && strings.TrimSpace(text) != "" {
					return text
				}
			}
		}
		return e.Reason
	}
	return ""
}

// refresh перечитывает состояние и конфиг и перерисовывает баннер. Только
// UI-поток.
func (b *chainBanner) refresh() {
	if b == nil || b.ac == nil || b.ac.FileService == nil {
		return
	}
	sig := configSignature(b.ac.FileService.ConfigPath)
	broken := brokenChains(b.ac)
	b.box.Objects = nil
	if len(broken) == 0 || (b.dismissed != "" && b.dismissed == sig) {
		b.box.Hide()
		b.box.Refresh()
		return
	}
	for i, c := range broken {
		if i == chainBannerMax {
			more := widget.NewLabel(locale.Tf("…and %d more hop chains are not built", len(broken)-chainBannerMax))
			more.Importance = widget.WarningImportance
			b.box.Add(more)
			break
		}
		title := widget.NewLabel("⚠ " + locale.Tf("Hop chain %q is not built", c.Tag))
		title.TextStyle.Bold = true
		title.Importance = widget.WarningImportance
		title.Wrapping = fyne.TextWrapWord
		b.box.Add(title)
		reason := c.Reason
		if reason == "" {
			reason = locale.T("The chain is not in the config: open it to see what to fix.")
		}
		hint := widget.NewLabel(reason)
		hint.Wrapping = fyne.TextWrapWord
		b.box.Add(hint)
		tag := c.Tag
		open := widget.NewButtonWithIcon(locale.T("Open chain"), theme.DocumentCreateIcon(), func() {
			var parent fyne.Window
			if b.ac.UIService != nil {
				parent = b.ac.UIService.MainWindow
			}
			configurator.ShowConfigWizardOpenSource(parent, tag)
		})
		open.Importance = widget.HighImportance
		b.box.Add(container.NewHBox(open))
	}
	hide := widget.NewButton(locale.T("Hide until the config changes"), func() {
		b.dismissed = configSignature(b.ac.FileService.ConfigPath)
		b.refresh()
	})
	hide.Importance = widget.LowImportance
	b.box.Add(container.NewHBox(hide))
	b.box.Add(widget.NewSeparator())
	b.box.Show()
	b.box.Refresh()
}
