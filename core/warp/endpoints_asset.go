package warp

// endpoints_asset.go — единый источник истины для WARP/MASQUE пулов. Данные
// живут в embedded assets/warp_endpoints.json (копия LxBox app/assets/
// warp_endpoints.json), а не в Go-литералах — чтобы обновление пула было правкой
// одного JSON, синхронной с LxBox, а не хардкода. Паттерн повторяет
// internal/locale/locale.go: embed → parse в init() → fail loud на ошибке.

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed assets/warp_endpoints.json
var warpEndpointsJSON []byte

// warpPools — форма ассета. Лишние ключи (напр. "_comment") json.Unmarshal
// молча игнорирует.
type warpPools struct {
	Prefixes      []string `json:"prefixes"`
	Ports         []int    `json:"ports"`
	SNIPool       []string `json:"sni_pool"`
	MasqueSNIPool []string `json:"masque_sni_pool"`
}

// Пулы, заполняемые из ассета в init(). endpointPrefixes/endpointPorts —
// unexported (инкапсулированы RandomEndpoint). SNIPool/MasqueSNIPool exported:
// UI кормит ими комбобоксы (SelectEntry) напрямую — тип []string сохранён, так
// что внешние call-site'ы не меняются.
var (
	endpointPrefixes []string
	endpointPorts    []int
	// SNIPool — masquerade SNI кандидаты (RU + international) для AWG id/ip=quic
	// masquerade-домена. Комбобокс WG-секции WARP-конфигуратора.
	SNIPool []string
	// MasqueSNIPool — SNI-пул для MASQUE (реальный TLS SNI QUIC/HTTP2-сессии к
	// CF; здесь допустим www.cloudflare.com — легит SNI, а не junk-домен). При
	// отсутствии masque_sni_pool в ассете падает обратно на SNIPool.
	MasqueSNIPool []string
)

func init() {
	p := mustParseWarpPools(warpEndpointsJSON)
	endpointPrefixes = p.Prefixes
	endpointPorts = p.Ports
	SNIPool = p.SNIPool
	MasqueSNIPool = resolveMasquePool(p.MasqueSNIPool, p.SNIPool)

	// Fail loud: пустой пул → RandomEndpoint/RandomSNI паникнут на Intn(0) при
	// первом 🎲. Битый/урезанный embedded ассет — ошибка сборки, а не runtime;
	// лучше упасть на старте, чем в середине подключения.
	if len(endpointPrefixes) == 0 || len(endpointPorts) == 0 || len(SNIPool) == 0 {
		panic(fmt.Sprintf("warp: warp_endpoints.json has empty pool(s): prefixes=%d ports=%d sni=%d",
			len(endpointPrefixes), len(endpointPorts), len(SNIPool)))
	}
}

// mustParseWarpPools парсит ассет или паникует (authoring-ошибка, не runtime).
func mustParseWarpPools(data []byte) warpPools {
	var p warpPools
	if err := json.Unmarshal(data, &p); err != nil {
		panic(fmt.Sprintf("warp: parse warp_endpoints.json: %v", err))
	}
	return p
}

// resolveMasquePool возвращает masque-пул, либо fallback на общий sni-пул, если
// masque_sni_pool в ассете пуст/отсутствует (контракт LxBox §130).
func resolveMasquePool(masque, sni []string) []string {
	if len(masque) == 0 {
		return sni
	}
	return masque
}
