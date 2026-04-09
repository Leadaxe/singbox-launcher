# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Internal / Refactoring

### Highlights

- **Wizard — Settings:** `wizard_template.json` declares configurable **`vars`**; the wizard **Settings** tab shows them, saves values in wizard state, and they substitute **`@name`** placeholders in the generated config. Optional **`{"separator": true}`** entries draw horizontal rules between rows (layout only).

- **macOS / TUN:** Turning **TUN** off in the wizard is blocked while the core is running; the wizard also refuses if a **sing-box** process is still visible to the OS (avoids deleting cache/logs while TUN/ports stay busy). **Stop** no longer marks the core as stopped if the privileged kill was cancelled or failed. After a successful **Stop**, the launcher may prompt for admin once to remove **`experimental.cache_file.path`** under **`bin/`** and core log files **`logs/sing-box.log`** / **`.old`** so a non-admin start can recreate them.

- **Win7 wizard:** The Win7 x86 launcher uses the same **`params`** TUN block as **`windows`/`linux`** (no separate **`win7`** section). Unset **`tun_stack`** defaults to **`gvisor`** on **`windows/386`** via **`default_value`** object in **`wizard_template.json`** (e.g. **`{"win7":"gvisor","default":"system"}`**); **`vars[].default_value`** may be a scalar or a platform-keyed JSON object (**`VarDefaultValue`**).

- **Linux:** If `sing-box` is on `PATH` (e.g. installed from your distro package), the launcher uses it automatically; otherwise it uses `bin/sing-box` next to the launcher. **Core → Download** still installs into local `bin/` only ([issue #48](https://github.com/Leadaxe/singbox-launcher/issues/48)).

- **Closed specs:** [032 — WIZARD_SETTINGS_TAB](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/032-F-C-WIZARD_SETTINGS_TAB/SPEC.md), [034 — HTTP_ENV_PROXY](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/034-F-C-HTTP_ENV_PROXY/SPEC.md), [019 — WIN7_ADAPTATION](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/019-F-C-WIN7_ADAPTATION/SPEC.md).

- **HTTP proxy (environment):** Outbound HTTP(S) from the launcher (subscriptions, SRS rule-sets, locale files, wizard template download from Core Dashboard) respects **`HTTP_PROXY`**, **`HTTPS_PROXY`**, and **`NO_PROXY`** (Go `http.ProxyFromEnvironment`). Local Clash/Mihomo API traffic is unchanged (still direct to localhost). Network error messages redact `user:password@` in URLs shown to the user.

### Technical / Internal

- **`vars[].default_value` object keys:** resolution matches **`params[].platforms`** semantics — **`GOOS`** names only (**`windows`**, **`linux`**, **`darwin`**, …), plus explicit **`win7`** ( **`windows`/`386`** only, before **`windows`**), then **`default`**. Combined keys like **`linux_amd64`** are no longer used in lookup (use **`linux`**).

- **HTTP outbound:** All `CreateHTTPClient` callers share one `http.Transport` (connection pooling and TLS reuse across subscriptions, SRS, locales, downloads). `MaxIdleConnsPerHost` is 32 (default Go transport uses 2).

- **Hysteria2 ports from subscriptions:** `mport` / `ports` now follow the official Hysteria 2 list format (comma-separated ports and `start-end` ranges). Multi-port in the URI authority (e.g. `host:443,20000-30000`) is recovered when `net/url` cannot parse it. Bare single ports map to `low:high` for sing-box `server_ports`.

- Build scripts `build/build_linux.sh` and `build/test_linux.sh` are stored in git with the executable bit; after clone, run `./build/...` without `chmod +x` on tracked files ([issue #49](https://github.com/Leadaxe/singbox-launcher/issues/49)).

---

## RU

### Внутреннее / Рефакторинг

### Основное

- **Визард — «Настройки»:** В шаблоне (`wizard_template.json`) объявляются пользовательские **`vars`**; лаунчер выводит их на вкладку **«Настройки»**, сохраняет в состоянии визарда и подставляет в собираемый конфиг по плейсхолдерам **`@name`**. Опционально **`{"separator": true}`** — горизонтальные линии между строками (только оформление).

- **macOS / TUN:** Снять **TUN** в визарде нельзя, пока ядро запущено; также блокировка, если в ОС всё ещё виден процесс **sing-box** (чтобы не удалять кеш/логи при занятом TUN/портах). **Stop** больше не помечает ядро остановленным, если привилегированное завершение отменено или не удалось. После успешного **Stop** при необходимости запрашивается пароль для удаления кеша в **`bin/`** и логов **`logs/sing-box.log`** / **`.old`**.

- **Визард Win7:** Win7 x86 использует тот же блок TUN в **`params`**, что и **`windows`/`linux`** (без отдельной секции **`win7`**). Незаданный **`tun_stack`** на **windows/386** — **`gvisor`** через объект **`default_value`** в **`wizard_template.json`** (например **`{"win7":"gvisor","default":"system"}`**); у **`vars`** поле **`default_value`** может быть скаляром или JSON-объектом с ключами платформ (**`VarDefaultValue`**).

- **Linux:** если `sing-box` есть в `PATH` (например, из пакета дистрибутива), лаунчер использует его; иначе — `bin/sing-box` рядом с лаунчером. Кнопка **Core → Download** по-прежнему кладёт бинарник только в локальный `bin/` ([issue #48](https://github.com/Leadaxe/singbox-launcher/issues/48)).

- **Закрытые спеки:** [032 — WIZARD_SETTINGS_TAB](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/032-F-C-WIZARD_SETTINGS_TAB/SPEC.md), [034 — HTTP_ENV_PROXY](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/034-F-C-HTTP_ENV_PROXY/SPEC.md), [019 — WIN7_ADAPTATION](https://github.com/Leadaxe/singbox-launcher/blob/develop/SPECS/019-F-C-WIN7_ADAPTATION/SPEC.md).

- **Прокси по переменным окружения:** Исходящие HTTP(S) лаунчера (подписки, SRS, локали, скачивание шаблона визарда с вкладки Core) учитывают **`HTTP_PROXY`**, **`HTTPS_PROXY`**, **`NO_PROXY`** (как в стандартной библиотеке Go). Запросы к локальному Clash/Mihomo API по-прежнему без этого прокси (прямо на localhost). В текстах сетевых ошибок пароли в URL скрываются.

### Техническое / Внутреннее

- **Ключи объекта `vars[].default_value`:** как у **`params[].platforms`** — только имена **`GOOS`** (**`windows`**, **`linux`**, **`darwin`**, …), плюс явный **`win7`** (только **windows/386**, раньше **`windows`**), затем **`default`**. Комбинированные ключи (**`linux_amd64`** и т.п.) в переборе не участвуют (используйте **`linux`**).

- **Исходящий HTTP:** Все вызовы `CreateHTTPClient` используют один общий `http.Transport` (пул соединений и повторное использование TLS между подписками, SRS, локалями и загрузками). `MaxIdleConnsPerHost` = 32 (у дефолтного транспорта Go — 2).

- **Hysteria2 порты из подписок:** Параметры `mport` / `ports` разбираются в официальном формате Hysteria 2 (список через запятую: порты и диапазоны `начало-конец`). Multi-port в authority URI (например `host:443,20000-30000`) восстанавливается, если стандартный парсер URL не принимает строку. Одиночный порт даёт диапазон `n:n` для sing-box `server_ports`.

- Скрипты `build/build_linux.sh` и `build/test_linux.sh` в репозитории с флагом исполняемого файла; после клона достаточно `./build/...` без `chmod +x` для отслеживаемых файлов ([issue #49](https://github.com/Leadaxe/singbox-launcher/issues/49)).
