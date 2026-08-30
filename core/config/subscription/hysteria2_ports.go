package subscription

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	hysteria2URLPrefix = "hysteria2://"
	hysteriaURLPrefix  = "hysteria://"
)

// hysteria2SplitHostAndPort splits host[:portspec] after the @ fragment.
// For IPv6, host is bracketed and portspec follows "]:".
func hysteria2SplitHostAndPort(hostPort string) (host, portSpec string, ok bool) {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return "", "", false
	}
	if strings.HasPrefix(hostPort, "[") {
		close := strings.Index(hostPort, "]")
		if close < 0 {
			return "", "", false
		}
		host = hostPort[:close+1]
		if close+1 < len(hostPort) && hostPort[close+1] == ':' {
			return host, hostPort[close+2:], true
		}
		return host, "", true
	}
	colon := strings.LastIndex(hostPort, ":")
	if colon < 0 {
		return hostPort, "", true
	}
	return hostPort[:colon], hostPort[colon+1:], true
}

// hysteria2FirstNumericPortFromSpec returns the first port number in the Hysteria2 multi-port spec
// (comma-separated segments; each segment is a port or start-end range). Used to build a URL net/url accepts.
func hysteria2FirstNumericPortFromSpec(portSpec string) (int, bool) {
	portSpec = strings.TrimSpace(portSpec)
	if portSpec == "" {
		return 0, false
	}
	seg := strings.TrimSpace(strings.Split(portSpec, ",")[0])
	if seg == "" {
		return 0, false
	}
	for _, sep := range []string{"-", ":"} {
		if i := strings.Index(seg, sep); i > 0 {
			seg = seg[:i]
			break
		}
	}
	seg = strings.TrimSpace(seg)
	p, err := strconv.Atoi(seg)
	if err != nil || p < 1 || p > 65535 {
		return 0, false
	}
	return p, true
}

// hysteria2AuthorityNeedsRecovery reports whether the port part uses Hysteria multi-port syntax
// that makes net/url.Parse fail (comma list or range in the authority).
func hysteria2AuthorityNeedsRecovery(portSpec string) bool {
	portSpec = strings.TrimSpace(portSpec)
	if portSpec == "" {
		return false
	}
	return strings.Contains(portSpec, ",") || strings.Contains(portSpec, "-") || strings.Contains(portSpec, ":")
}

// hysteria2RecoverMultiPortAuthority rebuilds hysteria2:// URIs whose authority uses Hysteria multi-port
// (see https://v2.hysteria.network/docs/advanced/Port-Hopping/) so net/url can parse them.
// Returns the parseable URL, the full authority port list string for merging into mport, and nil error on success.
func hysteria2RecoverMultiPortAuthority(raw string) (*url.URL, string, error) {
	// Hysteria v1 использует тот же синтаксис порт-хоппинга в authority
	// (клиенты 1.x, «port hopping»), поэтому восстановление общее на оба
	// префикса — иначе v1-ссылка с диапазоном терялась бы целиком.
	prefix := ""
	switch {
	case strings.HasPrefix(raw, hysteria2URLPrefix):
		prefix = hysteria2URLPrefix
	case strings.HasPrefix(raw, hysteriaURLPrefix):
		prefix = hysteriaURLPrefix
	default:
		return nil, "", fmt.Errorf("not hysteria")
	}
	rest := strings.TrimPrefix(raw, prefix)
	frag := ""
	if i := strings.Index(rest, "#"); i >= 0 {
		frag = rest[i:]
		rest = rest[:i]
	}
	query := ""
	if i := strings.Index(rest, "?"); i >= 0 {
		query = rest[i:]
		rest = rest[:i]
	}
	at := strings.Index(rest, "@")
	var userinfo, hostPath string
	if at >= 0 {
		userinfo = rest[:at]
		hostPath = rest[at+1:]
	} else {
		hostPath = rest
	}
	slash := strings.Index(hostPath, "/")
	hostPortPart := hostPath
	pathSuffix := ""
	if slash >= 0 {
		hostPortPart = hostPath[:slash]
		pathSuffix = hostPath[slash:]
	}
	host, portSpec, ok := hysteria2SplitHostAndPort(hostPortPart)
	if !ok || host == "" {
		return nil, "", fmt.Errorf("invalid hysteria host")
	}
	if !hysteria2AuthorityNeedsRecovery(portSpec) {
		return nil, "", fmt.Errorf("authority does not use multi-port syntax")
	}
	n, ok := hysteria2FirstNumericPortFromSpec(portSpec)
	if !ok {
		return nil, "", fmt.Errorf("no usable port in %q", portSpec)
	}
	rebuilt := prefix
	if userinfo != "" {
		rebuilt += userinfo + "@"
	}
	rebuilt += host + ":" + strconv.Itoa(n) + pathSuffix + query + frag
	u, err := url.Parse(rebuilt)
	if err != nil {
		return nil, "", err
	}
	return u, portSpec, nil
}

// hysteria2MportSpecToSingBoxServerPorts converts Hysteria2 mport / multi-port authority string to sing-box
// server_ports ([]string of "low:high" ranges). Official format: comma-separated list of ports and start-end ranges
// (hyphen in URI; colon in sing-box).
func hysteria2MportSpecToSingBoxServerPorts(spec string) []string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pr := strings.ReplaceAll(part, "-", ":")
		if !strings.Contains(pr, ":") {
			pr = pr + ":" + pr
		}
		out = append(out, pr)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeHysteria2ServerPortsSlice normalizes sing-box server_ports entries (e.g. bare "41000" -> "41000:41000").
func NormalizeHysteria2ServerPortsSlice(ranges []string) []string {
	if len(ranges) == 0 {
		return nil
	}
	return hysteria2MportSpecToSingBoxServerPorts(strings.Join(ranges, ","))
}

// hysteria2ServerPortsToMportQuery encodes sing-box server_ports back to Hysteria2 mport query (hyphens, comma-separated).
func hysteria2ServerPortsToMportQuery(ranges []string) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		parts = append(parts, strings.ReplaceAll(r, ":", "-"))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}
