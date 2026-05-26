# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- User-defined SRS rules no longer lose their downloaded file. Bug #77: file was saved under the URL-derived name, but the launcher looked it up under a label-derived name, and the orphan GC then deleted the actually-downloaded file on rebuild.
- SRS rule editing: opening an existing SRS rule for edit now correctly shows the "SRS" type and the source URL instead of falling back to "Custom JSON" with an empty body.

### Technical / Internal
- SPEC 063 — refactor: the redundant `state.Rule.ID` field is dropped from the state schema. Rule identity is now a pure function computed from `body.name` (for inline/srs) or `ref` (for preset). Legacy `state.json` with `"id":"…"` loads cleanly — Go JSON ignores the unknown field; the next Save no longer emits it. Single source of truth for SRS filenames on disk: `SRSTagFromURL(URL)` — used by downloader, build pipeline, and orphan GC.

## RU
### Основное
- Пользовательские SRS-правила больше не теряют скачанный файл. Issue #77: файл сохранялся под URL-derived именем, а лаунчер искал его под label-based именем и orphan GC потом удалял реально скачанный файл при rebuild'е.
- Редактирование SRS-правила: при открытии существующего SRS-правила корректно показывается тип «SRS» и исходный URL вместо fallback'а на «Custom JSON» с пустым body.

### Техническое / Внутреннее
- SPEC 063 — refactor: из state schema удалено избыточное поле `state.Rule.ID`. Identity правила теперь чистая функция от `body.name` (для inline/srs) или `ref` (для preset). Legacy `state.json` с `"id":"…"` грузится без ошибок — Go JSON игнорирует неизвестное поле; следующий Save его не эмитит. Единый источник правды для filename'ов SRS на диске: `SRSTagFromURL(URL)` — у downloader, build pipeline и orphan GC.
