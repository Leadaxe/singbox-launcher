# Sing-Box Launcher - Release Package

Этот пакет содержит все необходимые файлы для запуска **Sing-Box Launcher**.

## 📦 Содержимое пакета

### Исполняемые файлы
- `singbox-launcher.exe` (Windows) / `singbox-launcher` (macOS/Linux) - основной лаунчер
- `sing-box.exe` (Windows) / `sing-box` (macOS/Linux) - прокси-клиент (включен в релиз)

### Конфигурация
- `config.example.json` - пример конфигурации (скопируйте в `config.json` и настройте)

### Дополнительные файлы (Windows)
- `wintun.dll` - библиотека для TUN интерфейса (может быть включена в релиз)

## 🚀 Быстрый старт

### 1. Первый запуск

1. **Скопируйте `config.example.json` в `config.json`**:
   ```bash
   # Windows (в командной строке)
   copy bin\config.example.json bin\config.json
   
   # macOS/Linux
   cp bin/config.example.json bin/config.json
   ```

2. **Откройте `config.json`** и настройте:
   - Добавьте URL вашей подписки в блок `@ParserConfig`
   - Измените `secret` в секции `experimental.clash_api`
   - При необходимости настройте DNS и правила маршрутизации

3. **Запустите лаунчер**:
   - Windows: двойной клик на `singbox-launcher.exe`
   - macOS/Linux: `./singbox-launcher`

### 2. Если файлы отсутствуют

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
│   ├── config.json (создайте из config.example.json)
│   └── config.example.json
├── logs/ (создается автоматически)
│   ├── singbox-launcher.log
│   ├── sing-box.log
│   └── api.log
└── README.md (этот файл)
```

## ⚠️ Важная информация

### Included third-party binaries

This release includes prebuilt `sing-box.exe` (Windows) / `sing-box` (macOS/Linux) from the official project:

**Source:** [https://github.com/SagerNet/sing-box](https://github.com/SagerNet/sing-box)  
**License:** GPL-3.0

### Лицензии

- **Sing-Box Launcher**: MIT License
- **sing-box**: GPL-3.0
- **wintun.dll**: MIT License

Подробнее см. [LICENSE_NOTICE.md](../LICENSE_NOTICE.md) в корне проекта.

## 📖 Документация

- **Полная документация**: [README.md](../README.md)
- **Инструкции по сборке**: [BUILD_WINDOWS.md](../BUILD_WINDOWS.md)
- **Настройка парсера подписок**: [ParserConfig.md](../ParserConfig.md)

## 🔗 Ссылки

- **Репозиторий проекта**: [https://github.com/Leadaxe/singbox-launcher](https://github.com/Leadaxe/singbox-launcher)
- **Официальный sing-box**: [https://github.com/SagerNet/sing-box](https://github.com/SagerNet/sing-box)
- **Документация sing-box**: [https://sing-box.sagernet.org/](https://sing-box.sagernet.org/)

## 🆘 Поддержка

Если возникли проблемы:

1. Проверьте логи в папке `logs/`
2. Убедитесь, что все файлы на месте (используйте кнопку "Check Files" в лаунчере)
3. Откройте [Issue на GitHub](https://github.com/Leadaxe/singbox-launcher/issues)

---

**Примечание**: Этот проект не связан с официальным проектом sing-box. Это независимая разработка для удобного управления sing-box.
