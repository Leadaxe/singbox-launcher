// Package debugapi — SPEC 100 §3.3: зеркала /state/* на профиль удалённой
// машины. Тела хендлеров общие (stateAccess в state_endpoints.go); здесь —
// только резолв файла машины и per-machine мьютекс.
//
// Известное ограничение v1 (SPEC 100 §3.3): PATCH меняет state машины, но её
// config.json пересобирает только визард (Configure → Save) — программной
// сборки remote-конфига пока нет, поэтому цикл «PATCH → deploy» требует шага
// Save в UI. Endpoint action/rebuild-config появится вместе с выносом сборки
// из презентера.
package debugapi

import (
	"net/http"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// machineStatePath — state.json профиля машины (SPEC 098 layout).
func (s *Server) machineStatePath(id string) string {
	return platform.GetWizardStatePathFor(s.remote.ExecDir, constants.ConfigTargetRemote, id)
}

// machineStateAccess — stateAccess профиля машины.
//
// Save пишет файл машины напрямую, БЕЗ dirty-маркеров StateService: маркеры —
// про локальный config.json, а конфиг машины собирает её визард. Мьютекс —
// per-machine: PATCH двух разных машин не должен выстраиваться в очередь.
func (s *Server) machineStateAccess(id string) stateAccess {
	path := s.machineStatePath(id)
	return stateAccess{
		load: func() (*state.State, error) { return state.Load(path) },
		save: func(st *state.State) error { return st.Save(path) },
		mu:   s.machineMutex(id),
	}
}

func (s *Server) handleRemoteStateFull(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateFullWith(w, r, s.machineStateAccess(id))
	}
}

func (s *Server) handleRemoteStateRules(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateRulesWith(w, r, s.machineStateAccess(id))
	}
}

func (s *Server) handleRemoteStateDNS(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateDNSWith(w, r, s.machineStateAccess(id))
	}
}

func (s *Server) handleRemoteStateDNSRules(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateDNSRulesWith(w, r, s.machineStateAccess(id))
	}
}

func (s *Server) handleRemoteStateOutboundsResolved(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateOutboundsResolvedWith(w, r, s.machineStateAccess(id))
	}
}
