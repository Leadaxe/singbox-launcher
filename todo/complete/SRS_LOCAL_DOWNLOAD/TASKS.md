# Задачи: Локальное скачивание SRS

## Этап 1: Удаление go-any-way-githubusercontent

- [ ] Удалить outbound `go-any-way-githubusercontent` из `wizard_template.json`
- [ ] Удалить из `get_free.json`
- [ ] Убрать упоминания в `source_tab.go`
- [ ] Убрать упоминания в `ParserConfig.md`

## Этап 2: Подготовка и сервис скачивания

- [ ] Создать директорию `bin/rule-sets/` при инициализации (platform.EnsureDirectories или FileService)
- [ ] Реализовать `DownloadSRS(ctx, url, destPath) error` с поддержкой отмены
- [ ] Обработка ошибок, логирование
- [ ] Таймаут 60 сек

## Этап 3: UI Rules tab — кнопка SRS и логика

- [ ] Добавить кнопку ⬇/🔄/✔️ после иконки `?` для правил с SRS
- [ ] Чекбокс заблокирован, пока SRS не скачан; при клике — попап с подсказкой
- [ ] По нажатию ⬇ — скачивание; успех → ✔️, разблокировка чекбокса; ошибка → диалог
- [ ] По нажатию ✔️ — перезагрузка SRS
- [ ] При первом открытии: для default-правил с SRS — авто-скачивание; при ошибке — не включать
- [ ] Обработка правил с несколькими SRS (Russian blocked resources)

## Этап 4: Генерация конфига

- [ ] В `MergeRouteSection`: для rule_set с remote URL на raw.githubusercontent.com подставлять `type: "local"`, `path`
- [ ] Удалить `download_detour`, `update_interval` из rule_set в wizard_template.json

## Этап 5: Тестирование

- [ ] Включение правила без SRS → скачивание → успех → правило включено
- [ ] Включение правила при недоступном GitHub → ошибка → правило остаётся выключенным
- [ ] Первый запуск VPN — обращений к raw.githubusercontent.com нет
- [ ] Правило с несколькими SRS (Russian blocked resources) — все скачиваются
- [ ] Закрытие визарда во время скачивания — отмена без ошибок
