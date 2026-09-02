package subscription

import "strings"

// Коды деградации на узле (SPEC 103, фаза 2).
//
// До этого деградация уходила только в debuglog: пользователь видел «нода
// есть» и не знал, что у неё срезали обфускацию или заменили отпечаток, а
// контракт не мог строго сверять поведение двух приложений — сверять текст
// лога бессмысленно.
//
// Имена констант зеркалят contract/registry/warnings.json. Значение и есть
// код: он попадает в конверт корпуса и (в дальнейшем) в UI.
//
// Коды ставятся ТАМ, ГДЕ УЗЕЛ ПОД РУКОЙ. Нормализаторы (normalizeRealityShortID,
// canonicalUTLSFingerprint) остаются чистыми функциями: они вызываются из
// нескольких мест, включая пути без узла (санитайзер конфига), и протаскивать
// через них *ParsedNode ради диагностики значило бы переписать пол-пакета.
// Вместо этого рядом с нормализатором живёт предикат «значение будет
// испорчено», и вызывающий сам помечает узел.

const (
	// WarnRealityShortIDInvalid — sid вне hex или нечётной длины: снимается
	// целиком (укороченный sid — это ДРУГОЙ short_id, узел молча ломается).
	WarnRealityShortIDInvalid = "reality_short_id_invalid"
	// WarnUTLSFingerprintUnknown — отпечаток вне словаря ядра заменён на
	// канонический; чужое значение валит все outbound'ы.
	WarnUTLSFingerprintUnknown = "utls_fp_unknown"
	// WarnObfsUnknown — тип hysteria2-обфускации вне словаря ядра снят.
	WarnObfsUnknown = "obfs_unknown"
	// WarnObfsPasswordMissing — обфускация без пароля снята целиком: ядро
	// отвергает такой узел и роняет ВЕСЬ конфиг.
	WarnObfsPasswordMissing = "obfs_password_missing"
	// WarnPacketEncodingUnknown — packet_encoding вне словаря снят.
	WarnPacketEncodingUnknown = "packet_encoding_unknown"
	// WarnSSMethodInvalid — метод shadowsocks вне словаря ядра.
	WarnSSMethodInvalid = "ss_method_invalid"
	// WarnPortInvalid — порт вне 1..65535 заменён значением по умолчанию.
	WarnPortInvalid = "port_invalid"
	// WarnSSHUserDefault — ssh без пользователя: подставлен root.
	WarnSSHUserDefault = "ssh_user_default"
	// WarnNaivePaddingIgnored — naive padding=… не поддержан ядром.
	WarnNaivePaddingIgnored = "naive_padding_ignored"
	// WarnAmneziaContainerChoice — в vpn://-профиле несколько контейнеров,
	// одиночный путь взял дефолтный.
	WarnAmneziaContainerChoice = "amnezia_container_choice"
	// WarnAWGHeaderInvalid — AmneziaWG H1–H4 вне допустимого диапазона.
	WarnAWGHeaderInvalid = "awg_header_invalid"
	// WarnAWGHeadersOverlap — H1–H4 совпадают между собой.
	WarnAWGHeadersOverlap = "awg_headers_overlap"
	// WarnTuicCongestionInvalid — контроль перегрузки TUIC вне словаря.
	WarnTuicCongestionInvalid = "tuic_congestion_invalid"
	// WarnTuicUDPRelayModeInvalid — udp_relay_mode TUIC вне словаря.
	WarnTuicUDPRelayModeInvalid = "tuic_udp_relay_mode_invalid"
	// WarnAnyTLSMinIdleInvalid — min_idle_session не число.
	WarnAnyTLSMinIdleInvalid = "anytls_min_idle_invalid"
	// WarnMasqueVHTTPInvalid — masque vhttp-параметр не разобран.
	WarnMasqueVHTTPInvalid = "masque_vhttp_invalid"
	// WarnWSEarlyDataEDConverted — Xray-хвост ?ed=N разложен в
	// max_early_data + early_data_header_name.
	WarnWSEarlyDataEDConverted = "ws_early_data_converted"
	// WarnDialerProxyUnusable — цель streamSettings.sockopt.dialerProxy
	// непригодна: узел-владелец отбраковывается ЦЕЛИКОМ. Кода на узле не
	// бывает (узла не будет) — он едет в отбраковке, поэтому severity=error.
	WarnDialerProxyUnusable = "dialer_proxy_unusable"
)

// realityShortIDWouldDegrade сообщает, что нормализация ПОТЕРЯЕТ данные
// short_id: значение непустое, но после чистки обнулится или укоротится.
//
// Приведение регистра (ABCD → abcd) деградацией НЕ считается: hex
// регистронезависим, sing-box декодирует одинаково, и помечать такой узел
// значило бы кричать на каждую вторую reality-ноду.
//
// Проверяется ДО нормализации — после неё исходное значение уже потеряно.
func realityShortIDWouldDegrade(raw string) bool {
	if raw == "" {
		return false
	}
	return normalizeRealityShortID(raw) != strings.ToLower(strings.TrimSpace(raw))
}

// utlsFingerprintWouldDegrade сообщает, что отпечаток будет заменён
// каноническим (normalizeUTLSFingerprintEx уже отвечает на этот вопрос
// вторым значением — «мусор»).
func utlsFingerprintWouldDegrade(raw string) bool {
	if raw == "" {
		return false
	}
	_, junk := normalizeUTLSFingerprintEx(raw)
	return junk
}
