package ui

import (
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
)

// Фоновый опрос активной машины (heartbeat).
//
// Зачем: до него состояние машины перечитывалось ТОЛЬКО по действию
// пользователя — Connect, Start/Stop, Restart. В промежутке строка жила
// кешем: сервер за ней мог лечь, а маркер продолжал гореть зелёным и
// показывать «started», потому что таким его увидел последний опрос. То есть
// индикатор отвечал не на вопрос «как машина сейчас», а «как она была, когда
// я в последний раз нажимал кнопку».
//
// Опрашивается ровно одна машина — активная. Транспорт в APIService
// единственный, подключённой может быть только она; ходить по сети к
// остальным записям реестра без спроса нельзя (то же правило, что у Reload:
// показ списка — не повод стучаться к чужим хостам).

const (
	// heartbeatInterval — период опроса.
	//
	// 5 секунд: /admin/status — это один дешёвый GET, но он идёт по тому же
	// каналу, что и рабочие вызовы, и учащать смысла нет — падение сервера
	// не то событие, где важны миллисекунды. Реакция «до 10 секунд» (два
	// промаха до красного) для ручного управления машиной достаточна.
	heartbeatInterval = 5 * time.Second

	// heartbeatFailThreshold — сколько промахов подряд превращают жёлтый в
	// красный.
	//
	// Один пропущенный ответ ничего не доказывает: Wi-Fi моргнул, роутер
	// перестроил маршрут, демон занят применением конфига. Красить в красный
	// по первому промаху — значит мигать тревогой на ерунде, и тогда маркеру
	// перестают верить ровно так же, как залипшему зелёному.
	heartbeatFailThreshold = 2
)

// machineLiveness — свежесть последнего ответа машины.
type machineLiveness struct {
	// FailStreak — сколько опросов подряд не дошло (0 — последний ответил).
	FailStreak int
	// LastErr — ошибка последнего неудачного опроса.
	LastErr string
	// LastOK — когда машина отвечала в последний раз.
	LastOK time.Time
}

// startHeartbeat запускает фоновый опрос. Останавливается закрытием stop.
//
// Горутина одна на панель и живёт всё время её существования: она сама
// смотрит, есть ли активная машина, и в простое не делает ничего.
func (p *machineListPanel) startHeartbeat() {
	go func() {
		t := time.NewTicker(heartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-p.stopHeartbeat:
				return
			case <-t.C:
				p.pollActive()
			}
		}
	}()
}

// pollActive опрашивает активную машину и обновляет её состояние.
//
// Идёт по сети, поэтому зовётся только из горутины heartbeat. Результат
// применяется через fyne.Do — карты состояния читает UI-поток при отрисовке.
func (p *machineListPanel) pollActive() {
	id, _, ok := GetLxdRemoteOverride()
	if !ok || id == "" {
		return
	}
	// Соединение ещё устанавливается (идут повторы Connect) — там свой цикл
	// опроса со своей индикацией, и второй поверх него только путал бы
	// картину и удваивал нагрузку на канал. Карту попыток пишет UI-поток,
	// поэтому и читать её отсюда можно только через него — иначе
	// concurrent map read/write.
	connecting := false
	fyne.DoAndWait(func() { connecting = p.connectAttempt[id] > 0 })
	if connecting {
		return
	}
	h := p.registry.Health(id)
	fyne.Do(func() {
		// Пока ходили по сети, пользователь мог отключиться или переключиться
		// на другую машину. Ответ прежней машины к текущей строке отношения
		// не имеет — выбрасываем, иначе он оживил бы маркер уже отключённой.
		if nowID, _, stillOK := GetLxdRemoteOverride(); !stillOK || nowID != id {
			return
		}
		live := p.liveness[id]
		if h.Err != "" {
			live.FailStreak++
			live.LastErr = h.Err
			p.liveness[id] = live
			debuglog.WarnLog("machine heartbeat: %q no answer (%d in a row): %s",
				id, live.FailStreak, h.Err)
			// health НЕ перезаписываем ошибкой: строка обязана продолжать
			// показывать последнее ИЗВЕСТНОЕ состояние ядра, пока вердикт не
			// вынесен. Иначе один промах на жёлтом стирал бы версию, статус и
			// прятал кнопки — а машина, возможно, просто моргнула.
			p.redrawRows()
			return
		}
		live.FailStreak = 0
		live.LastErr = ""
		live.LastOK = time.Now()
		p.liveness[id] = live
		prev, had := p.health[id]
		p.health[id] = h
		p.redrawRows()
		// Ядро сменило состояние без нашего участия (упало, или его подняли
		// с самой машины) — левая колонка обязана догнать: при остановленном
		// ядре узлов на той стороне физически нет.
		if had && prev.CoreStatus != h.CoreStatus {
			debuglog.InfoLog("machine heartbeat: %q core %q → %q", id, prev.CoreStatus, h.CoreStatus)
			p.onCoreStatusChanged(h)
		}
	})
}

// onCoreStatusChanged синхронизирует левую колонку с новым состоянием ядра.
func (p *machineListPanel) onCoreStatusChanged(h services.RemoteHealth) {
	if h.CoreStatus == "started" {
		p.proxies.SetEnabled(true)
		p.loadNodes()
		return
	}
	if p.ac.APIService != nil {
		p.ac.APIService.ResetScope(services.ScopeRemote)
	}
	p.proxies.SetEnabled(false)
	p.proxies.Clear()
}

// markerState — что показывает кружок слева от имени машины.
type markerState int

const (
	// markerIdle — не подключались: про машину не знаем ничего.
	markerIdle markerState = iota
	// markerLive — последний опрос ответил, ядро в порядке.
	markerLive
	// markerFlaky — ответы перестали доходить, но вердикт не вынесен: идут
	// повторы. Отдельное состояние, потому что «моргнуло» и «легло» — разные
	// новости, и врать любой из них нельзя.
	markerFlaky
	// markerDown — не отвечает устойчиво, либо ответила, но с бедой в ядре.
	markerDown
)

// markerFor вычисляет состояние маркера машины.
func markerFor(connected bool, h services.RemoteHealth, live machineLiveness) markerState {
	if !connected {
		return markerIdle
	}
	switch {
	case live.FailStreak >= heartbeatFailThreshold:
		return markerDown
	case live.FailStreak > 0:
		return markerFlaky
	}
	// Машина отвечает — но ответ может быть плохой новостью: ядро упало
	// (fatal) или последнее применение конфига провалилось. Зелёный тут
	// значил бы «всё хорошо» ровно там, где всё плохо.
	if h.Err != "" || h.CoreStatus == "fatal" {
		return markerDown
	}
	return markerLive
}

// livenessOf / healthOf — доступ к состоянию для окна журнала (livenessSource).
func (p *machineListPanel) livenessOf(id string) machineLiveness { return p.liveness[id] }

func (p *machineListPanel) healthOf(id string) (services.RemoteHealth, bool) {
	activeID, _, ok := GetLxdRemoteOverride()
	h, has := p.health[id]
	// «Подключены» — это активность И наличие ответа, тот же инвариант, что
	// в строке: транспорт один, и health чужой машины к текущей не относится.
	return h, ok && activeID == id && has
}
