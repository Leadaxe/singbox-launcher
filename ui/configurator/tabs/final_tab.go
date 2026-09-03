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
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
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
	reportScroll.SetMinSize(fyne.NewSize(0, 160))

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

	body := container.NewVBox(
		hint,
		container.NewVBox(progressLabel, progress),
		statusLabel,
		reportScroll,
		buttons,
		backupSection(presenter, win),
	)
	return container.NewVScroll(body)
}

// finalReportWidgets — строки отчёта виджетами.
//
// У записи с source_id — кнопка перехода: отчёт называет источник, а чинят его
// на вкладке Sources, и заставлять пользователя искать там строку глазами
// значило бы оборвать маршрут на полпути.
func finalReportWidgets(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, lines []finalReportLine) []fyne.CanvasObject {
	if len(lines) == 0 {
		clean := widget.NewLabel(locale.T(finalCleanText))
		clean.Wrapping = fyne.TextWrapWord
		return []fyne.CanvasObject{clean}
	}
	out := make([]fyne.CanvasObject, 0, len(lines))
	for _, l := range lines {
		text := widget.NewLabel("• " + l.Text)
		text.Wrapping = fyne.TextWrapWord
		if l.SourceID == "" {
			out = append(out, text)
			continue
		}
		sourceID := l.SourceID
		jump := widget.NewButton(locale.T("Show source"), func() {
			revealSourceInWizard(presenter, guiState, sourceID)
		})
		jump.Importance = widget.LowImportance
		out = append(out, container.NewBorder(nil, nil, nil, jump, text))
	}
	return out
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
	onStart func()
	onDone  func(text string, err error)
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
	return wizardbusiness.BuildFinalReportConfig(r.presenter.Model())
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

	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	entry.SetText(text)
	// Только для чтения: править собранный конфиг здесь бессмысленно — он
	// пересобирается из состояния при каждой сборке. Но выделение и
	// копирование остаются, ради них поле и взято вместо Label.
	// Ввод ОТКАТЫВАЕТСЯ, а не игнорируется молча: пустой хэндлер оставлял
	// напечатанное на экране, и «правка» выглядела принятой, хотя никуда
	// не шла (паттерн jsonEntry в source_edit_window.go).
	entry.OnChanged = func(s string) {
		if s != text {
			entry.SetText(text)
		}
	}

	closeBtn := widget.NewButton(locale.T("Cancel"), func() { w.Close() })
	w.SetContent(container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), closeBtn),
		nil, nil,
		container.NewVScroll(entry),
	))
	w.Resize(fyne.NewSize(820, 640))
	w.CenterOnScreen()
	w.Show()
}
