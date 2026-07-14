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

// MasqueSNIPool живёт в endpoints_asset.go (загружается из embedded
// warp_endpoints.json, fallback на SNIPool при отсутствии masque_sni_pool).
