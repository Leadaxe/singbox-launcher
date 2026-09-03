// File source_preview_header.go — «визитка» источника над списком узлов на
// вкладке Preview окна источника (обкатка SPEC 116, заход 3).
//
// # Зачем
//
// Preview — первое, что человек открывает у подписки: он пришёл смотреть
// состав. А всё, чем подписка себя называет — имя от провайдера, его
// объявление, ссылка на поддержку, — жило на вкладке Overview, куда никто не
// заходит. Объявление Liberty («⚡ — Лучшие сервера / 💳 — Зарубежная оплата»)
// в обкатке искали среди узлов именно потому, что на экране состава его не
// было вовсе.
//
// # Почему объявление здесь с переносами строк
//
// `ProviderAnnounce.AnnounceMessage()` схлопывает переносы в пробелы: это
// правило ОДНОСТРОЧНЫХ поверхностей (пометка в списке Sources, отчёт «Итога»,
// строка kvRow в Overview), где многострочный текст порвал бы вёрстку. Здесь
// поверхность своя — блок под заголовком, и легенда провайдера из четырёх
// строк обязана читаться четырьмя строками, иначе она превращается в кашу.
// Поэтому текст берётся сырым (`announce.Message`) и только режется по тому же
// потолку `MaxAnnounceRunes` — общий предел на длину остаётся общим.
//
// # Support-кнопка
//
// Та же `supportLinkButton`, что у строки источника в списке Sources: она уже
// умеет выбирать иконку (Telegram для t.me/tg://, Link для прочих), проверять
// `urlsafe.IsSafeAnnounceURL` и открывать ссылку. Второй реализации не
// заводим — разъехались бы проверки безопасности.
package tabs

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// sourcePreviewCard — заголовок источника и объявление провайдера над списком.
//
// Возвращает nil, когда показывать нечего (имени нет, объявления нет, ссылки
// нет): пустая рамка над списком съедала бы высоту ни за чем.
//
// Заголовок — то же имя, каким источник зовут заголовок окна и шапка
// drill-down: profile_title провайдера, иначе собственное имя, иначе URL.
func sourcePreviewCard(src *corestate.Source) fyne.CanvasObject {
	if src == nil {
		return nil
	}
	meta := diagOf(src)

	title := sourcePreviewCardTitle(src, meta)
	announce := sourcePreviewAnnounceText(meta)
	support := supportLinkButton(meta, nil)

	if title == "" && announce == "" && support == nil {
		return nil
	}

	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	// Ловушка fyne-label-minwidth-trap: Label без переноса задаёт окну
	// min-width своей строкой. Имя провайдера бывает длинным, а у подписки без
	// имени сюда попадает URL — окно раздувалось бы на весь экран.
	titleLabel.Wrapping = fyne.TextWrapOff
	titleLabel.Truncation = fyne.TextTruncateEllipsis

	// Border, а не HBox: HBox отдал бы заголовку только его MinSize, и с
	// Truncation строка схлопнулась бы в «…» (та же ошибка, что была в шапке
	// drill-down).
	var titleRow fyne.CanvasObject = titleLabel
	if support != nil {
		titleRow = container.NewBorder(nil, nil, nil,
			container.NewHBox(support, layout.NewSpacer()), titleLabel)
	}

	box := container.NewVBox(titleRow)
	if announce != "" {
		msg := widget.NewLabel(announce)
		msg.Wrapping = fyne.TextWrapWord
		msg.Importance = widget.LowImportance
		box.Add(msg)
	}
	box.Add(widget.NewSeparator())
	return box
}

// sourcePreviewCardTitle — как источник зовут в шапке Preview.
func sourcePreviewCardTitle(src *corestate.Source, meta *sourceDiag) string {
	if t := strings.TrimSpace(meta.profileTitle()); t != "" {
		return t
	}
	return strings.TrimSpace(wizardbusiness.SourceDisplayName(*src))
}

// sourcePreviewAnnounceText — объявление провайдера С ПЕРЕНОСАМИ строк.
//
// Отличие от providerAnnounceText одно и намеренное: переносы сохраняются (см.
// шапку файла). Потолок длины — тот же `corestate.MaxAnnounceRunes`, чтобы
// провайдер не мог занять экран целиком.
func sourcePreviewAnnounceText(meta *sourceDiag) string {
	if meta == nil {
		return ""
	}
	a := meta.providerAnnounce()
	if a == nil {
		return ""
	}
	msg := strings.TrimSpace(a.Message)
	if msg == "" {
		return ""
	}
	// Пустые строки схлопываются, сами переносы остаются: провайдеры любят
	// присылать двойные \n\n, и они рвали бы блок пополам.
	lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, ln := range lines {
		if ln = strings.TrimSpace(ln); ln != "" {
			kept = append(kept, ln)
		}
	}
	msg = strings.Join(kept, "\n")

	runes := []rune(msg)
	if len(runes) > corestate.MaxAnnounceRunes {
		msg = strings.TrimSpace(string(runes[:corestate.MaxAnnounceRunes])) + "…"
	}
	return msg
}
