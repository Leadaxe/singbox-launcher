package subscription

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// --- MASQUE ---

// shareURIFromMasque encodes a sing-box masque outbound back into a masque://
// URI (reverse of parseMasqueURI). Round-trip is by meaning: the private key is
// the userinfo, the server key is publickey=, and ip/ipv6 join into address=.
func shareURIFromMasque(out map[string]interface{}) (string, error) {
	priv := mapGetString(out, "private_key")
	pub := mapGetString(out, "public_key")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if priv == "" || pub == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: masque needs private_key, public_key, server, server_port", ErrShareURINotSupported)
	}

	addrs := make([]string, 0, 2)
	if ip := mapGetString(out, "ip"); ip != "" {
		addrs = append(addrs, ip)
	}
	if ip6 := mapGetString(out, "ipv6"); ip6 != "" {
		addrs = append(addrs, ip6)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("%w: masque needs ip or ipv6", ErrShareURINotSupported)
	}

	q := url.Values{}
	q.Set("publickey", pub)
	q.Set("address", strings.Join(addrs, ","))
	if profile := mapGetString(out, "profile"); profile != "" {
		q.Set("profile", profile)
	}
	network := mapGetString(out, "network")
	if network == "" {
		network = "h3"
	}
	q.Set("network", network)
	if mtu := mapGetInt(out, "mtu"); mtu > 0 {
		q.Set("mtu", strconv.Itoa(mtu))
	}
	if sni := mapGetString(out, "sni"); sni != "" {
		q.Set("sni", sni)
	}
	if it := mapGetString(out, "idle_timeout"); it != "" {
		q.Set("idle_timeout", it)
	}
	if ka := mapGetString(out, "keep_alive_period"); ka != "" {
		q.Set("keep_alive", ka)
	}

	u := &url.URL{
		Scheme:   "masque",
		User:     url.User(priv),
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
