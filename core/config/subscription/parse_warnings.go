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
// transports#uri.ws.path своим `extract`…`code`; Xray-вход этот код не ставит.
//
// Страж TestRegistryWarningCodesAreActuallySet ищет ИМЯ КОНСТАНТЫ, поэтому
// осиротевшая константа его и роняет — это правильный сигнал: код без
// ставящего его кода на Go обязан жить в реестре, а не здесь.

const (
	// СНЯТЫ (контракт 1.1.42): xhttp_mode_forced_packet_up, xhttp_param_reset.
	// Оба кода СТАВЯТСЯ — но их ставит движок по данным реестра, а не парсер:
	// `on_implies_written` записи `mode` дописывает packet-up под
	// `uplink_data_placement: header` (transports.json), а `on_when_false`
	// снимает параметр, несовместимый с явным режимом. Движок берёт код
	// строкой из секции (`codeOf` в core/config/linkmap/exec.go) и Go-имён не
	// знает. Проверено корпусом на РЕАЛЬНОМ пути: кейсы
	// contract/corpus/uri/vless/xhttp_uplink_header_placement_adds_packet_up,
	// …_reset, xhttp_mode_invalid, xhttp_placement_bogus_reset.
	//
	// Рукописный транспортный вход, который ставил их копией, снят вместе с
	// остальным легаси ссылки. Константа Go рядом читалась бы как «код ставит
	// парсер» и звала бы написать вторую копию правила — ровно то, от чего
	// уходит кампания (тот же довод, что у awg_*/wg_key_invalid ниже).

	// WarnAmneziaContainerChoice — в vpn://-профиле несколько контейнеров,
	// одиночный путь взял дефолтный.
	WarnAmneziaContainerChoice = "amnezia_container_choice"
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
	// СНЯТЫ (контракт 1.1.60): tailscale_core_unsupported,
	// awg3_core_unsupported. Их называет реестр (`on_core_unsupported.code`
	// тела tailscale и полей AWG 3.x), а ставит общий узловой гейт ядра
	// nodeflow.NodeCoreRefusal на сборке.
	// WarnDialerProxyUnusable — цель streamSettings.sockopt.dialerProxy
	// непригодна: узел-владелец отбраковывается ЦЕЛИКОМ. Кода на узле не
	// бывает (узла не будет) — он едет в отбраковке, поэтому severity=error.
	WarnDialerProxyUnusable = "dialer_proxy_unusable"
	// WarnProtocolUnsupported — тип/протокол записи ядру неизвестен: запись
	// отбракована целиком (dropped[].code, контракт 1.1.49).
	WarnProtocolUnsupported = "protocol_unsupported"
	// WarnURITooLong — ссылка длиннее предела (limits.json): не разбиралась.
	WarnURITooLong = "uri_too_long"
	// WarnServiceRecordIgnored — строка состава со СЛУЖЕБНОЙ схемой
	// (`incy://routing/…`, `happ://routing/…`): команда маршрутизации
	// соседнему клиенту, а не сервер. Узла не будет, поэтому код едет в
	// отбраковке; severity=info — терять тут нечего, но выпадение обязано
	// быть названным, иначе оно неотличимо от потерянного узла
	// (source_kinds.json, uri_lines.service_schemes).
	WarnServiceRecordIgnored = "service_record_ignored"
	// WarnProviderBannerLink — запись-БАННЕР, притворившаяся ссылкой: цель
	// из списка «заведомо не сервер» (source_kinds.json,
	// uri_lines.banner_targets). Прежде признаком баннера было отсутствие
	// `://`, а Remnawave и 3x-ui пишут баннер СИНТАКСИЧЕСКИ ВАЛИДНОЙ
	// ссылкой (`vless://…@0.0.0.0:1`, `socks://127.0.0.1:1080`) — и он
	// становился полноценным узлом-пустышкой. severity=info: узла тут не
	// было никогда, но выпадение обязано быть названным.
	WarnProviderBannerLink = "provider_banner_link"
	// WarnSchemeUnsupported — схему строки не ведёт ни одна секция реестра.
	// Отбраковка, а не пометка (узла не будет), поэтому severity=error:
	// прежде причина ехала только текстом Go, и сверить её по коду вторая
	// сторона не могла (D-088).
	WarnSchemeUnsupported = "scheme_unsupported"
	// СНЯТ (контракт 1.1.65): body_dialect_unrecognized. Промах, который он
	// называл (конфиг Xray уходил в разбор sing-box), предотвращает
	// классификация — body_classify.go:classifyJSONObjectBody спрашивает
	// диалект до ветки sing-box. События нет, ставить код было некому.

	// Коды detour-цепочки импортируемого sing-box-конфига: их видно только
	// по ВСЕМУ телу (граф detour), одной записи для них мало. Узел живёт,
	// цепочка укорочена — код едет на узле (singboxChainInfo.attachChain).
	WarnDetourCycleBroken   = "detour_cycle_broken"
	WarnDetourTargetMissing = "detour_target_missing"
	WarnDetourToGroup       = "detour_to_group"
	WarnDetourChainTooDeep  = "detour_chain_too_deep"

	// WarnGroupEmpty — ни один член группы тела не разрешился в узел:
	// группа уходит в отбраковку с этим кодом (dropped[].code).
	WarnGroupEmpty = "group_empty"
	// WarnGroupMemberMissing — часть членов группы не разрешилась; группа
	// живёт без них, код на узле-группе с числом потерянных.
	WarnGroupMemberMissing = "group_member_missing"
	// WarnMaxNodesExceeded — тело длиннее капа узлов: хвост отброшен. Код
	// уровня ПОДПИСКИ (FetchWarning), а не узла.
	WarnMaxNodesExceeded = "max_nodes_exceeded"
)

// Предикаты «значение будет испорчено» (realityShortIDWouldDegrade,
// utlsFingerprintWouldDegrade) сняты вместе с правилами, ради которых жили:
// с SPEC 131 W2d значение судит санитайзер по реестру, и он видит и исходное
// значение, и результат — предикат «до нормализации» ему не нужен.
