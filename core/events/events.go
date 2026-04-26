// Package events содержит типизированный sync event-bus для развязки
// «кто меняет состояние» и «кто на это реагирует».
//
// Этот пакет — реализация SPEC 046 (TYPED_EVENT_BUS) и фундаментальная
// зависимость для SPEC 045 (STATE_CONFIG_DECOUPLING).
//
// Ключевые свойства:
//   - синхронная диспетчеризация (handler выполняется в той же goroutine,
//     что вызвала Publish);
//   - panic в одном handler'е не ломает доставку остальным;
//   - Subscribe возвращает Cancel-замыкание; Cancel идемпотентен;
//   - типизированный payload — Event.Payload приводится к конкретной
//     payload-структуре по Event.Kind.
//
// Этот пакет НЕ:
//   - не сериализует Publish'ы (concurrent publish — на свой страх; порядок
//     доставки в этом случае не определён);
//   - не персистит события (нет истории);
//   - не делает request/reply (только fire-and-forget).
//
// Минимальный пример:
//
//	bus := events.NewMemoryBus()
//	cancel := bus.Subscribe(events.ConfigBuilt, func(ev events.Event) {
//	    p := ev.Payload.(events.ConfigBuiltPayload)
//	    log.Printf("config rebuilt, ok=%v", p.OK)
//	})
//	defer cancel()
//	bus.Publish(events.Event{
//	    Kind:    events.ConfigBuilt,
//	    Payload: events.ConfigBuiltPayload{OK: true},
//	})
package events

// EventKind — строго типизированный идентификатор типа события.
//
// При добавлении нового типа: добавить константу ниже, payload-структуру
// в payloads.go, обновить String() для удобства логов.
type EventKind int

const (
	// StateChanged — пользователь сохранил state.json через Configurator.
	// Payload: StateChangedPayload (см. payloads.go).
	StateChanged EventKind = iota

	// ConfigBuilt — config.json пересобран и записан на диск.
	// Payload: ConfigBuiltPayload.
	ConfigBuilt

	// SubscriptionUpdated — обработка одной подписки завершилась.
	// Payload: SubscriptionUpdatedPayload.
	SubscriptionUpdated

	// VpnStateChanged — sing-box перешёл из/в running-состояние.
	// Payload: VpnStateChangedPayload.
	VpnStateChanged

	// ProxyActiveChanged — пользователь / селектор переключил активную ноду.
	// Payload: ProxyActiveChangedPayload.
	ProxyActiveChanged

	// PowerResume — система проснулась после sleep/hibernate.
	// Payload: nil (событие без данных).
	PowerResume

	// AutoUpdateStatus — auto-update подписок что-то сообщил
	// (старт цикла, успех, последовательная ошибка).
	// Payload: AutoUpdateStatusPayload.
	AutoUpdateStatus
)

// String — человеко-читаемое имя для логов и тестов.
// Не использовать для сериализации (значения констант не стабильны между релизами).
func (k EventKind) String() string {
	switch k {
	case StateChanged:
		return "StateChanged"
	case ConfigBuilt:
		return "ConfigBuilt"
	case SubscriptionUpdated:
		return "SubscriptionUpdated"
	case VpnStateChanged:
		return "VpnStateChanged"
	case ProxyActiveChanged:
		return "ProxyActiveChanged"
	case PowerResume:
		return "PowerResume"
	case AutoUpdateStatus:
		return "AutoUpdateStatus"
	default:
		return "Unknown"
	}
}

// Event — единица передачи через bus.
// Конкретный тип Payload определяется значением Kind (см. payloads.go).
type Event struct {
	Kind    EventKind
	Payload any
}

// Handler — обработчик события. Получает событие синхронно в goroutine
// вызвавшего Publish. Не должен делать долгие операции (IO, network) —
// для тяжёлой работы запускайте свою goroutine.
type Handler func(Event)

// Cancel — отменяет подписку. Идемпотентна: повторный вызов после первого
// или после удалённой Bus безопасен.
type Cancel func()

// Bus — интерфейс event-bus'а. Основная реализация — MemoryBus.
//
// Контракт реализаций:
//   - Publish синхронен: вызывает handler'ы в той же goroutine.
//   - Subscribe / SubscribeAll thread-safe.
//   - Cancel thread-safe и идемпотентен.
//   - Panic в handler'е не должна прерывать доставку другим handler'ам.
type Bus interface {
	// Publish доставляет событие всем подписчикам этого Kind + всем,
	// кто подписался через SubscribeAll. Порядок вызова handler'ов
	// не определён (в текущей реализации — порядок Subscribe, но
	// зависеть от этого не следует).
	Publish(ev Event)

	// Subscribe регистрирует handler на конкретный Kind.
	// Возвращает Cancel — вызвать для отписки.
	Subscribe(kind EventKind, h Handler) Cancel

	// SubscribeAll регистрирует handler на ВСЕ события (любой Kind).
	// Удобно для логирования / отладки. Для production-кода
	// предпочитайте точечный Subscribe.
	SubscribeAll(h Handler) Cancel
}
