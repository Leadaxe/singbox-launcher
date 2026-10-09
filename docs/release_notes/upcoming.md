# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Wizard → Rules: the list keeps its scroll position after drag-and-drop reorder, edit or delete (it used to jump back to the top).
- Wizard → Rules: an SRS rule with many rule-set URLs no longer fails to download on a slow connection. The whole group used to share a fixed 90-second limit, so rules with 8+ URLs kept failing on the same file; the limit now grows with the number of files, and the error dialog shows the URL that actually failed instead of the first one.

### Technical / Internal
-

## RU
### Основное
- Мастер → Rules: список сохраняет позицию прокрутки после перетаскивания, правки или удаления правила (раньше прыгал в начало).
- Мастер → Rules: SRS-правило с большим числом ссылок больше не падает с ошибкой загрузки на медленном канале. Вся группа делила фиксированные 90 секунд, и правило с 8+ ссылками раз за разом обрывалось на одном и том же файле; теперь лимит растёт с числом файлов, а окно ошибки показывает ссылку, которая реально не скачалась, а не первую.

### Техническое / Внутреннее
-
