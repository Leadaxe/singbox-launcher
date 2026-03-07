# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Содержимое перенесено в [0-8-3.md](0-8-3.md).**

---

## EN

### Highlights
(пункты для следующего релиза после 0.8.3)

- **Diagnostics Logs (Internal tab):** When the Logs window is open, the Internal tab now receives all log levels up to Trace, so Trace/Verbose/Info messages are visible even in release builds. File logging still respects the global level (Warn in release).

- **Wintun download:** First attempt (wintun.net) limited to 30 seconds; on failure, fallback to GitHub repo assets (`assets/wintun-0.14.1.zip`).

- **Download timeouts:** If no data is received for 1 minute during any file download (sing-box, wintun), the download is aborted with a "connection stalled" error instead of waiting for the full 5-minute timeout.

---

## RU

### Основное
(пункты для следующего релиза после 0.8.3)

- **Окно логов (вкладка Internal):** При открытом окне Logs во вкладку Internal теперь попадают все уровни до Trace, поэтому сообщения Trace/Verbose/Info видны и в release-сборке. Запись в файл по-прежнему ограничена глобальным уровнем (Warn в release).

- **Загрузка wintun:** Первая попытка (wintun.net) ограничена 30 секундами; при ошибке — запасной источник: архив в репозитории (`assets/wintun-0.14.1.zip`).

- **Таймауты загрузки:** Если в течение 1 минуты не приходит данных при скачивании файла (sing-box, wintun), загрузка прерывается с ошибкой «connection stalled», без ожидания полных 5 минут.

