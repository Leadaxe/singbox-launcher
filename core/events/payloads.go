package events

// Payload-структуры — конкретные типы для Event.Payload по Kind.
//
// При получении события подписчик приводит Payload к ожидаемому типу:
//
//	bus.Subscribe(events.StateChanged, func(ev events.Event) {
//	    p, ok := ev.Payload.(events.StateChangedPayload)
//	    if !ok { return } // защита от Publish с некорректным payload'ом
//	    // ... использовать p.Changed
//	})
//
// При добавлении новой константы EventKind — добавить здесь соответствующий *Payload.

// StateChangedPayload сопровождает Kind StateChanged.
type StateChangedPayload struct {
	// Changed — список «доменов» состояния, которые поменялись
	// ("proxies", "tun", "dns", "rules", "vars", ...).
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

// VpnStateChangedPayload сопровождает Kind VpnStateChanged.
type VpnStateChangedPayload struct {
	Running bool
}
