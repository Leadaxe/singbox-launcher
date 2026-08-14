package services

import (
	"testing"

	"singbox-launcher/api"
)

// SPEC 098: у вкладок Local и Remote независимые состояния списка.
//
// До разделения поля были одни на всё приложение: перейдя на Remote,
// пользователь видел узлы локального ядра, пока не нажмёт Start на сервере, а
// возврат на Local показывал уже данные машины. Разделение виджетов этого не
// лечило — данные оставались общими.
func newScopedService() *APIService {
	return &APIService{
		scope: ScopeLocal,
		scopes: map[ProxyScope]*proxyScopeState{
			ScopeLocal:  newProxyScopeState(),
			ScopeRemote: newProxyScopeState(),
		},
	}
}

func TestProxyScopesHoldIndependentState(t *testing.T) {
	svc := newScopedService()

	svc.SetProxyScope(ScopeLocal)
	svc.SetProxiesList([]api.ProxyInfo{{Name: "local-node"}})
	svc.SetSelectedClashGroup("proxy-out")
	svc.SetActiveProxyName("local-node")

	svc.SetProxyScope(ScopeRemote)
	svc.SetProxiesList([]api.ProxyInfo{{Name: "router-node-a"}, {Name: "router-node-b"}})
	svc.SetSelectedClashGroup("router-group")
	svc.SetActiveProxyName("router-node-b")

	// Возврат на Local обязан вернуть ровно то, что там было.
	svc.SetProxyScope(ScopeLocal)
	list := svc.GetProxiesList()
	if len(list) != 1 || list[0].Name != "local-node" {
		t.Errorf("local list clobbered by remote: %+v", list)
	}
	if g := svc.GetSelectedClashGroup(); g != "proxy-out" {
		t.Errorf("local group = %q, want proxy-out", g)
	}
	if n := svc.GetActiveProxyName(); n != "local-node" {
		t.Errorf("local active proxy = %q, want local-node", n)
	}

	svc.SetProxyScope(ScopeRemote)
	if list := svc.GetProxiesList(); len(list) != 2 {
		t.Errorf("remote list lost: %+v", list)
	}
	if g := svc.GetSelectedClashGroup(); g != "router-group" {
		t.Errorf("remote group = %q, want router-group", g)
	}
}

// Сброс области не должен задевать соседнюю: снятие транспорта машины
// очищает только её список.
func TestResetScopeLeavesOtherScopeIntact(t *testing.T) {
	svc := newScopedService()

	svc.SetProxyScope(ScopeLocal)
	svc.SetProxiesList([]api.ProxyInfo{{Name: "local-node"}})
	svc.SetProxyScope(ScopeRemote)
	svc.SetProxiesList([]api.ProxyInfo{{Name: "router-node"}})

	svc.ResetScope(ScopeRemote)

	if list := svc.GetProxiesList(); len(list) != 0 {
		t.Errorf("remote scope must be empty after reset: %+v", list)
	}
	svc.SetProxyScope(ScopeLocal)
	if list := svc.GetProxiesList(); len(list) != 1 || list[0].Name != "local-node" {
		t.Errorf("local scope damaged by remote reset: %+v", list)
	}
}

// Запомненный по группе выбор и ошибки пинга тоже принадлежат области:
// одинаковые имена групп на разных машинах не должны смешиваться.
func TestPerGroupSelectionIsScoped(t *testing.T) {
	svc := newScopedService()

	svc.SetProxyScope(ScopeLocal)
	svc.SetLastSelectedProxyForGroup("proxy-out", "local-pick")
	svc.SetLastPingError("shared-name", "local failure")

	svc.SetProxyScope(ScopeRemote)
	svc.SetLastSelectedProxyForGroup("proxy-out", "remote-pick")
	if got := svc.GetLastSelectedProxyForGroup("proxy-out"); got != "remote-pick" {
		t.Errorf("remote pick = %q, want remote-pick", got)
	}
	if got := svc.GetLastPingError("shared-name"); got != "" {
		t.Errorf("remote scope inherited local ping error: %q", got)
	}

	svc.SetProxyScope(ScopeLocal)
	if got := svc.GetLastSelectedProxyForGroup("proxy-out"); got != "local-pick" {
		t.Errorf("local pick overwritten: %q", got)
	}
	if got := svc.GetLastPingError("shared-name"); got != "local failure" {
		t.Errorf("local ping error lost: %q", got)
	}
}
