// File final_tab.go — вкладка «Итог»: последняя стадия Мастера.
//
// Прежде это была вкладка Files — три действия над готовым состоянием
// (собрать, посмотреть, перенести на другую машину). SPEC 115 сделал её
// стадией: вход на вкладку запускает ПОЛНУЮ сборку в памяти (парсерный кэш →
// резолв ссылок и цепочек → граф-санитайзер), но ничего не пишет и не
// перезапускает ядро. Смысл — показать, что именно соберётся, ДО того как это
// станет боевым config.json.
//
// Отсюда и порядок на экране: пока идёт сборка — прелоадер, по завершении —
// отчёт предупреждений, и только после отчёта появляется Save. Кнопка,
// доступная до сборки, приглашала бы сохранить непроверенное; ошибка сборки не
// показывает её вовсе — сохранять нечего.
//
// Конфиг по-прежнему показывается ПО КНОПКЕ и в ОТДЕЛЬНОМ окне: конфиг читают
// целиком, а внутри вкладки ему достаётся половина высоты, и та отнимается у
// отчёта.
package tabs

import (
	"errors"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/nodewarn"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// CreateFinalTab строит вкладку «Итог».
func CreateFinalTab(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState) fyne.CanvasObject {
	win := guiState.Window

	hint := widget.NewLabel(locale.T(finalTabHintText))
	hint.Wrapping = fyne.TextWrapWord

	// Прелоадер: сборка на большой подписке занимает секунды, и молчащая
	// вкладка в это время читается как «зависло».
	progress := widget.NewProgressBarInfinite()
	progress.Hide()
	progressLabel := widget.NewLabel(locale.T(finalBuildingText))
	progressLabel.Hide()

	// Строки отчёта — своим Label каждая, без лимита. Wrapping обязателен:
	// Label без переноса отдаёт всю строку как min-width и раздувает окно
	// Мастера на весь экран ([[fyne-label-minwidth-trap]]), а причина бывает
	// длинной — имя источника плюс имя ненайденного узла.
	reportBox := container.NewVBox()
	reportScroll := container.NewVScroll(reportBox)
	// Нижняя граница высоты, а не её значение: панель отчёта занимает всё,
	// что остаётся между шапкой и кнопками (Border-раскладка ниже), и на
	// высоком окне растёт вместе с ним. Прежние 160pt были потолком: три
	// предупреждения уже не помещались, список читался через скролл в
	// четыре строки, а полэкрана под ним пустовало.
	//
	// Минимум — одна строка: AppTabs считает свой min-size по ВСЕМ вкладкам,
	// и большой минимум здесь не дал бы сжать окно Мастера на невысоком
	// экране даже на других вкладках. Всё, что не влезло, читается скроллом.
	reportScroll.SetMinSize(fyne.NewSize(0, 48))
	// Подложка панели: фон поля ввода и тонкая рамка цвета разделителя.
	// Без них список предупреждений сливался с подсказкой над ним и
	// блоком Backup под ним — одна серая простыня, где не видно, что из
	// этого отчёт.
	reportPanel := reportPanelFrame(reportScroll)

	// SPEC 116 W12 фикс 4: явный статус сборки НАД списком. Исход перестаёт
	// читаться косвенно (пустой список / непустой / красная строка внутри
	// него) — главное («собралось или нет») стоит первой строкой.
	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	statusLabel.Hide()
	setStatus := func(text string, failed bool) {
		statusLabel.SetText(text)
		if failed {
			statusLabel.Importance = widget.DangerImportance
		} else {
			statusLabel.Importance = widget.MediumImportance
		}
		statusLabel.Refresh()
		statusLabel.Show()
	}

	// Иконка и стиль — те же, что у «Copy token» в Settings
	// (`ui/settings_tab.go`): одна операция, один облик (фикс 5).
	copyBtn := widget.NewButtonWithIcon(locale.T("Copy config"), theme.ContentCopyIcon(), nil)
	copyBtn.Hide()

	// Та же операция, что у кнопки Save справа внизу: записать state.json и
	// пересобрать рабочий config.json. Не «выгрузить копию куда-то» — диалог
	// «куда сохранить» здесь означал бы вторую, побочную копию, а
	// пользователю нужно применить настройки.
	saveBtn := widget.NewButton(locale.T("Save"), func() {
		presenter.SaveConfig()
	})
	saveBtn.Importance = widget.HighImportance
	saveBtn.Hide()

	showBtn := widget.NewButton(locale.T("Show config"), nil)
	showBtn.Hide()

	// Текст собранного конфига держим с последней сборки: «Показать конфиг»
	// обязан показывать ровно то, по чему составлен отчёт, а не результат
	// второй, отдельной сборки — они могли бы разойтись.
	var (
		builtMu   sync.Mutex
		builtText string
	)

	// Создаётся пустым и дозаполняется полями: onDone спрашивает у него же
	// собственное состояние для гейта Save, то есть замыкание ссылается на
	// объект, внутри которого его и объявляют. Копированием структуры это не
	// решить — в ней мьютекс.
	runner := &finalBuildRunner{presenter: presenter}
	runner.onProgress = func(disabled int) {
		progressLabel.SetText(locale.Tf("Checking servers… (%d disabled)", disabled))
		progressLabel.Show()
	}
	runner.onStart = func() {
		progress.Show()
		progressLabel.Show()
		statusLabel.Hide()
		reportBox.Objects = nil
		reportBox.Refresh()
		copyBtn.Hide()
		saveBtn.Hide()
		showBtn.Hide()
	}
	runner.onDone = func(text string, err error) {
		progress.Hide()
		progressLabel.Hide()

		builtMu.Lock()
		builtText = text
		builtMu.Unlock()

		if err != nil {
			// Ошибка вместо отчёта, Save не появляется: сохранять то, что
			// не собралось, нельзя, а подсовывать вместо причины пустой
			// «предупреждений нет» — прямая ложь.
			debuglog.ErrorLog("final: config build: %v", err)
			text, _ := finalBuildStatusText(err, 0)
			setStatus(text, true)
			// Список остаётся пустым: причина уже стоит статусом, и
			// дублировать её строкой внутри отчёта значило бы показать один
			// факт дважды (фикс 4).
			reportBox.Objects = nil
			reportBox.Refresh()
			updateGlobalSaveGate(guiState, false)
			return
		}

		entries, _, gen := config.BuildReport()
		if gen != runner.State().gen {
			// Между Finish и отрисовкой попытку перехватил другой писатель:
			// его записи — не тот отчёт, под который открывался бы Save.
			// Гейт и так закрыт (saveButtonVisible сверяет gen), но рисовать
			// чужие записи как «наш итог» — та же ложь в мягкой форме.
			statusLabel.Hide()
			reportBox.Objects = nil
			reportBox.Refresh()
			updateGlobalSaveGate(guiState, false)
			return
		}
		lines := finalReportLines(entries)
		statusText, failed := finalBuildStatusText(nil, len(lines))
		setStatus(statusText, failed)
		reportBox.Objects = finalReportWidgets(presenter, guiState, lines)
		reportBox.Refresh()

		// Кнопка зовётся «Copy config» и копирует именно конфиг: отчёт виден
		// на экране целиком, а собранный config.json — тысячи строк, которые
		// иначе достаются только выделением в отдельном окне.
		copyBtn.OnTapped = func() {
			builtMu.Lock()
			cfg := builtText
			builtMu.Unlock()
			fynewidget.SetClipboard(cfg)
		}
		copyBtn.Show()

		showBtn.OnTapped = func() {
			builtMu.Lock()
			text := builtText
			builtMu.Unlock()
			showConfigWindow(text)
		}
		showBtn.Show()

		// Гейт Save — один и тот же предикат для кнопки на вкладке и для
		// глобальной внизу окна: две независимые проверки разошлись бы на
		// первой же правке. Предикат спрашивает про попытку, чей отчёт сейчас
		// на экране, а не про реестр вообще: пока сборка шла, его мог
		// перехватить другой писатель.
		visible := saveButtonVisible(runner.State(), config.BuildReportReadyFor)
		if visible {
			saveBtn.Show()
		}
		updateGlobalSaveGate(guiState, visible)
	}
	guiState.RunFinalBuild = runner.Request
	// SPEC 115 §1: гейт Save — ФУНКЦИЯ состояния, а не разовое решение onDone.
	// Кнопку внизу окна показывают и другие пути (разбор источников зовёт
	// UpdateSaveButtonText("Save") на каждом успешном проходе), и без общего
	// предиката они открывали её поверх закрытого гейта. Предикат тот же, что
	// у кнопки на самой вкладке: одна проверка на оба места.
	guiState.SaveGateAllows = func() bool {
		return saveButtonVisible(runner.State(), config.BuildReportReadyFor)
	}

	buttons := container.NewHBox(
		layout.NewSpacer(),
		saveBtn,
		showBtn,
		copyBtn,
		layout.NewSpacer(),
	)

	// Border, а не VBox: центр — панель отчёта — получает всю высоту,
	// которую не заняли шапка и низ. В VBox каждый элемент стоит своим
	// min-size, и панель отчёта оставалась четырьмя строками при любом
	// размере окна.
	//
	// Снаружи — VScroll, и это не возврат к прежнему: Scroll отдаёт
	// содержимому max(его минимум, размер окна), то есть на высоком окне
	// Border растягивает панель отчёта, а на низком страница скроллится
	// целиком. Без внешнего скролла минимум вкладки — шапка плюс низ плюс
	// панель — стал бы минимумом всего Мастера: AppTabs считает его по всем
	// вкладкам, и окно переставало сжиматься на невысоком экране.
	top := container.NewVBox(
		hint,
		container.NewVBox(progressLabel, progress),
		statusLabel,
	)
	bottom := container.NewVBox(
		buttons,
		backupSection(presenter, win),
	)
	return container.NewVScroll(container.NewBorder(top, bottom, nil, nil, reportPanel))
}

// reportPanelFrame — панель отчёта с подложкой: фон поля ввода, рамка цвета
// разделителя, отступ внутри.
//
// Цвета берутся у текущей темы при сборке вкладки; смена темы на лету
// пересоздаёт Мастера целиком, так что следить за ней здесь не нужно.
func reportPanelFrame(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.InputRadiusSize()
	return container.NewStack(bg, container.NewPadded(content))
}

// finalReportWidgets — строки отчёта виджетами.
//
// Справа у каждой строки два значка, те же, что у строк узлов и команд в
// остальном приложении: ⓘ открывает карточку подробностей (что случилось,
// почему, что делать — по коду реестра, плюс переход к источнику), копия
// кладёт строку в буфер. Текстовая кнопка «Show source» прямо в строке
// ушла в карточку: на узком окне она отъедала у длинной причины треть
// ширины, и строка читалась в четыре переноса.
func finalReportWidgets(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, lines []finalReportLine) []fyne.CanvasObject {
	if len(lines) == 0 {
		clean := widget.NewLabel(locale.T(finalCleanText))
		clean.Wrapping = fyne.TextWrapWord
		return []fyne.CanvasObject{clean}
	}
	out := make([]fyne.CanvasObject, 0, len(lines)*2)
	for i, l := range lines {
		line := l
		// Без маркера «•»: субъект строки бывает одним длинным словом
		// (URL подписки без имени), перенос по словам уносит его на вторую
		// строку, и маркер оставался на первой один. Строки и так разделены
		// линиями.
		text := widget.NewLabel(line.Text)
		text.Wrapping = fyne.TextWrapWord

		info := widget.NewButtonWithIcon("", theme.InfoIcon(), func() {
			showReportDetails(presenter, guiState, line)
		})
		info.Importance = widget.LowImportance
		copyBtn := fynewidget.NewCopyButton(locale.T("Copy"), func() (string, bool) {
			return line.Text, true
		})
		copyBtn.Importance = widget.LowImportance
		// Правый кластер центрируется по высоте перенесённой строки — иначе
		// значки прижимаются к её верхнему краю.
		right := container.NewCenter(container.NewHBox(info, copyBtn))
		out = append(out, container.NewBorder(nil, nil, nil, right, text))
		if i < len(lines)-1 {
			out = append(out, widget.NewSeparator())
		}
	}
	return out
}

// showReportDetails — карточка подробностей строки отчёта.
//
// Содержимое — та же карточка, что у уведомления узла (nodewarn.Detail):
// один код реестра — одна карточка, где бы он ни встретился. Внизу —
// копирование карточки целиком и переход к источнику, если он у записи есть.
func showReportDetails(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, line finalReportLine) {
	if guiState == nil || guiState.Window == nil {
		return
	}
	t := finalReportDetail(line)
	body := container.NewVScroll(nodewarn.Detail(t))
	body.SetMinSize(fyne.NewSize(560, 260))

	var d dialog.Dialog
	actions := []fyne.CanvasObject{}
	if line.SourceID != "" {
		sourceID := line.SourceID
		jump := widget.NewButton(locale.T("Show source"), func() {
			if d != nil {
				d.Hide()
			}
			revealSourceInWizard(presenter, guiState, sourceID)
		})
		actions = append(actions, jump)
	}
	copyBtn := widget.NewButtonWithIcon(locale.T("Copy"), theme.ContentCopyIcon(), func() {
		fynewidget.SetClipboard(nodewarn.PlainText(t))
	})
	actions = append(actions, copyBtn)

	d = dialogs.NewCustom(t.Title, body, container.NewHBox(actions...), locale.T("Close"), guiState.Window)
	d.Show()
}

// revealSourceInWizard переключает Мастера на вкладку Sources и подсвечивает
// в ней строку источника.
//
// Две половины намеренно разделены: переключение вкладки знает только этот
// файл (Tabs — состояние окна), подсветка и прокрутка — только список
// источников (guiState.RevealSource). Смешав их, вкладка «Итог» стала бы
// зависеть от устройства чужого списка.
func revealSourceInWizard(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, sourceID string) {
	if guiState == nil || presenter == nil {
		return
	}
	// Логическая половина перехода: есть ли ещё такой источник. Источник
	// могли удалить между сборкой и кликом по строке отчёта — тогда
	// переключать вкладку не за чем, и молчаливый прыжок «в никуда» был бы
	// хуже бездействия.
	if sourceIndexByID(modelSourceIDs(presenter), sourceID) < 0 {
		debuglog.DebugLog("final: source %q from the report no longer exists — no jump", sourceID)
		return
	}
	if guiState.Tabs != nil {
		for _, item := range guiState.Tabs.Items {
			if item.Text == locale.T("Sources") {
				guiState.Tabs.Select(item)
				break
			}
		}
	}
	if guiState.RevealSource != nil {
		guiState.RevealSource(sourceID)
	}
}

// modelSourceIDs — ULID'ы источников в порядке модели.
//
// Отдельной функцией, чтобы чистая часть перехода (sourceIndexByID) не знала
// ни про презентер, ни про модель и оставалась проверяемой без запуска UI.
func modelSourceIDs(presenter *wizardpresentation.WizardPresenter) []string {
	m := presenter.Model()
	if m == nil {
		return nil
	}
	ids := make([]string, 0, len(m.Sources))
	for _, src := range m.Sources {
		ids = append(ids, src.ID)
	}
	return ids
}

// updateGlobalSaveGate прячет/показывает Save внизу окна.
//
// Глобальная кнопка живёт на последней вкладке — то есть на «Итоге», — и
// оставить её открытой, спрятав кнопку на самой вкладке, значило бы обойти
// гейт одним движением мыши ниже.
// Enable/Disable идут парой с Show/Hide: закрытый гейт кнопку ещё и гасит
// (UpdateSaveButtonText в закрытом состоянии делает то же), и открыть её одним
// Show значило бы показать неактивную кнопку.
func updateGlobalSaveGate(guiState *wizardpresentation.GUIState, visible bool) {
	if guiState == nil || guiState.SaveButton == nil {
		return
	}
	if visible {
		guiState.SaveButton.Show()
		guiState.SaveButton.Enable()
		return
	}
	guiState.SaveButton.Hide()
	guiState.SaveButton.Disable()
}

// finalBuildRunner сериализует сборки вкладки «Итог».
//
// Схлопывание повторных входов, а не «ускорение»: пользователь щёлкает по
// вкладкам туда-обратно, и без него на большой подписке шли бы две-три
// параллельные сборки, чьи отчёты применились бы в произвольном порядке —
// последним оказался бы не последний. Схема та же, что у selectorReloader и
// ping-all: пока сборка идёт, повторный вход только помечает «нужен ещё один
// прогон».
//
// Сборка идёт в горутине (диск, разбор подписок, весь конвейер); мутации
// виджетов — только через fyne.Do.
type finalBuildRunner struct {
	presenter *wizardpresentation.WizardPresenter

	mu      sync.Mutex
	running bool
	pending bool
	state   finalBuildState

	// onStart / onDone мутируют виджеты — зовутся ТОЛЬКО из fyne.Do.
	onStart    func()
	onDone     func(text string, err error)
	onProgress func(disabled int)
}

// State — текущее состояние сборки; читается гейтом Save.
func (r *finalBuildRunner) State() finalBuildState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Request просит пересобрать. Возврат мгновенный: работа уходит в горутину, и
// вход на вкладку не ждёт ни диска, ни разбора подписок.
func (r *finalBuildRunner) Request() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.running {
		r.pending = true
		r.mu.Unlock()
		return
	}
	r.running = true
	r.state = finalBuildState{running: true}
	r.mu.Unlock()

	if r.onStart != nil {
		r.onStart()
	}
	go r.loop()
}

func (r *finalBuildRunner) loop() {
	for {
		// MergeGUIToModel читает виджеты — ему на UI-поток, синхронно: сборка
		// обязана считать ровно то, что сейчас в форме, а не то, что было до
		// последнего нажатия клавиши.
		done := make(chan struct{})
		fyne.Do(func() {
			r.presenter.MergeGUIToModel()
			close(done)
		})
		<-done

		text, gen, err := r.build()

		r.mu.Lock()
		r.state = finalBuildState{done: err == nil, err: err, gen: gen}
		again := r.pending
		r.pending = false
		if !again {
			r.running = false
		}
		r.mu.Unlock()

		if !again {
			if r.onDone != nil {
				fyne.Do(func() { r.onDone(text, err) })
			}
			return
		}
	}
}

// build — одна сборка «Итога»: сначала парсерная стадия, потом всё остальное.
//
// Порядок обязателен, и обе половины идут В ЭТОЙ горутине. Сборка читает
// model.GeneratedOutbounds, а наполняет их ровно один код — ParseAndPreview.
// Без первой половины «Итог» вёл себя так:
//
//   - вход в Мастера сразу на «Итог» (кэш пуст): конфиг собирался УСПЕШНО, но
//     без единой прокси-ноды, и Save открывалась по этому вранью;
//   - вход мимо вкладки Направлений после правки: попытка отчёта сброшена, и
//     парсерные виды записей (source_excluded, chain_failed, naive_degraded) в
//     отчёт не попадали — половина причин объявлялась полным отчётом;
//   - клик на «Итог» во время фонового разбора: две горутины писали одни поля
//     модели, а StartBuildReport разбора выпотрашивал отчёт посреди сборки.
//
// PrepareFinalBuild закрывает все три одним механизмом — тем же, которым это
// делает Save (presenter_save): дождаться идущего разбора, при нужде прогнать
// свой, и убедиться, что попытка отчёта жива. Прелоадер честно висит всё это
// время: разбор подписок — часть сборки, а не подготовка к ней.
func (r *finalBuildRunner) build() (string, config.BuildGeneration, error) {
	if !r.presenter.PrepareFinalBuild() {
		return "", 0, errors.New(locale.T(finalNoNodesText))
	}
	text, gen, err := wizardbusiness.BuildFinalReportConfig(r.presenter.Model())
	if err != nil {
		return "", gen, err
	}
	progress := func(n int) {
		if r.onProgress == nil {
			return
		}
		fyne.Do(func() { r.onProgress(n) })
	}
	checked, _, loopErr := r.presenter.RunDraftRejectPreview(text, progress)
	if loopErr != nil {
		return text, gen, loopErr
	}
	if checked != "" {
		text = checked
	}
	if m := r.presenter.Model(); m != nil && m.BuildReportGen != 0 {
		gen = m.BuildReportGen
	}
	return text, gen, nil
}

// showConfigWindow открывает собранный конфиг в собственном окне.
//
// Application.NewWindow, а не модальный диалог: диалог живёт внутри канвы
// родителя и не может быть выше её, а конфиг — это тысячи строк, которые
// читают, прокручивают и копируют. Своё окно пользователь разворачивает и
// двигает, оставляя визард открытым рядом.
func showConfigWindow(text string) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	w := app.NewWindow(locale.T("Generated config.json"))

	// Только для чтения: править собранный конфиг здесь бессмысленно — он
	// пересобирается из состояния при каждой сборке. Но выделение и
	// копирование остаются, ради них просмотрщик и взят вместо Label.
	entry := fynewidget.NewJSONView(text)

	closeBtn := widget.NewButton(locale.T("Cancel"), func() { w.Close() })
	w.SetContent(container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), closeBtn),
		nil, nil,
		container.NewVScroll(entry.Object()),
	))
	w.Resize(fyne.NewSize(820, 640))
	fynewidget.CenterOnScreen(w)
	w.Show()
}
