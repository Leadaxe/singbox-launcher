# План: 040 — WIZARD_TEMPLATE_OPTION_TITLES

## 1. Parser (done)

- `TemplateVar.UnmarshalJSON` принимает три формы (see SPEC §2.1).
- `OptionTitles []string` — параллельный массив, nil для pure-legacy.
- Helper `OptionTitle(i int) string`.

## 2. Subst (done)

- `isIntCastVar()` вынесен, `urltest_tolerance` добавлен.

## 3. UI render (partially done, needs fix per §4 SPEC)

**Сейчас (неправильно):**
- `text` + options → `widget.NewSelectEntry` всегда (combo), даже когда title ≠ value.

**Надо:**
- `text` без options → `widget.NewEntry`.
- `text` + options (plain, title==value) → `widget.NewSelectEntry` (combo).
- `text` + options с title != value → валидатор отклоняет на load-time.
- `enum` + любые options → `widget.NewSelect` (дропдаун, title если есть).

## 4. Validator (TODO)

- В `ui/wizard/template/template_validate.go`:
  - Итерировать vars; для `Type == "text"` + `len(Options) > 0` — проверить все title == value, иначе return error.

## 5. Template (partially done, needs revisit)

- `urltest_url` → `text` + plain strings options (сейчас object-form, редизайн).
- `urltest_interval` → `enum` + object-form titles (сейчас `text`, редизайн).
- `urltest_tolerance` → `enum` + object-form titles (сейчас `text`, редизайн).

## 6. Тесты (partially done)

- JSON unit-тесты на 4 формы — есть.
- TODO: unit для `buildSettingsVarRow` на 5 сценариев (см. SPEC §4.4).
- TODO: тест валидатора — rejected на type=text + object-options.
