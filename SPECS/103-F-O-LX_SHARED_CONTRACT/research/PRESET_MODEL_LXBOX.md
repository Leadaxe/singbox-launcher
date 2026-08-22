# Модель пресетов LxBox — карта для миграции (D-049)

Источник: обход кода LxBox (фаза 3). Файл-шаблон — `app/assets/wizard_template.json`,
секция `selectable_rules` (:394). Код: `parser_config.dart` (модель),
`preset_expand.dart` (раскрытие), `custom_rules.dart` (применение),
`rule_order.dart` (порядок/seed), `dns_rules.dart` (DNS-мерж).

## 1. Схема пресета

Верхний уровень (`SelectableRule.fromJson`, parser_config.dart:752-794) —
читаются РОВНО эти ключи:

| Поле | Тип | Обяз. | Семантика |
|---|---|---|---|
| `preset_id` | string | **да** (пустое → FormatException, :753) | stable slug, ключ связи с состоянием |
| `ui` | object | нет | **единственное** место UI-метаданных; плоские поля верхнего уровня НЕ читаются (:761) |
| `rule_set` | array | нет | объявления rule_set → `route.rule_set` |
| `rules` \| `rule` | array \| object | нет | route-правила; обе формы, `rules ?? rule` (:779), одиночный Map нормализуется в список |
| `vars` | array | нет | переменные пресета (грамматика §2) |
| `dns_rules` \| `dns_rule` | array \| object | нет | DNS-правила, та же нормализация (:788) |
| `dns_servers` | array | нет | DNS-серверы пресета |

Поля `ui` (:766-772): `label`, `description`, `default` (bool → default_enabled),
`locked` (bool), `num` (int, дефолт 1000), `isSortable` (bool, дефолт true).

**Чего НЕТ** (важно для миграции — не изобретать): `id` (только `preset_id`),
`enabled` на уровне пресета, `required` на уровне пресета (это поле var),
`default_enabled` (это `ui.default`), `select: local|global` у var (см. §2 `ref`),
`platforms`, `if`/`if_or` на фрагментах, `outbounds{}`.

## 2. Переменные пресета

`WizardVar` (:489-596): `name`, `type`, `default_value`, `wizard_ui`, `options`,
`title`, `tooltip`, **`required` (bool, дефолт `true`!)**, `on_change`, `ref`.

Значения живут в состоянии, per-rule: `CustomRulePreset.varsValues`
(map string→string), НЕ в шаблоне.

Резолв (preset_expand.dart:106-145), порядок:
1. ключ в varsValues непустой → он;
2. ключ есть, значение пустое: `required` → warning + **весь пресет выпадает**;
   иначе → `null`;
3. ключа нет, `default_value` непустой → default;
4. ключа нет, default пуст, `required` → warning + **пресет выпадает**;
5. иначе `null` (Dropped-каскад).

**`ref`-переменные вместо `select: local|global`** (:558-568): запись
`{"ref": "resolve_enabled"}` объявляет var, значение которой берётся из
**глобальных** userVars (:152-156), а не из varsValues. Осиротевшие значения
ref-var чистятся при загрузке (rule_order.dart:249-279).
Плюс: все глобальные vars подмешиваются в контекст пресета через `putIfAbsent`
(:165-167) — не перетирая локальные. Поэтому `@vpn_mode` работает внутри
пресета, не будучи в нём объявленным.

**Магические имена** (спецобработка кодом):
- `outbound` — universal override: значение из состояния бьёт решение шаблона
  минуя подстановку (:328-337); `"reject"` → `action: reject` + снятие `outbound`;
- `dns_enable` — единственный тумблер DNS-блока пресета (custom_rules.dart:105-123);
- `dns_server` — фильтр эмиссии `dns_servers` (:427-443).

## 3. «Базовые правила» — механизм

Это **не** отдельная секция и **не** `config.route.rules` (там пустой список).
Это пресет с `ui.locked: true` + `ui.isSortable: false` + `ui.num: 0`
(единственный такой — `traffic-processing`, шаблон :396-429: sniff, hijack-dns,
resolve).

Неотчуждаемость держится **re-seed'ом на каждой сборке**, а не флагом:
`seedRequiredPresets` (rule_order.dart:84-111) отбирает `!isSortable` и
добавляет отсутствующее; `normalizeRuleOrder` зовётся из `build_config.dart:303`
при каждой генерации. Даже стёртый из состояния пресет вернётся.

Роли флагов разделены:
- `isSortable: false` — критерий **seed'а** и запрет drag'а (routing_screen.dart:844),
  исключение из каскада сдвига номеров;
- `locked: true` — запрет выключения/удаления в UI (свич `null`, delete скрыт);
- `num: 0` — гарантия первого места (sniff обязан быть до матчинга).

## 4. Порядок route.rules

1. `initialRules` из `template.config.route.rules` (в шаблоне LxBox пуст);
2. все custom rules **одним проходом** в порядке списка (custom_rules.dart:585-631) —
   сознательный фикс: два прохода ломали пользовательский порядок;
3. порядок списка = стабильная сортировка по `orderNum` (rule_order.dart:62-72);
   `orderNum` живёт в состоянии, `num` из шаблона — только стартовое значение;
4. внутри пресета — порядок правил как в шаблоне.

Ось номеров (шаг 10 — намеренный зазор):
`0 traffic-processing · 950 private-ip · 960 block-ads · 970 fcm-push ·
980 bittorrent · 990 vowifi · 1000-1100 зона пользователя · 1110 ru-inside ·
1120 ru-direct · 1130 fakeip · 1150 unknown-traffic`.
`placeRuleAfter` двигает **ленивно** — только сплошной занятый блок до первой
дырки, чтобы drag не съедал зазоры.

## 5. DNS пресета

Фрагменты (`dnsServers`+`dnsRules`) поднимаются наверх и мержатся отдельно
(dns_rules.dart:39-251), т.к. живут в другой секции конфига.

- гейт аспекта — var `dns_enable` (нет объявления → включено);
  главный гейт — `cr.enabled`: выключенный пресет не даёт ничего;
- фильтр серверов: **нет** var `dns_server` → эмитятся ВСЕ (выбирать нечего,
  иначе правило сошлётся в пустоту); есть → только выбранный, а если выбранный
  типа `group` — вместе с её членами (:434-441), иначе пустая группа = fatal;
- `detour` нормализуется: у `type: group` снимается безусловно (ядро падает на
  лишнем ключе), у прочих — если пуст/`direct-out`/канала нет;
- дедуп серверов по тегу: identical → skip молча, разные под одним тегом →
  first-wins + warning. DNS-**правила** не дедуплицируются (порядок значим);
- эмиссия: правило, чей `server` не дожил до `dns.servers`, тихо пропускается;
  mirror-группа правил пресета эмитится атомарно, в позиции якоря.

## 6. Гейты и warning'и

Пресет выпадает целиком: выключен (молча); `preset_id` не найден в шаблоне
(warning); required-var пустая/незаданная (warning).

Фрагмент `rule_set` выпадает: `enabled: "@var"` не в `true`; после подстановки
не объект / нет `tag`/`type`; `remote` без скачанного `.srs` (warning; иначе
переписывается в `{type: local, path: <cache>}`).

Route-правило выпадает: нет ни `outbound`, ни `action` (молча); ссылка на
незарегистрированный `rule_set` (warning); если из списка тегов выжил один —
даунгрейд списка до строки; невалидная форма `rule_set` — ссылка снимается,
правило живёт (warning).

DNS-правило выпадает: нет `server` и action не из serverless-набора
(`predefined`/`reject`/`route-options`).

Безусловные нормализации: `resolve`/`sniff`/`route-options` — промежуточные
действия, к ним НЕ применяется outbound-override; финальный `outbound: reject`
нормализуется в `action: reject` (иначе висячая ссылка на outbound = ядро не
стартует).
