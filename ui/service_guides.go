package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Ссылки окна Service на гайды (SPEC 161 §6): документы лаунчера и гайды
// форка, по локали — `.ru.md` для ru, иначе `.md`. Якоря GitHub для
// кириллических заголовков другие, поэтому таблица держит оба.

// serviceGuide — один гайд: подпись ссылки (имя документа, не переводится),
// база URL, имя файла без расширения и якоря EN/RU (пусто — без якоря).
type serviceGuide struct {
	Label              string
	Base, Name         string
	AnchorEN, AnchorRU string
}

var (
	guideLxd             = serviceGuide{Label: "lxd", Base: constants.CoreDocsBaseURL, Name: "lxd-daemon"}                               // l10n-exempt: document name
	guideOpenWrt         = serviceGuide{Label: "OpenWrt", Base: constants.CoreDocsBaseURL, Name: "openwrt-vpn-ssid"}                     // l10n-exempt: document name
	guideTroubleshooting = serviceGuide{Label: "Troubleshooting → Daemon", Base: constants.LauncherDocsBaseURL, Name: "TROUBLESHOOTING", // l10n-exempt: document name
		AnchorEN: "daemon", AnchorRU: "демон"}
	guideLxdDiagnose = serviceGuide{Label: "lxd-daemon → Diagnosing (§11)", Base: constants.CoreDocsBaseURL, Name: "lxd-daemon", // l10n-exempt: document name
		AnchorEN: "11-diagnosing-a-misbehaving-daemon", AnchorRU: "11-диагностика-неисправного-демона"}
	guideLxdMac = serviceGuide{Label: "lxd-daemon → macOS (§7)", Base: constants.CoreDocsBaseURL, Name: "lxd-daemon", // l10n-exempt: document name
		AnchorEN: "7-macos--automatic-installation", AnchorRU: "7-macos--автоматическая-установка"}
	guideLxdWindows = serviceGuide{Label: "lxd-daemon → Windows (§7a)", Base: constants.CoreDocsBaseURL, Name: "lxd-daemon", // l10n-exempt: document name
		AnchorEN: "7a-windows--automatic-installation", AnchorRU: "7a-windows--автоматическая-установка"}
	guideLxdLinux = serviceGuide{Label: "lxd-daemon → Linux (§8)", Base: constants.CoreDocsBaseURL, Name: "lxd-daemon", // l10n-exempt: document name
		AnchorEN: "8-linux--setup-approaches", AnchorRU: "8-linux--подходы-к-настройке"}
	guideLxdPairing = serviceGuide{Label: "lxd-daemon → Pairing (§9)", Base: constants.CoreDocsBaseURL, Name: "lxd-daemon", // l10n-exempt: document name
		AnchorEN: "9-pairing-a-client-the-same-on-every-os", AnchorRU: "9-сопряжение-клиента-одинаково-на-всех-ос"}
	guideLxdConfig = serviceGuide{Label: "lxd-daemon → daemon.json (§3)", Base: constants.CoreDocsBaseURL, Name: "lxd-daemon", // l10n-exempt: document name
		AnchorEN: "3-daemonjson--the-daemons-settings", AnchorRU: "3-daemonjson--настройки-демона"}
	guideDaemonRemote = serviceGuide{Label: "DAEMON_AND_REMOTE", Base: constants.LauncherDocsBaseURL, Name: "DAEMON_AND_REMOTE"} // l10n-exempt: document name
)

// guideURL — адрес гайда по текущей локали.
func guideURL(g serviceGuide) string {
	url, anchor := g.Base+g.Name+".md", g.AnchorEN
	if locale.GetLang() == "ru" {
		url, anchor = g.Base+g.Name+".ru.md", g.AnchorRU
	}
	if anchor != "" {
		url += "#" + anchor
	}
	return url
}

// guideLink — гиперссылка на гайд; открывается в браузере.
func guideLink(win fyne.Window, g serviceGuide) *widget.Hyperlink {
	url := guideURL(g)
	link := widget.NewHyperlink(g.Label, nil)
	_ = link.SetURLFromString(url)
	link.OnTapped = func() {
		if err := platform.OpenURL(url); err != nil {
			debuglog.ErrorLog("service window: open guide %s: %v", url, err)
			ShowError(win, err)
		}
	}
	return link
}

// serviceGuidesRow — строка «<label> ссылка · ссылка».
func serviceGuidesRow(win fyne.Window, label string, guides ...serviceGuide) fyne.CanvasObject {
	row := container.NewHBox(widget.NewLabel(label))
	for i, g := range guides {
		if i > 0 {
			row.Add(widget.NewLabel("·"))
		}
		row.Add(guideLink(win, g))
	}
	return row
}

// serviceCoreGuides — гайды установки ядра под init-систему машины.
func serviceCoreGuides(p core.ServicePlatform) []serviceGuide {
	switch p.Init {
	case core.ServiceInitLaunchd:
		return []serviceGuide{guideLxdMac}
	case core.ServiceInitSCM:
		return []serviceGuide{guideLxdWindows}
	case core.ServiceInitProcd:
		return []serviceGuide{guideLxdLinux, guideOpenWrt}
	}
	return []serviceGuide{guideLxdLinux}
}

// serviceHeaderGuides — гайды шапки: lxd, для OpenWrt — ещё openwrt-vpn-ssid.
func serviceHeaderGuides(p core.ServicePlatform) []serviceGuide {
	if p.Init == core.ServiceInitProcd {
		return []serviceGuide{guideLxd, guideOpenWrt}
	}
	return []serviceGuide{guideLxd}
}
