package services

import (
	"sync"
	"time"

	"singbox-launcher/internal/debuglog"
)

// TransportPool — кеш gRPC-транспортов к удалённым машинам для Debug API
// (SPEC 100 §6.2).
//
// Зачем: HTTP-handler живёт один запрос, а mTLS-handshake + HTTP/2 к роутеру —
// дорогие. Диалить на каждый `GET /remote/machines/{id}/proxies` значило бы
// платить рукопожатием за каждый вызов и плодить соединения на стороне демона.
// Пул держит по транспорту на машину, переиспользует его между запросами и
// закрывает по простою.
//
// UI пул не использует: у окон свой жизненный цикл транспортов (экземпляр
// на окно, гаснет вместе с ним) — здесь же вызывающих объединяет только id
// машины.
type TransportPool struct {
	registry *RemoteRegistry

	mu      sync.Mutex
	entries map[string]*poolEntry
}

type poolEntry struct {
	transport *LxdRemoteTransport
	lastUsed  time.Time
}

// poolIdleTTL — сколько транспорт живёт без обращений. Значение перекрывает
// типичную серию запросов агента (список → узлы → переключение), но не
// держит соединение к роутеру часами после разового вопроса.
const poolIdleTTL = 90 * time.Second

// poolSweepEvery — период фонового обхода. Грубее TTL в разы: закрыть простой
// транспорт на минуту позже — не расход, держать точный таймер на каждый — да.
const poolSweepEvery = 30 * time.Second

// NewTransportPool создаёт пул поверх реестра. Фоновая горутина-уборщик
// живёт, пока жив процесс, — пул создаётся один раз на Debug API-сервер.
func NewTransportPool(registry *RemoteRegistry) *TransportPool {
	p := &TransportPool{
		registry: registry,
		entries:  map[string]*poolEntry{},
	}
	go p.sweepLoop()
	return p
}

// Get возвращает транспорт машины, создавая его при первом обращении.
// Соединением владеет пул — вызывающий НЕ закрывает транспорт.
func (p *TransportPool) Get(id string) (*LxdRemoteTransport, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[id]; ok {
		e.lastUsed = time.Now()
		return e.transport, nil
	}
	t, err := p.registry.Transport(id)
	if err != nil {
		return nil, err
	}
	p.entries[id] = &poolEntry{transport: t, lastUsed: time.Now()}
	return t, nil
}

// Invalidate закрывает и выбрасывает транспорт машины. Обязателен после
// операций, меняющих канал: удаление машины, re-pair (новый пин и ключи),
// правка адреса — иначе пул продолжит говорить со старым каналом.
func (p *TransportPool) Invalidate(id string) {
	p.mu.Lock()
	e, ok := p.entries[id]
	delete(p.entries, id)
	p.mu.Unlock()
	if ok {
		if err := e.transport.Close(); err != nil {
			debuglog.WarnLog("transport pool: close %q: %v", id, err)
		}
	}
}

// CloseAll закрывает все транспорты (останов Debug API-сервера).
func (p *TransportPool) CloseAll() {
	p.mu.Lock()
	entries := p.entries
	p.entries = map[string]*poolEntry{}
	p.mu.Unlock()
	for id, e := range entries {
		if err := e.transport.Close(); err != nil {
			debuglog.WarnLog("transport pool: close %q: %v", id, err)
		}
	}
}

func (p *TransportPool) sweepLoop() {
	ticker := time.NewTicker(poolSweepEvery)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-poolIdleTTL)
		p.mu.Lock()
		var stale []*poolEntry
		for id, e := range p.entries {
			if e.lastUsed.Before(cutoff) {
				stale = append(stale, e)
				delete(p.entries, id)
			}
		}
		p.mu.Unlock()
		// Закрытие вне мьютекса: Close ходит в grpc и не должен держать Get.
		for _, e := range stale {
			_ = e.transport.Close()
		}
	}
}
