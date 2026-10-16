# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **SRS rules with many links: dead links are named, not hidden.** The real cause of "more than 7 SRS links fail" was a link that returns HTTP 404, not the group deadline: the download stopped at the first broken file and the dialog only said "see the log". Now every file of the rule is downloaded, and if some links do not exist the dialog lists them by file name so they can be removed from the rule.

### Technical / Internal
-

## RU
### Основное
- **SRS-правило с множеством ссылок: мёртвые ссылки называются поимённо.** Настоящая причина «больше 7 SRS-ссылок не работает» — ссылка, отвечающая HTTP 404, а не дедлайн группы: загрузка вставала на первом битом файле, а окно писало только «подробности в логе». Теперь качаются все файлы правила, и если каких-то ссылок не существует, окно перечисляет их по имени файла, чтобы их убрали из правила.

### Техническое / Внутреннее
-
