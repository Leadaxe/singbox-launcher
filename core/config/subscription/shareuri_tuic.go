package subscription

import (
	"fmt"
	"net/url"
)

// --- TUIC ---

// shareURIFromTuic encodes a sing-box tuic outbound back into a tuic:// URI
// (reverse of buildTuicOutbound). Round-trip is by meaning, not byte-exact:
// insecure is emitted as the canonical `insecure=1` (the parser also accepts
// `allow_insecure`).
func shareURIFromTuic(out map[string]interface{}) (string, error) {
	uuid := mapGetString(out, "uuid")
	pass := mapGetString(out, "password")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if uuid == "" || pass == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: tuic needs uuid, password, server, server_port", ErrShareURINotSupported)
	}

	q := url.Values{}
	if cc := mapGetString(out, "congestion_control"); cc != "" {
		q.Set("congestion_control", cc)
	}
	if urm := mapGetString(out, "udp_relay_mode"); urm != "" {
		q.Set("udp_relay_mode", urm)
	}
	if mapGetBool(out, "zero_rtt_handshake") {
		q.Set("zero_rtt_handshake", "1")
	}
	if hb := mapGetString(out, "heartbeat"); hb != "" {
		q.Set("heartbeat", hb)
	}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		if sni := mapGetString(tls, "server_name"); sni != "" {
			q.Set("sni", sni)
		}
		shareAppendALPNInsecure(q, tls) // alpn + insecure=1
		if utls, ok := tls["utls"].(map[string]interface{}); ok {
			if fp := mapGetString(utls, "fingerprint"); fp != "" {
				q.Set("fp", fp)
			}
		}
	}
	shareAppendDetourLiteral(q, out)

	u := &url.URL{
		Scheme:   "tuic",
		User:     url.UserPassword(uuid, pass),
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
