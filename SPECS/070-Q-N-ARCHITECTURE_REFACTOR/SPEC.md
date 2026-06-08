# SPEC 070 — Архитектурный рефакторинг и уборка

**Статус:** в работе (автономно, 2026-06-08)
**Авторизация:** пользователь дал полную автономию до ~13:00, «работай до финала», «все фиксируй», UI-изменения — отчёт по пунктам.

## Цель

Привести код к чистой, стройной архитектуре: явные слои абстракций, единые механизмы
подписки (event-broker) и состояния (state-broker), декомпозиция монолитов, удаление
дублей и исторического мусора, понятные имена, разнесение по файлам. Полностью обновить
техдокументацию (`docs/ARCHITECTURE.md` со схемами, зонами ответственности, описанием
файлов, ключевыми архитектурными решениями/ADR; `docs/DATA_FLOW.md`).

## Правила работы

- **Коммитить каждую фазу** (отдельный коммит, понятное сообщение). **НЕ пушить.**
- UI-изменения → отдельный пункт в отчёте для ревью пользователем.
- Сначала безопасные задачи (behavior-preserving), затем сложные.
- Тесты — в конце, когда каркас логики финализирован. Но `go build` + `go vet` после
  каждой фазы (билд — не тест, держим зелёным постоянно).
- Живой отчёт — `PROGRESS.md` в этой папке.

## Входные данные

- SPEC 068 (threat-model): архитектурные слабости — dual-state, split Save/Build/Start,
  EventBus + legacy callbacks (SPEC 047), naming split.
- SPEC 069 (cleanup audit): 309 находок, §5 needs-judgment (81) — источник конкретики.

## Фазы

| # | Фаза | Тип | Коммит |
|---|---|---|---|
| P0 | Инспекция (workflow): карта пакетов/событий/состояния/зависимостей/дублей/монолитов/doc-drift | read-only | — |
| P1 | Безопасный батч: source_tab MarkAsChanged; дедуп (matchesPlatform/SetToolTip/scalar-stringify/DNSOptionsRaw); gofmt; EN→locale.T; magic-consts; снос исторических/tombstone-комментариев | behavior-preserving | ✔ |
| P2 | Редизайн event-broker: ProxyActiveChanged/ConfigBuilt/SubscribeAll; унификация EventBus + legacy callbacks; единый паттерн подписки | structural (+behavior) | ✔ |
| P3 | Консолидация state-broker: dual-state, adapter/legacy_migration | structural | ✔ |
| P4 | Декомпозиция монолитов: add_rule_dialog, outbounds_configurator, core_dashboard_tab, clash_api_tab, config_service, process_service (crash/restart), outbound_generator | structural | ✔ |
| P5 | Обоснованные смены поведения: IsNetworkError, Add/Edit addOutbounds source и др. | behavior | ✔ |
| P6 | Документация: ARCHITECTURE.md (+ схемы/ADR/file-inventory), DATA_FLOW.md | docs | ✔ |
| P7 | Финал: build, vet, полный test, deadcode, reinstall | verify | ✔ |

## Принцип очерёдности

Высокий риск × низкая стоимость → структурный долг → косметика оптом. Безопасное раньше
сложного, чтобы накопить зелёные коммиты до рискованных правок.
