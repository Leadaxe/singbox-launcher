package warp

import (
	"fmt"
	"math/rand"
)

// Cloudflare WARP anycast blocks. Almost any IP in these /24s on any port from
// the list terminates a live WARP endpoint; the DPI only blocks the default
// engage.cloudflareclient.com:2408 by name. Picking a random ip:port here
// sidesteps that without an active endpoint scan (LxBox does the same). Kept in
// sync with LxBox app/assets/warp_endpoints.json.
var endpointPrefixes = []string{
	"162.159.192.",
	"162.159.195.",
	"188.114.96.",
	"188.114.97.",
	"188.114.98.",
}

var endpointPorts = []int{
	500, 854, 859, 864, 878, 880, 890, 891, 894, 903, 908, 928, 934, 939,
	942, 943, 945, 946, 955, 968, 987, 988, 1002, 1010, 1014, 1018, 1070,
	1074, 1180, 1387, 1701, 1843, 2371, 2408, 2506, 3138, 3476, 3581, 3854,
	4177, 4198, 4233, 4500, 5279, 5956, 7103, 7152, 7156, 7281, 7559, 8319,
	8742, 8854, 8886,
}

// SNIPool holds masquerade SNI candidates (RU + international) for the AWG
// id/ip=quic masquerade domain. Exposed so the UI can offer a random SNI.
var SNIPool = []string{
	"www.google.com", "www.microsoft.com", "www.bing.com", "www.apple.com",
	"www.wikipedia.org", "cdn.jsdelivr.net", "aws.amazon.com", "yandex.ru",
	"telemost.yandex.ru", "ozon.ru", "rutube.ru", "gosuslugi.ru",
}

// RandomEndpoint returns a random "ip:port" from the anycast pool. The last
// octet is 1..10 (WARP responds across the low host range). A nil rng falls
// back to the package default source; pass a seeded *rand.Rand in tests for
// determinism.
func RandomEndpoint(rng *rand.Rand) string {
	pick := func(n int) int {
		if rng != nil {
			return rng.Intn(n)
		}
		return rand.Intn(n)
	}
	prefix := endpointPrefixes[pick(len(endpointPrefixes))]
	octet := pick(10) + 1
	port := endpointPorts[pick(len(endpointPorts))]
	return fmt.Sprintf("%s%d:%d", prefix, octet, port)
}

// RandomSNI returns a random masquerade SNI from SNIPool.
func RandomSNI(rng *rand.Rand) string {
	if rng != nil {
		return SNIPool[rng.Intn(len(SNIPool))]
	}
	return SNIPool[rand.Intn(len(SNIPool))]
}
