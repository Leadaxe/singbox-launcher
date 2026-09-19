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
//
// SPEC 133: у схем НА ДВИЖКЕ константы здесь не заводятся вовсе. Код
// объявлен в самой секции реестра — `on_item_invalid.code`,
// `on_present.code`, `on_when_false.code`, — и движок ставит его строкой,
// не зная о Go-именах. Вместе с рукописными ветками naive и ssh отсюда ушли
// naive_extra_headers_invalid и naive_padding_ignored (оба теперь из
// registry/protocols/naive.json, тесты на реальном пути это проверяют) и
// ssh_user_default, который на URI-пути был недостижим и до кампании:
// ссылку с пустым userinfo отбивала валидация раньше подстановки root.
// С hysteria v1 — последней схемой рукописного пути — ушли ech_ignored (его
// ставит запись `ech` общего блока tls#uri своим on_present) и весь
// buildOutbound: у ссылочного входа рукописного пути больше НЕТ.
//
// С vmess ушёл и ws_early_data_converted: хвост ?ed=N раскладывает запись
// transports#uri.ws.path своим `extract`…`code`, а Xray-вход кода не ставил
// никогда (applyWSEarlyData там зовут, отбрасывая его признак).
//
// Страж TestRegistryWarningCodesAreActuallySet ищет ИМЯ КОНСТАНТЫ, поэтому
// осиротевшая константа его и роняет — это правильный сигнал: код без
// ставящего его кода на Go обязан жить в реестре, а не здесь.

const (
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
	// СНЯТЫ (контракт 1.1.11): awg_header_invalid, awg_headers_overlap,
	// awg3_field_invalid, awg3_header_key_invalid, awg3_padding_too_short,
	// wg_key_invalid. Эти коды ставит РЕЕСТР, а не парсер: правила уехали в
	// contract/registry/protocols/wireguard.json (on_invalid у полей, min_when
	// у s1..s4, связь body.relations ranges_disjoint), и производителя им
	// сверяет TestRegistryWarningCodesHaveAProducer по стороне реестра.
	// Константа Go рядом читалась бы как «код ставит парсер» и звала бы
	// написать вторую копию правила — ровно то, от чего кампания уходит
	// (решение владельца 19.09.2026).
	//
	// Сюда же ушёл awg3_random_trailers_wide_headers: сочетание
	// random_trailers с широким диапазоном h1–h4 судит связь
	// body.relations kind: cooccurrence с оператором $range_width, и
	// ставит код САНИТАЙЗЕР — то есть на всех входах, а не только на
	// ссылке. Рукописный awg3RandomTrailersWithWideHeaders снят
	// (SPEC 133, секция wireguard).
	// WarnAWG3CoreUnsupported — узел с AWG3-полями снят на сборке: ядро
	// старше 1.14.0-lx.32 или без with_awg. Выброс, а не пометка.
	WarnAWG3CoreUnsupported = "awg3_core_unsupported"
	// WarnDialerProxyUnusable — цель streamSettings.sockopt.dialerProxy
	// непригодна: узел-владелец отбраковывается ЦЕЛИКОМ. Кода на узле не
	// бывает (узла не будет) — он едет в отбраковке, поэтому severity=error.
	WarnDialerProxyUnusable = "dialer_proxy_unusable"
)

// Предикаты «значение будет испорчено» (realityShortIDWouldDegrade,
// utlsFingerprintWouldDegrade) сняты вместе с правилами, ради которых жили:
// с SPEC 131 W2d значение судит санитайзер по реестру, и он видит и исходное
// значение, и результат — предикат «до нормализации» ему не нужен.
