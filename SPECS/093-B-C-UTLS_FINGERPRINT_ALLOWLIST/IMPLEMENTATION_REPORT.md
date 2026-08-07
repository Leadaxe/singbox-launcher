# Implementation Report — SPEC 093

**Дата:** 2026-07-31 · **Статус:** Complete · **Версия:** v1.2.7 · **Коммит:** `e1d2a13`

## Что сделано

### 1. Валидация в парсере (`core/config/subscription/node_parser_transport.go`)

`NormalizeUTLSFingerprint` переписана с pass-through на allowlist + маппинг:

- `singboxUTLSFingerprints` — множество имён, которые принимает
  `uTLSClientHelloID` (sing-box `common/tls/utls_client.go`). Взято из module
  cache, не по памяти: помимо очевидных имён туда входят `chrome_psk`,
  `chrome_psk_shuffle`, `chrome_padding_psk_shuffle`, `chrome_pq`,
  `chrome_pq_psk`.
- `utlsAliasPrefixes` — упорядоченный список префиксов сырых uTLS-идентификаторов
  → семейство браузера. `hellorandomized` стоит **до** `hellorandom` (иначе
  `HelloRandomized` схлопнулся бы в `random`).
- Разделители (`_`, `-`, пробел) вычищаются перед сравнением префикса.
- Всё нераспознанное → `""`.

Все 15 вызовов `NormalizeUTLSFingerprint` уже либо проверяли результат на
непустоту (tuic, anytls, hysteria2, trojan, core, xray), либо имели фолбэк на
`"random"` (VLESS) — правки по вызовам не потребовались.

### 2. Барьер в эмиттере (`core/config/outbound_generator.go`)

`GenerateNodeJSON`: ключ `fingerprint` пишется только если нормализатор вернул
непустое значение (раньше на отклонённом значении записался бы
`"fingerprint":""`). Нужно для шаблонов — блок `tls.utls`, положенный руками или
засеянный в `wizard_template.json`, не проходит через парсер.

### 3. Тесты (5 новых кейсов)

- `core/config/subscription/node_parser_transport_test.go`:
  `TestNormalizeUTLSFingerprint` (таблица: имена sing-box, варианты Chrome,
  сырые идентификаторы, мусор), `TestParseNode_VLESS_RawUTLSIdentifierFingerprint`,
  `TestParseNode_VLESS_JunkFingerprintDropped`.
- `core/config/generator_test.go`: `TestGenerateNodeJSON_UTLSRawIdentifierMapped`,
  `TestGenerateNodeJSON_UTLSUnknownFingerprintOmitted`.

## Проверка

| Проверка | Результат |
|----------|-----------|
| `go build ./...` | OK (exit 0) |
| `go test ./core/...` | OK — все пакеты `ok` |
| `go vet ./core/config/...` | чисто |
| Существующий тест на регистр `QQ` (issue #45) | проходит, не сломан |
| REALITY-нода с `fp=HelloChrome_120` | `utls.fingerprint: "chrome"`, блок `reality` не тронут |

**Кросс-проверка против ядра.** Локального бинаря sing-box нет, а `go get` в
окружении режется по сети — поэтому все выходы нормализатора прогнаны через
дословную копию switch-а `uTLSClientHelloID` из module cache
(`sing-box@v1.13.11`). Подтверждено: ядро принимает каждое значение, которое
может вернуть функция, **и** исходный `hellochrome_120` действительно
отвергается (без второй половины проверка ничего не доказывала бы). Временный
тест удалён после прогона.

## Выпуск

- Release notes: `docs/release_notes/1-2-7.md`, индекс `RELEASE_NOTES.md`.
- Тег `v1.2.7`, CI зелёный, 5 артефактов, `isLatest: true`.
- Issue [#100](https://github.com/Leadaxe/singbox-launcher/issues/100) — заведён
  постфактум как справочный (чтобы ошибка находилась поиском) и закрыт.

## Побочная проверка — `flow`

По ходу проверялись соседние строгие enum-поля VLESS. Итог: **дефекта нет,
правок не вносилось.**

- `packet_encoding` — allowlist уже стоял (`node_parser_core.go`, ветка
  `packetEncoding`).
- `flow` — парсеры действительно пишут значение без проверки
  (`node_parser_core.go:394`, `xray_outbound_convert.go:115`), но фильтрует
  **эмиттер**: `outbound_generator.go` — whitelist `""`/`xtls-rprx-vision` плюс
  guard на несовместимость `xtls-rprx-vision` с v2ray-транспортами
  (ws/grpc/http/httpupgrade/xhttp). Оба случая логируются в debug.

Проверено end-to-end, а не по чтению кода: `xtls-rprx-direct`,
`xtls-rprx-origin`, `none`, `junk` прогнаны по полному пути
«URI → парсер → `GenerateNodeJSON`» — все отбрасываются; отдельно проверены
round-trip через `state.json` (`Outbound["flow"]`) и `xtls-rprx-vision` поверх
ws. Регресс уже покрыт `core/config/flow_transport_test.go:118`. Временные тесты
удалены. См. также SPEC 035 (исследование поля `flow`).
