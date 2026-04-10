# План: jump по `dialerProxy` — не только SOCKS

**Статус:** реализовано для **SOCKS** и **VLESS**; см. **IMPLEMENTATION_REPORT.md**.

## 1. Модель данных

- Вариант A: расширить **`ParsedJump`**: поля не только SOCKS (`Server`, `Port`, `Outbound` для `socks`), но и **`Scheme`**, универсальная outbound-map, как у основного `ParsedNode`.
- Вариант B: **`Jump`** заменить на срез структур hop’ов (`[]*ParsedHop`) с типом и готовой outbound-map для `GenerateNodeJSON`.
- Выбор зафиксировать в **TASKS** после прототипа; избегать дублирования логики с `ParsedNode` без необходимости (вынести общий «hop → JSON»).

## 2. Парсер Xray (`xray_json_array.go`, `xray_outbound_convert.go`)

- После `byTag[dialerRef]` вместо жёсткой проверки `protocol == socks` — **диспетчер** по `protocol` (строка, lower case).
- Для `socks` — текущий путь `xrayBuildJumpFromSocksOutbound`.
- Для `vless` / `vmess` / … — новые функции **`xrayXxxToSingBoxOutboundMap`** или реюз кусков из конвертации основного VLESS, чтобы получить `map[string]interface{}` в форме, ожидаемой **`GenerateNodeJSON`**.
- Неподдерживаемый `protocol` — `WarnLog`, `return nil, nil` (как сейчас при не-SOCKS).

## 3. Генерация (`outbound_generator.go`)

- Сейчас: при `Jump != nil` собирается временный `ParsedNode` со `Scheme: socks` и одним JSON.
- После: для произвольного hop — **`GenerateNodeJSON`** с тем же `Scheme`/outbound-map, что хранятся в расширенном `ParsedJump` (или в элементе среза hop’ов).
- Порядок вставки JSON-строк: **сначала все hop’ы снаружи внутрь** (ближний к клиенту → дальний), затем основной outbound с **`detour`** на тег **первого** hop’а; у промежуточных outbounds при цепочке >1 — свои **`detour`** на следующий тег (уточнить по схеме sing-box для многоуровневого dial).

## 4. Теги (`source_loader.go` — `applyTagsToXrayNode`)

- Расширить: для каждого hop’а с отдельным тегом применять prefix/postfix/mask и **`MakeTagUnique`** в согласованном порядке с основным тегом.

## 5. Тесты

- Фикстура JSON: минимальный элемент массива с `dialerProxy` → **VLESS** (или другой поддерживаемый тип) без секретов реальных пользователей.
- Юнит-тесты парсера и снапшот фрагмента сгенерированного JSON (порядок outbounds, поля `detour`).

## 6. Документация

- **`docs/ParserConfig.md`**: убрать формулировку «jump только SOCKS», заменить на список поддерживаемых типов на момент релиза.
- **`docs/release_notes/upcoming.md`**: EN/RU.

## 7. Файлы (ориентировочно)

| Файл | Изменения |
|------|-----------|
| `core/config/configtypes/types.go` | Расширение `ParsedJump` или новая структура hop’ов |
| `core/config/subscription/xray_json_array.go` | Диспетчер протокола для `jumpOb` |
| `core/config/subscription/xray_outbound_convert.go` | Маппинг Xray outbound → sing-box map для доп. типов |
| `core/config/outbound_generator.go` | Генерация N hop JSON + main |
| `core/config/subscription/source_loader.go` | `applyTagsToXrayNode` для нескольких тегов / типов |
| Тесты | `*_test.go`, `testdata/` |
