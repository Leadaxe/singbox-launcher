# Задачи: Поддержка подписок в формате JSON-массива конфигов

## Этап 1: Декодер и распознавание формата

- [ ] В `core/config/subscription/decoder.go`: для контента с `strings.HasPrefix(contentStr, "[")` выполнить проверку `json.Valid(content)`; если true — вернуть `(content, nil)` и залогировать распознавание формата «JSON array of configs», не возвращать ошибку «JSON configuration instead of subscription list».
- [ ] Оставить без изменений возврат ошибки для контента, начинающегося с `{` (одиночный JSON-объект).

## Этап 2: Парсинг JSON-массива в ноды

- [ ] Добавить в пакет `subscription` функцию `isProxyOutboundType(typeOrProtocol string) bool`: возвращать true для vless, vmess, trojan, shadowsocks, hysteria, hysteria2, ssh (и при необходимости иных прокси-типов, которые лаунчер поддерживает); false для freedom, block, dns, blackhole и т.п.
- [ ] Реализовать `ParseNodesFromJSONArrayConfigs(content []byte) ([]*config.ParsedNode, error)`: разбор JSON-массива; для каждого элемента — извлечение `outbounds` (массив); для каждого аутбаунда с прокси-типом создавать ParsedNode (Tag из аутбаунда или сгенерированный при отсутствии/коллизии; Outbound = map аутбаунда; при необходимости заполнять Server, Port, Scheme из аутбаунда для совместимости с GenerateNodeJSON).
- [ ] Обеспечить уникальность тегов в рамках одной подписки: при совпадении тегов между конфигами использовать префикс из поля `remarks` элемента массива (если есть) или суффикс по индексу.
- [ ] Обработать граничные случаи: пустой массив → 0 нод; элемент без `outbounds` или с не-массивом — пропуск; невалидный JSON → ошибка с понятным сообщением.

## Этап 3: Интеграция в LoadNodesFromSource

- [ ] В `core/config/subscription/source_loader.go` после получения `content` от FetchSubscription: если `strings.TrimSpace(string(content))` начинается с `[` и `json.Valid(content)` — вызвать `ParseNodesFromJSONArrayConfigs(content)` вместо разбора по строкам.
- [ ] Применить к полученному списку нод лимит MaxNodesPerSubscription, TagPrefix/TagPostfix/TagMask, MakeTagUnique и append в `nodes` (аналогично текущей ветке для строк).
- [ ] Добавить debuglog в точках start/success/error для ветки JSON array.

## Этап 4: Интеграция в визард (CheckURL / parseSubscriptionContent)

- [ ] В `ui/wizard/business/parser.go` в `processSubscriptionURL`: после FetchSubscription проверить, является ли контент JSON-массивом (starts with `[`, json.Valid).
- [ ] Если да — вызвать `ParseNodesFromJSONArrayConfigs(content)` и по количеству возвращённых нод обновить validCount и preview (например, добавлять в previewLines строки с тегами нод до MaxPreviewLines).
- [ ] Если нод 0 — добавить в errors сообщение «Subscription contains no valid proxy links» (как для пустой подписки по строкам).

## Этап 5: Генерация outbound из нод JSON-конфигов

- [ ] Проверить `core/config/outbound_generator.go` (GenerateNodeJSON): достаточно ли для ноды из JSON-конфига полей Tag и Outbound, или требуются Server, Port, Scheme. При необходимости заполнять эти поля при создании ParsedNode в ParseNodesFromJSONArrayConfigs или добавить в GenerateNodeJSON ветку «если Outbound уже полный — сериализовать его как один outbound».

## Этап 6: Тесты и отчёт

- [ ] Юнит-тест в decoder_test.go (или subscription_test): DecodeSubscriptionContent с телом `[]` и с телом `[{ "outbounds": [...] }]` — не ошибка, возвращается исходный контент.
- [ ] Юнит-тест ParseNodesFromJSONArrayConfigs: пустой массив; массив с одним конфигом (2 proxy outbounds + 1 freedom) → 2 ноды; аутбаунд без tag → нода с сгенерированным тегом; невалидный JSON / не массив → ошибка.
- [ ] После реализации заполнить IMPLEMENTATION_REPORT.md.
