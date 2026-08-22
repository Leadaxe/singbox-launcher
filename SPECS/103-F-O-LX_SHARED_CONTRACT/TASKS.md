# TASKS 103 — LX Shared Contract

Статус: план утверждён (D-004). Фазы 0 и 1 выполнены 2026-08-19; паритет URI-корпуса 282/282.
Фаза 3 (движок шаблонов) выполнена 2026-08-19: корпус 70/70 на обеих сторонах.
Модель пресетов и каналы вынесены в SPEC 106 (D-049…D-053).
`#enable`, рекурсия условий, помеченные ключевые слова и реактивный пересчёт — **SPEC 107 выполнен 2026-08-22** (D-065…D-076): корпус 94+16 deps, 100 % на обеих сторонах; issue #106 закрыт попутно.

## Фаза 0 — Скелет контракта ✅ (34 файла в `contract/`, все JSON валидны)

- [x] `contract/` каркас: VERSION (0.1.0), README.md
- [x] `schema/node.schema.json` — канонический узел (outbound|endpoint|group, chain, warnings, dropped)
- [x] `schema/registry.schema.json` + `registry/protocols/<scheme>.json` — 14 схем per-file + `transports.json`, `tls.json`, `allowlists.json` (7 списков), `containers.json`; написано агентами с построчной сверкой обоих парсеров, дефекты проверяющих исправлены
- [x] `registry/warnings.json` — 38 кодов (18 Dart-классов + 19 Go-деградаций + clash_yaml_unsupported)
- [x] `registry/limits.json` — 9 лимитов (включая найденные расхождения amnezia-cap 8/4 MiB, fetch 30/9 s)
- [x] `registry/vars.json` (47 имён, 15 portable) + `registry/presets.json` (21 id, 4 общих)
- [x] `diagrams/body_classify.mmd`, `parse_pipeline.mmd`, `emit_pipeline.mmd`
- [x] `docs/CANON.md`, `docs/IDENTITY.md`, `docs/TEMPLATE_LANG.md` (нормативно, разрывы C1–C9 + N1–N12), `docs/BACKUP.md`
- [x] Ревью пользователем: план утверждён целиком (D-004); по-фикстурные решения §9.E — в фазах 1–2 по принципам D-016
- [x] Живые баги из §9.E закрыты в фазе 1 (hysteria2 obfs без пароля, gecko, reserved из wgconf, httpupgrade `?ed`-хвост, reality sid, http-нода из импорта, WG-ключи на JSON-пути в LxBox). Реализация SPEC 102 при этом не тронута — только её обвязка (фикстуры, release notes, отчёт)

## Фаза 1 — Корпус URI + раннеры ✅ (паритет 282/282)

- [x] Перенос `LxBox/app/test/fixtures/**` (.uri/.conf) в `contract/corpus/uri/` + `corpus/README.md` (конвенции)
- [x] Экстракция inline-кейсов из Go `node_parser_*_test.go` и Dart `test/parser/*` — **282 фикстуры, все 13 схем**, данные синтетические, дублей нет
- [x] Go: `core/config/contract_canon_test.go` + `core/config/contract_test.go` (в пакете `config` — `subscription` импортировать нельзя, цикл; оба строго `*_test.go` — Win7 go1.20 их не компилирует)
- [x] Первый прогон с `-update`: 282 expected сгенерированы; **233 кейса парсятся, 49 в dropped** (из них ~24 ожидаемые отказы, ~25 — дыры лаунчера)
- [x] Dart: `app/tool/sync_contract.sh` + `contract.lock` + `test/contract/contract_test.dart`; прогон сгенерировал 282 `*.expected.lxbox.json` (перенесены в источник контракта)
- [x] **Дифф двух наборов ожиданий** → `research/CORPUS_DIFF.md`: entry совпал 144, оба дропают 20, дропает только лаунчер 29, только LxBox 8, entry различается 81
- [x] **Паритет URI-корпуса достигнут: 282/282** (256 совпали, 24 законных отказа с обеих сторон, 2 per-app override для desktop-расширений ssh; односторонних отказов и расхождений — ноль)
- [x] Выравнивания по диффу (по-фикстурно, принципы D-016):
  - [x] WG private key из query — Go (D-021) + дефолт `allowedips` (D-022)
  - [x] Схемы `proxy-http*` — Go (§9.B6); попутно починена потеря auth/path/headers у http-нод из sing-box-импорта
  - [x] `mport` / base64-payload hysteria2, `naive+quic`, `vpn://` строкой — Dart (§9.B1/B2/B12)
  - [x] Канон без дефолтных полей: WG `name`/`system`/`mtu` — Go (D-010/D-026); TUIC-дефолты, `transport.path:"/"`, `host`, `tls.enabled:false`, MTU — Dart
  - [x] uTLS: дефолт `random` для anytls (D-009) + единая junk-политика `chrome`+warning на всех путях (D-029); `utls` убран у QUIC (D-033)
  - [x] `early_data_header_name` дефолт — Dart (D-008); плоские `ed`/`eh` — Go
  - [x] reality на anytls — Go; дефолт plain-WG MTU отдан ядру (D-026); длительности с единицей измерения — Dart (D-024, живой баг)
  - [x] Двойное percent-кодирование пути — Go (D-028; решено эмпирикой `net/url`: путь уезжал на сервер как `%252F` → 404)
- [x] Документация парсеров: `docs/ParserConfig.md` (+.ru) и LxBox `docs/PROTOCOLS.md` ссылаются на `contract/registry` как нормативный источник (D-020); release notes дописаны (EN+RU)
- [x] Тесты обоих проектов зелёные (лаунчер целиком; LxBox 3340). Прогон в CI-пайплайнах — фаза 5
- [x] Проверка на живой системе: сборка с `-i`, перепарс подписок и пересборка конфига через debug API — 42 ноды, ни одна не потеряна, VPN не прерывался

## Фаза 2 — Тела, эмиссия, identity (Go ✅, Dart — остаток)

- [x] `corpus/body/` — 13 фикстур: uri-list, base64 (оба алфавита), singbox (4 формы + endpoints-WG), xray-массив, wgconf, vpn:// (сжатый и несжатый)
- [x] Go: ветка `BodyKindWGConf` в `ClassifySubscriptionBody` (B11) + `WGConfBodyToURIs`; раннер `TestContractCorpusBody` входит через decode-слой, не через кэш-хук
- [x] Go: `vpn://` как целое тело (`BodyKindVPNLink` → `ParseAmneziaVPNLinkAll`, ВСЕ контейнеры), несжатый профиль принимается
- [x] `corpus/emit/` — round-trip поверх корпуса URI (`TestContractCorpusEmitRoundTrip`): 256/258, 2 законных отказа; **8 багов эмиссии закрыто**, 2 асимметрии оставлены by-design (D-028, ws-Host из sni)
- [x] `docs/IDENTITY.md` нормативно + `core/config/identity_contract_test.go` (8 тестов; §3 закрыт и защищён от возврата)
- [x] Go: `ParsedNode.Warnings` — коды деградации из реестра; строгая сверка `warnings` в конверте корпуса
- [x] Sync-тесты реестра (Go): `registry_sync_test.go` — allowlist'ы кода против `registry/allowlists.json`; сразу поймал незакрытый gecko
- [ ] Dart: ветка `vpn://` в `parseUri` для строки внутри URI-списка (B12); отдельный лимит длины vpn:// (сейчас режется общим maxURILength 65536)
- [ ] Dart: раннер `corpus/body/` + коды warnings в конверте; sync-тесты реестра (`parseUri`, `kUtlsFingerprints`, …)

## Фаза 3 — Шаблоны (движок ✅, пресеты → SPEC 106)

- [x] `docs/TEMPLATE_LANG.md` — нормативная спека (ядро/расширения, толерантность к неизвестным секциям)
- [x] `corpus/template/` — 70 фикстур, **70/70 на обеих сторонах**; 7 разделов + `grammar/`
- [x] Выравнивание движков: Go — C4 (Dropped-каскад, D-054), C5 (каст по типу, D-055), C3 (невалидное `#if` → false, D-058), N9/N10 (warnings); Dart — C1/N12, C2, C6, C8, N10 (D-056/D-057). Осталось C7 → SPEC 106
- [x] `registry/presets.json`, `registry/vars.json` (в фазе 0); **выравнивание словарей снято** — D-046, шаблоны остаются разными
- [~] Схема пресета — модель канонизирована по LxBox (D-049) → перенос в **SPEC 106**
- [ ] LxBox: миграция тегов пресетов на префикс (C7) — heal-редиректы ссылок + release notes → **SPEC 106**
- [ ] Документация шаблонов в порядок: лаунчер `TEMPLATE_REFERENCE.md`+`WIZARD_TEMPLATE.md` (+.ru) ↔ LxBox `TEMPLATE.md` — сверка с contract/docs/TEMPLATE_LANG.md (D-020)

## Фаза 4 — LX Backup v1

- [ ] `schema/backup.schema.json` + `docs/BACKUP.md`
- [ ] Лаунчер: export/import (state v6 ↔ backup) + UI в конфигураторе + **хранение чужого блоба extensions** (новое поле state v6)
- [ ] LxBox: export/import (settings ↔ backup; allowlist §159 расширен под формат) + пункт на backup-экране + **хранение чужого блоба extensions** (новый ключ allowlist)
- [ ] Политика символических ссылок: несуществующая цель `rules[].outbound`/`route.final` → импорт выключенным / не применяется, с warning
- [ ] `corpus/backup/` — фикстуры экспорт→импорт в обе стороны (включая lossless round-trip через extensions)
- [ ] Перенос disabled-нод: только по identity-хешу (значения — unix seconds, конвертация в нативный формат); несовпавшие (патченные §302, SNI-схлопнутые) истекают по TTL — задокументировать в BACKUP.md
- [ ] Документация состояния/правил в порядок: лаунчер `WIZARD_STATE.md` (+.ru) ↔ LxBox `STORAGE.md` — дополнить форматом LX Backup (D-020)

## Фаза 5 — Процесс

- [ ] «Контракт раньше кода»: поправка в CONSTITUTION.md лаунчера; в LxBox — создать документ принципов (или раздел в AGENTS.md — CONSTITUTION там нет) + PR-чеклисты
- [ ] CI-пин contract.lock в LxBox; договорённость о версионировании (semver VERSION)
- [ ] (опц., решение пользователя) вынос `contract/` в отдельный репо `lx-contract`
