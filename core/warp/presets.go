package warp

// ObfuscationPreset — именованный профиль AmneziaWG-обфускации для WARP-конфигуратора
// (SPEC 084.2). В LxBox именованных пресетов нет (один дефолт + выбор ip), это
// десктоп-расширение: пресет заполняет поля QuicParams, юзер может дальше править
// в Advanced. Все пресеты держат WARP-безопасные s1=s2=0, h1..h4=1..4.
type ObfuscationPreset struct {
	Name   string
	Params QuicParams
}

// ObfuscationPresets — набор готовых профилей. Первый (WARP default) совпадает с
// единственным пресетом LxBox. Остальные меняют протокол masquerade (для dns/sip
// домен уходит на провод и лучше сливается с легит-трафиком) или усиливают junk.
func ObfuscationPresets() []ObfuscationPreset {
	base := DefaultQuicParams()
	dns := base
	dns.IP = "dns"
	stun := base
	stun.IP = "stun"
	sip := base
	sip.IP = "sip"
	aggressive := base
	aggressive.JC, aggressive.JMin, aggressive.JMax = 8, 50, 100

	return []ObfuscationPreset{
		{Name: "WARP default (QUIC masquerade)", Params: base},
		{Name: "Masquerade as DNS", Params: dns},
		{Name: "Masquerade as STUN", Params: stun},
		{Name: "Masquerade as SIP", Params: sip},
		{Name: "Aggressive junk (jc 8)", Params: aggressive},
	}
}

// PresetByName возвращает параметры пресета по имени (или дефолт, если не найден).
func PresetByName(name string) QuicParams {
	for _, p := range ObfuscationPresets() {
		if p.Name == name {
			return p.Params
		}
	}
	return DefaultQuicParams()
}

// MasqueSNIPool — SNI-пул для MASQUE (реальный TLS SNI QUIC/HTTP2-сессии к CF).
// В отличие от sni_pool WARP-junk, здесь допустим www.cloudflare.com (для MASQUE
// это легитимный SNI, а не палевный junk-домен).
var MasqueSNIPool = []string{
	"www.cloudflare.com", "cdn.jsdelivr.net", "aws.amazon.com", "www.google.com",
	"www.microsoft.com", "www.bing.com", "www.apple.com", "www.wikipedia.org",
	"yandex.ru", "telemost.yandex.ru", "ozon.ru", "rutube.ru", "gosuslugi.ru",
}
