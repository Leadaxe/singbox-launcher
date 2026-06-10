# PLAN 075 — Импорт профилей Amnezia (`vpn://`)

## Архитектура

Один новый файл-конвертер + две точечные правки роутера. Вся WG/AWG-логика (валидация, нормализация CIDR, AWG-promote, MTU-кламп, skip-фильтры, теги) переиспользуется через делегирование в `parseWireGuardURI` — конвертер только приводит профиль Amnezia к каноническому `wireguard://`-URI.

```
vpn://… ─ parseAmneziaVPNLink ─┬─ decodeAmneziaProfile   base64url → qUncompress(4B BE + zlib) → JSON
                               ├─ amneziaWGConfText      defaultContainer → containers[]; рекурсивный
                               │                         поиск [Interface]-текста (last_config → config)
                               ├─ wgConfToURI            INI [Interface]/[Peer] → wireguard://-URI
                               └─ parseWireGuardURI      существующий путь (SPEC 010/073)
```

## Изменения по файлам

| Файл | Изменение |
|---|---|
| `core/config/subscription/node_parser_amnezia.go` | **Новый.** `parseAmneziaVPNLink`, `decodeAmneziaProfile`, `amneziaWGConfText` + рекурсивный `findWGIniText`, `parseWGConfSections`, `wgConfToURI`, константы лимитов |
| `core/config/subscription/node_parser_core.go` | `IsDirectLink` += `vpn://`; `ParseNode`: ранняя ветка `vpn://` (до проверки `MaxURILength` — у vpn:// свой потолок) |
| `core/config/subscription/node_parser_amnezia_test.go` | **Новый.** qCompress-эмуляция, тесты по SPEC |
| `docs/ParserConfig.md` | Раздел «Amnezia (`vpn://`)» рядом с WireGuard |
| `docs/release_notes/upcoming.md` | EN/RU пункты |

## Ключевые решения

* **Лимиты:** ссылка ≤ 512 КБ (`maxAmneziaLinkLength`), распакованный JSON ≤ 8 МБ (`maxAmneziaProfileJSON`, защита от zlib-бомбы: проверка заявленной длины из заголовка qCompress + `io.LimitReader`).
* **Толерантный base64:** убрать пробелы/переносы, срезать `=`, сначала `RawURLEncoding`, fallback `RawStdEncoding` (ссылки из мессенджеров бывают переломаны/перекодированы).
* **Поиск конфига рекурсивный с лимитом глубины**, а не жёсткая схема `containers[i].awg.last_config.config`: схема профиля менялась между версиями Amnezia; приоритет известных ключей (`config`, `last_config`, `awg`, `wireguard`) делает выбор детерминированным.
* **MTU не трогаем в конвертере** — `parseWireGuardURI` уже клампит AWG до 1280 (SPEC 073); plain-WG сохраняет значение из конфига.
* **Секреты не логируются**: в логах только имена контейнеров, размеры и hostName.

## Порядок работ

1. `node_parser_amnezia.go` + правки `node_parser_core.go`.
2. Тесты `node_parser_amnezia_test.go`.
3. `go build ./... && go test ./... && go vet ./...`.
4. Документация (`ParserConfig.md`, `upcoming.md`), IMPLEMENTATION_REPORT.md.
