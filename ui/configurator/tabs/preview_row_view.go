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

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
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

// Знака info у ИМЕНИ больше нет (правка владельца к разведению уровней):
// previewRowTitleShown снят, и previewRowTitle — единственное имя строки.
// Про info говорит иконка в подстроке (nodewarn.InfoSubtitleLine), и решает
// её место тот же набор кодов, что и цвет подстроки: r.Warnings.

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
	if r.GroupCounted {
		// Честный размер пула из модели (annotatePreviewGroupRows). Ноль
		// живых членов = на сборке группа пуста: ⚠ как у сломанного узла,
		// а не «[N] fastest — типа всё нормально» (обкатка, заход 3).
		sub := previewGroupSubtitleCounted(r.Node, r.GroupAlive)
		if r.GroupAlive == 0 {
			return previewUnsupportedMark + " " + sub + " — " + locale.T("no working members")
		}
		return sub
	}
	// SPEC 131 §6: у здорового узла подстрока отвечает «что это такое», а у
	// узла с деградацией — «что с ним сделали». Второе важнее: состав узел
	// описывает и без подстроки (окно узла, тултип), а снятое поле не
	// показывает больше НИЧЕГО и молча меняет поведение.
	//
	// Глиф тот же, что у неразобранной записи: развилки «⚠ означает одно» и
	// «⚠ означает другое» в строке нет — есть один знак «с этим узлом что-то
	// не так», а подробности берёт на себя окно узла.
	if sub := nodewarn.Subtitle(r.Warnings); sub != "" {
		return sub
	}
	return previewNodeSubtitle(r.Node)
}

// previewRowWarn — подстроку красить цветом предупреждения.
//
// Одно место на все три списка: корневой, drill-down и Preview окна
// источника красили её каждый своим `if pr.Unsupported`, и добавление
// второго повода разъехалось бы по трём файлам.
//
// Из кодов узла красит только ПРОБЛЕМА (error/warning): оранжевая подстрока —
// это призыв разбираться, а info говорит ровно обратное. По `len(Warnings)`
// узел с единственным «к сведению» (reality_fp_not_chrome) выглядел
// сломанным, показывая при этом свой обычный состав «vless·tcp·Reality+Vision»
// — цвет тревоги и текст «всё в порядке» в одной строке.
func previewRowWarn(r previewRow) bool {
	if r.Unsupported {
		return true
	}
	if r.Node != nil && r.GroupCounted && r.GroupAlive == 0 {
		return true
	}
	return nodewarn.HasProblems(r.Warnings)
}

// previewRowToolTip — полный текст под курсором.
//
// У неразобранной записи это причина целиком плюс её исходник: в подстроке
// причина обрезается шириной окна, а исходник не показывается вовсе, и без
// тултипа строка «⚠ vless outbound rejected: empty…» была бы тупиком.
// У собравшегося узла тултипа нет: его подстрока помещается целиком.
func previewRowToolTip(r previewRow) string {
	if !r.Unsupported {
		// SPEC 131 §6: у выжившего узла тултип появляется только когда есть
		// что сказать — заголовки его деградаций. Подстрока показывает
		// первый из них и «+N», тултип раскрывает все: иначе про второй и
		// третий код узнать было бы негде, кроме окна узла.
		return nodewarn.ToolTip(r.Warnings)
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
			// Исходник неразобранной строки — это ссылка как приехала, и
			// она может нести приватный ключ (wireguard/masque/ssh
			// разбираются, даже когда узел из них не собрался). Признак
			// считаем по самой строке: тела у такой записи нет.
			fynewidget.ConfirmShareURISecretCopy(win, subscription.ShareURITextCarriesPrivateKey(origin), origin)
		}),
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), win.Canvas(), pe.AbsolutePosition)
}
