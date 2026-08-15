//go:build !go1.22

package debugapi

import (
	"net/http"
	"strings"
)

// pathParam на тулчейне go1.20 (win7-легаси, go.win7.mod).
//
// ServeMux до Go 1.22 не умеет {wildcard}-паттерны, поэтому маршруты с {id}
// на этой сборке регистрируются литералами и фактически недостижимы — но
// пакет обязан КОМПИЛИРОВАТЬСЯ, иначе падает вся сборка лаунчера. Параметр
// достаём позиционно: сегмент после якорного ("machines" → id и т.д.), чтобы
// поведение осталось осмысленным, если запрос всё же дойдёт до обработчика.
func pathParam(r *http.Request, name string) string {
	anchor, ok := map[string]string{
		"id":      "machines",
		"name":    "resources",
		"conn_id": "connections",
		"key":     "clients",
	}[name]
	if !ok {
		return ""
	}
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, s := range segs {
		if s == anchor && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	return ""
}
