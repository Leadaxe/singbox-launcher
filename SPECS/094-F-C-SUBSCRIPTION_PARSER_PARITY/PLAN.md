# PLAN 094 — Паритет парсера подписок с LxBox

Реализация SPEC 094. Четыре фазы, каждая — самостоятельно поставляемая.

---

## 0. Архитектурные решения (приняты при планировании)

### Р1. Не переписывать `ParsedNode`

Модель остаётся гибридной: скаляры + `Outbound map[string]interface{}`.
Обоснование: входной sing-box outbound **уже является** тем самым JSON, который
лаунчер эмитит. Импорт — это по сути «положить готовую map в `node.Outbound`»
плюс нормализация. Переход на sealed-иерархию не даёт ни одной пользовательской
функции и ломает 35 тест-файлов.

Следствие: фаза A — это **не портирование `NodeSpec`**, а тонкий слой поверх
существующей модели.

### Р2. Импорт sing-box outbound идёт через нормализацию, а не через реэмиссию

Входная map кладётся в `node.Outbound` после прогона через **существующие**
санитайзы (uTLS allowlist, reality pbk/short_id, hysteria2 obfs, flow whitelist,
packet_encoding). Мы не разбираем её на поля и не собираем обратно — это удвоило
бы поверхность ошибок.

Отсюда важное требование: санитайзы, сегодня встроенные в URI-путь, должны стать
вызываемыми над готовой map. Часть уже такова (`NormalizeUTLSFingerprint`,
`normalizeRealityShortID`, `isValidRealityPublicKey`), часть — нет.

### Р3. Цепочка — срез, а не связанный список

`ParsedJump` (`configtypes/types.go:204`) по полям почти совпадает с
`ParsedNode`. Вместо замены его на рекурсивную структуру вводится
`ParsedNode.Chain []*ParsedNode` — упорядоченный список хопов от ближнего к
дальнему. `Jump` сохраняется как **deprecated-поле** и вычисляется из `Chain[0]`
для обратной совместимости чтения/записи `state.json`.

Обоснование: эмиттер (`outbound_generator.go:848-862`) уже умеет
«сгенерировать jump-outbound, потом основной». Со срезом это разворачивается в
цикл без переписывания эмиссии.

### Р4. Классификатор формата — одна функция, один источник правды

Сегодня детект размазан: `IsXrayJSONArrayBody` (`xray_json_array.go:15`) в
`source_loader.go:146` и `DecodeSubscriptionContent` (`decoder.go:21`) в
`fetcher.go:293`. Фаза A вводит `ClassifySubscriptionBody(body string) BodyKind`
как единственную точку решения; обе существующие функции начинают опираться на
неё, а не решать самостоятельно.

Критично: `DecodeSubscriptionContent` сейчас **отвергает** тело, начинающееся с
`{` (`decoder.go:68-71`). Это отсекает целый конфиг ещё до парсера — правится в
фазе A.

### Р5. Импортированные группы — direct entries

`OutboundConfig.Ref` пустой (`configtypes/types.go:141` — full ownership).
Механика `Ref`/`Updates` из SPEC 057/058 не переиспользуется: там она про
template/preset-биндинг, здесь про данные из подписки. Смешение дало бы
неразрешимый конфликт при `SyncOutboundsWithActivePresets`.

---

## 1. Фаза A — импорт sing-box JSON

### Новые файлы

| Файл | Содержимое |
|---|---|
| `core/config/subscription/body_classify.go` | `BodyKind` enum + `ClassifySubscriptionBody` (Р4) |
| `core/config/subscription/singbox_import.go` | ядро: `ParseNodesFromSingboxConfigs`, `parseSingboxConfig`, `parseSingboxEntry` |
| `core/config/subscription/singbox_groups.go` | `selector`/`urltest` → `configtypes.OutboundConfig` |
| `core/config/subscription/singbox_sanitize.go` | санитайзы над готовой map (Р2) |
| `core/config/subscription/singbox_import_test.go` | тесты фазы A |
| `core/config/subscription/body_classify_test.go` | тесты классификатора |
| `core/config/subscription/testdata/singbox_full_config.json` | фикстура: целый конфиг с route/dns/inbounds/группами/цепочкой |

### Изменяемые файлы

| Файл | Изменение |
|---|---|
| `decoder.go:68-71` | не отвергать `{`; делегировать классификацию в `ClassifySubscriptionBody` |
| `source_loader.go:146` | ветвление по `BodyKind`, а не по `IsXrayJSONArrayBody` |
| `xray_json_array.go:15` | `IsXrayJSONArrayBody` → обёртка над классификатором |
| `configtypes/types.go` | `ProxySource` — способ вернуть импортированные группы (см. ниже) |

### Ключевые контракты

**`BodyKind`:**
```
BodyKindURIList | BodyKindXrayArray | BodyKindSingboxOutbound |
BodyKindSingboxOutboundArray | BodyKindSingboxConfig | BodyKindSingboxConfigArray
```
Порядок проверок — по SPEC §3.1 A1. `type` раньше `outbounds` (одиночный
`selector` несёт оба).

**Возврат групп.** `LoadNodesFromSource` сегодня возвращает `[]*ParsedNode`.
Группы возвращать отдельным каналом: расширить возврат до структуры
`SourceLoadResult{Nodes []*ParsedNode; ImportedOutbounds []configtypes.OutboundConfig}`.
Все три вызывающих (`outbound_generator.go:678`, `rebuild_raw_cache.go:90`,
`preview_cache.go:65`) правятся согласованно.

Импортированные группы мержатся в `ProxySource.Outbounds` **на время генерации**,
не записываясь в `state.json` — иначе refresh подписки будет плодить дубли и
конфликтовать с пользовательскими правками. Это же снимает риск из SPEC §5.

**Ядро разбора** (`singbox_import.go`):
```
ParseNodesFromSingboxConfigs(configs []map[string]any, skip) ([]*ParsedNode, []OutboundConfig, error)
  для каждого config:
    entries := outbounds ++ endpoints          // A2
    detourTargets, brokenEdges := analyzeDetour(entries)   // фаза B, в A — заглушка
    для каждого entry:
      служебный тип (direct/block/dns) → skip  // A3
      группа (selector/urltest) → в groups     // A5
      иначе → parseSingboxEntry → ParsedNode
    groups → resolveGroupMembers → OutboundConfig
```

**`parseSingboxEntry`** — минимальная работа: валидация `type`, `server`,
`server_port`, затем санитайзы Р2 над копией map. Каждый entry обёрнут в
`recover`-free защиту через явные `is`-проверки типов (в Go паника от
`map[string]any` возможна только при type assertion без `, ok` — их не пишем).

**A6 гранулярность:** ошибка одного entry → `debuglog.WarnLog` + `continue`.

**A4 игнорируемые секции:** собрать список фактически присутствовавших
(`route`/`dns`/`inbounds`/`experimental`) и вернуть в `SourceLoadResult` для
показа в превью.

---

## 2. Фаза B — detour-цепочки

### Изменяемые файлы

| Файл | Изменение |
|---|---|
| `configtypes/types.go:204,232` | `ParsedNode.Chain []*ParsedNode`; `Jump` → deprecated, синхронизируется с `Chain[0]` |
| `core/config/subscription/detour_chain.go` (новый) | `analyzeDetour`, `buildChain`, `findCycleEdges` |
| `outbound_generator.go:848-862` | цикл по `Chain` вместо одиночного `Jump` |
| `xray_outbound_convert.go:314` | `xrayBuildJumpFromOutbound` → возвращает звено цепочки |
| `core/state/*` | миграция чтения старого `Jump` |

### Алгоритм (порядок критичен — SPEC §3.2 B3)

```
1. findCycleEdges(entries)  → brokenEdges   // ПЕРВЫМ
2. detourTargets = {цель detour | ребро не в brokenEdges}
3. кандидаты = entries − служебные − группы − detourTargets
4. для каждого кандидата: buildChain(depth=8)
```

Если пункт 1 сделать после 2, узел в кольце попадёт в `detourTargets` и исчезнет
совсем (критерий приёмки 11).

`buildChain` останавливается на: лимите 8 (B2), повторе тега в текущей цепочке
(B3), отсутствующей цели (B5), группе (B5), служебном типе (B5 — молча).

**Миграция state.json:** при десериализации, если `Chain` пуст, а `Jump` есть —
`Chain = [Jump]`. При сериализации `Jump = Chain[0]` (или nil). Тест — критерий 13.

---

## 3. Фаза C — Xray-массив

### Изменяемые файлы

| Файл | Изменение |
|---|---|
| `xray_json_array.go:83-106` | снять фильтр `protocol != "vless"`; собирать все узлы элемента |
| `xray_outbound_convert.go` | конвертеры vmess/trojan/shadowsocks/hysteria2 |
| `xray_outbound_convert.go:222` | `xrayTransportFromStreamSettings` + `xhttp`, `httpupgrade` |
| `xray_json_array_test.go` | расширение |

### Именование (C3, критерий 18)

```
узлов в элементе == 1 → tag = remarks            // как сегодня, теги не меняются
узлов в элементе > 1  → tag = remarks + " " + (тег outbound | индекс+1)
```

Регрессия на `testdata/xray_provider_anon.json` обязана дать те же теги для
одноузловых элементов.

### Порядок (C6)

Порядок выдачи = порядок в подписке. `pickMainXrayVLESS` (`:172`) сегодня
переставляет узел с dialerProxy вперёд — при переходе на «все узлы» логика
выбора главного узла нужна только для именования, но **не должна** менять
порядок выдачи.

---

## 4. Фаза D — идентичность узла

### Новые файлы

| Файл | Содержимое |
|---|---|
| `core/config/subscription/node_identity.go` | `NodeIdentityKey`, `NodeIdentityHash` |
| `core/config/subscription/node_identity_test.go` | тесты |

### Контракты

**`NodeIdentityKey(node) string`** — `scheme|server|port|credential`.
`credential` = `UUID` либо пароль в зависимости от схемы. Транспорт и TLS не
входят (D1).

**`NodeIdentityHash(node) string`** — SHA-256 от эмитированного outbound-JSON
минус `tag` и `detour`, с отсортированными ключами.

Реализация обязана переиспользовать эмиссию, а не дублировать её: иначе хеш
разъедется с реальным конфигом при первом же изменении эмиттера
(см. память проекта: «Эмиттер и парсер ходят парой»).

Проблема: `GenerateNodeJSON` (`outbound_generator.go:85`) собирает **строку**
вручную и живёт в пакете `config`, а не `subscription` — прямой вызов даст
циклический импорт. Решение: хеш считается в пакете `config`
(`core/config/node_hash.go`), где эмиттер доступен; пакет `subscription`
получает его через уже существующий hook-механизм (как
`subscription.LookupCachedBody`, `source_loader.go:27`).

**Дедуп (D3)** — внутри источника, в `LoadNodesFromSource` после разбора,
до применения тегов.

**Выключение ноды (D4):**
- `ProxySource.DisabledNodes map[string]int64` — хеш → unix-время отметки;
- фильтрация в `LoadNodesFromSource`;
- GC: отметка старше TTL и не встреченная в текущем разборе — удаляется.
  TTL = `clamp(3 × интервал обновления, 24h, 30d)`;
- UI: чекбокс в превью узлов (`source_edit_window.go`, `previewNodeCap = 200`).

---

## 5. Тестовая стратегия

Соглашения репозитория (`AGENTS.md`, наблюдаемый стиль):
- **один файл на фичу**, не дописывать в `node_parser_test.go` (2057 строк);
- парсинг и эмиссия тестируются **по обе стороны границы** — при необходимости
  два файла: в `subscription/` и в `config/`;
- имена подтестов — фразы со ссылкой на спеку: `"whole config: route/dns ignored (SPEC 094 A4)"`;
- фикстуры в `testdata/` только для объёмных данных; преобладает литерал в тесте;
- сетевой регресс — `//go:build live`, уже существует
  (`live_subscriptions_test.go`), должен продолжать проходить.

Обязательный сквозной тест на каждую фазу: результат прогоняется через
`sing-box check` (критерий 8, 25) — не только сравнение map.

---

## 6. Порядок работ

A → B → C → D. Обоснование в SPEC §6.

Внутри каждой фазы: типы → ядро → интеграция в `source_loader` → тесты →
`go build ./... && go test ./... && go vet ./...`.

Документация при закрытии (AGENTS.md §4): `docs/release_notes/upcoming.md`,
`docs/ARCHITECTURE.md` (меняются потоки данных — `SourceLoadResult`),
`docs/ParserConfig.md` (новые поля `ProxySource`).
