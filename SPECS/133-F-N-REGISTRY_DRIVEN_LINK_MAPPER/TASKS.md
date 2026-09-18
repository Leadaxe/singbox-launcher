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
