# 089-F-N — Глубокое управление правилами (структурный редактор выражений)

**Тип:** Feature · **Статус:** N (спека; реализация spec-only — UI-heavy, нужен live-GUI)
**Дата:** 2026-07-12 · **Ядро:** rc.17 (rule engine полный — ядру ничего не нужно) · **Effort:** L

> Design + adversarial verify (Opus). Вердикт: `needs-fixes` (учтено ниже); `spec-only`.

## 1. Что уже даёт ядро и лаунчер

Ядро rc.17 принимает **всё**: `RawDefaultRule` (`option/rule.go:68-115`) —
`network/protocol/port/port_range/source_ip_cidr/ip_is_private/wifi_ssid/process_path_regex/invert`
и т.д.; `RawLogicalRule{Mode:"and|or", Rules:[], Invert}` (`option/rule.go:140-161`) — вложенные
логические выражения; actions `route/reject/hijack-dns/sniff/resolve/route-options`.

**Emit-путь уже пропускает произвольный JSON**: `model.CustomRules[i].Rule.Rule
map[string]interface{}` → `MergeRouteSection` клонирует map как есть, докидывая
outbound/action (`core/build/route_merge.go:114-125`). Значит `{"type":"logical",…}` **уже
сегодня** доедет до config.json через Raw-вкладку. **Не хватает только структурного редактора**
вместо ручного JSON.

**Приоритет как отдельная концепция НЕ нужен**: порядок first-match задаёт `model.RuleOrder
[]RuleSlot` + `moveSlotUp/Down` (SPEC 053/062). Правила эмитятся в порядке слотов.

## 2. Область

1. **Структурный редактор выражений** — декомпозиция `add_rule_dialog.go` (1154 LOC) в подпакет
   `ui/configurator/dialogs/rule_editor/` (dialog.go оркестратор, form_leaf.go поля листа,
   logical.go and/or/invert-группы). `add_rule_dialog.go` → тонкий шим, делегирует в
   `rule_editor.Show(...)`, сигнатуры `ShowAddRuleDialogFunc`/`CreateRulesTab` сохранены.
2. **Расширенный набор match-полей в форме** (сейчас форма даёт domains/ips/process, остальное —
   только Raw): `network/protocol/port/port_range/source_ip_cidr/ip_is_private/wifi_ssid`.
3. **Логические группы** and/or/invert в UI (вложенность).
4. **Drag-drop reorder** (итерация 2) поверх существующих ↑↓; `↑↓` остаются надёжным fallback.

## 3. Правки от adversarial verify

1. **Не переклассифицировать `Type`** на edit (его перетирает `ToPersistedCustomRule:131`).
   Режим редактора хранить в `Params["editor_mode"] = "logical"|"leaf"|"raw"` — как уже сделано
   для `domain_mode`/`path_mode`. Params переживает round-trip (`ToPersistedCustomRule:141` копирует).
   Это надёжнее и не задевает риск переклассификации чужих Raw-правил.
2. Если всё же вводить `RuleTypeLogical` как persisted Type — синхронно: const в
   `core/state/legacy_types.go`, ветка в `IsKnownRuleType`, ветка в `DetermineRuleType`, и
   проверить, что `ToPersistedCustomRule` не перетрёт.
3. **Nested route action**: ядро запрещает `action`/`outbound` на дочерних узлах logical
   (`rejectNestedRouteRuleAction`). Редактор ставит outbound/action ТОЛЬКО на корне правила.
4. **dual-state НЕ трогаем**: редактор пишет тот же `map[string]interface{}` в
   `InlineBody.Match` — канон/легаси не задеты.

## 4. Файлы

| Файл | Изменение |
|------|-----------|
| `ui/configurator/dialogs/rule_editor/` (новый подпакет) | dialog.go / form_leaf.go / logical.go / rule_editor_test.go |
| `ui/configurator/dialogs/add_rule_dialog.go` | рефактор → шим-делегат |
| `ui/configurator/models/rule_match.go` (новый, опц.) | `RuleMatchNode` = Leaf{Fields map} \| Logical{Mode, Children} |
| `ui/configurator/models/wizard_state_file.go` | `Params["editor_mode"]` для edit-открытия в нужном режиме |
| `ui/configurator/tabs/rules_unified_rows.go` | расширить summary-строку правила (показывать logical/расширенные поля) |
| `internal/fynewidget/draggable_row.go` (новый, итерация 2) | самодельный DnD по хендлу ⠿ |

## 5. Риски

- **Fyne perf на тысячах правил**: Rules tab строит все строки разом (`rules_unified_rows.go:42`);
  DnD усугубит. Мера: soft-cap/виртуализация (отдельная итерация), ↑↓ fallback всегда.
- **Fyne нет нативного DnD** для reorder — самодельный `fyne.Draggable` хрупок (расчёт индекса по
  координатам, скролл во время драга). ↑↓ остаются. DnD — вторая итерация.
- Round-trip Raw↔Form для logical: `FromSingboxRule(ToSingboxRule(node))==node` покрыть тестом.

## 6. DoD (реализация — отдельная сессия)

- [ ] rule_editor подпакет; add_rule_dialog → шим; сигнатуры сохранены.
- [ ] Расширенные match-поля + logical в форме; outbound/action только на корне.
- [ ] round-trip тесты (leaf со всеми полями, and/or/invert, edit чужого Raw logical).
- [ ] `go build/test/vet`; **live-GUI**: создать/редактировать/reorder сложное правило,
      config.json проходит `sing-box check`.

## 7. Почему spec-only

Задача чисто UI+модель (ядро уже всё принимает), но L-объём: декомпозиция файла на 1154 LOC +
новый редактор выражений + DnD. Требует live-GUI прогона (Fyne-виджеты, round-trip Raw↔Form,
perf на больших списках). Backend не меняется — риск только в UI-регрессиях, которые unit-тесты
не ловят.
