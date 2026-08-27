package lxdclient

import (
	"strconv"
	"sync"
	"time"
)

// Журнал обмена с демоном: что лаунчер спросил и что ему ответили.
//
// Зачем отдельный журнал, когда есть debuglog: там события лаунчера вперемешку
// со всем остальным и без привязки к машине, а вопрос у пользователя ровно
// один и адресный — «что сейчас происходит между мной и ЭТОЙ машиной». Ответ
// на него должен открываться из строки машины, а не поиском по общему логу.
//
// Пишем только метаданные: метод, путь, код, длительность, текст ошибки.
// Тела запросов сюда не попадают by design — через /admin/apply едет конфиг с
// приватными ключами узлов, и складывать его в буфер, который открывается
// кнопкой и копируется в тикет, нельзя.

// WireEvent — одна операция обмена.
type WireEvent struct {
	// When — момент ЗАВЕРШЕНИЯ операции: журнал читают как «что было
	// последним», и время ответа отвечает на это точнее времени отправки.
	When time.Time
	// Kind — REST | gRPC | stream.
	Kind string
	// Op — «GET /admin/status», «SubscribeStatus» и т.п.
	Op string
	// Status — HTTP-код (0 для не-REST и для не доехавших запросов).
	Status int
	// Took — сколько заняла операция.
	Took time.Duration
	// Err — почему не получилось; пусто, если всё прошло.
	Err string
}

// OK — операция завершилась успешно.
func (e WireEvent) OK() bool { return e.Err == "" && (e.Status == 0 || e.Status < 400) }

// wireLogMax — сколько событий помним на машину.
//
// Heartbeat опрашивает статус раз в несколько секунд, то есть журнал целиком
// оборачивается за считанные минуты. Больше держать незачем: вопрос к нему
// всегда про «прямо сейчас», а не про историю за сессию — для отказов
// соединения есть отдельный список в ⓘ.
const wireLogMax = 200

// wireLog — кольцевой буфер событий одной машины.
type wireLog struct {
	mu     sync.Mutex
	events []WireEvent
}

func (w *wireLog) add(e WireEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, e)
	if len(w.events) > wireLogMax {
		w.events = w.events[len(w.events)-wireLogMax:]
	}
}

func (w *wireLog) snapshot() []WireEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]WireEvent, len(w.events))
	copy(out, w.events)
	return out
}

// Журналы живут в пакете, а не в Client: клиент к машине создаётся заново на
// КАЖДЫЙ вызов (adminClient собирает его из реестра), и журнал внутри него
// умирал бы вместе с вызовом. Ключ — идентификатор записи реестра.
var (
	wireLogsMu sync.Mutex
	wireLogs   = map[string]*wireLog{}
)

func logFor(key string) *wireLog {
	wireLogsMu.Lock()
	defer wireLogsMu.Unlock()
	l, ok := wireLogs[key]
	if !ok {
		l = &wireLog{}
		wireLogs[key] = l
	}
	return l
}

// WireLog возвращает журнал обмена с машиной, свежие события — последними.
func WireLog(key string) []WireEvent {
	if key == "" {
		return nil
	}
	return logFor(key).snapshot()
}

// DropWireLog забывает журнал машины (её удалили из реестра).
func DropWireLog(key string) {
	wireLogsMu.Lock()
	defer wireLogsMu.Unlock()
	delete(wireLogs, key)
}

// RecordWire дописывает событие в журнал машины. Открыто наружу: стримы
// gRPC живут в services и пишут свои отметки сами.
func RecordWire(key string, e WireEvent) {
	if key == "" {
		return
	}
	if e.When.IsZero() {
		e.When = time.Now()
	}
	logFor(key).add(e)
}

// record — запись события REST-вызова этого клиента.
func (c *Client) record(op string, started time.Time, status int, err error) {
	if c.logKey == "" {
		return
	}
	e := WireEvent{
		When:   time.Now(),
		Kind:   "REST",
		Op:     op,
		Status: status,
		Took:   time.Since(started),
	}
	if err != nil {
		e.Err = err.Error()
	} else if status >= 400 {
		// Код без текста ничего не объясняет, а тело здесь уже прочитано
		// вызывающим. Ставим сам код — расшифровка приедет из текста ошибки
		// вызова, если он её сформирует.
		e.Err = "HTTP " + strconv.Itoa(status)
	}
	logFor(c.logKey).add(e)
}
