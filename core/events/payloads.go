package events

// Payload-структуры — конкретные типы для Event.Payload по Kind.
//
// При получении события подписчик приводит Payload к ожидаемому типу:
//
//	bus.Subscribe(events.StateChanged, func(ev events.Event) {
//	    p, ok := ev.Payload.(events.StateChangedPayload)
//	    if !ok { return } // защита от Publish с некорректным payload'ом
//	    // ... использовать p.Diff
//	})
//
// При добавлении новой константы EventKind — добавить здесь
// соответствующий *Payload и закомментировать связь.

// StateChangedPayload сопровождает Kind StateChanged.
//
// Поле Diff пока хранится как opaque-карта строк; конкретный тип
// `core/state.Diff` ещё не существует (см. фазу 3.2 SPEC 045).
// После появления этого пакета — заменить на `state.Diff`.
type StateChangedPayload struct {
	// Changed — список «доменов» состояния, которые поменялись.
	// Пока — строковые ярлыки ("proxies", "tun", "dns", "rules", "vars", ...).
	// Будут типизированы после реализации core/state.
	Changed []string
}

// ConfigBuiltPayload сопровождает Kind ConfigBuilt.
type ConfigBuiltPayload struct {
	// OK — true, если build + sing-box check прошли, файл записан.
	OK bool
	// Error — заполнено при OK == false.
	Error error
	// Warnings — non-fatal предупреждения от build/validate.
	Warnings []string
}

// SubscriptionUpdatedPayload сопровождает Kind SubscriptionUpdated.
type SubscriptionUpdatedPayload struct {
	SourceTag string
	Succeeded int
	Failed    int
}

// VpnStateChangedPayload сопровождает Kind VpnStateChanged.
type VpnStateChangedPayload struct {
	Running bool
}

// ProxyActiveChangedPayload сопровождает Kind ProxyActiveChanged.
type ProxyActiveChangedPayload struct {
	GroupTag    string
	NewSelected string
}

// AutoUpdateStatusPayload сопровождает Kind AutoUpdateStatus.
type AutoUpdateStatusPayload struct {
	// Stage — этап цикла: "started" / "succeeded" / "failed" / "disabled".
	Stage string
	// ConsecutiveFailures — текущее значение счётчика подряд-ошибок (0 при успехе).
	ConsecutiveFailures int
	// LastError — заполнено при Stage=="failed".
	LastError error
}
