# TASKS — SPEC 133 · Маппер ссылок от реестра

Нормы — `SPEC.md`; примитивы и инвентаризация — `PRIMITIVES.md`;
черновики секций `uri` — `SCHEMES.md`. Адреса на `5170a169`.

Каждая волна — отдельный набор коммитов в develop. Старый код живёт до
замены: парсер схемы удаляется **в том же коммите**, где корпус подтвердил
её перевод (`SPEC.md` §8), и ни минутой раньше.

**Политика проверок (AGENTS.md §3).** Локально — только раннеры корпуса по
имени: `go test ./core/config -run TestContractCorpusURI -count=1`,
`-run TestContractCorpusEmitRoundTrip`, плюс `go build ./...`.
Запрещено локально: `go test ./...`, прогон пакета целиком, `go vet ./...`,
Win7-сборка. Полный прогон — в CI после законченной волны:
`gh workflow run ci.yml --ref develop -f run_mode=tests`.
Новых юнит-тестов не писать: корпус — главный тест (один интеграционный на
волну, если он data-критичный).

Оценки — в часах агентской работы.

---

## W0 — грамматика, линтеры, движок (parse) — 24 ч

**Ход на 19.09.2026 (вечер).** Сделано: грамматика заморожена (`4a3dbf3f`,
`PRIMITIVES.md` §0); схема секции (`f43f86fe`); модель и загрузчик
(`core/config/registry/mapper.go`), движок `detect` обоих уровней, выбор по
`priority`, пространство источников (`28780ce6`); `blocks` у `tls`/
`transports` и секции trojan (`8cc6ac69`, контракт 1.1.13); секции vless,
`contract/docs/MAPPER_ENGINE.md`, трасса (`8e59e808`).
Осталось: `registry/sources.json`, линтеры, разворачивание `include`,
проходы A/B, фикстуры W0.5.

Без перевода схем: движок появляется, но ни одна схема на него ещё не идёт.
**Объём вырос** с расширением области (все источники, `detect` в данных).

- [ ] `schema/registry_mapper.schema.json` — JSON-схема секций
      `mappers.<kind>` (P1–P12 + P13–P26 `PRIMITIVES.md` §15); валидирует обе
      стороны. Отдельным файлом: `registry_body.schema.json` правит другой агент
- [ ] `registry/sources.json` — уровень документа: виды источника, `detect`,
      `priority`, `unwrap`/`redetect`, `max_unwrap_depth`, `on_unrecognized`
      (`SPEC.md` §3A.3). Переводит в данные `body_classify.go:90-124`
- [ ] Движок `detect` (P13) — общий для обоих уровней: `regex` (RE2 ∩
      ECMAScript), `json{required_keys,any_keys,key_absent,type_of,value_in}`,
      `ini{sections,keys}`, `text{prefix_fold,line_fold}`, `not`/`all`/`any`/`default`
- [ ] Модель грамматики в `core/config/registry`: разбор `uri.forms[]`,
      `source`, `userinfo`, `value_map`, `selector`, `sets`/`implies`,
      `extract`/`compose`, `list`, `default_from`, `decode_extra`, `emit_*`
- [ ] `core/config/linkmap` (новый пакет) — движок `Parse(text) (scheme, map, origin, err)`:
      формы и декодеры (`url`/`percent`/`base64`/`base64url`/`json`/`ini`/`reparse`),
      пространство источников, два прохода, фазы `SPEC.md` §4. go1.20-совместимо
- [ ] Общие нормализаторы: `base64_std`, `cidr_prefix`, `port_range_spec`,
      `duration_bare_seconds`, `range_order{swap,strict}`
- [ ] Линтеры реестра (`SPEC.md` §8) — один Go-тест, 7 проверок
- [ ] Линтеры `detect` (`SPEC.md` §3A.5) — 6 проверок, главная:
      **`detect`-ы одного уровня не пересекаются на корпусе** (каждый кейс
      опознаётся ровно одной секцией; ноль или два — красное). Гоняется по
      всему `contract/corpus/**` (334 URI + 94 body)
- [ ] `body_source` у каждой секции; линтер сверяет с `except_sources`
      правил реестра (у нас 5 значений: uri/singbox/xray/wgconf/amnezia)
- [ ] `decode.plus_literal`, выводимый из `format: base64*` — одно правило
      вместо четырёх заплат (`DELTAS.md` D133-7). Прогон «в двух режимах»
      ОТМЕНЁН: решение принято, исключение адресное
- [ ] `Registry.MapsTo` подключён (перестаёт быть мёртвым) либо снят как дубль
      нового API — по факту реализации

**Критерий:** линтеры зелёные на сегодняшнем реестре (после добавления
`source` в §W0.5); движок разбирает пилотную схему в тесте; корпус не тронут.

## W0.5 — фикстуры identity старым путём — 6 ч

Главная страховка кампании (`SPEC.md` §9.1). Делается **до** W1.

- [ ] Снять по всем 334 кейсам корпуса + по реальному `state.json` владельца:
      тело, `tag`, `label`, `origin.raw`, share-URI, `warnings`, хеш
- [ ] Раннер сравнения `TestLinkMapperIdentityFixtures` — гоняется на каждой
      волне, падает на любом сдвиге
- [ ] `source` проставлен всем `uri`-параметрам реестра (механическая правка,
      поведение не меняет — линтер W0 начинает проходить)

**Критерий:** фикстуры сняты, раннер зелёный на неизменённом коде.
**Параллелить нельзя** — это база сравнения для всего остального.

Фикстуры снимаются **старым путём до первой дельты** — поэтому решение
владельца «чинить сразу» (вопрос 2 = Б) базу сравнения не трогает:
любое расхождение сверяется со списком `DELTAS.md`, всё вне списка — красное.

## W1 — пилот: trojan — 8 ч

- [ ] Секция `uri` trojan в новой грамматике (`SCHEMES.md` §1)
- [ ] Движок проводит trojan целиком; `node_parser` trojan-ветка удалена
- [ ] 33 кейса корпуса зелёные без изменённых ожиданий; фикстуры W0.5 сходятся

**Критерий:** `SPEC.md` §8. Пилот проверяет grammar на selector/sets/
цепочке источников/list без форм и `extract`.
**Отчёт после W1 обязателен** — грамматика может потребовать правки до того,
как в неё вложены 15 схем.

## W2 — vless — 14 ч

Самая сложная: все транспорты, REALITY, plaintext-порты, три «сложных
случая» владельца (`SCHEMES.md` §2).

- [ ] `type`→транспорт (+`headerType`), `security`=reality, `?ed=N`
- [ ] plaintext-порты через `when` по `port`
- [ ] `flow=…-udp443` → `value_map`+`sets`; `packetEncoding=none` → `value_map`
- [ ] 103 кейса корпуса зелёные

**Зависит от W1** (грамматика утверждена). Дальше схемы параллелятся.

## W3 — vmess — 10 ч

- [ ] Две формы (v2rayN base64-JSON и legacy cleartext), один набор `maps_to`
- [ ] `net=h2 ⇒ TLS` через `sets`; `label` из `json.ps`
- [ ] 36 кейсов; **неинъективный `value_map` `httpupgrade→ws` зафиксировать
      как известный дефект эмита**, не чинить здесь

## W4 — ss / socks / http / ssh / naive / anytls — 14 ч

Параллелится по схемам (6 независимых кусков, ~2–3 ч каждый).

- [ ] ss: три формы userinfo, `reparse` legacy, `base64?` (SS2022);
      `plugin`/`plugin_opts` — **под вопросом 2 `SPEC.md` §13**
- [ ] socks: `scheme_sets`, `base64?` userinfo, `socks5h` в алиасы
- [ ] http: `scheme_sets` по суффиксу, headers `extract`, `when tls.enabled`
- [ ] ssh: списки, `when` у `private_key_path`
- [ ] naive: `scheme_sets` (+`bbr`), extra-headers, `single_into`
- [ ] anytls: userinfo целиком, duration, sni-эвристика
- [ ] 26+30+21+26+36+26 = 165 кейсов

## W5 — hysteria / hysteria2 / tuic / masque — 12 ч

Параллелится (4 куска).

- [ ] hysteria2: base64-обёртка, multi-port из `port_raw`, obfs-объект
- [ ] hysteria v1: цепочка `auth`, `obfs=xplus` как тип, mbps-суффикс, полоса
- [ ] tuic: `uuid:password`, duration; `disable_sni` — **вопрос 2**
- [ ] masque: свой парсер удаляется целиком; `split_into` для address;
      `insecure`/`sni` получают общие правила — **вопрос 2**
- [ ] 16+49+31+16 = 112 кейсов

## W6 — wireguard / awg / amnezia — 14 ч

- [ ] Четыре формы; одна таблица `ini.*` на ссылку и на `.conf`
- [ ] `source` картой url/ini; `preserve_plus`; `list{item:"int",len:3}`
- [ ] `range_order` в двух режимах (h1–h4 swap, AWG3 strict)
- [ ] Amnezia `vpn://` остаётся распаковщиком над текстом (не переводится)
- [ ] 110 кейсов

**Внимание:** `node_parser_wireguard.go`/`node_parser_amnezia.go` правит
другой агент — волна берётся после его завершения, по свежему HEAD.

## W7 — эмит от таблицы — 12 ч

Отдельный осторожный этап (`SPEC.md` §6).

- [ ] `Emit(scheme, body) (uri, err)` от той же таблицы наоборот
- [ ] `emit_form`/`form_from`, `param_order`, `emit_when`/`omit_default`,
      `compose`, `implicit`
- [ ] Не-round-trip случаи переводятся в данные (`round_trip: false` + причина)
- [ ] `max_uri_length` начинает применяться и на выходе
- [ ] Все `shareuri_*.go` удалены
- [ ] `TestContractCorpusEmitRoundTrip` зелёный; фикстуры share-URI сходятся

## W8 — Xray-JSON — 14 ч

Объём подтверждён расширением области (`SPEC.md` §3.4a, §12).

- [ ] Секция `mappers.xray`: формы `vnext`/`servers`/`flat` с `$base`
      (снимает дословный дубль выемки), `detect` по `protocol`
- [ ] Диалекты: `xhttpSettings` ∥ `splithttpSettings`, camel ∥ snake,
      `h2` ∥ `http`, auth/полоса под четырьмя именами — `source` списком
- [ ] `list.coerce_scalar` у `httpSettings.host` — чинит мусор
      `["[a.com b.com]"]` (`xray_outbound_convert.go:26`, `:363-365`)
- [ ] Молчаливые потери получают коды: `network` вне набора, `security: xtls`,
      `vnext[1..]`/`servers[1..]`/`users[1..]`
- [ ] `xray_outbound_convert.go`/`xray_protocols.go`/`xray_hysteria.go` удалены
- [ ] `xray_json_array.go`/`xray_balancer.go` **остаются** — сборка документа
- [ ] Кандидаты `D133-C8` (`keyShare`) и `D133-C9` (`tlsSettings.alpn`) —
      **только кейсы корпуса**, поведение не меняется до решения владельца

## W8.5 — sing-box JSON: диалекты форков — 10 ч

Уточнение владельца 19.09.2026: это **не** «почти identity» (`SPEC.md` §3.4).

- [ ] Секция `mappers.singbox`: формы `outbound`/`endpoint`, `type_synonyms`,
      `unknown_key: keep` + `json_field_unknown`
- [ ] Три per-type ветки `singbox_sanitize.go` (`:84`/`:109`/`:147`) → записи
      секции; **каждая получает код** (сегодня две из трёх молчат)
- [ ] `singboxTypeIsAddressless` (`if type ==`) и `singboxCredentialFromMap`
      (switch из 8 литералов) → таблицы реестра
- [ ] `xmux.*`: `source` списком snake ∥ camel ∥ `extra.xmux.*` (`lift`)
- [ ] `SanitizeSingboxOutboundMap` перестаёт возвращать `nil`: деградации
      доезжают до узла
- [ ] Reject'ы получают машинные коды (`addCoded` вместо `add`)

## W8.6 — `.conf` / ini напрямую — 8 ч

- [ ] Секция `mappers.conf` с `ini_dialect` (`SCHEMES.md` §13.4)
- [ ] `wgConfToURI`/`parseWGConfSections` удалены — стык «ini → текст ссылки
      → парсер» исчезает вместе с потерей `+` на `q.Encode()`
- [ ] Вторая `[Peer]` получает код `wgconf_extra_peer_dropped` (сегодня молча)
- [ ] `Endpoint` с голым IPv6 (`extract` + `on_no_match`)
- [ ] `Interface.DNS` получает info-код `wgconf_dns_ignored` (лоссы by-design,
      но не молча)
- [ ] Распаковщик Amnezia `vpn://` **остаётся** — отдаёт `.conf` этой секции

## W9 — зачистка и документация — 10 ч

- [ ] Ни одной схемной функции в пакете маппера (критерий конца — `SPEC.md` §2.1)
- [ ] Греп-страж (вопрос 4 = **Б**, принято): тест по пакету маппера
- [ ] `gendocs`: страницы протоколов + **таблица «как распознаётся источник»**
      из `detect` обоих уровней (`SPEC.md` §3A.6) — сегодня её нет нигде
- [ ] `contract/docs/CANON.md` — норма `SPEC.md` §11; `VERSION` bump
- [ ] `TASKS_LXBOX.md` — норма, шесть принятых дельт, сверка с их фичой 480
- [ ] `docs/release_notes/upcoming.md`, `docs/ARCHITECTURE.md`

---

## Сводка

| Волна | Часов | Параллелится |
|---|---|---|
| W0 грамматика + `detect` + линтеры + движок | 24 | нет |
| W0.5 фикстуры identity | 6 | нет |
| W1 trojan (пилот) | 8 | нет |
| W2 vless | 14 | после W1 |
| W3 vmess | 10 | ‖ |
| W4 ss/socks/http/ssh/naive/anytls | 14 | ‖ (6 кусков) |
| W5 hysteria/hy2/tuic/masque | 12 | ‖ (4 куска) |
| W6 wireguard/awg | 14 | ‖ |
| W7 эмит | 12 | после W2–W6 |
| W8 xray | 14 | после W7 |
| W8.5 sing-box диалекты | 10 | ‖ с W8 |
| W8.6 .conf/ini | 8 | ‖ с W8 |
| W9 зачистка и доки | 10 | нет |
| **Итого** | **164 ч** | |

Критический путь при полном параллелизме W2–W6 и W8/W8.5/W8.6:
24+6+8+14+12+14+10 = **88 ч**.

Релизится: W0–W1 без релиза (внутреннее); W2–W6 — patch по мере готовности;
W7 и W8 — patch; W9 — вместе с последним.


---

# Состояние и следующий шаг (передача, 20.09.2026 вечер)

Предыдущие передачи — в истории (`ddf3b563`, `09557746`, `cec660fe`,
`ece947b5`, `9a3969c7`, `1659fd97`).

## Что готово этой волной (с sha)

| sha | Что |
|---|---|
| `81408f41` | Порядок подблоков в склейке блока — по ОБЪЯВЛЕНИЮ, не по алфавиту. Латентно (каждая запись закрыта своим `when: transport.type`), но порядок в плане нормативен. Находка агента LxBox, п. 3 |
| `c28f378e` | **wireguard/awg на движке — ПОСЛЕДНЯЯ схема.** Обе формы ссылки одной таблицей, парная `mappers.conf`. Пять дефектов движка, два за его пределами, два спавших кода. Контракт 1.1.23 |
| `216ffc3d` | **Атрибут `live` снят — движок единственный путь.** Заодно закрыта дыра: shadowsocks сверялась НУЛЁМ кейсов. Контракт 1.1.24 |
| `edada62f` | **`single_into` обязателен везде + линтер.** Поправка ответа про `dialer#uri` (Q133-53). Контракт 1.1.25 |
| `446d5dc0` | **Корпус xray-входа с 6 до 62 кейсов** — страховка ДО перевода |

**Кампания перевода ССЫЛОК закончена.** Все 14 схем на движке,
рукописных парсеров ссылок нет, развилки нет. Зелено: `go build ./...`,
пакеты `core/config`, `core/config/linkmap`, `core/config/subscription`,
`core/config/nodeflow`.

## СЛЕДУЮЩИЙ ШАГ — XRAY-ВХОД НА ДВИЖОК (страховка готова)

Корпус `contract/corpus/body/xray/` вырос до **62 кейсов**: 56 входов взяты
у LxBox (`app/test/fixtures/xray/pipeline_identity_before.json`), ожидания
сняты НАШИМ сегодняшним конвертером. Это снимок поведения, которое перевод
обязан воспроизвести, — а не эталон желаемого.

**Зачем он был нужен раньше кода.** У LxBox сверка на 45 входах сходилась
байт в байт, а ~25 полей XHTTP терялись: секция объявляла только
`mode/path/host`, и фикстуры этих полей не несли. Снимок сразу показал, что
**у нас потери НЕТ** — `xhttp_full_field_set` доезжает с 25 ключами
`transport` плюс `xmux` из пяти. Перевод обязан это сохранить.

### Что в корпусе есть (покрытие, которого не было)

полный набор полей XHTTP · четыре варианта знака `sockopt` keepalive ·
`?ed=` и в пути, и в настройках · `eh` без `ed` · splithttp-алиас ·
snake_case в `extra` · `extra` против плоского поля · цепочки через
`dialerProxy` (в т.ч. vless-релей) · balancer · `malformed_stream` ·
мультинода (310 узлов, одинаковые теги) · отбраковки.

### Решения, принятые до кода (из ТЗ)

1. `tls#xray.alpn` — `maps_to: null` (кроме hysteria).
2. `include dialer#xray` — со сверкой.
3. `server_port` — `required` без дефолта (**ждёт владельца**).
4. `detect` формы по `type_of {streamSettings: "object"}` + кейс
   `malformed_stream` — сразу.
5. Секция `socks` — оставить в реестре с пометкой «launcher не исполняет»
   (у нас socks это звено цепочки `dialerProxy`).

### Что понадобится движку (не реализовано, проверено грепом)

| Примитив | Зачем |
|---|---|
| `unknown_key.ignore[]` | иначе каждая xray-секция вешает узлу `json_field_unknown` на собственный `protocol`/`tag`/`remarks` |
| изъятие записи блока при совпадении имени И набора `source` | `GRAMMAR_SYNC` «Агенту движка», п. 1 |
| `when` `lt`/`gt` | нужен `dialer#xray`: `tcpKeepAliveIdle` 0 = не задано, отрицательное ⇒ `disable_tcp_keep_alive: true` (находка LxBox, п. 2) |

`default_from: "body.<путь>"` — **СДЕЛАНО** (`exec.go:644-669`, читает путь
тела фолбэком, префикс не нужен).

### После зелёного — удалить

`xray_outbound_convert.go`, `xray_protocols.go`, `xray_hysteria.go` —
**кроме** сборки цепочек уровня документа.

## Хвосты (что осталось от задачи 2)

- **Эмит-объявления `GRAMMAR_SYNC` §9, п. 6–11** — входят в волну эмита,
  до неё не трогать (иначе регрессия по правилу 5).
- **Три копии scheme-switch'а «где лежат учётные данные»**:
  `singboxCredentialFromMap`, `canonicalCredential`, `credentialFromBody`.
- **Кейс «tuic с пустым паролем»** — расхождение зафиксировано обеими
  сторонами (§24.31), корпус его не видит.
- **`dialer#uri` у vless/trojan** — Q133-53: ветки расходятся В ТЕЛЕ, а не
  только в кодах. Подключение меняет тела живых узлов, **ждёт владельца**.

## Ловушки этой волны (проверено, не повторять)

- **Факт, снятый по коду, живёт до следующей волны того же кода.** Ответ
  LxBox про `tcp_keep_alive` устарел ровно так (Q133-53). Перемеряй, прежде
  чем ссылаться на своё же измерение.
- **Поимённый список в тесте прячет дыру, которую сам же создаёт.**
  `switched.json` называл КАТАЛОГ корпуса, вывод из реестра дал СХЕМУ — у
  shadowsocks это `ss`, каталога нет, 51 кейс сверялся нулём молча.
- **Круг `parse(emit(node))` слеп к симметричной ошибке.** Двойное
  кодирование userinfo в эмиттере пряталось за вторым `PathUnescape` в
  разборе; видно стало, только когда разбор стал делать один проход.
- **Дефолт не должен спорить за путь.** `materialize_default` перебивал
  прочитанное значение, и порядок объявления решал, доедет ли оно до тела.
- **Тест, написанный на выход маппера, ломается по построению**, когда
  правило переезжает в тело (D133-22): нормализацию ключа и достройку CIDR
  проверяет КОРПУС, а не `node.Outbound`.
- **Ширина типа Go сверкой не считается.** Контракт — число JSON; `int` и
  `int64` в теле равнозначны, требовать конкретный тип значит проверять
  реализацию.

---

# Состояние и следующий шаг (передача, 20.09.2026)

Предыдущие передачи — в истории (`ddf3b563`, `09557746`, `cec660fe`,
`ece947b5`, `9a3969c7`).

## Что готово (с sha)

| sha | Что |
|---|---|
| `f4e66888` | Два красных теста CI: `normalize: trim_lower` у записей `fp`; `?password=` у ssh — ненаписанное написание (Q133-49) |
| `80989841` | **Снят рукописный разбор восьми схем**, −1182 строки |
| `2e02ba83` | **vmess на движке.** Пять дефектов, вскрытых формой-контейнером; три спавших примитива; блок `tls#uri_security`. Контракт 1.1.17 |
| `064c5894` | **hysteria2** — первая секция, принятая от LxBox. `scheme_sets: "*"`, `port_range_spec`, `prepend_group`, нормализация списка по элементам. Контракт 1.1.18 |
| `c6211da4` | **tuic.** Звено цепочки метки нормализуется ДО принятия (их находка, подтвердилась на их же цепочке). Ушла воронка userinfo-пароля в `node.Query`. Контракт 1.1.19 |
| `77cd289b` | **masque** — принята БЕЗ правок. Лексер не режет путь внутри userinfo; `split_into`, `default_when`, `cidr_prefix`; `required` у записи без `maps_to`. Дельта D133-21. Контракт 1.1.20 |
| `5c5f5aea` | **Норма рода узла.** `kind_when` у маппера / `when.source_kind` у санитайзера — две половины одного канала; блокер wireguard снят |
| `4aeca7a7` | **Реестровые заготовки под wireguard.** `when.source_kind` + `SanitizeFromKind`; связь `cooccurrence` с `$range_width` вместо рукописного awg3-правила; `cidr_prefix`/`base64_std` переехали в тело (**D133-22**); коды `wgconf_dns_ignored`, `wgconf_extra_peer_dropped`. Контракт 1.1.22 |
| `90dfa6e9` | **Движок: три примитива под wireguard.** Пространство `ini` (`SetINI` не звал никто — было объявлено и пусто), `on_no_match.take_all`, `kind_when` → `Result.Kind` |

**Двенадцать схем на движке.** На рукописном пути остались **две**:
`hysteria` v1 и `wireguard`/`awg`. Зелено: `go build ./...`, пакеты
`core/config`, `core/config/linkmap`, `core/config/subscription` целиком.

## СЛЕДУЮЩИЙ ШАГ — СЕКЦИЯ wireguard (движок под неё готов)

**Блокер снят, заготовки влиты, примитивы движка есть.** Осталось написать
саму секцию и снять рукописный путь.

### Что уже сделано (не переделывать)

| Готово | sha |
|---|---|
| Норма рода: `kind_when` / `when.source_kind` | `5c5f5aea` |
| Реестр: `source_kind` в условии, `cooccurrence`, `cidr_prefix`/`base64_std` в теле, два кода `wgconf_*` | `4aeca7a7` |
| Движок: пространство `ini`, `on_no_match.take_all`, `kind_when` → `Result.Kind` | `90dfa6e9` |

Секции LxBox прочитаны целиком и лежат снимками в scratchpad
(`lxbox_uri_wireguard.json`, `lxbox_conf_wireguard.json`). Их `kind_when`
совпадает с принятым решением буква в букву; `emit.$impl` их секции `uri`
заранее описывает ровно наш переход на `when.source_kind`.

### Что осталось

1. **Секция `mappers.uri`** в `contract/registry/protocols/wireguard.json`:
   две формы — `url` и `conf_b64` (`decode: [base64, ini]`, `space: "ini"`,
   `detect`: нет `@` и весь текст из алфавита base64). Брать секцию LxBox за
   основу, расхождения — точечно.
2. **Секция `mappers.conf`** — тот же набор записей поверх `space: "ini"`,
   `body_source: "wgconf"`.
3. **Цепочка метки — ЕДИНСТВЕННОЕ известное расхождение с LxBox.** У нас
   ссылочная форма читает `fragment` → **`name`** (параметр `?name=`) →
   фолбэк-тег; у них `fragment` → `path`. У `.conf`: наш
   `wgPeerNameComment` → хост `Endpoint`, у них `ini.$comment.Peer` →
   `hint` → литерал `WireGuard`. Источник **`hint`** (имя от вызывающего)
   движком НЕ поддержан; решить — заводить его или оставить нашу цепочку
   (второе дешевле и корпус ждёт именно её).
4. **`single_into` у userinfo** — у wireguard userinfo несёт приватник;
   у LxBox запись `into` не объявляет НАМЕРЕННО (их `impl` объясняет:
   иначе userinfo уехал бы в тело сырым, раньше записи с `normalize`).
   Проверить, что у нас та же механика.
5. **Снять рукописный путь** — в том же коммите, где корпус подтвердил
   перевод: `node_parser_wireguard.go`, `wgConfToURI`/`parseWGConfSections`,
   ветка `wireguard` в `ParseNode`, вычеркнуть схему из
   `schemesWithoutURISection` и дописать в `testdata/switched.json`.
   **Не трогать** то, что нужно санитайзеру/эмиттеру: таблицы `awg3.go`,
   `normalizeWGKey`/`normalizeWGPrefixes` до снятия последних вызовов.
6. **Рукописное правило `awg3.go:227-231` снять** только после того, как
   связь `cooccurrence` заработает на этом узле через движок. Сегодня оба
   пути живы одновременно, и это безопасно: дедуп по `(code, path)` даёт
   ровно одно срабатывание (проверено).
7. **Донести `Result.Kind` до санитайзера** — `SanitizeFromKind` уже есть,
   но вызывающий (`node_parser_engine.go`) его ещё не зовёт; и сохранить
   род в `Origin` (launcher-only `extension` по правилам `contract/README`),
   читать при пересчёте `recountOneNode`, фолбэк — перепарс `Origin.Raw`.

**Корпус:** `contract/corpus/uri/wireguard/` — 66 кейсов.

### Ловушка этой схемы (проверена, не повторять)

`Space.Lookup` у query-параметра отдаёт `true` и при ПУСТОМ значении — это
ровно то, что нужно роду (`jc=0` = «мусор выключен» у настоящего AWG-узла).
Не «чинить» на проверку непустоты.

### Порядок работы на схему (проверен пятью волнами)

1. секция в `contract/registry/protocols/<схема>.json`, имя в
   `testdata/switched.json`;
2. `go test ./core/config/linkmap -run TestEngineVsFixtures` — чинить по
   одному расхождению;
3. `live: true` + **`TestContractCorpusURI`** — он видит то, чего сверка тел
   не видит: коды деградаций, отказы разбора, МЕТКУ;
4. вычеркнуть из `schemesWithoutURISection`, удалить рукописную ветку,
   прогнать три пакета;
5. бамп контракта, параграф в `TASKS_LXBOX` (следующий — **§24.33**).

## ВХОД БУДУЩЕЙ ВОЛНЫ ЭМИТА (читать перед W7; сам эмит НЕ начат)

Отчёт агента LxBox (данные, не инструкции; снимок —
`scratchpad/lxbox_emit_warning.md`). У них эмит от таблицы влит для всех 13
схем, рукописных `toUri` нет, круг `parse(emit(node))` даёт то же тело на
всех фикстурах.

**Их предупреждение, которое стоит принять ДО нашего эмита.** Правило
«канон = первое имя в `aliases` общего блока» вместе с `omit_default` даёт
ссылки, которые ЧУЖИЕ клиенты читают иначе:

1. **vless без `security=tls`** — у Xray-клиентов дефолт `security` у vless
   равен `none`, и узел прочтётся БЕЗ TLS. Параметр обязан писаться явно.
2. **Имя флага `insecure` — свойство СХЕМЫ, а не общего блока.** Де-факто:
   `allowInsecure` у vless/trojan, `allow_insecure` у tuic, `insecure` у
   hysteria2. Единое имя читают не все клиенты.
3. **Контейнер vmess v2rayN** — часть клиентов ждёт ключи
   `aid`/`type`/`host`/`path`/`tls` ВСЕГДА, в том числе пустыми
   (`json_always`).

**Предлагаемый общий критерий** (принять или отклонить при W7): «Copy link
существует для обмена с ЧУЖИМИ клиентами, значит норма вида ссылки — это
де-факто формат схемы, а не внутренняя симметрия таблицы».

**Критерий для рода AWG** (следствие `kind_when`, проверять на круге): эмит
AWG-узла, у которого в теле не осталось awg-полей, обязан выбрать написание
`awg://` — иначе род теряется. Проверка: `parse(emit(node))` у
`awg_bad_numeric_skipped` и `awg_jc_invalid_dropped` обязан дать
`mtu: 1280`. У LxBox это `emit.form_from: {any_set: …}` — обращение
`kind_when`; сегодня такой узел уезжает у них как `wireguard://`, и место
роду в своей модели они заведут к синку с нашим wireguard.

---

## Очередь ПОСЛЕ схем (не начата)

1. **`GRAMMAR_SYNC.md` §9** (sha `8602a857`) — осталось **6
   эмит-объявлений** (п. 6–11), они входят в волну эмита. Закрыто: оба
   дефекта лексера (`#`-резка в `2e02ba83`, `/`-резка в `77cd289b`),
   `normalize cidr_prefix` и `base64_std` в body и `relation cooccurrence`
   для AWG3 — всё в `4aeca7a7`.
2. **`GRAMMAR_SYNC.md`, «Агенту движка — списком»** (sha `5d8adc80`) — 7
   пунктов: изъятие записи блока при совпадении имени И набора `source`,
   новые предикаты `detect`, `when` `lt`/`gt`, `default_from` `body.*`
   (**сделано** в `2e02ba83`), `unknown_key.ignore`, уровень документа
   `sources.json`, правки xray-секций.
3. **`single_into` во ВСЕХ секциях с userinfo** — договорённость
   (`TASKS_LXBOX` §24.29 п. 3): умолчания «первое имя `into`» быть не
   должно, атрибут обязателен, плюс линтер. Сегодня проставлен у naive,
   http и tuic; движок без него берёт первое имя.
4. ~~**`kind_when` / `when.source_kind`**~~ — **ЗАКРЫТО** (`5c5f5aea`,
   `4aeca7a7`, `90dfa6e9`). Решение: две половины одного канала — маппер
   объявляет род по условию на ВХОД, санитайзер читает его общим оператором
   `when.source_kind`. Осталось лишь донести `Result.Kind` до `Sanitize` и
   сохранить род в `Origin` — это пункты 6–7 «Что осталось» выше.
5. **Три копии scheme-switch'а «где лежат учётные данные»**:
   `singboxCredentialFromMap` (`singbox_import.go:437`),
   `canonicalCredential` (`canonical_emit.go:507`), `credentialFromBody`
   (`node_parser_engine.go`). Первые две — рукописные списки схем ровно
   того вида, который кампания снимает.
6. **Кейс корпуса «tuic с пустым паролем» + per-app override** — у LxBox
   `password.required: true`, у нас узел живёт. Расхождение зафиксировано
   обеими сторонами (§24.31), но корпус его не видит.

## Ловушки этих волн (проверено, не повторять)

- **Лексер не должен резать по синтаксису части, до которой не дошёл.**
  Дважды за кампанию: `splitAuthorityPart` резал base64-блоб по `?`/`/`
  (это байты алфавита), `lexURI` резал путь по `/` внутри userinfo (там
  лежит ключ). Оба — молчаливая потеря половины значения.
- **Сверка ТЕЛ не видит метку.** Дефект «звено цепочки принято до
  нормализации» дал узлам имя «/» и был зелёным на `TestEngineVsFixtures`.
  Гоняй `TestContractCorpusURI`.
- **Удаление ветки роняет страж кодов деградации, и это ПРАВИЛЬНО.**
  `TestRegistryWarningCodesAreActuallySet` ищет ИМЯ Go-КОНСТАНТЫ; код,
  который теперь ставит движок из секции, обязан её лишиться.
- **Тесты, зовущие `buildOutbound` напрямую, ломаются по построению** —
  переписывать на вход ССЫЛКОЙ, а не подпирать хелпером.
- **Общий блок отдаёт своё ВСЕМ, кто его включил** (REALITY Q133-47, затем
  `security` у vmess). Прежде чем класть запись в общий блок, проверь ВСЕХ
  потребителей.
- **`Source.All()` — метод ЛИНТЕРА**, исполнению нужен `ForForm(id)`.
- **Секции LxBox стоит читать целиком до вливания**: их `impl` объясняют
  замысел («почему masque НЕ подключает `tls#uri`», «почему `priority: 100`
  у `disable_sni`») и экономят часы разбора.

## Документы

`SPEC.md` · `PRIMITIVES.md` · `SCHEMES.md` · `DELTAS.md` (**D133-21**) ·
`QUIRKS.md` (**Q133-49…52**) · `GRAMMAR_SYNC.md` (**§9 и «Агенту движка»** —
очередь) · `contract/docs/MAPPER_ENGINE.md` · `contract/TASKS_LXBOX.md`
**§24.29–24.32**.

---

# Прежняя передача (19.09.2026 ночь)

Предыдущие передачи — в истории (`ddf3b563`, `09557746`, `cec660fe`).

## Что готово (с sha)

| sha | Что |
|---|---|
| `f4e66888` | Два красных теста CI с прошлой волны. `fp=QQ` уезжал в конфиг как есть — таблица `{prefix,strip}` лоуэркейсит ключ для ПОИСКА, но на промахе отдаёт значение неизменным; `normalize: trim_lower` у всех четырёх записей с `$ref: tls.fp_dialect`. `?password=` у ssh — ненаписанное написание, жившее за счёт воронки старого пути (Q133-49) |
| `80989841` | **Снят рукописный разбор восьми схем**, −1182 строки. Удалены `node_parser_{http,naive,anytls,ssh}.go` целиком; `{ss,socks}.go` держали по функции ДЛЯ ЭМИТТЕРА — переехали к нему, файлы сняты. Каскад мёртвого: `trojanTLSFromNode`, `vlessTLSFromNode`, `shouldVLESSSkipTLSForPort`, `utlsFingerprintOrDefault`, `IsSocksScheme`, `socksVersionForScheme` |
| `2e02ba83` | **vmess на движке; `notLiveYet` ПУСТ.** Контракт 1.1.17. Пять дефектов движка, вскрытых формой-КОНТЕЙНЕРОМ, три спавших примитива, новый блок `tls#uri_security`. Подробности — в чейнджлоге `contract/README.md` и `TASKS_LXBOX` §24.29 |

**Все девять написанных секций `uri` ведут разбор**: trojan, vless, anytls,
socks, ssh, http, naive, shadowsocks, vmess. Зелено: `go build ./...`,
пакеты `core/config`, `core/config/linkmap`, `core/config/subscription`
ЦЕЛИКОМ.

**Атрибут `live` и старый вход снимать РАНО**: `schemesWithoutURISection`
не пуст (hysteria, hysteria2, masque, tuic, wireguard), и без старого входа
их ссылки остались бы без разбора вовсе.

## СЛЕДУЮЩИЙ ШАГ — секции пяти оставшихся схем

Порядок задан владельцем: **hysteria2, tuic, masque** (секции берутся у
LxBox, ТОЛЬКО чтение:
`git -C ~/projects/LxBox show origin/develop:app/assets/contract_draft/uri/hysteria2.json`,
также `tuic.json`, `masque.json`), затем **hysteria v1** (секции у них нет —
писать из нашего `SCHEMES.md`), затем **wireguard/awg**.

Порядок работы на схему, проверенный тремя волнами:

1. секция в `contract/registry/protocols/<схема>.json`, имя в
   `testdata/switched.json`;
2. `go test ./core/config/linkmap -run TestEngineVsFixtures` — чинить по
   одному расхождению;
3. `live: true` + **`TestContractCorpusURI`** — он видит то, чего сверка тел
   не видит: коды деградаций, отказы разбора, написание схемы;
4. вычеркнуть из `schemesWithoutURISection`, удалить рукописную ветку,
   прогнать три пакета;
5. бамп контракта, параграф в `TASKS_LXBOX` (следующий — **§24.30**).

## Очередь ПОСЛЕ схем (не начата)

1. **`GRAMMAR_SYNC.md` §9** (sha `8602a857`) — 11 пунктов агенту движка:
   два дефекта лексера `parse.go:351-366`, `normalize` `cidr_prefix`/
   `base64_std` в body, `relation cooccurrence` для AWG3 **ДО перевода
   wireguard**, шесть эмит-объявлений.
2. **`GRAMMAR_SYNC.md`, раздел «Агенту движка — списком»** (sha `5d8adc80`) —
   7 пунктов: изъятие записи блока при совпадении имени И набора `source`,
   новые предикаты `detect`, `when` `lt`/`gt`, `default_from` `body.*`
   (**сделано** в `2e02ba83`), `unknown_key.ignore`, уровень документа
   `sources.json`, правки xray-секций.
3. **`single_into` во ВСЕХ секциях с userinfo** — договорённость с LxBox
   (`TASKS_LXBOX` §24.29 п. 3): умолчания «первое имя `into`» быть не
   должно, атрибут обязателен, плюс линтер. Сегодня он проставлен только у
   naive и http; движок без него берёт первое имя. Следующим бампом.
4. **`kind_when` / `max_when.when.source_kind`** — решить **ДО перевода
   wireguard** (`TASKS_LXBOX` §24.29). Наши кейсы `awg_bad_numeric_skipped`
   и `awg_jc_invalid_dropped` проходят сегодня лишь потому, что wireguard
   ещё на рукописном пути: клампом занимается код, знающий написание
   `awg://` до всякого тела. Предложен вариант «род как часть контекста
   санитайзера», ответ LxBox ждём.
5. **Три копии scheme-switch'а «где лежат учётные данные»**:
   `singboxCredentialFromMap` (`singbox_import.go:437`),
   `canonicalCredential` (`canonical_emit.go:507`) и
   `credentialFromBody` (`node_parser_engine.go`). Первые две — рукописные
   списки схем ровно того вида, который кампания снимает. Свести на реестр.

## Ловушки этой волны (проверено, не повторять)

- **Удаление ветки роняет страж кодов деградации, и это ПРАВИЛЬНО.**
  `TestRegistryWarningCodesAreActuallySet` ищет ИМЯ Go-КОНСТАНТЫ. Код,
  который теперь ставит движок из секции, обязан лишиться константы:
  так ушли `naive_extra_headers_invalid`, `naive_padding_ignored`,
  `ssh_user_default`, `ws_early_data_converted`.
- **Тесты, зовущие `buildOutbound` напрямую, ломаются по построению.** Они
  строят `ParsedNode` руками и кладут значения в `node.Query`, полагаясь на
  договорённость «парсер разложил значения обратно». У движка её нет и быть
  не должно — переписывать на вход ССЫЛКОЙ, а не подпирать хелпером.
- **Общий блок отдаёт своё ВСЕМ, кто его включил** — второй раз за кампанию
  (после REALITY, Q133-47). Прежде чем класть запись в общий блок, проверь
  ВСЕХ потребителей: «истина TLS» у ссылочных форм и у контейнера
  выражается разными ключами.
- **`Source.All()` — метод ЛИНТЕРА.** Исполнению нужен `ForForm(id)`.
  Склейка всех форм отсортирована по ИМЕНИ ФОРМЫ, то есть порядок
  источников определяется алфавитом, а не объявлением.
- **Лексер не должен резать то, что ещё не разобрано.** `?` и `/` —
  синтаксис ссылки; под base64 это байты алфавита.
- **Сверка тел не видит кодов и отказов** (повтор прошлой волны,
  подтвердился снова): у vmess `TestEngineVsFixtures` был зелёным при 15
  красных на `TestContractCorpusURI`. Гоняй оба.

## Документы

`SPEC.md` · `PRIMITIVES.md` · `SCHEMES.md` · `DELTAS.md` · `QUIRKS.md`
(**Q133-49…52**) · `GRAMMAR_SYNC.md` (**§9 и «Агенту движка»** — очередь) ·
`contract/docs/MAPPER_ENGINE.md` · `contract/TASKS_LXBOX.md` **§24.29**.

---

# Прежняя передача (19.09.2026 поздний вечер)

Предыдущие передачи — в истории (`ddf3b563`, `09557746`). Ниже состояние
после волны «движок на продакшен-пути».

## Что готово (с sha)

| sha | Что |
|---|---|
| `3f22a57c` | **http и naive на движке.** Примитивы `$key`/`$value` (extract по элементам list), `$default_port`, перенаправление пространства у `single_into`. `PRIMITIVES` §0.10 |
| `90206933` | **Движок ВЕДЁТ РАЗБОР на продакшен-пути**, 8 схем `live: true`. Кэш планов, страж остатка, пять новых атрибутов записи, область `decode`, `space: "json"` исполняемы. Контракт 1.1.16 |
| `64377c76` | Разбор находок LxBox по сверке 1.1.15: REALITY вынесен в `tls#uri_reality` (trojan и http его не получают), `format: "pem"` исполняется, целые из реестра приводятся к int. Тесты пакета `subscription` переведены на реальный путь |

**На движке восемь схем:** trojan, vless, anytls, socks, ssh, http, naive,
shadowsocks. Зелено: `TestContractCorpusURI` (весь корпус на РЕАЛЬНОМ пути),
`TestEngineVsFixtures`, `TestContractMapperSectionsMatchSchema`,
`TestMappersWithoutLive`, `TestNoSchemeNamesInEngine`, весь пакет
`core/config/subscription`.

## СЛЕДУЮЩИЙ ШАГ — удалить рукописные ветки восьми схем

Работа начата не была: осмотр сделан, удаление НЕТ. Файл
`node_parser_core.go` делят другие агенты — удалять аккуратно и проверять
после каждой схемы.

**Что удалять** (проверено грепом, внешних читателей нет):

| Файл | Что с ним |
|---|---|
| `node_parser_http.go` | удалить целиком; вызов — `node_parser_core.go:375` |
| `node_parser_naive.go` | оставить `parseNaiveExtraHeaders`? НЕТ — движок её не зовёт (в `exec.go` только упоминание в комментарии). Удалить целиком, вызов `buildNaiveOutbound` — `:957` |
| `node_parser_anytls.go` | удалить целиком, вызов `:953` |
| `node_parser_ssh.go` | удалить целиком, вызов `:955` |
| `node_parser_ss.go` | **НЕ целиком**: `isValidShadowsocksMethod` зовёт эмиттер `shareuri_ss.go:19` |
| `node_parser_socks.go` | **НЕ целиком**: `socksSchemeForVersion` зовёт `shareuri_socks.go:27`; `isSocksScheme`/`socksVersionForScheme` держат ветки `node_parser_core.go` |

**Ветки `node_parser_core.go`:** dispatch-кейсы на строках 187 (vless), 190
(trojan), 193 (ss), 307 (anytls), 312 (ssh), 316/327/335/340 (socks), 356
(naive), 368 (http); ветки `buildOutbound` на `:945-965`. До них разбор уже
не доходит — `parseURIByEngine` возвращает раньше.

**Эмиттеры `shareuri_*.go` НЕ трогать** — это волна W7, обратное
направление.

Порядок: по одной схеме, после каждой `go build ./core/...` +
`go test ./core/config -run TestContractCorpusURI` +
`go test ./core/config/subscription`.

## Дальше по плану

1. **vmess** — единственная секция с `live: false` (страж
   `TestMappersWithoutLive`, список `notLiveYet`). Формы: base64-JSON
   (`space: "json"` уже исполняем) и legacy cleartext.
2. **hysteria2 / tuic / masque** — брать секции LxBox
   (`~/projects/LxBox/app/assets/contract_draft/uri/`, голова `dcdb6db4`,
   ТОЛЬКО чтение), сверять с нашим рукописным парсером и корпусом.
   `hysteria` v1 ссылкой у них нет — брать из `SCHEMES.md`.
   Список `schemesWithoutURISection` в страже сокращать по мере написания.
3. **wireguard / awg** — у LxBox в переключении, ждать.
4. **Xray-вход** — тем же способом: секции `mappers.xray` → `live` →
   удалить конвертер. У нас socks с Xray-входа = ЗВЕНО ЦЕПОЧКИ, не узел.

## Ловушки этой волны (проверено, не повторять)

- **Сверка ТЕЛ не видит трёх классов расхождений.** Продакшен-прогон нашёл
  29 красных там, где `TestEngineVsFixtures` был зелёным: коды деградаций,
  отказы разбора (`parse_error` против `emit_error` санитайзера) и
  написание схемы в `ParsedNode.Scheme`. Переключая схему, гоняй
  `TestContractCorpusURI`, а не только сверку.
- **`ParsedNode.Scheme` — НАПИСАНИЕ, а не имя схемы реестра.** `socks5`
  остаётся `socks5`: канонизация переименовала бы тег у живых узлов. Решает
  это `label.fallback.scheme_source`, и фолбэк тега обязан брать
  `node.Scheme`, а не имя схемы.
- **`ParsedNode.UUID` — ПЕРВЫЙ компонент userinfo**, а не «секрет». У naive
  секрет это `password` (второй компонент), а в UUID прежний путь клал
  `username`. Отметка `secret` в body.fields для этого НЕ годится.
- **Целое из реестра приезжает float64** и молча даёт ноль коду с
  ассертом `.(int)`. Канон сверки этого не видит: `json.Marshal` печатает
  одинаково. Q133-48.
- **Общий блок отдаёт своё ВСЕМ, кто его включил.** REALITY в `tls#uri`
  доставался trojan и http, хотя строит его только `vlessTLSFromNode`.
  Корпус молчал — у trojan и http нет фикстуры с `pbk`. Прежде чем класть
  запись в общий блок, проверь ВСЕХ его потребителей. Q133-47.
- **Тесты пакета читали внутренности старого пути.** Помощник
  `nodeBody(t, node)` + `bodyStrings`/`bodyHeaders`
  (`node_body_testhelper_test.go`) снимают разницу; файл удаляется вместе с
  `buildOutbound`.

## Документы

`SPEC.md` · `PRIMITIVES.md` (**§0.9 и §0.10 — добавления к FROZEN**) ·
`SCHEMES.md` · `DELTAS.md` (**D133-20**) · `QUIRKS.md` (**Q133-41…48**) ·
`contract/docs/MAPPER_ENGINE.md` · `contract/TASKS_LXBOX.md` **§24.26**
(ответы по шести находкам LxBox).


---

## Прежняя передача (19.09.2026 ночь)


Предыдущая передача (вечер) — в истории, коммит `ddf3b563`. Ниже — состояние
после волны W1: движок ИСПОЛНЯЕТ секции, пять схем переключены.

## Что готово (с sha)

| sha | Что |
|---|---|
| `dc94d7d1` | **Движок исполняет секцию.** `plan.go` (развёртка `include`, порядок объявления из текста JSON, множество «объявленных», `$ref`), `exec.go` (все FROZEN-примитивы), `parse.go` (свой лексер ссылки, декодеры оболочки), раннер сверки, греп-страж. **trojan переключён**, 31 кейс |
| `37cdc5b6` | Сверка прогоняет тело через `nodeflow.SanitizeFrom` — стадию 6 конвейера. Без неё сверка невозможна по построению: фикстуры сняты ПОСЛЕ санитайзера. vless 61 → 15 |
| `963dc6d6` | **vless переключён**, 103 кейса без дельт. Примитивы `overlay` и `not_in` добавлены к FROZEN-грамматике. Весь плоский набор XHTTP v2, слой `extra`, `ech` с `on_present`. Контракт 1.1.15 |
| (этот) | **anytls, socks, ssh переключены.** `duration` + `duration_bare_seconds`, подстановка `$host` в присваиваниях, `scheme` в `NewContent` |

**158 кейсов корпуса ведёт движок**, разрешённая дельта одна
(`trojan/ws_path_broken_percent_kept` — путь с битым `%zz` снимает
санитайзер, а не маппер).

Зелено: `go build ./core/...`, `TestEngineVsFixtures`,
`TestNoSchemeNamesInEngine`, `TestLinkmapEngineW0`,
`TestTraceCanonicalSerialization`, все три раннера корпуса.

## Как продолжать

`core/config/linkmap/testdata/switched.json` — список схем на движке. Ставишь
схему туда, гоняешь `go test ./core/config/linkmap -run TestEngineVsFixtures`,
чинишь расхождения по одному. Каждое расхождение = правка JSON секции, строка
`DELTAS.md`, либо недостающий примитив. Странность старого кода — СНАЧАЛА в
`QUIRKS.md`, потом чинить.

## Что осталось по уже влитым секциям

Порядок — по возрастанию цены.

1. **`http` — 4 кейса.** Нужны два примитива:
   - **произвольные заголовки** (`headers.<Имя>` из query-параметров с
     префиксом): `?header_X-Token=abc` → `headers: {"X-Token": "abc"}`.
     Сегодня это `node_parser_http.go`. Примитив общий — тот же нужен naive;
   - **`$default_port`** из `scheme_sets`: служебный путь, который движок
     сейчас пишет в тело литералом. Должен попадать в `defaults` секции, а
     не в тело.
2. **`naive` — 5 кейсов.** Те же заголовки (`extra_headers`), плюс разбор
   `password_only_userinfo` (одиночный userinfo без `:` = ПАРОЛЬ, конвенция
   DuckSoft; в секции объявлен `single_into`, движок его исполняет, но кейс
   падает — смотреть трассой).
3. **`shadowsocks` — 8 кейсов.** Нужна форма с **base64 на userinfo**, а не
   на всём тексте: `ss://base64(method:password)@host:port` и legacy
   `ss://base64(method:password@host:port)`. Сегодня `applyDecoder` умеет
   декодировать только текст целиком. Примитив: `forms[].decode` с областью
   применения (`userinfo` / `authority` / `all`) либо второй `overlay` над
   userinfo — решать по месту, но это ДОБАВЛЕНИЕ к FROZEN, и его надо
   записать в `PRIMITIVES` §0.9 и схему.
   **Дефолт порта ss (443 против 8388) НЕ трогать** — ждёт владельца,
   держится открытым пунктом в `DELTAS.md`.
4. **`vmess` — 16 кейсов.** `vmess://base64(JSON)` — форма, где после
   декодирования пространство становится JSON, а не ссылкой. В грамматике это
   уже есть (`forms[].decode: ["base64", "json"]` + `space: "json"`), но
   `UnwrapURI` после декодеров всегда зовёт `lexURI`. Нужна ветка по
   `form.Space`. Там же вторая форма — legacy cleartext
   `method:uuid@host:port`.

## Ловушки волны W1 (проверено, не повторять)

- **`priority` = МЕНЬШЕ значит РАНЬШЕ, и первый писавший держит путь.**
  Занятый путь перебивается только объявленным `merge`. Реализация «кто позже,
  тот и прав» даёт обратный результат на паре `flow` (10) / `packetEncoding`
  (20, `overwrite`).
- **Порядок объявления восстанавливается из ИСХОДНОГО текста JSON**
  (`objectKeyOrder`). Go-карта его теряет, а он нормативен.
- **`sets` с пустым набором `{}` ≠ `sets` с `null`.** Первое — «ничего не
  писать», второе — СНЯТЬ путь. На этом горели `security=none` у trojan и
  `$plaintext_port` у vless: соседняя запись со своим `default_from` успевала
  написать в блок, которого быть не должно (Q133-34).
- **Запись, объявляющая СТРУКТУРУ, обязана быть `selector`.** Иначе записи
  блока со своим `when: {tls.enabled: true}` исполнятся раньше неё и увидят
  не то тело. Так пришлось сделать `pbk` и `anytls.security`.
- **Записи протокола перекрывают одноимённые из `include` позицией**: план
  кладёт `include` ПЕРВЫМ, и запись секции объявлена позже. На этом стоят
  `vless.fp` и `anytls.fp` (дефолт `random`, D-009).
- **Описательные `aliases` движок не читает.** Написания параметра обязаны
  быть в ИСПОЛНЯЕМОМ `source`: девять имён `insecure` лежали в
  `tls.params.insecure`, и живой `allowInsecure=1` терялся.
- **Раннер сверки обязан звать санитайзер.** Без него сверка требует от
  движка чужой работы (нормализацию short_id, снятие мусорного flow).
- **Схему для сверки выбирает detect реестра, а не префикс ссылки**:
  `hy2://` → hysteria2, `socks5://` → socks, `naive+quic://` → naive.
- **Греп-страж читает только НЕтестовые файлы и только AST.** Комментарии
  обязаны называть вещи своими именами, иначе обоснование нечитаемо.

## Документы

`SPEC.md` · `PRIMITIVES.md` (**§0 — замороженная грамматика, §0.9 —
добавления волны W1**) · `SCHEMES.md` · `DELTAS.md` · `QUIRKS.md`
(**Q133-32…36 — найдено сверкой**) · `contract/docs/MAPPER_ENGINE.md`
(норма движка, общая с LxBox) · `contract/TASKS_LXBOX.md` §24.25.
