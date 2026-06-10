# TASKS 075 — Импорт профилей Amnezia (`vpn://`)

## Этап 1 — парсер
- [x] `core/config/subscription/node_parser_amnezia.go`: decode (base64url + qUncompress с лимитами) → выбор контейнера (defaultContainer → порядок) → INI → `wireguard://`-URI → `parseWireGuardURI`
- [x] `node_parser_core.go`: `IsDirectLink` += `vpn://`; ранняя ветка в `ParseNode`

## Этап 2 — тесты
- [x] qCompress-эмуляция в тесте (4 байта BE + zlib)
- [x] AWG-профиль: поля в endpoint root, типы, MTU-кламп 1420→1280, label из `description`
- [x] plain-WG-профиль: без AWG-ключей, MTU честный
- [x] defaultContainer предпочитается при нескольких контейнерах
- [x] профиль без WG-контейнера → ошибка с именами контейнеров
- [x] битый base64 / обрезанный поток / не-JSON / объявленная zlib-бомба → ошибка, не паника
- [x] `IsDirectLink("vpn://…")`
- [x] `go build ./... && go test ./... && go vet ./...`

## Этап 3 — документация
- [x] `docs/ParserConfig.md`: раздел Amnezia `vpn://`
- [x] `docs/release_notes/upcoming.md`: EN/RU
- [x] `IMPLEMENTATION_REPORT.md`

Доп. (вне исходного плана, зафиксировано в отчёте): кросс-валидация Go-парсера против независимой Python-реализации формата.
