# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixes
- Xray JSON subscriptions: the `users[0].encryption` value of VLESS nodes (VLESS Encryption / ML-KEM, `mlkem768x25519plus…`) is now carried into the sing-box outbound as is; before, it was dropped and servers requiring VLESS Encryption refused the connection (`bad http protocol version`). Empty or `none` still emits no field, same as the share-link parser (#121).

### Technical / Internal
- Backup format **1.0** — the file is the launcher state itself, so everything the state holds travels: per-node rule and DNS sections, folders with their settings and contents, chain hops addressed at a folder, subscription identity. It is **off by default**: the export dialog has a checkbox "Backup format 1.0 (new; requires LxBox with 1.0 import)", and without it the launcher keeps writing the previous format 0.12 — a file the phone app cannot read yet is worse than an older one. **Import reads both formats always** and never asks which one you have. A record of a kind that node sections do not allow is dropped on import and named to you, instead of disappearing into the log.
- Transferring settings is available over the debug API too, at parity with the buttons on the Files tab: `GET /backup/export?format=1.0|0.12` hands back the file itself (losses in the `X-Backup-Warnings` header, or in the body with `?envelope=1`), `POST /backup/import` merges a file of either format and rebuilds `config.json`, `GET /backup/formats` reports what this build reads and writes. Paired machines mirror export and import under `/remote/machines/{id}/backup/*`.
- Launcher state schema **v8** (`state.json`): one shape for every record — application metadata on the record, `body` = the sing-box object as is. Route rules carry `num` (former `order_num`), `name`, `refs[]` (former `srs_url` + `srs_urls`) and `vars` as fields, while `body` holds the matchers together with the target (`outbound` / `action`); a preset record has no body at all. DNS servers and rules keep their sing-box body in `body`, with the server tag in the `tag` field. The two root keys were renamed: `dns_options` → `dns`, `warp_accounts` → `warp`.
- A v7 file is migrated on the first load and a copy of the original is kept next to it as `state.json.v7.bak`. Nothing is dropped: rules of an unknown kind, DNS bodies, node sections, order numbers, toggles and preset variables all travel across, and the generated `config.json` is byte-for-byte the same as before.
- Remote mode requires the same major schema (8) on both machines — a desktop of an older version will not attach to a launcher on v8.

## RU
### Основное
-

### Исправления
- Подписки в формате Xray JSON: значение `users[0].encryption` у VLESS-узлов (VLESS Encryption / ML-KEM, `mlkem768x25519plus…`) теперь переносится в outbound sing-box как есть; раньше поле терялось, и серверы с обязательным VLESS Encryption рвали соединение (`bad http protocol version`). Пусто или `none` по-прежнему не даёт поля — как у парсера share-ссылок (#121).

### Техническое / Внутреннее
- Формат бэкапа **1.0** — файл теперь и есть состояние лаунчера, поэтому едет всё, что состояние хранит: секции правил и DNS у отдельных узлов, папки с их настройками и составом, адресные хопы цепочек в папку, идентификация подписки. По умолчанию он **выключен**: в диалоге экспорта появился чекбокс «Backup format 1.0 (new; requires LxBox with 1.0 import)», без него лаунчер по-прежнему пишет прежний формат 0.12 — файл, который телефон ещё не прочитает, хуже файла старого формата. **Импорт читает оба формата всегда** и не спрашивает, какой у вас. Запись вида, которого секции узла не допускают, на импорте отбрасывается и называется вам, а не исчезает в логе.
- Перенос настроек доступен и через debug API, наравне с кнопками вкладки «Файлы»: `GET /backup/export?format=1.0|0.12` отдаёт сам файл (потери — заголовком `X-Backup-Warnings` либо в теле при `?envelope=1`), `POST /backup/import` сливает файл любого формата и пересобирает `config.json`, `GET /backup/formats` говорит, что эта сборка читает и пишет. У сопряжённых машин есть зеркала экспорта и импорта под `/remote/machines/{id}/backup/*`.
- Схема состояния лаунчера **v8** (`state.json`): у каждой записи одна форма — метаданные приложения полями, `body` = объект sing-box как есть. У правил маршрута полями лежат `num` (бывший `order_num`), `name`, `refs[]` (бывшие `srs_url` + `srs_urls`) и `vars`, а в `body` — матчеры вместе с целью (`outbound` / `action`); у записи пресета тела нет вовсе. У DNS-серверов и DNS-правил тело sing-box лежит в `body`, тег сервера — в поле `tag`. Два корневых ключа переименованы: `dns_options` → `dns`, `warp_accounts` → `warp`.
- Файл v7 мигрирует при первой загрузке, копия исходного остаётся рядом под именем `state.json.v7.bak`. Ничего не теряется: правила неизвестного вида, тела DNS, секции узлов, номера порядка, тумблеры и переменные пресетов переезжают как есть, а собранный `config.json` остаётся байт-в-байт прежним.
- Удалённый режим требует одинаковой мажорной схемы (8) на обеих машинах — десктоп старой версии к лаунчеру на v8 не подключится.
