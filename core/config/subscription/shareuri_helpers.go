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
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", p)
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
		// SPEC 071: Xray splithttp transport, round-tripped verbatim.
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
		if v, ok := tr["no_grpc_header"].(bool); ok && v {
			q.Set("no_grpc_header", "true")
		}
	}
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
