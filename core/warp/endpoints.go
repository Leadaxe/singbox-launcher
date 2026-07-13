package warp

import (
	"fmt"
	"math/rand"
)

// Cloudflare WARP anycast blocks and SNI pools live in embedded
// assets/warp_endpoints.json (see endpoints_asset.go), copied from LxBox
// app/assets/warp_endpoints.json. Almost any IP in those /24s on any port from
// the list terminates a live WARP endpoint; the DPI only blocks the default
// engage.cloudflareclient.com:2408 by name. Picking a random ip:port here
// sidesteps that without an active endpoint scan (LxBox does the same).

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

// RandomSNI returns a random masquerade SNI from SNIPool (WG/AWG junk domain).
func RandomSNI(rng *rand.Rand) string {
	return pickString(SNIPool, rng)
}

// RandomMasqueSNI returns a random SNI from MasqueSNIPool (the real TLS SNI of
// the MASQUE QUIC/HTTP session — distinct pool from the AWG junk SNIPool).
func RandomMasqueSNI(rng *rand.Rand) string {
	return pickString(MasqueSNIPool, rng)
}

func pickString(pool []string, rng *rand.Rand) string {
	if len(pool) == 0 {
		return ""
	}
	if rng != nil {
		return pool[rng.Intn(len(pool))]
	}
	return pool[rand.Intn(len(pool))]
}
