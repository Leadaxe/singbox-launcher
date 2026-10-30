// Package debugapi — SPEC 100 §3.3: зеркала /state/* на профиль удалённой
// машины. Тела хендлеров общие (stateAccess в state_endpoints.go); здесь —
// только резолв файла машины и per-machine мьютекс.
//
// Правка state машины её config.json не трогает (config_rebuilt:false): конфиг
// машины собирается шагом Save её Мастера, и через API этот шаг —
// POST /remote/machines/{id}/config/rebuild (issue #152), а не побочный эффект
// каждой правки.
package debugapi

import (
	"net/http"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// machineStatePath — state.json профиля машины (SPEC 098 layout).
func (s *Server) machineStatePath(id string) string {
	return platform.GetWizardStatePathFor(s.remote.DataDir, constants.ConfigTargetRemote, id)
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
		// Успешная запись извещает окно Конфигуратора этой машины (SPEC 160 §E).
		save: func(st *state.State) error {
			if err := st.Save(path); err != nil {
				return err
			}
			s.facade.NotifyStateChanged(constants.ConfigTargetRemote, id)
			return nil
		},
		mu:   s.machineMutex(id),
		path: path,
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

// SPEC 160: CRUD-точки по одной записи. POST/DELETE /state/rules и
// /state/dns/rules обслуживают те же хендлеры, что GET/PATCH выше.
func (s *Server) handleRemoteStateServers(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateServersWith(w, r, s.machineStateAccess(id))
	}
}

func (s *Server) handleRemoteStateFolders(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateFoldersWith(w, r, s.machineStateAccess(id))
	}
}

func (s *Server) handleRemoteStateDNSServers(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.stateDNSServersWith(w, r, s.machineStateAccess(id))
	}
}
