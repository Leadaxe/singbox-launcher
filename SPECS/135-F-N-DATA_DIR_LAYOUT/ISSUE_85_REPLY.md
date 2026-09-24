# Черновик ответа в issue #85 (публикует владелец)

Ниже текст комментария. Сроков и обещаний релиза нет намеренно.

---

Сделано в ветке `spec-135-impl` (SPEC 135), идёт приёмка.

**Что изменилось.** Пути разведены на три роли: папка программы только читается, данные и логи пишутся в свои каталоги. На Linux это `$XDG_DATA_HOME/singbox-launcher` (по умолчанию `~/.local/share/singbox-launcher`) и `$XDG_STATE_HOME/singbox-launcher/logs`; на macOS `~/Library/Application Support/singbox-launcher` и `~/Library/Logs/singbox-launcher`; на Windows `%LOCALAPPDATA%\singbox-launcher`. Read-only каталог бинаря (NixOS, Guix, Flatpak, snap) больше не роняет старт.

**Чем отличается от предложенного в issue.**
- Не `os.UserConfigDir()`: на Windows это roaming `%APPDATA%`, куда не место 70 МБ ядра и кэшам; на Linux это `~/.config`. Один корень данных без дробления на config/data/cache, чтобы «чистильщики» кэша не ломали `config.json`.
- Поставляемое (шаблон, локали, ядро) не копируется при первом запуске, а читается на месте по цепочке «данные → программа». Копия появляется только когда лаунчер сам что-то скачал.
- Существующие данные не «пересоздаются»: установки с данными рядом с бинарём продолжают работать там же (portable по маркеру `portable.txt` или по факту наличия данных рядом с пишущимся бинарём), а на macOS данные из бандла переносятся в `~/Library` при первом запуске.
- Вместо переименования функций путей — именованные типы `AppDir`/`DataDir`/`LogDir`: передать папку программы в пишущий вызов не даёт компилятор, плюс страж в CI.

**Переменные окружения:** `SINGBOX_LAUNCHER_DATA_DIR`, `SINGBOX_LAUNCHER_LOG_DIR` (для Flatpak-обёрток и пакетов), `SINGBOX_LAUNCHER_CORE` (явный путь к ядру). Порядок поиска ядра на Linux теперь: папка данных → рядом с программой → `PATH`; дистрибутивный `sing-box` больше не перекрывает скачанный форк.

**Посмотреть пути:** Settings → Storage → Copy paths, либо `singbox-launcher -paths` без окна. Удаление с очисткой: Settings → Storage → Remove all data… или `-purge-data` / `-purge-data -yes`.

Проверка на NixOS/Flatpak с вашей стороны будет полезна: интересны вывод `-paths` и первая строка лога `layout: …`.
