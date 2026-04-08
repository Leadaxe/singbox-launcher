# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Internal / Refactoring

### Highlights

- **Wizard — Settings:** `wizard_template.json` declares configurable **`vars`**; the wizard **Settings** tab shows them, saves values in wizard state, and they substitute **`@name`** placeholders in the generated config.

- **Win7 wizard:** The Win7 x86 launcher uses the same **`params`** TUN block as **`windows`/`linux`** (no separate **`win7`** section). Unset **`tun_stack`** defaults to **`gvisor`** on **`windows/386`** via **`default_value`** object in **`wizard_template.json`** (e.g. **`{"win7":"gvisor","default":"system"}`**); **`vars[].default_value`** may be a scalar or a platform-keyed JSON object (**`VarDefaultValue`**).

- **Linux:** If `sing-box` is on `PATH` (e.g. installed from your distro package), the launcher uses it automatically; otherwise it uses `bin/sing-box` next to the launcher. **Core → Download** still installs into local `bin/` only ([issue #48](https://github.com/Leadaxe/singbox-launcher/issues/48)).

- **Closed specs:** [032 — WIZARD_SETTINGS_TAB](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/032-F-C-WIZARD_SETTINGS_TAB/SPEC.md), [019 — WIN7_ADAPTATION](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/019-F-C-WIN7_ADAPTATION/SPEC.md).

### Technical / Internal

- **`vars[].default_value` object keys:** resolution matches **`params[].platforms`** semantics — **`GOOS`** names only (**`windows`**, **`linux`**, **`darwin`**, …), plus explicit **`win7`** ( **`windows`/`386`** only, before **`windows`**), then **`default`**. Combined keys like **`linux_amd64`** are no longer used in lookup (use **`linux`**).

- **Hysteria2 ports from subscriptions:** `mport` / `ports` now follow the official Hysteria 2 list format (comma-separated ports and `start-end` ranges). Multi-port in the URI authority (e.g. `host:443,20000-30000`) is recovered when `net/url` cannot parse it. Bare single ports map to `low:high` for sing-box `server_ports`.

- Build scripts `build/build_linux.sh` and `build/test_linux.sh` are stored in git with the executable bit; after clone, run `./build/...` without `chmod +x` on tracked files ([issue #49](https://github.com/Leadaxe/singbox-launcher/issues/49)).

---

## RU

### Внутреннее / Рефакторинг

### Основное

- **Визард — «Настройки»:** В шаблоне (`wizard_template.json`) объявляются пользовательские **`vars`**; лаунчер выводит их на вкладку **«Настройки»**, сохраняет в состоянии визарда и подставляет в собираемый конфиг по плейсхолдерам **`@name`**.

- **Визард Win7:** Win7 x86 использует тот же блок TUN в **`params`**, что и **`windows`/`linux`** (без отдельной секции **`win7`**). Незаданный **`tun_stack`** на **windows/386** — **`gvisor`** через объект **`default_value`** в **`wizard_template.json`** (например **`{"win7":"gvisor","default":"system"}`**); у **`vars`** поле **`default_value`** может быть скаляром или JSON-объектом с ключами платформ (**`VarDefaultValue`**).

- **Linux:** если `sing-box` есть в `PATH` (например, из пакета дистрибутива), лаунчер использует его; иначе — `bin/sing-box` рядом с лаунчером. Кнопка **Core → Download** по-прежнему кладёт бинарник только в локальный `bin/` ([issue #48](https://github.com/Leadaxe/singbox-launcher/issues/48)).

- **Закрытые спеки:** [032 — WIZARD_SETTINGS_TAB](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/032-F-C-WIZARD_SETTINGS_TAB/SPEC.md), [019 — WIN7_ADAPTATION](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/019-F-C-WIN7_ADAPTATION/SPEC.md).

### Техническое / Внутреннее

- **Ключи объекта `vars[].default_value`:** как у **`params[].platforms`** — только имена **`GOOS`** (**`windows`**, **`linux`**, **`darwin`**, …), плюс явный **`win7`** (только **windows/386**, раньше **`windows`**), затем **`default`**. Комбинированные ключи (**`linux_amd64`** и т.п.) в переборе не участвуют (используйте **`linux`**).

- **Hysteria2 порты из подписок:** Параметры `mport` / `ports` разбираются в официальном формате Hysteria 2 (список через запятую: порты и диапазоны `начало-конец`). Multi-port в authority URI (например `host:443,20000-30000`) восстанавливается, если стандартный парсер URL не принимает строку. Одиночный порт даёт диапазон `n:n` для sing-box `server_ports`.

- Скрипты `build/build_linux.sh` и `build/test_linux.sh` в репозитории с флагом исполняемого файла; после клона достаточно `./build/...` без `chmod +x` для отслеживаемых файлов ([issue #49](https://github.com/Leadaxe/singbox-launcher/issues/49)).
