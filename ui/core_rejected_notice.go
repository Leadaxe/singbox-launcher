// File core_rejected_notice.go — показ страховки «ядро отвергло узел»
// (SPEC 132, волна 5 UI). Тексты — §6 SPEC.md, дословно.
//
// # Три поверхности, один источник
//
// Цикл страховки живёт в общей функции сборки (`core/rebuild_corereject.go`) и
// о UI не знает вовсе. Наружу он отдаёт три вещи, и каждой отвечает своя
// поверхность здесь:
//
//	progress(disabled)          → строка статуса вкладки Local, пока идёт цикл
//	decide(disabled) bool       → диалог предела после 10 кругов
//	ConfigBuilt.DisabledNodes   → ПЛАШКА над списком узлов + окно «Показать»
//
// # Почему плашка, а не всплывашка
//
// Решение владельца №5: «Сообщение — плашка на главном экране, а не
// всплывашка». Выключение уже случилось, ответа от человека не требуется, и
// модальный диалог тут просил бы подтвердить свершившийся факт. Плашка стоит,
// пока её не закрыли крестиком, и держит при себе кнопку «Показать».
//
// # Кому достаётся диалог предела
//
// Только входам С ОКНОМ: нажатие Start, кнопка Rebuild, сборка после Save
// Конфигуратора. Фоновые (автообновление подписок, debug API) получают
// decide=nil и идут молча до жёсткого потолка — §10.4 SPEC.md. Окно в трее
// приравнено к фоновому входу: диалог за скрытым окном никто не увидит, а
// цикл встал бы намертво в ожидании ответа.
//
// # Потоки
//
// Цикл крутится в ФОНЕ (горутина сборки), а Fyne трогать оттуда нельзя.
// Поэтому decide показывает диалог через fyne.Do и ждёт ответа на КАНАЛЕ:
// блокируется фоновая горутина цикла, UI-поток свободен. Тот же приём у
// progress — только ждать там нечего.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package ui

import (
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/events"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
	"singbox-launcher/ui/components"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
// Все — дословно из §6 SPEC 132, словарь общий с LxBox: rejected / turned off
// / disabled / checking; «servers», не «nodes»; слово scanning не используется.
const (
	// §6.1 — плашка после успешной сборки.
	coreRejectedTitleOneText  = "1 server disabled"                                                        // l10n-key
	coreRejectedTitleManyText = "%d servers disabled"                                                      // l10n-key
	coreRejectedBodyOneText   = "The core rejected it, so it was turned off to let the VPN start: %s"      // l10n-key
	coreRejectedBodyManyText  = "The core rejected them, so they were turned off to let the VPN start: %s" // l10n-key
	coreRejectedMoreText      = "+%d more"                                                                 // l10n-key
	coreRejectedShowText      = "Show"                                                                     // l10n-key

	// §6.2 — диалог предела.
	coreRejectedLimitTitleText = "%d servers disabled — there may be more"                                                                                                  // l10n-key
	coreRejectedLimitBody1Text = "The core has rejected %d servers so far, and each was turned off. Keep checking the rest? On a large subscription this can take a while." // l10n-key
	coreRejectedLimitBody2Text = "If you stop, the VPN will not start. The servers already turned off will stay off."                                                       // l10n-key
	coreRejectedLimitStopText  = "Stop"                                                                                                                                     // l10n-key
	coreRejectedLimitKeepText  = "Keep checking"                                                                                                                            // l10n-key

	// §6.3 — состояние во время цикла.
	coreRejectedCheckingText = "Checking servers… (%d disabled)" // l10n-key

	// Окно «Показать».
	coreRejectedListTitleText = "Servers turned off by the core" // l10n-key
	coreRejectedCloseText     = "Close"                          // l10n-key
)

// coreRejectedNamesShown — сколько имён узлов влезает в текст плашки, дальше
// хвост «+%d more» (§6.1: «имён до трёх, дальше хвост»).
const coreRejectedNamesShown = 3

// coreRejectedNotice — состояние плашки главного окна.
//
// Живёт в App: плашка одна на окно, а событий сборки много, и вторая полоса
// под первой означала бы, что человек читает историю сборок вместо текущего
// положения дел.
type coreRejectedNotice struct {
	mu sync.Mutex
	// bar — сам виджет; nil до первой сборки с выключениями.
	bar *fynewidget.NoticeBar
	// holder — контейнер, в котором плашка стоит: он и прячется целиком.
	// Пустой Stack в Border'е места не занимает, поэтому пока плашки нет,
	// раскладка вкладки не меняется вовсе.
	holder *fyne.Container
	// last — состав ПОСЛЕДНЕГО показа: его открывает «Показать».
	last []events.DisabledNode
}

// Отдельного признака «закрыта крестиком» тут НЕТ, и это осознанно: его уже
// несёт видимость holder'а, а второе поле про тот же факт разъехалось бы с
// ней при первой же правке. Крестик прячет holder, событие С выключениями
// показывает его снова — «не дублируется при повторных сборках без новых
// выключений» обеспечено тем, что сборка без выключений плашку не трогает
// вовсе (подписчик не зовёт showCoreRejected при пустом DisabledNodes).

// coreRejectedBar строит контейнер плашки для вкладки Local.
//
// Возвращается ПУСТОЙ контейнер: плашки ещё нет, места она не занимает, и
// раскладка вкладки при первом же выключении не скачет — Stack просто
// получает содержимое.
func (app *App) coreRejectedBar() fyne.CanvasObject {
	if app.rejected == nil {
		app.rejected = &coreRejectedNotice{}
	}
	app.rejected.holder = container.NewStack()
	app.rejected.holder.Hide()
	return app.rejected.holder
}

// showCoreRejected показывает плашку по итогу сборки.
//
// Вызывается ТОЛЬКО из UI-потока (fyne.Do у подписчика события).
func (app *App) showCoreRejected(list []events.DisabledNode) {
	n := app.rejected
	if n == nil || n.holder == nil || len(list) == 0 {
		return
	}
	n.mu.Lock()
	n.last = list
	n.mu.Unlock()
	// Показ поверх закрытой крестиком плашки — намеренно: это не та же
	// плашка, а новый факт, узлы выключили ещё раз.

	title, body := coreRejectedTexts(list)
	if n.bar == nil {
		n.bar = fynewidget.NewNoticeBar(title, body,
			locale.T(coreRejectedShowText),
			func() { app.openCoreRejectedList() },
			func() { app.dismissCoreRejected() },
		)
		n.holder.Add(n.bar)
	} else {
		n.bar.SetText(title, body)
	}
	n.holder.Show()
	n.holder.Refresh()
}

// dismissCoreRejected убирает плашку до следующего события с выключениями.
func (app *App) dismissCoreRejected() {
	n := app.rejected
	if n == nil || n.holder == nil {
		return
	}
	n.holder.Hide()
	n.holder.Refresh()
}

// coreRejectedTexts — заголовок и текст плашки по составу (§6.1).
//
// Число ПЕРВЫМ словом — требование текста; форма «1 server disabled» отдельной
// строкой, а не «%d servers» с единицей, потому что английское множественное
// число иначе не сходится, а русский перевод у обеих форм один
// («Отключено серверов: %d») и подставляет то же число.
func coreRejectedTexts(list []events.DisabledNode) (title, body string) {
	if len(list) == 1 {
		title = locale.T(coreRejectedTitleOneText)
	} else {
		title = locale.Tf(coreRejectedTitleManyText, len(list))
	}
	names := coreRejectedNames(list)
	if len(list) == 1 {
		return title, locale.Tf(coreRejectedBodyOneText, names)
	}
	return title, locale.Tf(coreRejectedBodyManyText, names)
}

// coreRejectedNames — до трёх имён через запятую плюс хвост «+N more».
func coreRejectedNames(list []events.DisabledNode) string {
	shown := len(list)
	if shown > coreRejectedNamesShown {
		shown = coreRejectedNamesShown
	}
	names := make([]string, 0, shown)
	for i := 0; i < shown; i++ {
		names = append(names, coreRejectedNodeName(list[i]))
	}
	s := strings.Join(names, ", ")
	if rest := len(list) - shown; rest > 0 {
		s += " " + locale.Tf(coreRejectedMoreText, rest)
	}
	return s
}

// coreRejectedNodeName — чем узел зовётся в плашке и в списке.
//
// Финальный тег: именно им узел назвало ядро, и именно его человек видит в
// списке прокси. Пустой тег невозможен (страховка выключает только
// сопоставленный узел), но полагаться на это молча нельзя.
func coreRejectedNodeName(n events.DisabledNode) string {
	if tag := strings.TrimSpace(n.Tag); tag != "" {
		return tag
	}
	return locale.T("(unnamed)")
}

// openCoreRejectedList — окно «Показать»: список выключенных в этом заходе.
//
// Отдельное ОКНО (Application.NewWindow), а не модальный попап: список
// произвольной длины, у каждой строки — текст ядра с переносом, а высокий
// модальный попап в Fyne раздувается на весь экран (память
// `fyne-label-minwidth-trap`).
//
// Кнопки «включить всё обратно» здесь НЕТ — решение владельца (§6.1). Узел
// возвращается тем же чекбоксом, которым его выключили бы рукой: единый
// сеттер стирает вердикт и сборка проверит узел заново.
func (app *App) openCoreRejectedList() {
	n := app.rejected
	if n == nil || app.core == nil || app.core.UIService == nil ||
		app.core.UIService.Application == nil {
		return
	}
	n.mu.Lock()
	list := n.last
	n.mu.Unlock()
	if len(list) == 0 {
		return
	}

	win := app.core.UIService.Application.NewWindow(locale.T(coreRejectedListTitleText))
	rows := make([]fyne.CanvasObject, 0, len(list)*2)
	for i, item := range list {
		if i > 0 {
			rows = append(rows, widget.NewSeparator())
		}
		rows = append(rows, coreRejectedListRow(item))
	}
	closeBtn := widget.NewButton(locale.T(coreRejectedCloseText), func() { win.Close() })
	body := container.NewVBox(rows...)
	win.SetContent(container.NewPadded(container.NewBorder(
		nil, container.NewPadded(closeBtn), nil, nil,
		components.WrapInScrollWithGutter(body),
	)))
	// Высота по содержимому с потолком: три строки не должны открывать окно на
	// пол-экрана, а тридцать — уезжать за его край. Ширина фиксированная:
	// тексты ядра длинные, и узкое окно превращало бы каждый в простыню.
	win.Resize(fyne.NewSize(560, coreRejectedListHeight(len(list))))
	fynewidget.CenterOnScreen(win)
	win.Show()
}

// coreRejectedListRowHeight — оценка высоты одной строки списка (имя,
// источник, текст ядра в пару строк).
const coreRejectedListRowHeight = 96

// coreRejectedListHeight — высота окна по числу строк, с полом и потолком.
func coreRejectedListHeight(rows int) float32 {
	h := float32(rows*coreRejectedListRowHeight) + 90 // +кнопка Close и поля
	if h < 220 {
		h = 220
	}
	if h > 640 {
		h = 640
	}
	return h
}

// coreRejectedListRow — одна строка списка: имя узла жирным, источник
// приглушённо, ниже текст ядра мелким с переносом.
//
// Вёрстка — по образцу раздела «Уведомления» (internal/nodewarn/section.go):
// это те же сведения об узле, и разъезжаться им нельзя.
func coreRejectedListRow(item events.DisabledNode) fyne.CanvasObject {
	name := widget.NewLabel(nodewarn.ErrorMark + " " + coreRejectedNodeName(item))
	name.TextStyle.Bold = true
	// Имя узла бывает длинным (эмодзи-флаг, страна, номер) — Truncation, а не
	// Wrapping: заголовок строки обязан держать одну строку, целиком он виден
	// в списке прокси.
	name.Wrapping = fyne.TextWrapOff
	name.Truncation = fyne.TextTruncateEllipsis

	rows := make([]fyne.CanvasObject, 0, 3)
	if src := strings.TrimSpace(item.SourceLabel); src != "" {
		label := widget.NewLabel(src)
		label.Importance = widget.LowImportance
		label.Wrapping = fyne.TextWrapOff
		label.Truncation = fyne.TextTruncateEllipsis
		// Имя и источник — одной строкой: «что» и «откуда» читаются вместе.
		rows = append(rows, container.NewBorder(nil, nil, name, nil, label))
	} else {
		rows = append(rows, name)
	}

	// Текст ядра — произвольной длины, Wrapping ОБЯЗАТЕЛЕН (Л19).
	reason := widget.NewLabel(strings.TrimSpace(item.Reason))
	reason.Wrapping = fyne.TextWrapWord
	reason.Importance = widget.MediumImportance
	rows = append(rows, reason)
	return container.NewVBox(rows...)
}

// --- Колбэки цикла: строка хода и диалог предела ---

// installCoreRejectHooks отдаёт контроллеру UI-колбэки цикла страховки.
//
// До этой волны обе функции возвращали nil, и все входы вели себя как
// фоновые (§10.4 SPEC.md). Теперь вход С ОКНОМ получает диалог и строку хода,
// а фоновый — по-прежнему тишину: гейт «окно есть и оно видимо» стоит внутри
// самих колбэков, потому что цикл один на все входы и различить их он не
// может.
func (app *App) installCoreRejectHooks(controller *core.AppController) {
	if controller == nil {
		return
	}
	controller.SetCoreRejectProgress(func(disabled int) {
		app.coreRejectProgress(disabled)
	})
	controller.SetCoreRejectDecider(func(disabled int) bool {
		return app.coreRejectDecide(disabled)
	})
}

// coreRejectProgress — строка «Checking servers… (%d disabled)» в статусе
// вкладки Local, пока идёт цикл.
//
// Пишет в ту же строку, что «Sing-box is stopped.» и «Pinging 5/9…»: это
// строка состояния СПИСКА узлов, а цикл как раз и решает, каким узлам в этом
// списке быть. Возврат к обычному тексту делает следующее обновление списка
// (ResetAPIState / загрузка прокси), как и после «Pinging…».
func (app *App) coreRejectProgress(disabled int) {
	if app.core == nil || app.core.UIService == nil {
		return
	}
	label := app.core.UIService.ListStatusLabel
	if label == nil {
		return
	}
	// Из ФОНОВОЙ горутины сборки: Fyne трогаем только через fyne.Do.
	fyne.Do(func() {
		label.SetText(locale.Tf(coreRejectedCheckingText, disabled))
	})
}

// coreRejectDecide — диалог предела (§6.2). true = Keep checking.
//
// Зовётся из ФОНОВОЙ горутины цикла и обязан её заблокировать до ответа:
// цикл продолжается ровно тем, что человек выбрал. Поэтому диалог
// показывается через fyne.Do, а ответ приезжает КАНАЛОМ — UI-поток при этом
// свободен и рисует диалог.
//
// Окна нет либо оно скрыто (трей) — ведём себя как фоновый вход: молча
// продолжаем. Иначе цикл встал бы навсегда в ожидании ответа, которого некому
// дать.
func (app *App) coreRejectDecide(disabled int) bool {
	if app.core == nil || app.core.UIService == nil {
		return true
	}
	win := app.core.UIService.MainWindow
	if win == nil || !app.windowVisible() {
		return true
	}

	answer := make(chan bool, 1)
	fyne.Do(func() {
		// Два абзаца отдельными Label'ами: пустая строка внутри одного
		// Label'а с Wrapping даёт непредсказуемый отступ, а VBox кладёт
		// ровно theme.Padding.
		//
		// Wrapping ОБЯЗАТЕЛЕН на обоих: без него min-width равна длине всей
		// строки, и диалог раздувается на весь экран (Л19).
		p1 := widget.NewLabel(locale.Tf(coreRejectedLimitBody1Text, disabled))
		p1.Wrapping = fyne.TextWrapWord
		p2 := widget.NewLabel(locale.T(coreRejectedLimitBody2Text))
		p2.Wrapping = fyne.TextWrapWord
		content := container.NewVBox(p1, p2)

		d := dialog.NewCustomConfirm(
			locale.Tf(coreRejectedLimitTitleText, disabled),
			locale.T(coreRejectedLimitKeepText), // подтверждение — основная
			locale.T(coreRejectedLimitStopText), // отказ
			content,
			func(keep bool) { answer <- keep },
			win,
		)
		// Ширина задаётся явно: Label с Wrapping просит min-width в одно
		// слово, и диалог без Resize схлопнулся бы в колонку по слову.
		d.Resize(fyne.NewSize(460, 260))
		d.Show()
	})
	return <-answer
}

// windowVisible — видно ли главное окно человеку.
//
// Плашка и диалог за скрытым окном бессмысленны: первую никто не прочтёт, а
// второй остановил бы цикл навсегда. Признак ведём сами (core/uiservice его
// не хранит) — по тем же хукам OnWindowShown/OnWindowHidden, которыми
// пользуется авто-обновление Remote.
func (app *App) windowVisible() bool {
	if app == nil {
		return false
	}
	app.windowVisibleMu.Lock()
	defer app.windowVisibleMu.Unlock()
	return app.windowShown
}

func (app *App) setWindowVisible(on bool) {
	if app == nil {
		return
	}
	app.windowVisibleMu.Lock()
	app.windowShown = on
	app.windowVisibleMu.Unlock()
}
