package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// --- VMess ---

func shareURIFromVMess(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	id := mapGetString(out, "uuid")
	tag := fragmentFromTag(out)
	if server == "" || port <= 0 || id == "" {
		return "", fmt.Errorf("%w: vmess needs server, server_port, uuid", ErrShareURINotSupported)
	}
	vm := map[string]interface{}{
		"v":    "2",
		"ps":   tag,
		"add":  server,
		"port": port,
		"id":   id,
		"aid":  0,
		"scy":  mapGetString(out, "security"),
		"net":  "tcp",
		"type": "none",
		"host": "",
		"path": "",
		"tls":  "",
	}
	if vm["scy"] == "" || vm["scy"] == nil {
		vm["scy"] = "auto"
	}
	if aid := mapGetInt(out, "alter_id"); aid > 0 {
		vm["aid"] = aid
	}
	if tr, ok := out["transport"].(map[string]interface{}); ok {
		net := strings.ToLower(mapGetString(tr, "type"))
		vm["net"] = net
		switch net {
		case "ws":
			vm["path"] = mapGetString(tr, "path")
			if h, ok := tr["headers"].(map[string]interface{}); ok {
				vm["host"] = mapGetString(h, "Host")
			}
		case "grpc":
			vm["path"] = mapGetString(tr, "service_name")
			if vm["path"] == "" {
				vm["path"] = mapGetString(tr, "path")
			}
		case "http":
			vm["path"] = mapGetString(tr, "path")
			if hv := tr["host"]; hv != nil {
				switch h := hv.(type) {
				case []interface{}:
					if len(h) > 0 {
						vm["host"] = mapGetString(map[string]interface{}{"x": h[0]}, "x")
					}
				case []string:
					if len(h) > 0 {
						vm["host"] = h[0]
					}
				}
			}
		case "httpupgrade":
			vm["net"] = "ws"
			vm["path"] = mapGetString(tr, "path")
			vm["host"] = mapGetString(tr, "host")
		}
	}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		if mapGetBool(tls, "enabled") {
			vm["tls"] = "tls"
			sni := mapGetString(tls, "server_name")
			if sni == "" {
				sni = server
			}
			vm["sni"] = sni
			if alpn, ok := tls["alpn"].([]interface{}); ok && len(alpn) > 0 {
				parts := make([]string, 0, len(alpn))
				for _, a := range alpn {
					parts = append(parts, mapGetString(map[string]interface{}{"v": a}, "v"))
				}
				vm["alpn"] = strings.Join(parts, ",")
			} else if alpn, ok := tls["alpn"].([]string); ok && len(alpn) > 0 {
				vm["alpn"] = strings.Join(alpn, ",")
			}
			if utls, ok := tls["utls"].(map[string]interface{}); ok {
				if fp := mapGetString(utls, "fingerprint"); fp != "" {
					vm["fp"] = fp
				}
			}
			if mapGetBool(tls, "insecure") {
				vm["insecure"] = "1"
			}
		}
	}
	raw, err := json.Marshal(vm)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(raw), nil
}
