package subscription

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
// ЗДЕСЬ ОСТАЛИСЬ ТОЛЬКО КОДЫ, КОТОРЫЕ СТАВИТ САМ ПАРСЕР (SPEC 131 W2d) —
// то есть те, где сведения есть у него одного: форма ссылки (`?ed=N` в пути,
// битая пара extra-headers, выбор контейнера в vpn://-профиле), гейты сборки
// (ядро без with_tailscale/with_awg) и разбор AWG-полей.
//
// Коды о ЗНАЧЕНИЯХ полей отсюда ушли в реестр контракта: их ставит санитайзер
// (core/config/nodeflow), и ставит одинаково для ссылки, JSON-тела и
// Xray-объекта. Прежде те же двенадцать кодов жили константами здесь и
// проставлялись копиями в четырёх парсерах — копии расходились между собой
// (набор insecure, список отпечатков с гибридным шаром), а два кода
// (ss_method_invalid, port_invalid) не ставились вовсе: их «деградация» была
// на деле жёстким дропом узла (DRIFT §4).

const (
	// WarnNaiveExtraHeadersInvalid — пара из naive `extra-headers` отброшена:
	// нет ':', запрещённые символы в имени или CR/LF/NUL в значении.
	//
	// Прочие пары той же ссылки живут, узел живёт — отсюда severity=info; но
	// до этого отброс уходил только в debuglog, и в отчёте сборки человек не
	// видел, что заголовок, которым он открывает доступ, до сервера не доедет.
	WarnNaiveExtraHeadersInvalid = "naive_extra_headers_invalid"
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
	// WarnWSEarlyDataEDConverted — Xray-хвост ?ed=N разложен в
	// max_early_data + early_data_header_name.
	WarnWSEarlyDataEDConverted = "ws_early_data_converted"
	// WarnECHIgnored — в ссылке был Xray-параметр `ech=`: он несёт ключ
	// ЧУЖОГО клиента (public_name ≠ SNI узла), и рукопожатие с ним не
	// состоится (device-verified, §320 LxBox). Параметр снят, узел жив —
	// отсюда severity=info.
	//
	// Код ставит МАППЕР, а не санитайзер: различить «ECH из ссылки» и
	// «валидный блок tls.ech из sing-box-JSON» может только тот, кто знает
	// источник значения. Нативный блок проходит нетронутым — ECH в ядре
	// скомпилирован всегда (D-122, пересмотр D-006).
	WarnECHIgnored = "ech_ignored"
	// WarnDialerProxyUnusable — цель streamSettings.sockopt.dialerProxy
	// непригодна: узел-владелец отбраковывается ЦЕЛИКОМ. Кода на узле не
	// бывает (узла не будет) — он едет в отбраковке, поэтому severity=error.
	WarnDialerProxyUnusable = "dialer_proxy_unusable"
)

// Предикаты «значение будет испорчено» (realityShortIDWouldDegrade,
// utlsFingerprintWouldDegrade) сняты вместе с правилами, ради которых жили:
// с SPEC 131 W2d значение судит санитайзер по реестру, и он видит и исходное
// значение, и результат — предикат «до нормализации» ему не нужен.
