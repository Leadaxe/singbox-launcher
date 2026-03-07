# План: Поддержка подписок в формате JSON-массива конфигов

## 1. Архитектура

### 1.1 Поток данных

```
FetchSubscription(url)
       │
       ▼
  Raw content []byte
       │
       ▼
DecodeSubscriptionContent(content)
       │
       ├── try base64 decode ──► decoded text ──► split by "\n" ──► lines (existing)
       │
       ├── [NEW] starts with "[" && valid JSON array
       │         │
       │         ▼
       │    ParseJSONArrayConfigs(content) ──► []*ParsedNode
       │         │
       │         ▼
       │    Return decoded bytes: how? → Decoder returns content as-is for JSON path,
       │    and caller must branch on "was this JSON array?" to avoid splitting by "\n".
       │    Better: DecodeSubscriptionContent returns ([]byte, format) or only []byte,
       │    and "JSON array" is handled in FetchSubscription / LoadNodesFromSource:
       │    if DecodeSubscriptionContent detects "[", return special sentinel or second
       │    return value so that fetcher/loader does not split by newlines but calls
       │    new parser. So we have two options:
       │
       │    A) Decoder returns (decoded []byte, isJSONArray bool). If isJSONArray,
       │       decoded is raw content; loader/fetcher check isJSONArray and call
       │       ParseNodesFromJSONArrayConfigs(decoded) instead of split-by-newline.
       │
       │    B) Decoder only decodes; fetcher after decode checks if result looks like
       │       JSON array (starts with [) and then calls ParseNodesFromJSONArrayConfigs
       │       and returns... but FetchSubscription returns []byte. So we'd need
       │       FetchSubscription to return (content, format) or a wrapper type.
       │
       │    Simplest: In DecodeSubscriptionContent, when content starts with "["
       │    and is valid JSON array, do NOT return error; return the same content
       │    (so that "decoded" is the raw JSON). Then in LoadNodesFromSource (and
       │    in parseSubscriptionContent in wizard) we check: if decoded content
       │    starts with "[" and is valid JSON, branch to "parse as JSON array"
       │    and produce nodes from it; otherwise split by "\n" and parse lines.
       │    So DecodeSubscriptionContent only needs to NOT return error for "[";
       │    it can return (content, nil) for JSON array. No new return type.
       │
       ├── starts with "{" ──► error (unchanged)
       └── plain text (contains "://") ──► return content (existing)
```

Итог: декодер для ответа, начинающегося с `[` и являющегося валидным JSON-массивом, возвращает тот же `content` без ошибки. Разбор по строкам и по JSON-массиву выполняется в вызывающем коде (source_loader, wizard parser): по префиксу `[` и валидности JSON выбирается ветка «parse JSON array» вместо «split by newline».

### 1.2 Новые компоненты

- **ParseNodesFromJSONArrayConfigs(content []byte)** (в пакете `subscription`) — принимает сырое тело (JSON-массив), разбирает в `[]*config.ParsedNode`; возвращает ошибку при невалидном JSON или не-массиве.
- **Вспомогательные**: определение «прокси-аутбаунд» (тип не freedom/block/dns/blackhole), извлечение тега из аутбаунда, генерация уникального тега при отсутствии или коллизии (например, префикс из `remarks` элемента массива).
- **Конвертация** из сырого аутбаунда sing-box в `ParsedNode`: заполнить `Tag` и `Outbound = outbound map`; при необходимости выставить `Server`, `Port`, `Scheme` из аутбаунда для совместимости с `GenerateNodeJSON` (если генератор ожидает эти поля — уточнить по коду outbound_generator).

---

## 2. Изменения по файлам

### 2.1 core/config/subscription/decoder.go

- Убрать возврат ошибки для контента, начинающегося с `[`.
- Добавить проверку: если `strings.HasPrefix(contentStr, "[")` — попытаться `json.Valid(content)`; если да — вернуть `(content, nil)` (считать формат «JSON array», не подменять контент). Логировать, что распознан формат JSON array.
- Проверку на `{` оставить: одиночный объект по-прежнему даёт ошибку.

### 2.2 core/config/subscription/ — новый файл или node_parser.go

- Добавить функцию **ParseNodesFromJSONArrayConfigs(content []byte) ([]*config.ParsedNode, error)**:
  - `json.Unmarshal(content, &rawArray)`; если не массив — ошибка.
  - Для каждого элемента: type assertion в `map[string]interface{}`, взять `outbounds` (slice); при отсутствии — пропуск.
  - Для каждого элемента в `outbounds`: type assertion в map, взять `type` или `protocol`; если тип прокси (vless, vmess, trojan, shadowsocks, hysteria, hysteria2, ssh) — создать ParsedNode (Tag из аутбаунда или сгенерировать; Outbound = этот map; при необходимости Server/Port/Scheme из полей аутбаунда).
  - Сборка тега: если у конфига есть `remarks`, использовать как префикс при коллизии тегов; иначе достаточно тега аутбаунда + счётчик в рамках подписки для уникальности.
- Список «прокси-типов» вынести в константу или функцию `isProxyOutboundType(t string) bool`.

### 2.3 core/config/subscription/source_loader.go (LoadNodesFromSource)

- После получения `content` от FetchSubscription: проверить, не JSON array ли это: `strings.TrimSpace(string(content))` начинается с `[` и `json.Valid(content)`.
- Если да — вызвать `ParseNodesFromJSONArrayConfigs(content)`; применить лимит MaxNodesPerSubscription, TagPrefix/TagPostfix/TagMask, MakeTagUnique, append в `nodes`. Не вызывать разбор по строкам.
- Иначе — текущая логика (content как текст, split by "\n", ParseNode по каждой строке).

### 2.4 ui/wizard/business/parser.go (parseSubscriptionContent)

- После получения `content` от subscription.FetchSubscription (в processSubscriptionURL): аналогичная проверка: если контент — JSON array, вызвать `ParseNodesFromJSONArrayConfigs(content)` и подсчитать валидные ноды; в preview добавить строки по одной на ноду (например, тег ноды). Иначе — текущий цикл по строкам и IsDirectLink/ParseNode.

### 2.5 Совместимость с GenerateNodeJSON (core/config/outbound_generator.go)

- Проверить: для ноды, созданной из сырого аутбаунда JSON, поле `node.Outbound` уже содержит полную структуру sing-box. Если `GenerateNodeJSON` строит JSON из полей ParsedNode (Server, Port, …) и частично из Outbound — возможно, для нод из JSON-конфига достаточно отдавать в итоговый конфиг сериализованный `node.Outbound` (как один outbound). Уточнить по коду и при необходимости добавить ветку «если нода из JSON-конфига, выводить Outbound как готовый outbound» или заполнять Server/Port/Scheme из Outbound при создании ParsedNode.

---

## 3. Тесты

- Юнит-тест DecodeSubscriptionContent: тело `[{...}]` (валидный массив) — не ошибка, возвращается тот же контент.
- Юнит-тест ParseNodesFromJSONArrayConfigs: пустой массив → 0 нод; массив с одним конфигом с 2 proxy outbounds и 1 freedom → 2 ноды; невалидный JSON / не массив → ошибка.
- Юнит-тест: аутбаунд без тега — генерируется уникальный тег.
- Интеграционный (опционально): LoadNodesFromSource с моком URL, возвращающим JSON-массив конфигов — в результате список нод не пустой и теги уникальны.

---

## 4. Чеклист изменений

| Файл | Действие |
|------|----------|
| core/config/subscription/decoder.go | Не возвращать ошибку для `[` + valid JSON array; возвращать content; логировать формат |
| core/config/subscription/ (новый или node_parser.go) | ParseNodesFromJSONArrayConfigs; isProxyOutboundType; формирование ParsedNode из аутбаунда |
| core/config/subscription/source_loader.go | Ветка «если content — JSON array» → ParseNodesFromJSONArrayConfigs; лимит и tag-обработка как для строк |
| ui/wizard/business/parser.go | В parseSubscriptionContent ветка для JSON array → подсчёт нод и preview |
| core/config/outbound_generator.go | При необходимости: поддержка нод с уже заполненным Outbound из JSON (проверить GenerateNodeJSON) |
| *_test.go | Тесты декодера и ParseNodesFromJSONArrayConfigs |
