# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Saved states switcher is safer.** The Core dashboard "Switch state…" dropdown now lists **● Current (active)** as the first item and shows it as the selected value, so a stray tap on the top of the list is a no-op instead of switching away from the live state. (Switching to a named state already asks first — Save current / Discard / Cancel — that confirm dialog is unchanged.)

- **Self-describing debug API.** `GET /` now returns a manifest (api/spec/launcher/core/auth, a version-pinned docs link, and the endpoint list) and `GET /help` returns just the endpoint list — point an agent at the base URL + token and it discovers the surface itself. Settings → Debug API has a new **Copy API info** button that copies a connection card (base URL, token, versions, docs) to hand to an agent. (SPEC 078)

### Technical / Internal
- `core_dashboard_tab.go`: `refreshStateSelector` prepends the localized `core.state_current_option` anchor and `selectCurrentStateSilently()` keeps it shown as selected (without firing OnChanged) on refresh and on confirm-dialog cancel; OnChanged treats the current/empty selection as a no-op.
- `core/debugapi`: single endpoint registry (`s.endpoints()`) drives routing, `GET /` and `GET /help` (single source of truth — docs can't drift from wiring); `DocsURL()` pins the docs link to the release tag (dev → main); `ConnectionCardJSON()` builds the settings card. Tests in `manifest_test.go` assert every advertised endpoint is wired. (SPEC 078)

## RU
### Основное
- **Переключатель сохранённых состояний стал безопаснее.** В дашборде Core выпадающий список «Сменить state…» теперь первым пунктом показывает **● Текущее (активно)** и отображает его как выбранное — случайный тап по верху списка ничего не делает, а не уводит с живого состояния. (Переключение на именованный state по-прежнему спрашивает — Сохранить текущее / Не сохранять / Отмена — этот модал не менялся.)

- **Самоописываемый debug API.** `GET /` теперь отдаёт манифест (api/spec/launcher/core/auth, ссылку на доку, привязанную к версии, и список эндпоинтов), а `GET /help` — только список эндпоинтов: дай агенту base URL + токен, и он сам разберётся. В Settings → Debug API появилась кнопка **Копировать инфо API** — кладёт в буфер карточку подключения (base URL, токен, версии, docs) для передачи агенту. (SPEC 078)

### Техническое / Внутреннее
- `core_dashboard_tab.go`: `refreshStateSelector` добавляет первым локализованный якорь `core.state_current_option`, а `selectCurrentStateSilently()` держит его выбранным (без триггера OnChanged) на refresh и на Cancel модала; OnChanged трактует выбор текущего/пустого как no-op.
- `core/debugapi`: единый реестр (`s.endpoints()`) питает роутинг, `GET /` и `GET /help` (один источник правды — дока не расходится с кодом); `DocsURL()` привязывает ссылку на доку к релизному тегу (dev → main); `ConnectionCardJSON()` строит карточку для настроек. Тесты в `manifest_test.go` проверяют, что каждый заявленный эндпоинт реально зарегистрирован. (SPEC 078)
