# SPEC: Template vars — option titles и URLTest preset-vars

Задача: (1) позволить авторам `wizard_template.json` давать подписи опциям дропдаунов отдельно от подставляемых значений; (2) вынести параметры `auto-proxy-out` (url / interval / tolerance) в template vars с пресетами, чтобы их можно было менять через Wizard → Settings без ручной правки JSON.

**Статус:** фундамент реализован (коммиты `45730a7` — WizardOption; `cfc0634` — URLTest vars), **требуется доработка widget-рендерера** (см. §4 «Критический TODO»).
**Спека написана ретроспективно.**

**Связанные спеки:**
- `SPECS/032-F-C-WIZARD_SETTINGS_TAB/` — исходная Wizard → Settings таб с vars-подстановкой.

---

## 1. Проблема

### 1.1 До изменений

- `vars[].options` — массив строк `["a", "b"]`. Подпись дропдауна == подставляемое значение. Нельзя писать «5m (default)» в подписи — оно уйдёт в `interval` конфига и sing-box отклонит.
- Параметры URL-теста (`auto-proxy-out.url / interval / tolerance`) — захардкожены в JSON. Пользователь, желающий сменить probe-URL или сделать interval 10m, лезет править конфиг руками / через wizard-source-edit.

### 1.2 Цель

1. `vars[].options` принимает **object-form** `[{"title": "5m (default)", "value": "5m"}]` параллельно со старой `["5m"]` формой.
2. Render'ер UI показывает `title` в дропдауне; при выборе маппится обратно к `value`, которое идёт в subst.
3. URLTest три vars: `urltest_url`, `urltest_interval`, `urltest_tolerance` — с пресетами.

---

## 2. Требования

### 2.1 Модель — `TemplateVar` (коммит `45730a7`)

- `Options []string` — значения (для subst).
- Новое `OptionTitles []string` — параллельные подписи. `nil` если все подписи == значениям (pure-legacy).
- Helper: `func (v TemplateVar) OptionTitle(i int) string` — возвращает title если есть, иначе value.
- Кастомный `UnmarshalJSON` на `TemplateVar` принимает три формы:
  - `["5m", "30m"]` → `Options=["5m","30m"]`, `OptionTitles=nil`.
  - `[{"title":"5m (default)","value":"5m"}, ...]` → `Options=["5m", ...]`, `OptionTitles=["5m (default)", ...]`.
  - Mixed `["plain", {"title":"Fancy","value":"fancy"}]` — допускается, каждый элемент парсится независимо.
  - Пустая title в object-form → fallback на `value` в `OptionTitles[i]`.

### 2.2 Template validator (TODO, не сделано)

- `ui/wizard/template/template_validate.go`:
  - Для `type: "text"` + `options` с любой парой `title != value` — **warning / skip var / error** (см. §4).
  - Для `type: "enum"` — любая форма options допустима.

### 2.3 UI рендерер — правило widget'а (TODO, см. §4)

**Правило:**

| `type` | `options` | Widget |
|---|---|---|
| `enum` | `["a","b"]` или `[{title,value}]` | `widget.NewSelect` (pure dropdown) |
| `text` без `options` | — | `widget.NewEntry` |
| `text` + `options`, все `title == value` | plain strings | `widget.NewSelectEntry` (combo: text + preset-menu) |
| `text` + `[{title, value}]` с title ≠ value | — | **запрещено** (валидатор) |

Семантика: combo предполагает что пользователь может печатать свой текст — ему должна соответствовать закрытая пара title==value, иначе свободный ввод ломает маппинг.

### 2.4 URLTest template vars (коммит `cfc0634`)

В `bin/wizard_template.json`:

```json
{
  "tag": "auto-proxy-out",
  "type": "urltest",
  "options": {
    "url": "@urltest_url",
    "interval": "@urltest_interval",
    "tolerance": "@urltest_tolerance",
    ...
  }
}
```

Три vars в блоке Settings:

- `urltest_url` — `type: "text"` (кастомные URL хочется разрешить), `options` — plain strings (возможность custom-ввода).
- `urltest_interval` — `type: "enum"` (только пресеты, кастомные duration'ы не нужны), `options` — object-form с подписями «5m (default)» etc.
- `urltest_tolerance` — `type: "enum"`, object-form с подписями «100 ms (default)» etc. Целочисленная подстановка через `isIntCastVar`.

### 2.5 Subst — integer cast

В `ui/wizard/template/substitute.go`:

- `isIntCastVar(name) bool` — хелпер, перечисляет vars с числовым типом в JSON.
- Добавлено: `urltest_tolerance` (к `tun_mtu`, `mixed_listen_port`, `proxy_in_listen_port`).

---

## 3. Тесты

- `ui/wizard/template/vars_options_test.go` — четыре unit-теста:
  - Legacy `["a", "b"]` form.
  - Object-form `[{title, value}]`.
  - Mixed (string среди объектов).
  - Пустая `title` → fallback на value.

---

## 4. Критический TODO: widget-рендерер и validator

**На момент ревью фичи подтвердилась infra (`45730a7`): дропдауны отрисовывают титлы корректно, subst идёт по value.** Но выбран неверный widget — для `type: "text"` + object-options используется `widget.NewSelectEntry` (combo с полем ввода). Это семантически несовместимо (см. §2.3): пользователь может допечатать произвольный текст, который не маппится обратно ни на один `value`.

### 4.1 Что сделать

1. **Рендерер** в `ui/wizard/tabs/settings_tab.go`:
   ```go
   case "text":
       if len(options) > 0 {
           se := widget.NewSelectEntry(options)  // Options, не OptionTitles
           se.OnChanged = onChanged              // напрямую value==текст
           return row
       }
       // plain Entry
   case "enum":
       sel := widget.NewSelect(optionTitles, ...)  // титулы
       sel.OnChanged = func(t string) {
           val := valueForTitle(t)
           onChanged(val)
       }
   ```
   Выкинуть ветку «`text` + `{title, value}`» — validator не пропустит такой шаблон.

2. **Validator** в `ui/wizard/template/template_validate.go`:
   ```go
   for _, v := range vars {
       if v.Separator { continue }
       if strings.EqualFold(v.Type, "text") && len(v.Options) > 0 {
           for i := range v.Options {
               if v.OptionTitle(i) != v.Options[i] {
                   return fmt.Errorf("vars[%s]: type=text с options требует title==value для каждой опции", v.Name)
               }
           }
       }
   }
   ```

3. **Шаблон** в `bin/wizard_template.json`:
   - `urltest_url` → оставить `type: "text"`, `options` превратить в plain strings (титулы не нужны, просто несколько URL'ов на выбор + возможность вбить свой).
   - `urltest_interval`, `urltest_tolerance` → `type: "enum"` (были `text`), `options` в object-form.

4. **Unit-test** для `buildSettingsVarRow`:
   - `enum` + plain options → Select.
   - `enum` + object options → Select, отображает titles.
   - `text` без options → Entry.
   - `text` + plain options → SelectEntry.
   - `text` + object options → validator error до рендера.

---

## 5. Инварианты

1. **Legacy-совместимость.** Шаблоны с `["a", "b"]` options работают без изменений (title == value).
2. **Object-form обратно-совместима** — никаких новых обязательных полей, `title` optional (empty = fallback value).
3. **Subst не изменился.** Только render'ер и UI видят title; всё, что идёт в JSON конфиг — `value` как раньше.
4. **Правило «text + {title,value} = запрещено»** — инвариант безопасности маппинга. Валидатор обязателен.

---

## 6. Совместимость

- Шаблоны без новых vars / старого формата options — работают как раньше.
- Поле `parser_config.version` не меняется (ничего в data-model state'а не поменялось).

---

## 7. Не-цели

- Не добавляем i18n для титлов (пока — один язык, что в шаблоне написано). Локализация титлов — отдельная задача.
- Не добавляем `enum` с произвольными типами значений (кроме string) — value остаётся string.
- Не делаем «nested options» (grouped dropdowns) — один плоский список.

---

## 8. Open questions

- Нужны ли **иконки** в option-titles (например, `"🇯🇵 Japan (Osaka)"`)? Сейчас title — plain string, эмодзи работают.
- Нужен ли `tooltip` per-option? Сейчас tooltip только на var-уровне (в `TemplateVar.Tooltip`), не на опции.
