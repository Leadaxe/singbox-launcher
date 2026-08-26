# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- Duplicate subscription entries collapse again (v1.5.2 regression). A subscription that lists the same server, port and password 32 times — differing only in the `#name` — now yields one node, not 32. The first entry wins, so the name you see is the first one the provider sent. The key is the connection itself (`scheme|server|port|credential`), so one server with one password behind different SNI is now a single node too.
- A source excluded from the config is no longer silent. When its detour hop can't be resolved, the source drops out fail-closed as before — but now the Wizard → Sources row carries a ⚠ mark with the reason, and a toast appears on the Local tab after the build.

### Technical / Internal
- Dedup lives in the parse layer (`serverConnKey`), not in the identity model: it lasts one source parse, is never written to state, and does not touch disabled marks, node references or identity. The Wizard source-node counter goes through the same parse, so preview and build agree.
- Contract amendment (IDENTITY.md §1.2, §4): the by-design dedup divergence with LxBox `nodeIdentityKey` has collapsed — both sides now use the same key.

## RU
### Основное
- Дубли записей подписки снова схлопываются (регресс v1.5.2). Подписка, перечисляющая один и тот же сервер, порт и пароль 32 раза с разным только `#именем`, снова даёт один узел, а не 32. Побеждает первая запись — её имя вы и увидите. Ключ — само подключение (`схема|сервер|порт|креденшл`), поэтому один сервер с одним паролем за разными SNI теперь тоже один узел.
- Исключение источника из конфига перестало быть молчаливым. Когда detour-хоп не находится, источник по-прежнему выпадает fail-closed — но теперь строка в Мастере → Источники несёт пометку ⚠ с причиной, а после сборки на вкладке Local появляется уведомление.

### Техническое / Внутреннее
- Дедуп живёт на parse-слое (`serverConnKey`), а не в модели идентичности: он существует один разбор источника, в состояние не пишется и не трогает ни отметки выключения, ни ссылки на узлы, ни идентичность. Счётчик узлов в Мастере идёт тем же разбором, поэтому превью и сборка совпадают.
- Амендмент контракта (IDENTITY.md §1.2, §4): by-design различие по дедупу с LxBox `nodeIdentityKey` схлопнулось — обе стороны используют один ключ.
