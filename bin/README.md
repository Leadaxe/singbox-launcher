# Sing-Box Launcher - Release Package

Этот пакет содержит все необходимые файлы для запуска **Sing-Box Launcher**.

## 📦 Содержимое пакета

### Исполняемые файлы
- `singbox-launcher.exe` (Windows) / `singbox-launcher` (macOS/Linux) - основной лаунчер
- `sing-box.exe` (Windows) / `sing-box` (macOS/Linux) - прокси-клиент (включен в релиз)

### Конфигурация
- `wizard_template.json` — единый шаблон для Config Wizard (лежит в репозитории в `bin/`; при отсутствии локально может подтягиваться через вкладку **Core**)
- `config.example.json` — пример конфигурации (для справки)

Устаревшие имена **`config_template.json`** / **`config_template_macos.json`** из старых версий и релизных заметок не используются текущим кодом; см. `docs/release_notes/0-8-5.md`.

### Дополнительные файлы (Windows)
- `wintun.dll` - библиотека для TUN интерфейса (может быть включена в релиз)

## 🚀 Быстрый старт

### 1. Первый запуск

1. **Запустите лаунчер**:
   - Windows: двойной клик на `singbox-launcher.exe`
   - macOS/Linux: `./singbox-launcher`

2. **Откройте Config Wizard**:
   - Нажмите кнопку "Config Wizard" в главном окне
   - Или выберите "Config Wizard" из меню

3. **Настройте конфигурацию через визард**:
   - **Sources**: URL подписки или прямые ссылки на прокси
   - **Rules**: правила маршрута
   - **Settings**: переменные шаблона (`vars` в `wizard_template.json` — лог, Clash API, TUN на macOS и т.д.)
   - **Files**: сборка `config.json` в файл; просмотр — по кнопке, в отдельном окне
   - **Save** для сохранения

4. **Запустите VPN**:
   - Вернитесь в главное окно
   - Нажмите кнопку "Start" для запуска sing-box

### 2. Ручная настройка (не рекомендуется)

Если вы хотите настроить конфигурацию вручную:

1. **Скопируйте `config.example.json` в `config.json`**:
   ```bash
   # Windows (в командной строке)
   copy bin\config.example.json bin\config.json
   
   # macOS/Linux
   cp bin/config.example.json bin/config.json
   ```

2. **Отредактируйте `config.json`** вручную:
   - Добавьте URL вашей подписки
   - Настройте DNS и правила маршрутизации
   - Измените `secret` в секции `experimental.clash_api`

3. **Сохраните файл** - sing-box автоматически перезагрузит конфигурацию

### 3. Если файлы отсутствуют

Если в релизе нет `sing-box` или `wintun.dll`, скачайте их:

- **sing-box**: [https://github.com/SagerNet/sing-box/releases](https://github.com/SagerNet/sing-box/releases)
- **wintun.dll** (только Windows): [https://www.wintun.net/](https://www.wintun.net/)

Поместите скачанные файлы в папку `bin/`.

## 📋 Структура папок

```
singbox-launcher/
├── bin/
│   ├── singbox-launcher.exe (или singbox-launcher)
│   ├── sing-box.exe (или sing-box)
│   ├── wintun.dll (только Windows)
│   ├── wizard_template.json (шаблон для Config Wizard)
│   ├── config.json (создается Config Wizard или вручную)
│   └── config.example.json (пример конфигурации)
├── logs/ (создается автоматически)
│   ├── singbox-launcher.log
│   ├── sing-box.log
│   └── api.log
└── README.md (этот файл)
```

## 🧙 Config Wizard

**Config Wizard** - это визуальный интерфейс для настройки конфигурации без редактирования JSON.

### Основные возможности:
- 📝 Ввод URL подписок и прямых ссылок на прокси
- ✅ Выбор правил маршрутизации (блокировка рекламы, российские домены, игры и т.д.)
- 👁️ Предпросмотр сгенерированного `config.json`
- 💾 Сохранение и загрузка состояний конфигурации
- 🔄 Автоматический парсинг подписок

### Файл шаблона:
- **`wizard_template.json`** — шаблон визарда (секции `parser_config`, `config`, `selectable_rules`, `params`, `vars`, …)

Подробнее о создании собственных шаблонов: [docs/WIZARD_TEMPLATE.ru.md](../docs/WIZARD_TEMPLATE.ru.md)

## ⚠️ Важная информация

### Included third-party binaries

This release includes prebuilt `sing-box.exe` (Windows) / `sing-box` (macOS/Linux) from the official project:

**Source:** [https://github.com/SagerNet/sing-box](https://github.com/SagerNet/sing-box)  
**License:** GPL-3.0

### Лицензии

- **Sing-Box Launcher**: GPL-3.0 (dual licensing — [LICENSING.md](../LICENSING.md))
- **sing-box**: GPL-3.0
- **wintun.dll**: MIT License

Подробнее см. [docs/LICENSE_NOTICE.md](../docs/LICENSE_NOTICE.md) в папке docs.

## 📖 Документация

- **Полная документация**: [README.md](../README.md)
- **Русская версия**: [README.ru.md](../README.ru.md)
- **Инструкции по сборке**: [docs/BUILD_WINDOWS.md](../docs/BUILD_WINDOWS.md)
- **Создание шаблонов для Config Wizard**: [docs/WIZARD_TEMPLATE.ru.md](../docs/WIZARD_TEMPLATE.ru.md)
- **Настройка парсера подписок**: [docs/ParserConfig.md](../docs/ParserConfig.md)

## 🔗 Ссылки

- **Репозиторий проекта**: [https://github.com/Leadaxe/singbox-launcher](https://github.com/Leadaxe/singbox-launcher)
- **Официальный sing-box**: [https://github.com/SagerNet/sing-box](https://github.com/SagerNet/sing-box)
- **Документация sing-box**: [https://sing-box.sagernet.org/](https://sing-box.sagernet.org/)

## 🆘 Поддержка

Если возникли проблемы:

1. Проверьте логи в папке `logs/`
2. Убедитесь, что все файлы на месте (используйте кнопку "Check Files" в лаунчере)
3. Попробуйте пересоздать конфигурацию через Config Wizard
4. Откройте [Issue на GitHub](https://github.com/Leadaxe/singbox-launcher/issues)

---

**Примечание**: Этот проект не связан с официальным проектом sing-box. Это независимая разработка для удобного управления sing-box.
