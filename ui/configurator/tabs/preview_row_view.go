// File preview_row_view.go — как строка списка Preview выглядит и что
// показывает по клику (SPEC 116 W11).
//
// Строк два вида (preview_rows.go): за одной стоит собравшийся узел, за
// другой — неразобранная запись тела. Тексты и обработчики развилку знают
// ЗДЕСЬ, одним местом; список их только вызывает.
package tabs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
)

// previewUnsupportedMark — знак неразобранной записи в подстроке строки.
//
// «⚠» U+26A0 — тот же знак, которым отмечены недоступные цели detour и
// потери бэкапа: новых глифов волна не заводит.
const previewUnsupportedMark = "⚠"

// previewRowTitle — имя строки.
//
// У собравшегося узла это его ФИНАЛЬНЫЙ тег (то, чем узел зовётся в
// конфиге), у неразобранной записи — её сырой тег: финального у неё нет и
// быть не может, тег-машину она не проходит вовсе.
func previewRowTitle(r previewRow) string {
	if r.Unsupported {
		return textnorm.NormalizeProxyDisplay(r.RawTag)
	}
	if r.Node == nil {
		return textnorm.NormalizeProxyDisplay(r.RawTag)
	}
	return nodeDisplayLine(r.Node)
}

// previewRowReason — причина отбраковки на языке пользователя.
//
// В состоянии причина хранится АНГЛИЙСКИМ ключом (то же правило, что у фраз
// эмиссионных деградаций): иначе смена языка переписывала бы `nodes[]`, а
// merge сравнивал бы переведённые строки. Перевод — здесь, на показе, одной
// точкой на все три места, где причина видна (подстрока, тултип, окно записи).
// Ключ без записи в каталоге проходит насквозь: технический текст парсера
// («vless outbound rejected: empty user id») переводить нечем и незачем.
func previewRowReason(r previewRow) string {
	if r.Reason == "" {
		return locale.T("could not be parsed")
	}
	return locale.T(r.Reason)
}

// previewRowSubtitle — вторая строка: «протокол·транспорт·security» у узла,
// «⚠ причина» у неразобранной записи.
//
// Причина занимает место протокола не случайно: подстрока отвечает на вопрос
// «что это такое», и у записи, которую не разобрали, честный ответ —
// «вот почему её нет».
func previewRowSubtitle(r previewRow) string {
	if r.Unsupported {
		return previewUnsupportedMark + " " + previewRowReason(r)
	}
	if r.Node == nil {
		// Узел в составе есть, эмиссия его не выпустила: он выключен.
		return locale.T("off")
	}
	return previewNodeSubtitle(r.Node)
}

// previewRowToolTip — полный текст под курсором.
//
// У неразобранной записи это причина целиком плюс её исходник: в подстроке
// причина обрезается шириной окна, а исходник не показывается вовсе, и без
// тултипа строка «⚠ vless outbound rejected: empty…» была бы тупиком.
// У собравшегося узла тултипа нет: его подстрока помещается целиком.
func previewRowToolTip(r previewRow) string {
	if !r.Unsupported {
		return ""
	}
	tip := previewRowReason(r)
	if r.OriginRaw != "" {
		tip += "\n" + r.OriginRaw
	}
	return tip
}

// showPreviewRowContextMenu — меню строки правым кликом.
//
// У неразобранной записи из всего меню осмысленны два пункта: открыть её в
// том же окне узла (read-only: тела нет, есть причина и исходник) и
// скопировать исходник (починить строку можно только вставив её исправленной
// — руками, в папку). Пункты про JSON, тег и операции над узлом ей не
// подходят: узла за ней нет.
//
// W13 заход 2: отдельного окна «Node info…» строки больше нет — и карандаш
// строки, и первый пункт меню открывают ОДНО окно правки узла
// (`showPreviewNodeEditWindow`), принцип «меню = кнопки».
func showPreviewRowContextMenu(
	win fyne.Window,
	r previewRow,
	rawTag string,
	ops *previewNodeOps,
	pe *fyne.PointEvent,
) {
	if win == nil || pe == nil {
		return
	}
	if !r.Unsupported {
		// Узел может быть nil — выключенный узел эмиссию не проходит, но
		// строкой остаётся, и операции над ним (перенести, переименовать,
		// удалить) обязаны работать: он в составе, просто не в конфиге.
		showPreviewNodeContextMenu(win, r, rawTag, ops, pe)
		return
	}
	origin := r.OriginRaw
	items := []*fyne.MenuItem{
		fyne.NewMenuItem(locale.T("Node info…"), func() {
			showPreviewNodeEditWindow(r, rawTag, ops)
		}),
		fyne.NewMenuItem(locale.T("Copy source line"), func() {
			fynewidget.SetClipboard(origin)
		}),
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), win.Canvas(), pe.AbsolutePosition)
}
