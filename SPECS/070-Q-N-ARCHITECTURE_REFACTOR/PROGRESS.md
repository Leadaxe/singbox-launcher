# SPEC 070 — Живой отчёт о прогрессе

> Обновляется по ходу работы. Источник правды о том, что сделано и что осталось.
> Стартовая точка: HEAD `f9f2d06`, ветка `develop`, дерево чистое.

## Текущая фаза: **Stage C — domain monolith splits (workflow running)**

### Стадии — статус
- ✅ **Stage A** done — `e09749e`. Event-bus + DI cleanup.
- ✅ **Stage B** done — `7a3a1de`. Dedup: subscription utf8/base64 helpers, state
  buildTagSpec, fynewidget SetToolTipSafe (~9 inline copies), business/presentation
  template-dedup. B3 (build ruleset-convert) намеренно SKIP — два конвертера различаются
  ключеванием path (tag vs content-hash) → merge сменил бы emitted JSON. build+vet+targeted
  tests зелёные.
- 🔄 **Stage C** — workflow `wf_d228b683-d2a`, 4 агента (pure file-splits):
  core/state (load.go, adapter.go), subscription (node_parser.go, share_uri_encode.go),
  api (clash.go), internal/platform (wintun_cleanup_windows.go, GOOS=windows verify).
- ⏭ Отложено: transport_builder/tls_builder (golden — Stage D), ResolvedEntryMetadata
  embed (low value), DownloadStateComponent+srs_downloader (Stage F), остаток tooltip-
  сайтов в core_dashboard_tab/clash_api_tab (Stage F при декомпозиции).
- Stage G (docs ARCHITECTURE.md + file-inventory) — в конце, после всех split'ов
  (инвентарь файлов меняется декомпозицией). Делегирую агенту с synthesis.json.

## Принятый план исполнения (из synthesis.json — see also zone_maps.json)

8-слойная модель L0..L7 (platform → shared-internal → core-domain → services/lifecycle →
api → ui-presentation → ui-views → ui-widgets). 7 ADR (в synthesis.json). Стадии
(safe→complex), каждая = коммит, build+vet зелёные:

- **Stage A** (synth-P1) Pure deletions: мёртвые EventKind (SubscriptionUpdated,
  AutoUpdateStatus, PowerResume), Bus.SubscribeAll, ProxyActiveChanged-subscriber;
  удалить GetController fallback + GetControllerOrPanic; консолидировать DI no-op
  callbacks; фактические правки docs/ARCHITECTURE.md.
- **Stage B** (synth-P2) Behavior-preserving dedup: subscription utf8/encoding/transport/
  tls builders; convertRemoteRuleSetToLocal; ResolvedEntryMetadata; buildTagSpec hoist;
  UI-хелперы (SetToolTipSafe, DownloadStateComponent, srs_downloader); business dedup.
- **Stage C** (synth-P3) Domain monolith splits: state/load.go, adapter.go, node_parser.go,
  share_uri_encode.go, api/clash.go, wintun_cleanup_windows.go.
- **Stage D** (synth-P4) Build pipeline: outbound_generator.go → validity + JSONBuilder + filters.
- **Stage E** (synth-P5) Lifecycle: process_service CrashHandler; controller split
  (ProcessLifecycleManager+CacheManager); config_service split; SPEC 047 ph.6 event-wiring.
- **Stage F** (synth-P6) Dual-state elimination (canonical Rules/DNS единственная правда);
  UI-view декомпозиция (clash_api_tab, add_rule_dialog, edit_dialog, presenter_state,
  wizard_dns); фикс layering-edges (click_redirect, core_dashboard_tab).
- **Stage G** Документация: ARCHITECTURE.md (+схемы/ADR/file-inventory), DATA_FLOW.md, PIPELINE.md.
- **Stage H** Финал: build, vet, полный test, deadcode, reinstall.

> Риск-замечание: Stage F (dual-state) — самый крупный и рискованный; делаю с жёсткими
> build/golden-гейтами, при нехватке стабильности оставляю задокументированным остатком.

## ~~P0 — Инспекция~~ ✓ завершена
Workflow `wf_5c40ebf9-185` (10 агентов, 557s). Результат → synthesis.json + zone_maps.json.

## Лог

### 2026-06-08 — старт
- Заведена таска #101, SPEC 070, 15-мин safety-loop.
- Базовая карта репо: ~81k LOC, ~50 пакетов. Монолиты (LOC): core_dashboard_tab 1482,
  clash_api_tab 1422, add_rule_dialog 1146, outbound_generator 1086, edit_dialog 1071,
  config_service 1066, share_uri_encode 883, source_tab 790, source_edit_window 781,
  process_service 759, controller 747.
- Запускаю Phase 0 workflow (параллельные ридеры → карта архитектуры + decision-sheet).

## Сделано (коммиты)
- `ffb6f7a` docs(spec070): план + трекер.
- `acaae74` refactor(spec070) P1 safe fixes: dedup matchesPlatform→VarAppliesOnGOOS;
  source delete-handler MarkAsChanged; preview cap → const previewNodeCap; gofmt loader.go.
- _(pending commit)_ inline osStatLocal → os.Stat в core/build/preset_merge.go.

### P0 — статус
Workflow `wf_5c40ebf9-185` (9 zone-readers + synthesis) запущен, ждём результат для
систематического P2–P6. Параллельно сделаны независимые P1-items выше.

## UI-изменения для ревью пользователя
1. **Удаление источника теперь зажигает кнопку Save** (`source_tab.go`): раньше после
   удаления строки источника состояние не помечалось dirty и Save не активировался —
   приходилось делать ещё одно изменение. Теперь delete сразу помечает изменения.

## Открытые риски / решения
- gofmt-дрейф в ~60 файлах (CI не гейтит) — сделаю единым sweep-коммитом в конце,
  чтобы не пересекаться с декомпозицией.
- SetToolTip-дедуп, EN→locale.T, чистка исторических комментариев — отложены до
  synthesis (нужен точный перечень мест; пересекаются с P4-декомпозицией).
