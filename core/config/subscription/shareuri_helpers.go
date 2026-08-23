package subscription

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func shareAppendDetourLiteral(q url.Values, out map[string]interface{}) {
	if q == nil || out == nil {
		return
	}
	if d := strings.TrimSpace(mapGetString(out, "detour")); d != "" {
		q.Set("detour", d)
	}
}

func mapGetString(m map[string]interface{}, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

func mapGetInt(m map[string]interface{}, k string) int {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func mapGetBool(m map[string]interface{}, k string) bool {
	v, ok := m[k]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

func fragmentFromTag(out map[string]interface{}) string {
	return mapGetString(out, "tag")
}

func hostPort(server string, port int) string {
	if server == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(server, strconv.Itoa(port))
}

// --- transport → query (VLESS / Trojan) ---

func transportToQuery(q url.Values, tr map[string]interface{}) {
	if len(tr) == 0 {
		return
	}
	typ := strings.ToLower(strings.TrimSpace(mapGetString(tr, "type")))
	switch typ {
	case "ws":
		q.Set("type", "ws")
		// Re-encode WebSocket early data back into the Xray `?ed=N` path tail so a
		// node → share-link → node round-trip preserves it (issue #96).
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", appendEarlyDataToPath(p, mapGetInt(tr, "max_early_data")))
		}
		if h, ok := tr["headers"].(map[string]interface{}); ok {
			if host := mapGetString(h, "Host"); host != "" {
				q.Set("host", host)
			}
		}
	case "grpc":
		q.Set("type", "grpc")
		if sn := mapGetString(tr, "service_name"); sn != "" {
			q.Set("serviceName", sn)
		} else if p := mapGetString(tr, "path"); p != "" {
			q.Set("serviceName", p)
		}
	case "http":
		q.Set("type", "http")
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", p)
		}
		if hv := tr["host"]; hv != nil {
			switch h := hv.(type) {
			case []interface{}:
				if len(h) > 0 {
					q.Set("host", mapGetString(map[string]interface{}{"x": h[0]}, "x"))
				}
			case []string:
				if len(h) > 0 {
					q.Set("host", h[0])
				}
			case string:
				if h != "" {
					q.Set("host", h)
				}
			}
		}
	case "httpupgrade":
		// SPEC 071: httpupgrade is its own type — previously mislabeled as xhttp.
		q.Set("type", "httpupgrade")
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", p)
		}
		if h := mapGetString(tr, "host"); h != "" {
			q.Set("host", h)
		}
	case "xhttp":
		// SPEC 071 base + SPEC 002 v2: Xray splithttp transport, round-tripped
		// verbatim. Keys are written snake_case (the parser reads snake_case and
		// camelCase alike), so parseUri(toUri(spec)) ≈ spec. Only non-empty
		// fields are written — an absent field and a field-at-default decode to
		// the same spec.
		q.Set("type", "xhttp")
		if m := mapGetString(tr, "mode"); m != "" {
			q.Set("mode", m)
		}
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", p)
		}
		if h := mapGetString(tr, "host"); h != "" {
			q.Set("host", h)
		}
		if pad := mapGetString(tr, "x_padding_bytes"); pad != "" {
			q.Set("x_padding_bytes", pad)
		}
		// Таблицы полей — те же, что читает парсер (эмиттер и парсер ходят
		// парой): булевы флаги, строки, диапазоны, int-поля и вложенный xmux.
		// До SPEC 102-фикса эмиттер знал только старый набор — xmux,
		// sc_max_buffered_posts и no_sse_header терялись при «Скопировать
		// ссылку», и импортировавший получал узел с другим XHTTP-тюнингом.
		for _, f := range xhttpBoolFields {
			if v, ok := tr[f.jsonKey].(bool); ok && v {
				q.Set(f.jsonKey, "true")
			}
		}
		for _, f := range xhttpStringFields {
			if val := mapGetString(tr, f.jsonKey); val != "" {
				q.Set(f.jsonKey, val)
			}
		}
		for _, f := range xhttpRangeFields {
			if val := mapGetString(tr, f.jsonKey); val != "" {
				q.Set(f.jsonKey, val)
			}
		}
		for _, f := range xhttpIntFields {
			if val := xhttpNumString(tr[f.jsonKey]); val != "" {
				q.Set(f.jsonKey, val)
			}
		}
		if xmux, ok := tr["xmux"].(map[string]interface{}); ok {
			for _, f := range xhttpXmuxFields {
				if val := mapGetString(xmux, f.jsonKey); val != "" {
					q.Set(f.jsonKey, val)
				}
			}
			for _, f := range xhttpXmuxIntFields {
				if val := xhttpNumString(xmux[f.jsonKey]); val != "" {
					q.Set(f.jsonKey, val)
				}
			}
		}
	}
}

// xhttpNumString — числовое значение транспорта в строку query-параметра.
// Число приходит int64 из парсера, но float64/json.Number после JSON
// round-trip'а — все формы обязаны эмититься одинаково.
func xhttpNumString(v interface{}) string {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case json.Number:
		return n.String()
	}
	return ""
}

func shareAppendALPNInsecure(q url.Values, tls map[string]interface{}) {
	if alpn, ok := tls["alpn"].([]interface{}); ok && len(alpn) > 0 {
		parts := make([]string, 0, len(alpn))
		for _, a := range alpn {
			s := mapGetString(map[string]interface{}{"v": a}, "v")
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			q.Set("alpn", strings.Join(parts, ","))
		}
	} else if alpn, ok := tls["alpn"].([]string); ok && len(alpn) > 0 {
		q.Set("alpn", strings.Join(alpn, ","))
	}
	if mapGetBool(tls, "insecure") {
		q.Set("insecure", "1")
	}
}
