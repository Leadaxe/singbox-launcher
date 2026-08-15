//go:build go1.22

package debugapi

import "net/http"

// pathParam — {wildcard} из паттерна маршрута (ServeMux Go 1.22+).
//
// Единственная точка обращения к r.PathValue: win7-сборка идёт тулчейном
// go1.20 (go.win7.mod), где ни PathValue, ни wildcard-паттернов ещё нет, —
// см. парный pathparam_legacy.go.
func pathParam(r *http.Request, name string) string {
	return r.PathValue(name)
}
