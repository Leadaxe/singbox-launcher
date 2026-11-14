# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Windows: files and folders with non-Latin names (Cyrillic and others) can be picked in the system file dialog again — Taildrop **Send files**, config import and backup save failed on such paths because the path came back in the console code page.
- Logs window, **Core** tab: multi-line entries (goroutine dumps from the service) no longer paint over each other, and terminal colour codes are removed.
- Russian UI: the pattern row label in the emoji picker and in the server filter no longer runs under the field.

### Technical / Internal
-

## RU
### Основное
- Windows: файлы и папки с нелатинскими именами (кириллица и другие) снова выбираются в системном диалоге — Taildrop **Send files**, импорт конфига и сохранение бэкапа падали на таких путях: путь приходил в кодовой странице консоли.
- Окно логов, вкладка **Core**: многострочные записи (дампы горутин от службы) больше не рисуются друг поверх друга, цветовые коды терминала убраны.
- Русский интерфейс: подпись строки шаблона в выборе эмодзи и в фильтре серверов больше не уезжает под поле.

### Техническое / Внутреннее
-
