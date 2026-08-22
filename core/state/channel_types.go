package state

// Каналы роутинга (SPEC 104) — модель, перенесённая из LxBox.
//
// Зачем: правила лаунчера ссылались на глобальные outbound'ы шаблона, правила
// LxBox — на каналы `vpn-N`. Из-за разных неймспейсов целей бэкап переносился
// только с деградацией: правило с телефона на десктопе указывало в никуда.
//
// Канал — это ИМЕНОВАННАЯ точка выбора: пользователь настраивает фильтр нод и
// авторежим, а правила ссылаются на канал, а не на конкретный узел подписки
// (теги узлов генерируются на лету и меняются при каждом обновлении).
// На сборке канал материализуется в `selector` (+ парный `urltest`).

import "encoding/json"

// ChannelTagPrefix — префикс системных идентификаторов каналов.
//
// Тег неизменяем и не редактируется пользователем: на него ссылаются
// правила, и переименование канала не должно ломать ссылки. Отображаемое
// имя живёт отдельно, в Label.
const ChannelTagPrefix = "vpn-"

// MaxChannels — потолок числа каналов (как в LxBox).
const MaxChannels = 10

// ChannelAuto — параметры парной urltest-группы канала.
//
// nil у канала означает, что автовыбор выключен и `<tag>-auto` не эмитится
// вовсе — пустая или лишняя группа в конфиге не нужна.
type ChannelAuto struct {
	// URL — адрес проверки задержки.
	URL string `json:"url,omitempty"`
	// Interval — период перепроверки ("5m").
	Interval string `json:"interval,omitempty"`
	// Tolerance — на сколько миллисекунд новый лидер должен опередить
	// текущего, чтобы произошло переключение.
	Tolerance int `json:"tolerance,omitempty"`
	// IdleTimeout — через сколько простоя перестать проверять.
	IdleTimeout string `json:"idle_timeout,omitempty"`
	// InterruptExistConnections — рвать ли живые соединения при смене лидера.
	InterruptExistConnections bool `json:"interrupt_exist_connections,omitempty"`
}

// Channel — пользовательский канал роутинга.
type Channel struct {
	// Tag — системный идентификатор `vpn-N`. Неизменяем: на него ссылаются
	// правила.
	Tag string `json:"tag"`

	// Label — отображаемое имя («Моя Германия»). Единственное, что вводит
	// пользователь.
	Label string `json:"label,omitempty"`

	// Enabled — включён ли канал.
	Enabled bool `json:"enabled"`

	// IncludeDirect добавляет `direct-out` опцией в селектор канала.
	IncludeDirect bool `json:"include_direct,omitempty"`

	// IncludeBlock добавляет блокирующий outbound опцией в селектор.
	IncludeBlock bool `json:"include_block,omitempty"`

	// NodeFilter — регулярное выражение по ИТОГОВОМУ тегу узла. Пусто —
	// в канал попадают все узлы.
	NodeFilter string `json:"node_filter,omitempty"`

	// NodeFilterInvert инвертирует фильтр: в канал попадают узлы, чей тег
	// НЕ совпал. При пустом NodeFilter не значит ничего.
	NodeFilterInvert bool `json:"node_filter_invert,omitempty"`

	// DefaultFilter — регулярное выражение; первый совпавший узел
	// становится `default` селектора. Пусто — `default` не выставляется.
	DefaultFilter string `json:"default_filter,omitempty"`

	// InterruptExistConnections — рвать ли живые соединения при
	// переключении канала.
	InterruptExistConnections bool `json:"interrupt_exist_connections,omitempty"`

	// Auto — параметры парной urltest-группы; nil = автовыбор выключен.
	Auto *ChannelAuto `json:"auto,omitempty"`
}

// AutoTag — тег парной urltest-группы канала.
func (c Channel) AutoTag() string {
	if c.Tag == "" {
		return ""
	}
	return c.Tag + "-auto"
}

// DisplayLabel — имя для интерфейса: Label, а при его отсутствии — тег.
func (c Channel) DisplayLabel() string {
	if c.Label != "" {
		return c.Label
	}
	return c.Tag
}

// UnmarshalJSON читает канал, восстанавливая умолчания.
//
// Отдельный размаршалер нужен из-за Enabled и InterruptExistConnections:
// у обоих умолчание true, а нулевое значение bool — false. Без этого канал,
// записанный без явного "enabled", читался бы выключенным.
func (c *Channel) UnmarshalJSON(data []byte) error {
	type raw Channel
	tmp := raw{Enabled: true, InterruptExistConnections: true}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*c = Channel(tmp)
	return nil
}
