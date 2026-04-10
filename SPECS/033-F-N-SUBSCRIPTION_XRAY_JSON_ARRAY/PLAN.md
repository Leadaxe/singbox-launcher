# План: JSON-массив Xray/V2Ray конфигов и цепочка dialerProxy → detour

## 1. Связь с 016

- **016-F-C-SUBSCRIPTION_JSON_ARRAY** закрыта **без реализации**; SPEC/PLAN 016 остаются референсом для будущего **follow-up**: разбор массива конфигов **sing-box**.
- **033 (MVP):** изменение `decoder.go` — тело с **`[`** и валидным JSON-массивом **не** отклонять; в `LoadNodesFromSource` / визард — ветка «массив конфигов» → разбор **только Xray-элементов** (см. §2). Разбор sing-box-массива по 016 **не** входит в MVP 033.

## 2. Классификация элемента массива (MVP)

После `json.Unmarshal` в `[]interface{}` или `[]json.RawMessage` для каждого элемента:

1. Привести к `map[string]interface{}`.
2. Взять `outbounds` — только массив.
3. Если элемент удовлетворяет эвристике **Xray** из SPEC §2.2 (например, в outbounds есть **`protocol`** как строка и кандидат на извлечение ноды) → вызвать `parseXraySubscriptionConfigElement` (имя условное).
4. Иначе — **пропустить элемент** с записью в `debuglog` (в т.ч. «чистый» sing-box из 016 — до реализации follow-up).

**Follow-up (016):** отдельная ветка для элементов с sing-box `type` в outbounds без Xray-диалекта.

## 3. Выбор основного outbound (Xray)

Рекомендуемый алгоритм (уточнить в коде комментарием):

1. Собрать индекс `tag → outbound map`.
2. Кандидаты: outbounds с `protocol` ∈ поддерживаемый набор (`vless` в MVP; опционально расширить в TASKS).
3. Если ровно у одного кандидата в `streamSettings.sockopt` есть **`dialer`** / **`dialerProxy`** — взять его как основной.
4. Если кандидат с `dialerProxy` один среди vless — взять его.
5. Если несколько — предпочесть тег `proxy`, иначе первый по порядку + `WarnLog`.
6. Если ни у кого нет цепочки — единственный vless или первый поддерживаемый прокси.

## 4. Разбор jump

- Прочитать строку `dialerProxy` (или поле Xray, фактически используемое в образце; в PLAN §3 учитывать и **`dialer`**, если встретится).
- Найти outbound с `tag == dialerProxy` и `protocol == socks`.
- Извлечь `settings.servers[0]`: `address`, `port`; учётные данные — из `users[0]` **если есть**, иначе SOCKS **без** `username`/`password` (анонимный SOCKS допустим).

**Если** outbound с тегом не найден **или** `protocol != socks`: **не создавать ноду из этого элемента** (пропуск элемента), **`WarnLog`** с понятным контекстом (тег, индекс элемента).

## 5. Маппинг VLESS Xray → sing-box outbound map

Структурировать отдельным файлом, например `subscription/xray_outbound_convert.go`:

- `vnext[0].address` → `server`; `port` → `server_port`.
- `users[0].id` → `uuid`; `encryption` при необходимости; `flow` → `flow` (нормализация `xtls-rprx-vision-udp443` как в `node_parser.go`).
- `streamSettings.network` → при `tcp` без WS — без `transport` или минимальный; при других сетях — переиспользовать идеи из `node_parser_transport.go` где применимо.
- `streamSettings.security == reality` → `tls.enabled: true`, `tls.server_name` из `realitySettings.serverName`, `tls.utls.fingerprint` из `fingerprint`, `tls.reality.public_key` из `publicKey`, `short_id` из `shortId`, `allowInsecure`/инъекции по правилам sing-box.
- Игнорировать для первой версии Xray-only поля, не имеющие аналога в sing-box, или логировать `DebugLog`.

Итог: заполнить `ParsedNode`: `Scheme: vless`, `Server`, `Port`, `UUID`, `Flow`, `Outbound` (map в форме, которую ожидает `GenerateNodeJSON`), `Label` из `remarks`.

## 6. Представление цепочки в `ParsedNode`

Минимальный вариант без ломки всего пайплайна:

- Добавить в `configtypes.ParsedNode` опциональное поле, например **`Jump *ParsedJump`** (отдельный маленький struct: `Tag`, `Outbound map[string]interface{}`, `LabelSuffix` — по необходимости), **или**
- Два тега: **`Tag`** основного, **`JumpTag`** + `JumpOutbound` map.

Генерация:

- Если `Jump != nil`:  
  - emit JSON для SOCKS с тегом `Jump.Tag`;  
  - emit JSON для основного с `"detour": "<Jump.Tag>"`.
- Если `Jump == nil`: текущее поведение `GenerateNodeJSON` без изменений.

Вынести общий кусок сборки полей сервера/TLS так, чтобы не дублировать `GenerateNodeJSON` целиком (например, функция `appendDetour(parts []string, tag string)` и условный блок в конце).

## 7. Уникальность тегов

- Базовые теги из Xray (`proxy`, `ru-upstream`) часто **совпадают** между элементами массива.
- Базовые теги sing-box до префикса: из **slug** непустого **`remarks`** → основной **`{slug}`**, jump **`{slug}_jump_server`**; при пустом `remarks` → **`xray-{i}`** / **`xray-{i}_jump_server`**. Затем **`tag_prefix` / `tag_postfix` / `tag_mask`**, **`textnorm.NormalizeProxyDisplay`**, **`MakeTagUnique`** для main и jump (как в **`applyTagsToXrayNode`**).
- Поле **`detour`** в основном outbound ссылается на **итоговый** (уникализированный) тег jump.

## 8. Файлы (ориентировочно)

| Файл | Изменения |
|------|-----------|
| `core/config/subscription/decoder.go` | Разрешить `[` + `json.Valid` (не сделано в 016 — внедрить в 033) |
| `core/config/subscription/source_loader.go` | Ветка: массив → парсер нод |
| `core/config/configtypes/types.go` | Опциональные поля для jump / цепочки |
| `core/config/outbound_generator.go` | `detour` + двойная строка при jump |
| `core/config/subscription/xray_json_array.go` (новый) | Парсинг массива, разбор Xray-элементов (sing-box — follow-up) |
| `core/config/subscription/xray_outbound_convert.go` (новый) | VLESS (+ SOCKS jump) → maps |
| `ui/wizard/business/parser.go` | Та же ветка при CheckURL / превью |
| Тесты | `*_test.go`: золотой фрагмент JSON (без секретов — фиктивные UUID/ключи) |

## 9. Тестовые данные

- Не коммитить пользовательский файл с реальными учётками.
- Составить **минимальный** анонимизированный фрагмент: 2 элемента массива, у каждого свой SOCKS + VLESS + reality + разные `remarks`.

## 10. Документация после реализации

- Обновить `docs/ParserConfig.md` (поддержка Xray JSON array + detour).
- `docs/release_notes/upcoming.md` по **IMPLEMENTATION_PROMPT**.
