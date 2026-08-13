package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/components"
	"singbox-launcher/ui/configurator"
)

// Правая колонка вкладки Remote — список удалённых машин (SPEC 098 §2.1).
//
// Заменяет собой три разбросанных места: дропдаун выбора машины в шапке
// Servers, блок питания там же и область Remote в окне подключения. Смысл
// сведения — не эстетика: пока выбор машины и таргет конфига были разными
// переключателями, можно было собрать конфиг для роутера и задеплоить его на
// VPS. Здесь у каждой строки свои Configure и Deploy, и промахнуться нечем.

// Повторы Connect: короткий провал L2/ARP (роутер перезагружается, точка
// роняет клиента) даёт мгновенный «no route to host» на connect(), хотя через
// пару секунд адрес снова доступен. Пять попыток с паузой в 3 секунды
// перекрывают такой провал; дальше это уже настоящая недоступность, и
// молотить сеть смысла нет.
const (
	connectAttempts   = 5
	connectRetryDelay = 3 * time.Second
)

// machineListPanel — состояние правой колонки.
type machineListPanel struct {
	ac        *core.AppController
	registry  *services.RemoteRegistry
	list      *fyne.Container
	container *fyne.Container
	// proxies — панель списка узлов слева. Панель машин ею управляет: до
	// Connect она скрыта, после — показывается и перечитывается.
	proxies *ProxyListPanel
	// health — последний известный статус каждой машины, чтобы перерисовка
	// строки не ходила в сеть заново.
	health map[string]services.RemoteHealth
	// errLog — история сетевых отказов по машинам за текущую сессию.
	//
	// Строка показывает только последнюю ошибку, а разбирать «почему не
	// соединяется» приходится по картине: сбоит ли постоянно или гасится
	// повтором, менялся ли текст. Держать это в памяти процесса дешевле, чем
	// просить пользователя лезть в лог-файл, и честнее, чем показывать одну
	// последнюю строку как всю правду.
	errLog map[string][]connectFailure
	errMu  sync.Mutex
	// connectAttempt — номер текущей попытки соединения по машинам (0 = не
	// соединяемся).
	//
	// Нужно для индикации: с повторами Connect занимает до
	// connectAttempts*connectRetryDelay секунд, и без признака работы это
	// выглядит как зависшая кнопка.
	connectAttempt map[string]int
	// moreOpen — раскрыта ли панель дополнительных инструментов у машины.
	moreOpen map[string]bool
}

// connectFailure — одна неудачная попытка соединения.
type connectFailure struct {
	When    time.Time
	Attempt int
	Err     string
}

// maxConnectFailures — сколько отказов помним на машину.
//
// Ограничение нужно: при недоступной машине пользователь может жать Connect
// десятки раз, и без предела список рос бы всю сессию. Держим последние —
// именно они объясняют текущее состояние.
const maxConnectFailures = 50

// revealDuration — длительность выезда блока «Ещё». Короткая: это подсказка
// «раскрылось вот это», а не эффект ради эффекта.
const revealDuration = 180 * time.Millisecond

// CreateMachineListPanel строит правую колонку вкладки Remote.
func CreateMachineListPanel(ac *core.AppController, proxies *ProxyListPanel) fyne.CanvasObject {
	p := &machineListPanel{
		ac:             ac,
		registry:       services.NewRemoteRegistry(ac.FileService.ExecDir),
		proxies:        proxies,
		health:         make(map[string]services.RemoteHealth),
		errLog:         make(map[string][]connectFailure),
		connectAttempt: make(map[string]int),
		moreOpen:       make(map[string]bool),
	}
	p.list = container.NewVBox()

	addBtn := widget.NewButton(locale.T("remote.machines.add"), func() {
		OpenAddMachineWindow(ac, func() {
			// Только что добавленная машина ещё не подключена — узлов у неё
			// для нас нет, и Refresh ушёл бы с пустой группой.
			p.Reload()
		})
	})
	addBtn.Importance = widget.MediumImportance

	header := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(locale.T("remote.machines.title"), fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true}),
		addBtn,
	)

	scroll := container.NewVScroll(p.list)
	p.container = container.NewBorder(container.NewVBox(header, widget.NewSeparator()), nil, nil, nil, scroll)
	p.Reload()

	// Активная машина меняется и снаружи панели: переход на вкладку Local
	// снимает транспорт (SPEC 098 — Local всегда про своё ядро). Без этой
	// подписки маркер ● оставался бы висеть на машине, с которой разговор
	// уже не идёт.
	//
	// health-кеш здесь НЕ сбрасывается: соединение живёт, пока его не снял
	// явный Disconnect, и переключение вкладок его не рвёт. Сброс по этому
	// сигналу возвращал строку к «Connect» после каждого захода на Local —
	// связь была жива, а кнопка предлагала подключиться заново.
	//
	// Свой кеш чистит сам disconnectMachine, там же, где рвёт канал.
	OnOverrideChanged(func() {
		fyne.Do(p.redrawRows)
	})
	return p.container
}

// Reload перечитывает реестр и перерисовывает строки.
func (p *machineListPanel) Reload() {
	list, err := p.registry.List()
	if err != nil {
		debuglog.WarnLog("machine list: read registry: %v", err)
	}
	p.list.RemoveAll()
	if len(list) == 0 {
		// Пустой список — не ошибка: пользователь ещё не сопрягался ни с чем.
		// Говорим, что делать, вместо пустого места.
		hint := widget.NewLabel(locale.T("remote.machines.empty"))
		hint.Wrapping = fyne.TextWrapWord
		p.list.Add(hint)
		p.list.Refresh()
		return
	}
	activeID, _, _ := GetLxdRemoteOverride()
	for _, d := range list {
		p.list.Add(p.buildRow(d, d.ID == activeID))
	}
	p.list.Refresh()
	// Никаких сетевых опросов здесь: показ списка — не повод стучаться к
	// чужим хостам. Состояние машины появляется только после явного Connect.
}

// buildRow — одна строка машины: имя, платформа, адрес, статус и кнопки.
func (p *machineListPanel) buildRow(d services.RemoteDaemon, active bool) fyne.CanvasObject {
	// Соединения с машиной ещё не было — про её ядро мы не знаем НИЧЕГО, и
	// узнать можем только сходив по сети, чего без спроса делать нельзя.
	health, connected := p.health[d.ID]

	// Маркер состояния канала: зелёный — соединены, красный — соединялись, но
	// машина не отвечает, серый — не подключались.
	//
	// Цветом рисуется круг, а не текст: widget.Label в Fyne красить нечем, а
	// цвет здесь несёт смысл (это первое, что видно в списке из нескольких
	// машин). Форма при этом одна — цвет не единственный различитель: рядом
	// стоит либо Connect, либо Disconnect со статусом.
	markerColor := theme.Color(theme.ColorNameDisabled)
	switch {
	case connected && health.Err != "":
		markerColor = theme.Color(theme.ColorNameError)
	case connected:
		markerColor = theme.Color(theme.ColorNameSuccess)
	}
	// Круг занимает ВСЮ выданную площадь (его MinSize = 1×1, а Layout
	// растягивает), поэтому и GridWrap, и Center раздували точку до размера
	// ячейки. Фиксируем размер собственной раскладкой и центрируем по
	// вертикали, чтобы маркер стоял вровень с именем.
	markerBox := container.New(&dotLayout{size: 10}, canvas.NewCircle(markerColor))

	name := widget.NewLabelWithStyle(d.Name, fyne.TextAlignLeading,
		fyne.TextStyle{Bold: active})

	tgt := d.Target()
	meta := widget.NewLabel(fmt.Sprintf("%s/%s   %s", tgt.GOOS, tgt.GOARCH, d.Addr))

	// Connect/Disconnect — напротив АДРЕСА: кнопка про канал именно к нему,
	// а не про ядро на той стороне (тем занимается Start/Stop у статуса).
	var connBtn *widget.Button
	if connected {
		connBtn = widget.NewButton(locale.T("remote.machines.disconnect"), func() {
			p.disconnectMachine()
		})
	} else if attempt := p.connectAttempt[d.ID]; attempt > 0 {
		// Идёт попытка: кнопка неактивна и говорит, что происходит. Иначе
		// пользователь жмёт её повторно и запускает второй цикл повторов
		// поверх первого.
		// Номер попытки прямо в кнопке: с повторами ожидание доходит до
		// 15 секунд, и «идёт вторая из пяти» отвечает на вопрос «оно ещё
		// живо или зависло» лучше, чем неподвижная надпись.
		connBtn = widget.NewButton(
			locale.Tf("remote.machines.connecting_try", attempt, connectAttempts), nil)
		connBtn.Disable()
	} else {
		connBtn = widget.NewButton(locale.T("remote.machines.connect"), func() {
			p.connectMachine(d)
		})
		connBtn.Importance = widget.HighImportance
	}
	metaRow := container.NewBorder(nil, nil, nil, connBtn, meta)

	editBtn := ttwidget.NewButton("✎", func() {
		p.editMachine(d)
	})
	editBtn.SetToolTip(locale.T("remote.machines.edit_tooltip"))
	editBtn.Importance = widget.LowImportance

	removeBtn := ttwidget.NewButton("✕", func() {
		p.removeMachine(d)
	})
	removeBtn.SetToolTip(locale.T("remote.machines.remove_tooltip"))
	removeBtn.Importance = widget.LowImportance

	// Правка и удаление — напротив ИМЕНИ: это операции над самой записью,
	// а не над её ядром, поэтому доступны и без соединения.
	nameRow := container.NewBorder(nil, nil, markerBox,
		container.NewHBox(editBtn, removeBtn), name)

	rows := []fyne.CanvasObject{nameRow, metaRow}

	if !connected {
		// До Connect строка показывает только паспорт машины. Ни статуса, ни
		// Start/Stop, ни Deploy, ни Configure: всё это требует ответа от
		// демона, а выдумывать его состояние — врать пользователю.
		//
		// Configure здесь именно поэтому: конфиг машины ссылается на её
		// ресурс-стор `<state_dir>/resources/…`, а state_dir приезжает из
		// /admin/info при соединении. Дать настраивать без него значило бы
		// собрать конфиг с путём, которого на той стороне нет, — и узнать об
		// этом только когда ядро не поднимется после Deploy.
		rows = append(rows, widget.NewSeparator())
		return container.NewVBox(rows...)
	}

	// Соединены: state_dir известен, конфиг соберётся с верными путями.
	configureBtn := widget.NewButton(locale.T("remote.machines.configure"), func() {
		configurator.ShowConfigWizardForMachine(p.ac.UIService.MainWindow, d)
	})

	// Соединились: показываем настоящий статус ядра и открываем управление.
	//
	// Статусы демона: idle | started | fatal (RemoteHealth.CoreStatus).
	statusText := locale.T("remote.machines.reachable")
	switch {
	case health.Err != "":
		statusText = locale.Tf("remote.machines.unreachable", health.Err)
	case health.CoreStatus != "":
		statusText = health.CoreStatus
	}
	// Версия ядра идёт вместе со статусом: без неё непонятно, поддерживает ли
	// та сторона то, что мы деплоим, — а это первый вопрос при разборе сбоя.
	if health.Version != "" {
		statusText = health.Version + " · " + statusText
	}
	status := widget.NewLabel(statusText)
	status.Wrapping = fyne.TextWrapWord

	running := health.CoreStatus == "started"
	powerLabel := locale.T("servers.power.start")
	if running {
		powerLabel = locale.T("servers.power.stop")
	}
	powerBtn := widget.NewButton(powerLabel, func() {
		p.togglePower(d, running)
	})
	if health.Err != "" {
		// Машина не отвечает — управлять её ядром нечем.
		powerBtn.Disable()
	}

	// (i) — всё, что демон сообщил о себе: хеши конфигов, последняя ошибка,
	// state-dir. В строку это не влезает, а при разборе «почему на машине не
	// то» нужно целиком.
	infoBtn := ttwidget.NewButton("ⓘ", func() {
		p.showHealthDetails(d, health)
	})
	infoBtn.SetToolTip(locale.T("remote.machines.info_tooltip"))
	infoBtn.Importance = widget.LowImportance

	deployBtn := widget.NewButton(locale.T("servers.power.deploy"), func() {
		p.deployTo(d)
	})
	if health.Err != "" {
		deployBtn.Disable()
	}

	// RES — управление файлами, на которые ссылается конфиг машины
	// (SPEC 063). Рядом с Deploy, потому что это его обратная сторона:
	// Deploy заливает недостающее молча, а здесь видно, что реально лежит на
	// той стороне и совпадает ли оно с нашим.
	resBtn := widget.NewButton(locale.T("remote.res.button"), func() {
		OpenMachineResourcesWindow(p.ac, d)
	})
	if health.Err != "" {
		resBtn.Disable()
	}

	// Start/Stop — напротив СТАТУСА, который он меняет: кнопка стоит там,
	// где виден её результат.
	statusRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(infoBtn, powerBtn), status)
	// Дополнительные инструменты — под строкой, раскрытием вниз (тот же
	// Accordion, что «Advanced» в окне добавления машины). Не popup-меню:
	// содержимое остаётся на месте, не перекрывает соседние машины и не
	// исчезает при промахе мышью.
	profilerBtn := widget.NewButton(locale.T("remote.more.profiler"), func() {
		OpenMachineProfiler(p.ac, d)
	})
	// Телеметрия ХОСТА — рядом с профайлером, потому что это соседний вопрос
	// с другим ответом: профайлер показывает трафик ядра, а «почему роутер
	// тормозит» решается только описанием машины (CPU, память, диски, темп.).
	hostBtn := widget.NewButton(locale.T("remote.more.host"), func() {
		OpenMachineHostWindow(p.ac, d)
	})
	// Своя кнопка вместо Accordion: тот в горизонтальном ряду растягивает
	// свою шторку на всю ширину и уводит содержимое вбок — заголовок и
	// раскрытый блок оказываются в одной строке. Здесь стрелка остаётся
	// кнопкой в ряду, а блок живёт отдельной строкой под ним.
	//
	// Состояние раскрытия помним на машину: строка перерисовывается на каждом
	// health-ответе, и без этого панель схлопывалась бы под курсором.
	open := p.moreOpen[d.ID]
	arrow := "▾"
	if open {
		arrow = "▴"
	}
	moreBtn := widget.NewButton(arrow, func() {
		p.moreOpen[d.ID] = !p.moreOpen[d.ID]
		p.redrawRows()
	})
	moreBtn.Importance = widget.LowImportance

	// Стрелка прижата к правому краю: она про раскрытие всей строки, а не
	// продолжение ряда действий — встык к RES читалась бы как четвёртая
	// кнопка того же ряда.
	rows = append(rows,
		statusRow,
		container.NewBorder(nil, nil, nil, moreBtn,
			container.NewHBox(configureBtn, deployBtn, resBtn)),
	)
	if open {
		// Раскрытие с анимацией высоты: мгновенный скачок содержимого не
		// показывает, ЧТО раскрылось, — глаз теряет связь между стрелкой и
		// появившимся блоком.
		rows = append(rows, newRevealBox(container.NewHBox(profilerBtn, hostBtn)))
	}
	rows = append(rows, widget.NewSeparator())
	return container.NewVBox(rows...)
}

// selectMachine делает машину активной: список прокси слева начинает
// показывать её узлы.
//
// Выбор эфемерный (SPEC 097 §4.3) — не переживает перезапуск, чтобы лаунчер
// всегда стартовал со своим ядром.
func (p *machineListPanel) connectMachine(d services.RemoteDaemon) {
	if err := SetLxdRemoteOverride(p.ac, d.ID); err != nil {
		debuglog.WarnLog("machine list: connect %q: %v", d.ID, err)
		dialog.ShowError(err, p.ac.UIService.MainWindow)
		return
	}
	// Спрашиваем состояние ядра — сетевой поход по явной команде
	// пользователя. Блокирующий вызов, поэтому в горутине: недоступная машина
	// отвечает по таймауту REST-клиента.
	//
	// Повторяем до connectAttempts раз с паузой: наблюдался отказ, при котором
	// connect() возвращал «no route to host» мгновенно, хотя curl к тому же
	// адресу в ту же секунду отвечал 200 — короткий провал L2/ARP (роутер
	// перезагружался). Одна попытка делала это отказом на ровном месте,
	// а повтор через несколько секунд проходит.
	p.connectAttempt[d.ID] = 1
	p.redrawRows()
	go func() {
		var h services.RemoteHealth
		for attempt := 1; attempt <= connectAttempts; attempt++ {
			if attempt > 1 {
				fyne.Do(func() {
					p.connectAttempt[d.ID] = attempt
					p.redrawRows()
				})
			}
			h = p.registry.Health(d.ID)
			if h.Err == "" {
				break
			}
			debuglog.WarnLog("machine list: %q unreachable (attempt %d/%d): %s",
				d.ID, attempt, connectAttempts, h.Err)
			p.recordFailure(d.ID, attempt, h.Err)
			if attempt < connectAttempts {
				time.Sleep(connectRetryDelay)
			}
		}
		fyne.Do(func() {
			p.connectAttempt[d.ID] = 0
			p.health[d.ID] = h
			p.redrawRows()

			if h.Err != "" {
				// Без попапа: маркер строки краснеет, а полный текст последней
				// ошибки лежит в ⓘ. Модальное окно на каждую неудачу мешает
				// повторить попытку и ничего не добавляет к тому, что уже
				// видно в строке.
				debuglog.WarnLog("machine list: %q unreachable after %d attempts: %s",
					d.ID, connectAttempts, h.Err)
				return
			}
			// Узлы читаем ТОЛЬКО если ядро на машине запущено. При idle их
			// физически нет: демон ответит «service is not started», и
			// пользователь получил бы ошибку там, где достаточно нажать Start.
			if h.CoreStatus != "started" {
				debuglog.InfoLog("machine list: %q connected, core is %q — press Start to load nodes",
					d.ID, h.CoreStatus)
				return
			}
			p.proxies.SetEnabled(true)
			p.loadNodes()
		})
	}()
}

// showHealthDetails показывает всё, что демон сообщил о себе.
//
// Отдельное окно, а не диалог: строк много и они длинные (хеши по 64 символа,
// путь к state-dir), а высокий модальный попап Fyne раздувает на весь экран.
//
// Значения показываются как есть, без «причёсывания»: это диагностика, и
// подмена пустого поля прочерком или домысленным текстом здесь стоила бы
// дороже, чем пустая строка.
func (p *machineListPanel) showHealthDetails(d services.RemoteDaemon, h services.RemoteHealth) {
	rows := [][2]string{
		{locale.T("remote.info.machine"), d.Name},
		{locale.T("remote.info.addr"), d.Addr},
		{locale.T("remote.info.platform"), fmt.Sprintf("%s/%s", d.Target().GOOS, d.Target().GOARCH)},
		{locale.T("remote.info.daemon_version"), h.Version},
		{locale.T("remote.info.core_status"), h.CoreStatus},
		{locale.T("remote.info.state_dir"), h.StateDir},
		{locale.T("remote.info.active_sha"), h.ActiveSHA},
		{locale.T("remote.info.last_good_sha"), h.LastGoodSHA},
	}
	if h.InterruptedApply {
		rows = append(rows, [2]string{locale.T("remote.info.interrupted"), locale.T("remote.info.interrupted_yes")})
	}
	if h.LastError != "" {
		rows = append(rows, [2]string{locale.T("remote.info.last_error"), h.LastError})
	}
	if h.Err != "" {
		rows = append(rows, [2]string{locale.T("remote.info.unreachable"), h.Err})
	}
	// Сводка ядра, если открыт профайлер машины: связи, скорости, память.
	// Стрим статуса живёт вместе с его окном, поэтому до открытия строк нет —
	// и выдумывать нули вместо «не знаем» нельзя.
	if st, ok := MachineStatus(d.ID); ok {
		rows = append(rows,
			[2]string{locale.T("remote.info.connections"),
				locale.Tf("remote.info.connections_value", st.ConnectionsIn, st.ConnectionsOut)},
			[2]string{locale.T("remote.info.traffic"),
				locale.Tf("remote.info.traffic_value",
					sizeOrDash(st.Uplink, true), sizeOrDash(st.Downlink, true),
					sizeOrDash(st.UplinkTotal, true), sizeOrDash(st.DownlinkTotal, true))},
			[2]string{locale.T("remote.info.runtime"),
				locale.Tf("remote.info.runtime_value", sizeOrDash(int64(st.Memory), true), st.Goroutines)},
		)
	}

	// История отказов за сессию: одна последняя ошибка не отвечает на вопрос
	// «это разово или постоянно». Здесь видно, гасится ли сбой повтором
	// (attempt 2/5 и дальше тишина) или машина не отвечает вовсе.
	if fails := p.failures(d.ID); len(fails) > 0 {
		var b strings.Builder
		for i := len(fails) - 1; i >= 0; i-- { // свежие сверху
			f := fails[i]
			fmt.Fprintf(&b, "%s  [%d/%d]  %s\n",
				f.When.Format("15:04:05"), f.Attempt, connectAttempts, f.Err)
		}
		rows = append(rows, [2]string{
			locale.Tf("remote.info.failures", len(fails)),
			strings.TrimRight(b.String(), "\n"),
		})
	}

	// Признак «поле длинное» несёт сама строка таблицы, а не наличие \n в
	// значении: история из ОДНОГО отказа переносов не содержит, но всё равно
	// длиннее строки, и однострочный Entry показывал бы её началом с
	// обрезанием.
	multiline := map[string]bool{locale.Tf("remote.info.failures", len(p.failures(d.ID))): true}

	items := make([]*widget.FormItem, 0, len(rows))
	var plain strings.Builder
	for _, r := range rows {
		// Значение — не Label, а поле только для чтения: хеш и путь нужно
		// уметь выделить и скопировать, иначе диагностику не перенести в тикет.
		// Многострочное значение (история отказов) — в MultiLineEntry:
		// однострочный Entry показал бы только первую строку, а именно
		// последовательность попыток и объясняет картину.
		if multiline[r[0]] || strings.Contains(r[1], "\n") {
			m := widget.NewMultiLineEntry()
			m.SetText(r[1])
			// Перенос по словам: строка отказа длиннее окна, и без него
			// пришлось бы возить поле горизонтально, чтобы прочитать причину.
			m.Wrapping = fyne.TextWrapWord
			m.SetMinRowsVisible(6)
			items = append(items, widget.NewFormItem(r[0], m))
			fmt.Fprintf(&plain, "%s:\n%s\n", r[0], r[1])
			continue
		}
		v := widget.NewEntry()
		v.SetText(r[1])
		v.Wrapping = fyne.TextWrapOff
		items = append(items, widget.NewFormItem(r[0], v))
		fmt.Fprintf(&plain, "%s: %s\n", r[0], r[1])
	}

	win := p.ac.UIService.Application.NewWindow(locale.Tf("remote.info.window_title", d.Name))
	copyBtn := widget.NewButton(locale.T("dialog.copy"), func() {
		setClipboard(plain.String())
	})
	closeBtn := widget.NewButton(locale.T("dialog.close"), func() { win.Close() })
	closeBtn.Importance = widget.HighImportance

	body := container.NewVBox(
		widget.NewForm(items...),
		container.NewBorder(nil, nil, copyBtn, closeBtn),
	)
	// Gutter резервирует место под полосу прокрутки, иначе она ложится на
	// правый край полей — здесь это ровно те поля, из которых копируют хеши.
	win.SetContent(container.NewPadded(components.WrapInScrollWithGutter(body)))
	win.Resize(fyne.NewSize(600, 460))
	win.CenterOnScreen()
	win.Show()
}

// loadNodes перечитывает группы и список узлов активной машины.
//
// Сброс идёт первым: он перечитывает группы У ЭТОЙ машины и чистит прежний
// список. Без него запрос уходил бы с группой предыдущего источника —
// отсюда «Daemon: group "" not found».
func (p *machineListPanel) loadNodes() {
	if p.ac.UIService != nil && p.ac.UIService.ResetAPIStateFunc != nil {
		p.ac.UIService.ResetAPIStateFunc()
	}
	// Порядок обязателен: сначала узнать группы ЭТОЙ машины, потом грузить
	// узлы. Иначе запрос уходит с пустой группой, и ядро отвечает
	// «group "" not found» на штатное подключение.
	p.proxies.ReloadGroups()
	p.proxies.Refresh()
}

// disconnectMachine рвёт связь с машиной: строка сворачивается обратно к
// паспорту с кнопкой Connect, список слева пустеет.
//
// Список именно ОЧИЩАЕТСЯ, а не перезагружается: Refresh пошёл бы за узлами,
// когда транспорта уже нет и группа пуста, — и пользователь получал бы
// «Daemon: group "" not found» в ответ на штатное отключение.
func (p *machineListPanel) disconnectMachine() {
	// Профайлер живёт на стриме этой машины — рвём вместе с каналом, иначе
	// окно продолжало бы показывать поток, которого уже нет.
	if id, _, ok := GetLxdRemoteOverride(); ok {
		CloseMachineProfiler(id)
		// Телеметрия хоста ходит по тому же каналу: без остановки её опрос
		// продолжал бы стучаться к машине, с которой разговор уже окончен.
		CloseMachineHostWindow(id)
	}
	p.proxies.SetEnabled(false)
	ClearLxdRemoteOverride(p.ac)
	p.health = make(map[string]services.RemoteHealth)
	p.Reload()
	p.proxies.Clear()
}

// editMachine — правка имени и адреса. Платформа правится там же: это
// свойство машины, и менять его из визарда нельзя (§2.4).
func (p *machineListPanel) editMachine(d services.RemoteDaemon) {
	nameEntry := widget.NewEntry()
	nameEntry.SetText(d.Name)
	addrEntry := widget.NewEntry()
	addrEntry.SetText(d.Addr)

	tgt := d.Target()
	goosSelect := widget.NewSelect([]string{"linux", "darwin", "windows"}, nil)
	goosSelect.SetSelected(tgt.GOOS)
	goarchSelect := widget.NewSelect([]string{"amd64", "arm64", "arm", "386", "mips", "mipsle"}, nil)
	goarchSelect.SetSelected(tgt.GOARCH)

	form := widget.NewForm(
		widget.NewFormItem(locale.T("remote.machines.field_name"), nameEntry),
		widget.NewFormItem(locale.T("remote.machines.field_addr"), addrEntry),
		widget.NewFormItem(locale.T("remote.machines.field_platform"), goosSelect),
		widget.NewFormItem(locale.T("remote.machines.field_arch"), goarchSelect),
	)

	dlg := dialog.NewCustomConfirm(locale.T("remote.machines.edit_title"),
		locale.T("dialog.button_save"), locale.T("dialog.button_cancel"), form,
		func(ok bool) {
			if !ok {
				return
			}
			if err := p.registry.Update(d.ID, nameEntry.Text, addrEntry.Text); err != nil {
				dialog.ShowError(err, p.ac.UIService.MainWindow)
				return
			}
			if err := p.registry.SetPlatform(d.ID, goosSelect.Selected, goarchSelect.Selected); err != nil {
				dialog.ShowError(err, p.ac.UIService.MainWindow)
				return
			}
			p.Reload()
		}, p.ac.UIService.MainWindow)
	dlg.Resize(fyne.NewSize(460, 300))
	dlg.Show()
}

// removeMachine удаляет машину со всем её имуществом (§3.1.9).
//
// Предупреждение про отзыв доступа обязательно: мы забываем ключ у себя, но
// регистрация на СТОРОНЕ демона остаётся, и снять её может только он сам.
// Промолчать здесь значило бы дать пользователю ложное чувство, что доступ
// отозван.
func (p *machineListPanel) removeMachine(d services.RemoteDaemon) {
	dialog.ShowConfirm(
		locale.T("remote.machines.remove_title"),
		locale.Tf("remote.machines.remove_body", d.Name),
		func(ok bool) {
			if !ok {
				return
			}
			// Машины не станет — её профайлер и телеметрия тоже должны уйти.
			CloseMachineProfiler(d.ID)
			CloseMachineHostWindow(d.ID)
			activeID, _, _ := GetLxdRemoteOverride()
			if activeID == d.ID {
				// Снимаем выбор до удаления: иначе левая колонка осталась бы
				// с транспортом на машину, которой уже нет.
				ClearLxdRemoteOverride(p.ac)
			}
			if err := p.registry.Remove(d.ID); err != nil {
				dialog.ShowError(err, p.ac.UIService.MainWindow)
				return
			}
			p.Reload()
			if activeID == d.ID {
				p.proxies.SetEnabled(false)
				p.proxies.Clear()
			}
		}, p.ac.UIService.MainWindow)
}

// togglePower запускает или останавливает ядро на машине.
//
// Останов подтверждается, старт — нет: остановка рвёт VPN у всех, кто ходит
// через эту машину, а запуск безобиден.
func (p *machineListPanel) togglePower(d services.RemoteDaemon, running bool) {
	run := func() {
		go func() {
			var err error
			if running {
				err = p.registry.StopCore(d.ID)
			} else {
				err = p.registry.StartCore(d.ID)
			}
			// Перечитываем состояние: оно только что изменилось, и рисовать
			// строку по домыслу («нажали Start — значит started») нельзя —
			// ядро могло не подняться.
			h := p.registry.Health(d.ID)
			fyne.Do(func() {
				if err != nil {
					debuglog.WarnLog("machine list: power %q: %v", d.ID, err)
					dialog.ShowError(err, p.ac.UIService.MainWindow)
				}
				p.health[d.ID] = h
				p.redrawRows()

				if h.CoreStatus == "started" {
					// Ядро поднялось — вот теперь у машины есть группы и узлы.
					p.loadNodes()
					return
				}
				// Ядро остановлено: список слева должен опустеть, а не
				// показывать узлы, которых на машине уже нет.
				if p.ac.APIService != nil {
					p.ac.APIService.ResetScope(services.ScopeRemote)
				}
				p.proxies.Clear()
			})
		}()
	}
	if running {
		dialog.ShowConfirm(
			locale.T("servers.power.stop_title"),
			locale.Tf("servers.power.stop_body", d.Name),
			func(ok bool) {
				if ok {
					run()
				}
			}, p.ac.UIService.MainWindow)
		return
	}
	run()
}

// deployTo отправляет машине ЕЁ СОБСТВЕННЫЙ конфиг (§2.4).
//
// Путь считается от ID той же строки, из которой нажали, — поэтому промах
// «собрал для одной, задеплоил на другую» тут невозможен по конструкции, а не
// по проверке.
func (p *machineListPanel) deployTo(d services.RemoteDaemon) {
	path := platform.GetRemoteConfigPathFor(p.ac.FileService.ExecDir, d.ID)
	config, err := os.ReadFile(path)
	if err != nil {
		// Конфиг ещё не собирали. Говорим, что делать, вместо сырой ошибки
		// чтения — и указываем на Configure ИМЕННО этой машины.
		dialog.ShowInformation(locale.T("servers.power.deploy_title"),
			locale.Tf("remote.machines.deploy_missing", d.Name), p.ac.UIService.MainWindow)
		return
	}
	dialog.ShowConfirm(
		locale.T("servers.power.deploy_title"),
		locale.Tf("servers.power.deploy_body", d.Name, len(config)),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				// Сначала ресурсы, потом конфиг: конфиг ссылается на
				// <state_dir>/resources/<name>, и без файлов ядро на той
				// стороне не поднимется — apply пройдёт, а инстанс упадёт.
				// Демон и сам требует этого порядка: PUT на имя, занятое
				// живой ссылкой, отбивается 409.
				resources, resErr := services.CollectDeployResources(p.ac.FileService.ExecDir, d.ID, config)
				if resErr == nil && len(resources) > 0 {
					resErr = p.registry.SyncResources(d.ID, resources)
				}
				if resErr != nil {
					fyne.Do(func() {
						debuglog.WarnLog("machine list: deploy %q: resources: %v", d.ID, resErr)
						dialog.ShowError(resErr, p.ac.UIService.MainWindow)
					})
					return
				}
				applyErr := p.registry.ApplyConfig(d.ID, config)
				fyne.Do(func() {
					if applyErr != nil {
						debuglog.WarnLog("machine list: deploy %q: %v", d.ID, applyErr)
						dialog.ShowError(applyErr, p.ac.UIService.MainWindow)
						return
					}
					dialog.ShowInformation(locale.T("servers.power.deploy_title"),
						locale.Tf("servers.power.deploy_done", d.Name), p.ac.UIService.MainWindow)
					p.Reload()
				})
			}()
		}, p.ac.UIService.MainWindow)
}

// redrawRows перерисовывает строки из кеша health, не трогая реестр.
func (p *machineListPanel) redrawRows() {
	list, err := p.registry.List()
	if err != nil {
		return
	}
	activeID, _, _ := GetLxdRemoteOverride()
	p.list.RemoveAll()
	for _, d := range list {
		p.list.Add(p.buildRow(d, d.ID == activeID))
	}
	p.list.Refresh()
}

// dotLayout рисует единственный объект кругом фиксированного диаметра,
// центрируя его по вертикали в выданной площади.
//
// Нужен, потому что canvas.Circle сообщает MinSize 1×1 и растягивается на всю
// ячейку: и GridWrap, и Center давали точку размером с иконку. Здесь размер
// задаёт раскладка, а не сам объект.
type dotLayout struct{ size float32 }

func (d *dotLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	// Ширина с полем справа — маркер не должен липнуть к имени.
	return fyne.NewSize(d.size+6, d.size)
}

func (d *dotLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(fyne.NewSize(d.size, d.size))
		o.Move(fyne.NewPos(0, (size.Height-d.size)/2))
	}
}

// recordFailure запоминает неудачную попытку соединения.
//
// Зовётся из горутины Connect, поэтому под мьютексом: карту читает UI-поток
// при отрисовке ⓘ.
func (p *machineListPanel) recordFailure(id string, attempt int, msg string) {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	list := append(p.errLog[id], connectFailure{
		When:    time.Now(),
		Attempt: attempt,
		Err:     msg,
	})
	if len(list) > maxConnectFailures {
		list = list[len(list)-maxConnectFailures:]
	}
	p.errLog[id] = list
}

// failures возвращает копию истории отказов машины.
func (p *machineListPanel) failures(id string) []connectFailure {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	out := make([]connectFailure, len(p.errLog[id]))
	copy(out, p.errLog[id])
	return out
}

// newRevealBox оборачивает содержимое в контейнер, который «выезжает» по
// высоте от нуля до своей минимальной.
//
// Fyne не анимирует появление объектов сам: без этого блок возникает рывком,
// и связь между нажатой стрелкой и тем, что раскрылось, теряется — особенно
// когда машин в списке несколько.
func newRevealBox(content fyne.CanvasObject) fyne.CanvasObject {
	box := &revealBox{content: content}
	box.ExtendBaseWidget(box)
	box.animate()
	return box
}

type revealBox struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	progress float32 // 0 — свёрнут, 1 — раскрыт целиком
}

func (b *revealBox) animate() {
	anim := canvas.NewSizeAnimation(
		fyne.NewSize(0, 0), fyne.NewSize(0, 1), revealDuration,
		func(s fyne.Size) {
			b.progress = s.Height
			b.Refresh()
		})
	anim.Curve = fyne.AnimationEaseOut
	anim.Start()
}

func (b *revealBox) CreateRenderer() fyne.WidgetRenderer {
	return &revealRenderer{box: b}
}

type revealRenderer struct{ box *revealBox }

func (r *revealRenderer) Layout(size fyne.Size) {
	r.box.content.Resize(fyne.NewSize(size.Width, r.box.content.MinSize().Height))
	r.box.content.Move(fyne.NewPos(0, 0))
}

func (r *revealRenderer) MinSize() fyne.Size {
	full := r.box.content.MinSize()
	return fyne.NewSize(full.Width, full.Height*r.box.progress)
}

func (r *revealRenderer) Refresh()                     { canvas.Refresh(r.box) }
func (r *revealRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.box.content} }
func (r *revealRenderer) Destroy()                     {}
