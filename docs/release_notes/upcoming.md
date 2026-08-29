# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Your settings are migrated automatically on first launch.** The state file moves to a new format (schema v7) where every node of a subscription is stored explicitly instead of being re-parsed from a cached response on every build. The original file is kept next to it as `state.json.v6.bak` — nothing is overwritten until the new file has been written successfully.
- **A migration report tells you what changed.** Anything that could not be carried over one-to-one is listed by name — never dropped silently — and shown once when you first open the configurator (it is also written to `bin/migration_report.txt`). Typical entries: a subscription that has never been fetched has no nodes yet (they appear after the first update); a folded subscription's auto-group is renamed from `PREFIX:auto` to `<replace tag>-auto`, so the core's saved pick for that group resets once.
- **Per-subscription defaults moved into application settings.** "Default update interval" and "Default max nodes" now live in Settings and apply to subscriptions that do not set their own value. Each subscription can still override both.
- **The node limit is now real.** `max nodes` actually stops parsing once the limit is reached and marks the subscription as truncated, instead of being counted after the fact. The hard ceiling is 3000 nodes per subscription; a larger value entered in Settings is clamped to it.

### Technical / Internal
- Config assembly no longer parses subscription bodies. Outbounds are emitted from the materialized `nodes[]` in the state; the raw response cache (`bin/subscriptions/*.raw`) is read once by the migration and then removed.
- Per-node on/off is stored on the node itself (`enabled`) instead of a separate map of hashed identity keys with a TTL and garbage collection. Keys that could not be matched during migration are reported.
- Folding a subscription became "replace": the same modes, plus an explicit tag field, so the group name no longer has to be derived from the tag prefix.
- Backup files stay on contract 0.11: export/import converts between the new model and the old file format, and every lossy conversion is named in the warnings list shown after the operation.
- Remote profiles: `POST /profile/copy-from` and `PATCH /state/*` refuse a state file whose schema major version is newer than this build's, naming both versions. Older files are accepted and migrated. `GET` is not gated, so diagnostics keep working during a mismatch.

## RU
### Основное
- **Настройки мигрируют сами при первом запуске.** Файл состояния переезжает на новый формат (схема v7): узлы подписки хранятся явно, а не разбираются заново из кэша ответа на каждой сборке. Исходный файл остаётся рядом как `state.json.v6.bak` — он не трогается, пока новый не записан успешно.
- **Отчёт миграции показывает, что изменилось.** Всё, что не удалось перенести один-в-один, названо поимённо (молчаливых потерь нет) и показывается один раз при первом открытии конфигуратора; отчёт также пишется в `bin/migration_report.txt`. Типичные пункты: у подписки, которая ни разу не обновлялась, узлов пока нет (появятся после первого обновления); у свёрнутой подписки авто-группа переименована из `ПРЕФИКС:auto` в `<тег замены>-auto`, поэтому сохранённый в ядре выбор для неё один раз сбрасывается.
- **Умолчания подписок переехали в настройки приложения.** «Интервал обновления по умолчанию» и «Максимум узлов по умолчанию» живут в Настройках и применяются к подпискам без собственного значения. У каждой подписки по-прежнему можно задать своё.
- **Ограничение на число узлов стало настоящим.** `max nodes` реально останавливает разбор по достижении лимита и помечает подписку как усечённую, а не считается постфактум. Жёсткий потолок — 3000 узлов на подписку; большее значение в Настройках к нему прижимается.

### Техническое / Внутреннее
- Сборка конфига больше не разбирает тела подписок: outbound'ы эмитятся из материализованных `nodes[]` состояния. Кэш сырых ответов (`bin/subscriptions/*.raw`) читается один раз миграцией и удаляется.
- Отметка «узел выключен» хранится на самом узле (`enabled`), а не отдельной картой хэш-ключей с TTL и сборкой мусора. Ключи, которые миграция не смогла сопоставить, попадают в отчёт.
- Свёртка подписки стала «заменой»: те же режимы плюс явное поле тега — имя группы больше не обязано быть производным от префикса тегов.
- Файлы бэкапа остаются на контракте 0.11: экспорт/импорт конвертирует между новой моделью и старым форматом, и каждая потеря конвертации названа в списке предупреждений после операции.
- Удалённые профили: `POST /profile/copy-from` и `PATCH /state/*` отклоняют файл состояния, чья мажорная версия схемы новее этой сборки, называя обе версии. Файлы постарше принимаются и мигрируются. `GET` не гейтуется — диагностика обязана работать в момент расхождения.
