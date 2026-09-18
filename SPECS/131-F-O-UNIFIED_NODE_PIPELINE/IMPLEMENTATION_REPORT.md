# IMPLEMENTATION_REPORT — SPEC 131 · Единый конвейер добавления узла

Статус: **O — реализовано в develop, ждёт релиза** (18.09.2026). Закрывается
в **C** после релиза лаунчера с этими изменениями (релиз — решение владельца).

## Что сделано

| Волна | Коммит | Содержимое |
|---|---|---|
| hotfix | `5627be78` | vmess `security` по набору ядра (−`aes-128-ctr`, +`aes-128-cfb`), гард enum-полей xhttp на трёх входах; 3 кейса корпуса |
| W0+W1 | `b56a93ad` | ТЗ (SPEC/TASKS/CODEMAP/DRIFT/CORE_SCHEMA), контракт 1.1.0: `body`-секции (384 поля), `on_invalid`/`maps_to`/`advisory`, тексты 69 кодов en+ru, CANON §6/§8, схемы, линтер реестра, D-122, TASKS_LXBOX §24 |
| — | `7d4053ab` | §24.5 — ответы LxBox |
| W2b | `946dd4ec` | `configtypes.Warning`, `state.Node.Warnings`, швы обеих воронок, релеи, бэкап, раннер корпуса с объектными warnings |
| W2a | `e9f7e8eb` | `contract/embed.go`, `core/config/registry`, `core/config/nodeflow` (Sanitize/Emit/GateForCore) |
| W4 | `2c30fa4c` | `contract/tools/gendocs` → `contract/docs/generated` (23 страницы), workflow `Contract`, `docs/Protocols*.md` → ссылка |
| W3 | `1ac403f5` | ⚠ в списках, секция «Предупреждения» с ссылкой на якорь кода, превью источника, AWG-форма через конвейер, `internal/nodewarn`, locale |
| W2c | `95791e73` | все входы через `node_materialize.go`, гейт ядра на сборке, миграция warnings при загрузке, `max_uri_length` 65536, `TestCorpusBodiesPassSingboxCheck`; §24.6 |
| pin | `ccb33731` | ядро 1.14.1-lx.7 (SPEC 090/091/092 ядра) |
| W2d | см. лог | парсеры → мапперы (21 правило снято, 6 в реестр, 14 структурных), алиасы из реестра, naive userinfo=password, `nodeflow_pipeline_test.go`, `TestRegistryWarningCodesHaveAProducer`; `W2D_CHANGES.md`, §24.7 |
| fix xmux | `9379de95` | связи судят ЗНАЧЕНИЕ («0»/«0-0» = не задано), `all_or_nothing` без действия санитайзера, код `partial_object_defaulted` снят; кейс `body/singbox/vless_xhttp_xmux_zero_neighbours`; мёртвый `codes` в `SanitizeSingboxOutboundMap`; DRIFT §7.20, §24.9 |
| Docs v2 | см. лог | `contract/docs/generated` только по-английски, списки вместо широких таблиц; страница схемы самодостаточна (Link parameters → Body fields → Diagnosed problems → Replacements → Degradation); `cause_*`/`fix_*` у всех кодов, `registry.WarningAdvice`, причина и решения в карточке узла; секция `mapper` (28 правил) со схемой и линтером; старый `degrade[]` снят; контракт 1.1.1 |
| gRPC #130 откат | см. лог | Ядро `1.14.1-lx.8` (SPEC 093) само разбирает ведущий «/» в `service_name` как custom path Xray (посегментное экранирование, хвост `|…` отбрасывается), поэтому перевод `/service/Tun` → `service` снят **целиком**: правило `normalize: grpc_service_name`, значение в enum схемы, `registryNormalizeModes` линтера и `normalizeGRPCServiceName` с юнитами удалены; значение едет ядру как есть. Пин `RequiredCoreVersion` → `1.14.1-lx.8`, гейт `min_core` не вводится. Корпус: 3 кейса перенормированы, 4 новых (`/x/Tun`, `/a/b/Tun`, `/a/Stream`, `a/b`). Контракт 1.1.3, DRIFT §7.21, `TASKS_LXBOX.md` §24.12 |
| QUIC в реестр | см. лог | Срез `utls`/`reality` на QUIC — правило реестра: `forbidden_for` у `tls.json` `body.fields.utls`/`reality` + четыре схемы (`hysteria` v1 названа впервые — проверено по `sing-quic hysteria/client.go:264`), новый атрибут `forbidden_codes` (схема → код) и код `tls_not_applicable_quic` (`info`, один на блок). Снята частная ветка `quicOutboundTypes` в `singbox_sanitize.go` и молчаливый пропуск `fp` в URI-парсерах `hysteria`/`hysteria2`/`tuic` — маппер переводит `fp`/`pbk`/`sid`/`key_share` общей `applyTLSCamouflageFromQuery`. Тела не изменились, узел теперь получает код на ОБОИХ входах. Генератор перестал обещать запрещённое поле рабочим (Link parameters + Body fields, запрет ищется по всему пути). Парные кейсы `uri/hysteria2/reality_fp_stripped_quic_pair` ↔ `body/singbox/hysteria2_quic_tls_pair`, `uri/tuic/fp_stripped_quic_pair` ↔ `body/singbox/tuic_quic_tls_pair`. Контракт 1.1.4, DRIFT §7.22, `TASKS_LXBOX.md` §24.13, аудит остатков — `LEGACY_AUDIT.md` |
| info — значок в подстроке | см. лог | Правка владельца к разведению уровней: текстовый «(i)» у ИМЕНИ узла снят (`nodewarn.InfoMarkFor`/`WithInfoMark`, `previewRowTitleShown` удалены — имя снова чистый тег), info показывает иконка темы `theme.InfoIcon()` («i в кружке», SVG, есть и на Win7) во ВТОРОЙ строке: слева от состава у здорового узла, в конце подстроки у узла с `✖`/`⚠`. Общая `nodewarn.InfoSubtitleLine` (12pt под кегль подстроки, цвет `ColorNamePlaceHolder`, в `widget.List` создаётся в шаблоне и переключается `Show`/`Hide`) на всех поверхностях: Servers, Core runtime, Preview окна источника, drill-down папки. В карточке узла та же иконка у подзаголовка «К сведению»; в тултипе остаётся текстовый `InfoMark` — виджет туда не вставить. `TASKS_LXBOX.md` §24.11 |

## Критерии приёмки (SPEC §9)

1. Схемных ветвлений в санитайзере/эмиттере нет — ✅ (`nodeflow` без `if scheme ==`; per-scheme switch в `outbound_generator.go` остался только для `LegacyNodeIdentityHash` и превью цепочек — не на пути тела узла).
2. Пара ссылка↔JSON даёт равные тело и warnings — ✅ (`uri/vless/junk_pair_with_body` ↔ `body/singbox/vless_junk_pair`, тест `nodeflow_pipeline_test.go`).
3. Пункты DRIFT закрыты кейсом или DECISIONS — ✅ (D-122; §7.1–7.18; порт вне диапазона в ссылке — форма `dropped[]`, отдельное решение, W2D_CHANGES §6).
4. `sing-box check` на сводном конфиге из всех кейсов — ✅ (`TestCorpusBodiesPassSingboxCheck`, 239 outbound + 43 endpoint).
5. ⚠ виден, причина в карточке, ссылка на существующий якорь — ✅ (`warnings.md#<code>` генерируется).
6. `go generate ./contract/...` идемпотентен, CI-диф — ✅ (workflow `Contract`).
7. Win7 (go1.20) — ✅ (`go vet -modfile=go.win7.mod` на новых пакетах; без `slices`/`maps`/`min`/`max`/`clear`).

## Проверки

`go build ./...`, `go vet ./...`, `go test ./core/... ./contract/... ./internal/... -count=1` — зелёные (полный прогон после Docs v2). Корпус: 282 тела, 21 per-app override (было 24). `go generate ./contract/...` идемпотентен (два прогона, побайтно). Win7: `go vet -modfile=go.win7.mod` на `nodeflow`/`nodewarn`/`subscription` — чисто (файлы go.win7.* восстановлены после прогона).

Прогон настоящего state владельца (186 узлов) после фикса xmux: тело не переписывается ни у одного узла (было — у 13), кодов `field_conflict`/`partial_object_defaulted` — 0, ошибок 0; остаются 12 info `reality_fp_not_chrome`. Единственное расхождение с origin.raw — WG-узел с правками AWG-формы в теле (известно, не баг).

## Открытые хвосты

- Порт вне 1–65535 в ссылке: остаётся отбраковка узла — без `server_port` узел ядру не нужен, оба пути дают один результат (вопрос снят 18.09.2026).
- `fp→random` (vless) и `vhttp→h3` (masque) остались в парсерах как кросс-проектные конвенции идентичности (W2D_CHANGES §6), не дефолты ядра.
- naive userinfo=password — ограничение по дате снято владельцем 18.09.2026; LxBox догоняет своим релизом, до тех пор в корпусе per-app override `.expected.lxbox.json`.
- Кириллица в `desc_ru`/`impl` реестра остаётся (её читает UI и разработчик); в `contract/docs/generated/` её ноль — проверяется грепом по диапазону.
- Закрытие папки в **C** после релиза.
