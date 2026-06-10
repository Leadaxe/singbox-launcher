# PLAN 076 — Вставка голого WG/AWG `.conf`-текста

## Архитектура

Конвертер уже есть (SPEC 075: `parseWGConfSections` + `wgConfToURI`). Добавляется только разрезка вставленного текста на блоки и врезка в классификатор. Хранение не меняется: `.conf`-текст не сохраняется — он немедленно конвертируется в канонический `wireguard://`-URI и живёт как обычный `Source{Type: server}` (round-trip, share, парсинг — существующие пути).

```
поле Add ─ AppendURLsToSources ─ classifyInputLines
                                   ├─ ExtractWGConfBlocks   rest → построчно как раньше
                                   └─ ConvertWGConfText     блок → wireguard://-URI → connections
```

## Изменения по файлам

| Файл | Изменение |
|---|---|
| `core/config/subscription/wgconf_text.go` | **Новый.** `ExtractWGConfBlocks`, `ConvertWGConfText` (label = хост Endpoint, без скобок IPv6) |
| `core/config/subscription/wgconf_text_test.go` | **Новый.** Разрезка, конвертация AWG/plain, смешанный ввод, невалидный блок |
| `ui/configurator/business/parser.go` | `classifyInputLines`: выделение блоков до построчного разбора, warn на невалидный блок |
| `docs/ParserConfig.md` | Абзац в разделе Amnezia/WireGuard про вставку `.conf`-текста |
| `docs/release_notes/upcoming.md` | EN/RU пункт |

## Ключевые решения

* **Блоки режутся по строкам `[Interface]`** (EqualFold): всё от первой такой строки до следующей — один блок; текст до первого блока — обычные строки-ссылки. Это позволяет смешивать ссылки и conf в одной вставке.
* **Label** — хост из `Endpoint` (IPv6 — без скобок): фрагмент URI подхватывается и `AppendURLsToSources` (Label источника), и `parseWireGuardURI` (tag узла). `singbox-wg0` как дефолт не годится — все вставки получали бы одинаковое имя.
* **Ошибка блока не валит вставку** — политика как у битых строк подписки: warn и продолжить.

## Порядок работ

1. `wgconf_text.go` + тесты.
2. Врезка в `classifyInputLines`.
3. `go build ./... && go test ./... && go vet ./...`.
4. Документация, IMPLEMENTATION_REPORT.md.
