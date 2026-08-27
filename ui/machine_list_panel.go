package ui

import (
	"fmt"
	"image/color"
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

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	machinesDeployMissingText = "No config built for %s yet. Press Configure on its row, set it up and press Save — that writes the config this button sends."
	machinesRemoveBodyText    = "Remove %s? Its config, wizard states and client keys are deleted from this launcher.\n\nAccess on the machine itself stays registered — revoke it there with `sing-box lxd client remove`."
	powerDeployBodyText       = "Send the config to %s (%d bytes)? The daemon validates it before swapping the instance and rolls back to last-good if the new one fails to start."
	powerRestartBodyText      = "Restart the core on %s? Everyone routing through that machine loses VPN for a moment; if the start fails, the machine is left without a core."
)

// Правая колонка вкладки Remote — список удалённых машин (SPEC 098 §2.1).
//
// Заменяет собой три разбросанных места: дропдаун выбора машины в шапке
// Servers, блок питания там же и область Remote в окне подключения. Смысл
// сведения — не эстетика: пока выбор машины и таргет конфига были разными
// переключателями, можно было собрать конфиг для роутера и задеплоить его на
// VPS. Здесь у каждой строки свои Configure и Deploy, и промахнуться нечем.

// Повторы Connect: наблюдался отказ, когда connect() возвращал «no route to
// host» мгновенно, хотя curl к тому же адресу в ту же секунду отвечал —
// диагноз 2026-08-25: это флап политики macOS «Локальная сеть», nehelper не
// мог прочитать подпись бандла (см. platform.DiagnoseLanDenial). Плюс обычные
// короткие провалы сети (роутер перезагружается, точка роняет клиента). Пять
// попыток с паузой в 3 секунды перекрывают и то и другое; дальше это уже
// устойчивый отказ, и молотить сеть смысла нет — по нему запускается
// диагностика, и её вердикт дописывается к тексту ошибки.
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
	// liveness — свежесть ответов машины: сколько опросов подряд не дошло.
	//
	// Отдельно от health намеренно: health — это то, что машина СКАЗАЛА о
	// себе, liveness — доходят ли ответы вообще. Смешать их значило бы
	// стирать последнее известное состояние ядра на первом же промахе.
	liveness map[string]machineLiveness
	// stopHeartbeat закрывается при закрытии панели и гасит фоновый опрос.
	stopHeartbeat chan struct{}
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
		liveness:       make(map[string]machineLiveness),
		stopHeartbeat:  make(chan struct{}),
	}
	p.list = container.NewVBox()

	addBtn := widget.NewButton(locale.T("+ Add"), func() {
		OpenAddMachineWindow(ac, func() {
			// Только что добавленная машина ещё не подключена — узлов у неё
			// для нас нет, и Refresh ушёл бы с пустой группой.
			p.Reload()
		})
	})
	addBtn.Importance = widget.MediumImportance

	header := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(locale.T("MACHINES"), fyne.TextAlignLeading,
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
	// Фоновый опрос активной машины: без него строка живёт кешем последнего
	// действия пользователя и показывает зелёный маркер над лежащим сервером
	// (см. ui/machine_heartbeat.go).
	p.startHeartbeat()
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
		hint := widget.NewLabel(locale.T("No remote machines yet. Press “+ Add” and paste the invite printed by `sing-box lxd` on the machine you want to manage."))
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
	//
	// active — обязательное условие, а не только признак жирного имени:
	// транспорт в APIService ОДИН, и подключённой может быть ровно одна
	// машина. Без этой связки строка машины, чей канал уже перехватила
	// соседняя, продолжала бы показывать Disconnect, статус и Deploy — а
	// кнопки эти ходили бы по чужому каналу и отправляли бы конфиг не на ту
	// машину, что написана в строке.
	health, hasHealth := p.health[d.ID]
	connected := active && hasHealth

	// Маркер состояния канала: зелёный — соединены, красный — соединялись, но
	// машина не отвечает, серый — не подключались.
	//
	// Цветом рисуется круг, а не текст: widget.Label в Fyne красить нечем, а
	// цвет здесь несёт смысл (это первое, что видно в списке из нескольких
	// машин). Форма при этом одна — цвет не единственный различитель: рядом
	// стоит либо Connect, либо Disconnect со статусом.
	// Состояние считает markerFor: цвет здесь — только его отображение.
	// Жёлтый (markerFlaky) означает «ответы перестали доходить, идут
	// повторы» — промежуток между «всё хорошо» и «легло», в котором честного
	// ответа ещё нет.
	live := p.liveness[d.ID]
	state := markerFor(connected, health, live)
	var markerColor color.Color = theme.Color(theme.ColorNameDisabled)
	switch state {
	case markerLive:
		markerColor = theme.Color(theme.ColorNameSuccess)
	case markerFlaky:
		markerColor = theme.Color(theme.ColorNameWarning)
	case markerDown:
		markerColor = theme.Color(theme.ColorNameError)
	}
	// Круг занимает ВСЮ выданную площадь (его MinSize = 1×1, а Layout
	// растягивает), поэтому и GridWrap, и Center раздували точку до размера
	// ячейки. Фиксируем размер собственной раскладкой и центрируем по
	// вертикали, чтобы маркер стоял вровень с именем.
	//
	// Маркер кликабельный: он первым показывает, что с машиной что-то не так,
	// и логично, что по нему же открывается разговор с ней. Форма при этом
	// одна — цвет не единственный различитель: рядом стоит либо Connect, либо
	// Disconnect со статусом, а подсказка называет состояние словами.
	markerBox := newMachineMarker(state, markerColor, func() {
		OpenMachineWireLogWindow(p.ac, d, p)
	})

	name := widget.NewLabelWithStyle(d.Name, fyne.TextAlignLeading,
		fyne.TextStyle{Bold: active})

	tgt := d.Target()
	meta := widget.NewLabel(fmt.Sprintf("%s/%s   %s", tgt.GOOS, tgt.GOARCH, d.Addr))

	// Connect/Disconnect — напротив АДРЕСА: кнопка про канал именно к нему,
	// а не про ядро на той стороне (тем занимается Start/Stop у статуса).
	var connBtn *widget.Button
	if connected {
		connBtn = widget.NewButton(locale.T("Disconnect"), func() {
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
			locale.Tf("Connecting… %d/%d", attempt, connectAttempts), nil)
		connBtn.Disable()
	} else {
		connBtn = widget.NewButton(locale.T("Connect"), func() {
			p.connectMachine(d)
		})
		connBtn.Importance = widget.HighImportance
	}
	metaRow := container.NewBorder(nil, nil, nil, connBtn, meta)

	editBtn := ttwidget.NewButton("✎", func() {
		p.editMachine(d)
	})
	editBtn.SetToolTip(locale.T("Edit name, address and platform"))
	editBtn.Importance = widget.LowImportance

	removeBtn := ttwidget.NewButton("✕", func() {
		p.removeMachine(d)
	})
	removeBtn.SetToolTip(locale.T("Remove this machine"))
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
	configureBtn := widget.NewButton(locale.T("Configure"), func() {
		configurator.ShowConfigWizardForMachine(p.ac.UIService.MainWindow, d)
	})

	// Соединились: показываем настоящий статус ядра и открываем управление.
	//
	// Статусы демона: idle | started | fatal (RemoteHealth.CoreStatus).
	statusText := locale.T("reachable")
	switch {
	case health.Err != "":
		statusText = locale.Tf("unreachable: %s", health.Err)
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
	powerLabel := locale.T("Start")
	if running {
		powerLabel = locale.T("Stop")
	}
	powerBtn := widget.NewButton(powerLabel, func() {
		p.togglePower(d, running)
	})
	if health.Err != "" {
		// Машина не отвечает — управлять её ядром нечем.
		powerBtn.Disable()
	}

	// ↻ — рестарт ядра одним действием (Stop + Start). Тот же глиф, что у
	// пульс-кнопки обновления списка (refreshPulseGlyph): он гарантированно
	// есть в шрифте темы и читается как «перезапустить». Активна только на
	// работающем ядре: перезапуск остановленного — это просто Start.
	restartBtn := ttwidget.NewButton(refreshPulseGlyph, func() {
		p.restartCore(d)
	})
	restartBtn.SetToolTip(locale.TN(1, "Restart the core"))
	if health.Err != "" || !running {
		restartBtn.Disable()
	}

	// (i) — всё, что демон сообщил о себе: хеши конфигов, последняя ошибка,
	// state-dir. В строку это не влезает, а при разборе «почему на машине не
	// то» нужно целиком.
	infoBtn := ttwidget.NewButton("ⓘ", func() {
		p.showHealthDetails(d, health)
	})
	infoBtn.SetToolTip(locale.T("What the daemon reports about itself"))
	infoBtn.Importance = widget.LowImportance

	deployBtn := widget.NewButton(locale.T("Deploy config"), func() {
		p.deployTo(d)
	})
	if health.Err != "" {
		deployBtn.Disable()
	}

	// RES — управление файлами, на которые ссылается конфиг машины
	// (SPEC 063). Рядом с Deploy, потому что это его обратная сторона:
	// Deploy заливает недостающее молча, а здесь видно, что реально лежит на
	// той стороне и совпадает ли оно с нашим.
	resBtn := widget.NewButton(locale.T("RES"), func() {
		OpenMachineResourcesWindow(p.ac, d)
	})
	if health.Err != "" {
		resBtn.Disable()
	}

	// Start/Stop — напротив СТАТУСА, который он меняет: кнопка стоит там,
	// где виден её результат.
	statusRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(infoBtn, powerBtn, restartBtn), status)
	// Дополнительные инструменты — под строкой, раскрытием вниз (тот же
	// Accordion, что «Advanced» в окне добавления машины). Не popup-меню:
	// содержимое остаётся на месте, не перекрывает соседние машины и не
	// исчезает при промахе мышью.
	profilerBtn := widget.NewButton(locale.T("Traffic profiler"), func() {
		OpenMachineProfiler(p.ac, d)
	})
	// Телеметрия ХОСТА — рядом с профайлером, потому что это соседний вопрос
	// с другим ответом: профайлер показывает трафик ядра, а «почему роутер
	// тормозит» решается только описанием машины (CPU, память, диски, темп.).
	hostBtn := widget.NewButton(locale.T("Host telemetry"), func() {
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
	// Соединение ровно одно: транспорт в APIService единственный, и
	// SetLxdRemoteOverride закрывает предыдущий. Прежняя машина обязана
	// отключиться и в UI — иначе её строка остаётся с Disconnect, статусом и
	// Deploy, а кнопки эти уже ходят по каналу ДРУГОЙ машины: Deploy отправил
	// бы конфиг не туда, куда показывает строка.
	//
	// Гасим до переключения, пока прежний транспорт ещё жив: его окна
	// (профайлер, телеметрия) должны закрыться по своей машине, а не по новой.
	if prevID, _, ok := GetLxdRemoteOverride(); ok && prevID != d.ID {
		CloseMachineProfiler(prevID)
		CloseMachineHostWindow(prevID)
		delete(p.health, prevID)
		delete(p.connectAttempt, prevID)
		delete(p.liveness, prevID)
		CloseMachineWireLogWindow(prevID)
		debuglog.InfoLog("machine list: %q disconnected — connecting to %q", prevID, d.ID)
	}
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
	// адресу в ту же секунду отвечал 200. Диагноз 2026-08-25 — не провал
	// L2/ARP, а флап политики macOS «Локальная сеть» (nehelper не читает
	// подпись бандла, см. platform.DiagnoseLanDenial); повтор через несколько
	// секунд нередко попадает в окно, когда политика перепроверилась.
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
		// Мгновенный «no route to host» на macOS — сигнатура блокировки
		// «Локальной сети» (NECP при сломанной подписи бандла), а не сетевой
		// отказ; контрольный dial отличает её и от настоящей недоступности,
		// и от уже прошедшего флапа. Сетевой вызов — потому здесь, до fyne.Do.
		if h.Err != "" && strings.Contains(h.Err, "no route to host") {
			verdict := platform.DiagnoseLanDenial(d.Addr)
			if hint := lanDenialHint(verdict); hint != "" {
				debuglog.WarnLog("machine list: %q lan diagnosis: %s", d.ID, verdict)
				h.Err += "\n\n" + hint
			}
		}
		fyne.Do(func() {
			p.connectAttempt[d.ID] = 0
			p.health[d.ID] = h
			// Connect — точка отсчёта для heartbeat: его вердикт заменяет
			// всё, что накопилось до него.
			live := machineLiveness{}
			if h.Err != "" {
				// Соединиться не удалось за все попытки — это уже устойчивый
				// отказ, а не «моргнуло»: маркер обязан быть красным сразу,
				// а не жёлтым до следующих двух промахов heartbeat.
				live.FailStreak = heartbeatFailThreshold
				live.LastErr = h.Err
			} else {
				live.LastOK = time.Now()
			}
			p.liveness[d.ID] = live
			p.redrawRows()

			if h.Err != "" {
				// Без попапа: маркер строки краснеет, а полный текст последней
				// ошибки лежит в ⓘ. Модальное окно на каждую неудачу мешает
				// повторить попытку и ничего не добавляет к тому, что уже
				// видно в строке.
				debuglog.WarnLog("machine list: %q unreachable after %d attempts: %s",
					d.ID, connectAttempts, h.Err)
				// Слева могли остаться узлы машины, с которой мы разорвали
				// связь ради этой попытки. Показывать их под недоступной
				// машиной значит показывать данные не той машины — очищаем.
				p.proxies.SetEnabled(false)
				p.proxies.Clear()
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
		{locale.T("Machine"), d.Name},
		{locale.T("Address"), d.Addr},
		{locale.T("Platform"), fmt.Sprintf("%s/%s", d.Target().GOOS, d.Target().GOARCH)},
		{locale.T("Daemon version"), h.Version},
		{locale.T("Core status"), h.CoreStatus},
		{locale.T("State dir"), h.StateDir},
		{locale.T("Active config sha256"), h.ActiveSHA},
		{locale.T("Last-good config sha256"), h.LastGoodSHA},
	}
	if h.InterruptedApply {
		rows = append(rows, [2]string{locale.T("Interrupted apply"), locale.T("yes — the core runs the last-good config")})
	}
	if h.LastError != "" {
		rows = append(rows, [2]string{locale.T("Last error"), h.LastError})
	}
	if h.Err != "" {
		rows = append(rows, [2]string{locale.T("Unreachable"), h.Err})
	}
	// Сводка ядра, если открыт профайлер машины: связи, скорости, память.
	// Стрим статуса живёт вместе с его окном, поэтому до открытия строк нет —
	// и выдумывать нули вместо «не знаем» нельзя.
	if st, ok := MachineStatus(d.ID); ok {
		rows = append(rows,
			[2]string{locale.T("Connections"),
				locale.Tf("%d in / %d out", st.ConnectionsIn, st.ConnectionsOut)},
			[2]string{locale.T("Traffic"),
				locale.Tf("↑%s/s  ↓%s/s   (total ↑%s ↓%s)",
					sizeOrDash(st.Uplink, true), sizeOrDash(st.Downlink, true),
					sizeOrDash(st.UplinkTotal, true), sizeOrDash(st.DownlinkTotal, true))},
			[2]string{locale.T("Core runtime"),
				locale.Tf("%s, goroutines %d", sizeOrDash(int64(st.Memory), true), st.Goroutines)},
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
			locale.Tf("Connection failures this session (%d)", len(fails)),
			strings.TrimRight(b.String(), "\n"),
		})
	}

	// Признак «поле длинное» несёт сама строка таблицы, а не наличие \n в
	// значении: история из ОДНОГО отказа переносов не содержит, но всё равно
	// длиннее строки, и однострочный Entry показывал бы её началом с
	// обрезанием.
	multiline := map[string]bool{locale.Tf("Connection failures this session (%d)", len(p.failures(d.ID))): true}

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

	win := p.ac.UIService.Application.NewWindow(locale.Tf("%s — daemon status", d.Name))
	copyBtn := widget.NewButton(locale.T("Copy"), func() {
		setClipboard(plain.String())
	})
	closeBtn := widget.NewButton(locale.T("Close"), func() { win.Close() })
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
	//
	// Группы спрашиваем с ретраями в горутине: сразу после Start демон уже
	// отвечает «started», но ядро внутри ещё поднимается и групп не отдаёт.
	// Единственная синхронная попытка промахивалась, список оставался пустым
	// («Reading the machine's selector groups…») до случайного тика
	// health-опроса. Заодно сетевой Groups() уходит с UI-потока.
	go func() {
		for attempt := 0; attempt < 15; attempt++ {
			groups, isRemote, err := RemoteDaemonGroups()
			if !isRemote || (err == nil && len(groups) > 0) {
				break
			}
			time.Sleep(time.Second)
		}
		fyne.Do(func() {
			p.proxies.ReloadGroups()
			p.proxies.Refresh()
		})
	}()
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
	// Свежесть ответов забывается вместе с состоянием: иначе следующий
	// Connect начинался бы с накопленным FailStreak и красил маркер красным
	// ещё до первого опроса.
	p.liveness = make(map[string]machineLiveness)
	p.Reload()
	p.proxies.Clear()
}

// editMachine — паспорт записи плюс действия над самой записью: пере-сопряжение
// и перенос настроек с другой машины. Платформа правится там же: это свойство
// машины, и менять его из визарда нельзя (§2.4).
//
// Отдельное окно вместо прежнего модального диалога: к четырём полям добавились
// два действия со своими полями ввода и статусами, а высокий модальный попап в
// Fyne раздувается на весь экран.
func (p *machineListPanel) editMachine(d services.RemoteDaemon) {
	OpenEditMachineWindow(p.ac, p.registry, d, func() {
		// После re-pair прежний канал разговаривает по отозванному мандату:
		// строка обязана вернуться к Connect, а не показывать «connected».
		if id, _, ok := GetLxdRemoteOverride(); ok && id == d.ID {
			p.disconnectMachine()
			return
		}
		p.Reload()
	})
}

// removeMachine удаляет машину со всем её имуществом (§3.1.9).
//
// Предупреждение про отзыв доступа обязательно: мы забываем ключ у себя, но
// регистрация на СТОРОНЕ демона остаётся, и снять её может только он сам.
// Промолчать здесь значило бы дать пользователю ложное чувство, что доступ
// отозван.
func (p *machineListPanel) removeMachine(d services.RemoteDaemon) {
	dialog.ShowConfirm(
		locale.T("Remove machine"),
		locale.Tf(machinesRemoveBodyText, d.Name),
		func(ok bool) {
			if !ok {
				return
			}
			// Машины не станет — её профайлер и телеметрия тоже должны уйти.
			CloseMachineProfiler(d.ID)
			CloseMachineHostWindow(d.ID)
			// Журнал обмена — про машину, которой больше не будет в реестре.
			CloseMachineWireLogWindow(d.ID)
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
			locale.T("Stop the core"),
			locale.Tf("Stop the core on %s? Everyone routing through that machine loses VPN until it is started again.", d.Name),
			func(ok bool) {
				if ok {
					run()
				}
			}, p.ac.UIService.MainWindow)
		return
	}
	run()
}

// restartCore перезапускает ядро машины: Stop, затем Start, одним действием.
//
// С подтверждением, как у Stop: между остановкой и стартом VPN у всех, кто
// ходит через машину, моргнёт, а неудачный Start оставит её без ядра вовсе.
func (p *machineListPanel) restartCore(d services.RemoteDaemon) {
	dialog.ShowConfirm(
		locale.T("Restart the core"),
		locale.Tf(powerRestartBodyText, d.Name),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				err := p.registry.StopCore(d.ID)
				if err == nil {
					err = p.registry.StartCore(d.ID)
				}
				// Как в togglePower: состояние перечитываем, а не домысливаем —
				// ядро могло не подняться.
				h := p.registry.Health(d.ID)
				fyne.Do(func() {
					if err != nil {
						debuglog.WarnLog("machine list: restart %q: %v", d.ID, err)
						dialog.ShowError(err, p.ac.UIService.MainWindow)
					}
					p.health[d.ID] = h
					p.redrawRows()
					if h.CoreStatus == "started" {
						p.loadNodes()
						return
					}
					if p.ac.APIService != nil {
						p.ac.APIService.ResetScope(services.ScopeRemote)
					}
					p.proxies.Clear()
				})
			}()
		}, p.ac.UIService.MainWindow)
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
		dialog.ShowInformation(locale.TN(1, "Deploy config"),
			locale.Tf(machinesDeployMissingText, d.Name), p.ac.UIService.MainWindow)
		return
	}
	dialog.ShowConfirm(
		locale.TN(1, "Deploy config"),
		locale.Tf(powerDeployBodyText, d.Name, len(config)),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				// Вся цепочка (ресурсы строго раньше конфига) — в
				// services.Deploy: её же зовёт Debug API, поэтому «через API
				// деплоится не так, как кнопкой» невозможно (SPEC 100).
				_, deployErr := p.registry.Deploy(d.ID, config)
				fyne.Do(func() {
					if deployErr != nil {
						debuglog.WarnLog("machine list: deploy %q: %v", d.ID, deployErr)
						dialog.ShowError(deployErr, p.ac.UIService.MainWindow)
						return
					}
					dialog.ShowInformation(locale.TN(1, "Deploy config"),
						locale.Tf("Config applied on %s.", d.Name), p.ac.UIService.MainWindow)
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
