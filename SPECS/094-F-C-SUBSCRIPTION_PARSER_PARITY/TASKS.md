# TASKS 094 — Паритет парсера подписок с LxBox

Чеклист по фазам. Реализация — по PLAN.md, границы — по SPEC.md.

---

## Фаза A — импорт sing-box JSON

### A.1 Классификатор формата
- [x] `core/config/subscription/body_classify.go`: тип `BodyKind` + `ClassifySubscriptionBody`
- [x] Порядок проверок по SPEC §3.1 A1 (`type` раньше `outbounds`)
- [x] `IsXrayJSONArrayBody` (`xray_json_array.go:15`) → обёртка над классификатором
- [x] `DecodeSubscriptionContent` (`decoder.go:68-71`) перестаёт отвергать `{`
- [x] `body_classify_test.go`: все 6 видов + пограничные (одиночный selector, пустой массив, битый JSON)

### A.2 Санитайзы над готовой map
- [x] `core/config/subscription/singbox_sanitize.go`: `SanitizeSingboxOutboundMap`
- [x] Переиспользовать существующие: `NormalizeUTLSFingerprint`, `normalizeRealityShortID`, `isValidRealityPublicKey`, hysteria2 obfs, flow whitelist, packet_encoding
- [x] Тесты: битый pbk → plain TLS; неизвестный fp → блок не пишется; мусорный short_id → поле снято

### A.3 Ядро импорта
- [x] `core/config/subscription/singbox_import.go`: `ParseNodesFromSingboxConfigs`, `parseSingboxConfig`, `parseSingboxEntry`
- [x] `outbounds` + `endpoints` одним списком (A2)
- [x] Служебные типы `direct`/`block`/`dns` отбрасываются (A3)
- [x] Ошибка одного entry не роняет остальные (A6)
- [x] Порядок: узлы в порядке появления, группы после узлов (A7)
- [x] Сбор списка проигнорированных секций (A4)

### A.4 Группы
- [x] `core/config/subscription/singbox_groups.go`: `selector`/`urltest` → `configtypes.OutboundConfig`
- [x] Членство по идентичности узла, не по сырому тегу (A5)
- [x] Неразрешимый член отбрасывается, счётчик в лог
- [x] Пустая группа не создаётся
- [x] `Ref` пустой, `Updates` не используется (Р5)

### A.5 Интеграция
- [x] `SourceLoadResult{Nodes, ImportedOutbounds, IgnoredSections}` вместо `[]*ParsedNode`
- [x] `source_loader.go:146`: ветвление по `BodyKind`
- [x] Правка трёх вызывающих: `outbound_generator.go:678`, `rebuild_raw_cache.go:90`, `preview_cache.go:65`
- [x] Импортированные группы мержатся на время генерации, в `state.json` не пишутся
- [x] Превью показывает проигнорированные секции

### A.6 Тесты и приёмка
- [x] `testdata/singbox_full_config.json` — целый конфиг с route/dns/inbounds/группами/цепочкой
- [x] `singbox_import_test.go`: критерии 1–7
- [x] Сквозной: результат проходит `sing-box check` (критерий 8)
- [x] `go build ./... && go test ./... && go vet ./...`

---

## Фаза B — detour-цепочки

### B.1 Модель
- [x] `configtypes/types.go`: `ParsedNode.Chain []*ParsedNode`
- [x] `Jump` помечен deprecated, синхронизация с `Chain[0]` в обе стороны
- [x] Миграция чтения `state.json`: пустой `Chain` + непустой `Jump` → `Chain = [Jump]`
- [x] Тест на старую форму state.json (критерий 13)

### B.2 Алгоритм
- [x] `core/config/subscription/detour_chain.go`: `findCycleEdges`, `analyzeDetour`, `buildChain`
- [x] Порядок: кольца ПЕРВЫМИ, до вычисления `detourTargets` (B3)
- [x] Лимит глубины 8 (B2)
- [x] Обрывы: висячая ссылка, группа, служебный тип (B5)

### B.3 Интеграция
- [x] `outbound_generator.go:848-862`: цикл по `Chain`
- [x] `sanitizeNodeDetours` (`:908`) сохраняется как второй рубеж (B6)
- [x] Импорт цепочек из sing-box конфига (связь с A.3)

### B.4 Тесты
- [x] Критерии 9–13
- [x] Кольцо A→B→A даёт обе ноды, `sing-box check` проходит
- [x] `go build ./... && go test ./... && go vet ./...`

---

## Фаза C — Xray-массив

### C.1 Протоколы
- [x] `xray_json_array.go:83-106`: снять фильтр `protocol != "vless"`
- [x] `xray_outbound_convert.go`: конвертеры vmess, trojan, shadowsocks, hysteria2
- [x] Служебные Xray-протоколы (`freedom`/`blackhole`/`dns`/`loopback`) не становятся узлами
- [x] Неподдержанный protocol логируется, не исчезает молча (C1)

### C.2 Все узлы элемента
- [x] Сбор всех непослужебных outbound'ов элемента (C2)
- [x] Именование по C3: 1 узел → чистый `remarks`; N узлов → `remarks` + различитель
- [x] `pickMainXrayVLESS` используется только для именования, не меняет порядок выдачи (C6)

### C.3 Транспорты и цепочки
- [x] `xrayTransportFromStreamSettings` (`:222`) + `xhttp`, `httpupgrade` (C5)
- [x] `xrayBuildJumpFromOutbound` (`:314`) — все протоколы C1, глубина из фазы B (C4)

### C.4 Тесты
- [x] Критерии 14–17
- [x] Регрессия на `testdata/xray_provider_anon.json`: теги одноузловых элементов не изменились (критерий 18)
- [x] `go build ./... && go test ./... && go vet ./...`

---

## Фаза D — идентичность узла

### D.1 Ключ и хеш
- [x] `core/config/subscription/node_identity.go`: `NodeIdentityKey` (D1)
- [x] `core/config/node_hash.go`: `NodeIdentityHash` в пакете `config` (доступ к эмиттеру)
- [x] Hook для вызова из `subscription` без циклического импорта (по образцу `LookupCachedBody`)
- [x] Исключение `tag` и `detour`, детерминированный порядок ключей (D2)

### D.2 Дедупликация
- [x] Дедуп внутри источника в `LoadNodesFromSource`, до применения тегов (D3)
- [x] Между источниками не применяется

### D.3 Выключение отдельной ноды
- [x] `ProxySource.DisabledNodes map[string]int64` (хеш → время отметки)
- [x] Фильтрация в `LoadNodesFromSource`
- [x] GC по TTL = `clamp(3 × интервал, 24h, 30d)`, только при успешном сетевом обновлении
- [x] UI: чекбокс в превью узлов (`source_edit_window.go`)

### D.4 Тесты
- [x] Критерии 19–22
- [x] Хеш стабилен к переименованию и `tag_prefix`, меняется при смене порта
- [x] `go build ./... && go test ./... && go vet ./...`

---

## Закрытие задачи

- [x] `go test -tags=live ./core/config/subscription/... -run TestLiveParsePublicSubscriptionFiles` (критерий 24)
- [x] Ни один сценарий не роняет весь config.json (критерий 25)
- [x] `docs/release_notes/upcoming.md` — EN + RU
- [x] `docs/ARCHITECTURE.md` — потоки данных (`SourceLoadResult`)
- [x] `docs/ParserConfig.md` — описано новое поле `ProxySource.disabled_nodes`
- [x] `IMPLEMENTATION_REPORT.md`
- [x] Переименовать папку в `094-F-C-SUBSCRIPTION_PARSER_PARITY`
