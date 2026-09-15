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
	// WarnRealityFPNotChrome — у узла эмитится reality с явным uTLS-отпечатком
	// не из chrome-семейства (D-119). Отпечаток уходит в конфиг как есть.
	//
	// Зачем сказать: REALITY-сервер Xray ≥ v26.9.8 требует в ClientHello
	// key_share X25519MLKEM768, который несут только chrome-спеки uTLS, и без
	// него МОЛЧА уводит соединение на камуфляжный сайт. Отпечаток — выбор
	// подписки, лаунчер его не подменяет; если соединение не устанавливается —
	// стоит попробовать chrome. Пустой fp и
	// наш дефолт `random` под код не попадают (см. realityFingerprintRisky).
	WarnRealityFPNotChrome = "reality_fp_not_chrome"
	// WarnNaiveExtraHeadersInvalid — пара из naive `extra-headers` отброшена:
	// нет ':', запрещённые символы в имени или CR/LF/NUL в значении.
	//
	// Прочие пары той же ссылки живут, узел живёт — отсюда severity=info; но
	// до этого отброс уходил только в debuglog, и в отчёте сборки человек не
	// видел, что заголовок, которым он открывает доступ, до сервера не доедет.
	WarnNaiveExtraHeadersInvalid = "naive_extra_headers_invalid"
	// WarnObfsUnknown — тип hysteria2-обфускации вне словаря ядра снят.
	WarnObfsUnknown = "obfs_unknown"
	// WarnObfsPasswordMissing — обфускация без пароля снята целиком: ядро
	// отвергает такой узел и роняет ВЕСЬ конфиг.
	WarnObfsPasswordMissing = "obfs_password_missing"
	// WarnPacketEncodingUnknown — packet_encoding вне словаря снят.
	WarnPacketEncodingUnknown = "packet_encoding_unknown"
	// WarnXHTTPModeForcedPacketUp — у XHTTP-узла был `uplink_data_placement:
	// header` без режима, и режим доопределён в `packet-up`.
	//
	// Молчать нельзя: пара «header вне packet-up» роняет ВЕСЬ конфиг ядра
	// (проверено на 1.14.0-lx.30), то есть без правки человек остаётся без
	// VPN, — но и правка меняет проволочный протокол узла, и он вправе об
	// этом знать.
	WarnXHTTPModeForcedPacketUp = "xhttp_mode_forced_packet_up"
	// WarnXHTTPParamReset — XHTTP-параметр снят, потому что ядро отвергает
	// его в заданном режиме. Сегодня это `uplink_data_placement: header` при
	// явном режиме, отличном от packet-up: режим пользователя мы не
	// переписываем, снимается одно поле.
	WarnXHTTPParamReset = "xhttp_param_reset"
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
	// WarnTailscaleCoreUnsupported — узел tailscale снят: ядро собрано без
	// with_tailscale (SPEC 122). Не пометка на живом узле, а его выброс —
	// оставленный, он завалил бы `sing-box check` для всего конфига.
	WarnTailscaleCoreUnsupported = "tailscale_core_unsupported"
	// WarnTailscaleFromSubscription — узел tailnet приехал ПОДПИСКОЙ.
	// Узел живёт, поэтому info: но связки (MagicDNS + маршрут) подписка не
	// приносит, а идентичность машины в tailnet — местная (NODE_SECTIONS.md §6).
	WarnTailscaleFromSubscription = "tailscale_from_subscription"
	// WarnAWGHeaderInvalid — AmneziaWG H1–H4 вне допустимого диапазона.
	WarnAWGHeaderInvalid = "awg_header_invalid"
	// WarnAWGHeadersOverlap — H1–H4 совпадают между собой.
	WarnAWGHeadersOverlap = "awg_headers_overlap"
	// AmneziaWG 3.x (SPEC 123).
	// WarnAWG3FieldInvalid — диапазон/булево AWG3 с мусором или N>M: поле
	// снято, узел живёт.
	WarnAWG3FieldInvalid = "awg3_field_invalid"
	// WarnAWG3HeaderKeyInvalid — header_protection_key не base64 32 байта
	// или все нули: узел выброшен (без ключа хендшейк невозможен).
	WarnAWG3HeaderKeyInvalid = "awg3_header_key_invalid"
	// WarnAWG3PaddingTooShort — при header_protection_key один из s1–s4 < 12:
	// узел выброшен (ядро отвергает конфиг целиком).
	WarnAWG3PaddingTooShort = "awg3_padding_too_short"
	// WarnAWG3RandomTrailersWideHeaders — random_trailers при широких
	// диапазонах h1–h4: потери на data-пакетах, свойство протокола (info).
	WarnAWG3RandomTrailersWideHeaders = "awg3_random_trailers_wide_headers"
	// WarnAWG3CoreUnsupported — узел с AWG3-полями снят на сборке: ядро
	// старше 1.14.0-lx.32 или без with_awg. Выброс, а не пометка.
	WarnAWG3CoreUnsupported = "awg3_core_unsupported"
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
