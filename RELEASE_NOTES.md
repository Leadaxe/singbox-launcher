# Release Notes

## v0.6.2

<details>
<summary><b>🇷🇺 Русская версия / Russian Version</b></summary>

### ✨ Новые функции

#### Поддержка протокола SSH
- **Добавлена поддержка протокола SSH**: Теперь можно использовать SSH-туннели через формат `ssh://`
  - Поддерживается формат URI: `ssh://user:password@server:port#Tag`
  - Поддержка приватных ключей через параметр `private_key_path`
  - Поддержка host keys для проверки подлинности сервера
  - Поддержка passphrase для приватных ключей
  - Настраиваемая версия клиента SSH

#### macOS Installation Script
- **Добавлен скрипт установки для macOS**: Теперь можно установить приложение одной командой
  ```bash
  curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash -s -- 0.6.2
  ```
- Скрипт автоматически скачивает, распаковывает, устанавливает и запускает приложение
- Автоматически исправляет атрибуты quarantine и права доступа macOS
- Устанавливает приложение в `~/Applications/Singbox-Launcher/`

### 🔧 Улучшения

#### Hysteria2
- **Улучшена поддержка Hysteria2**: Добавлена поддержка короткого формата `hy2://` (официальная спецификация Hysteria 2)
- **Обновлена документация**: Добавлена подробная документация по формату URI для всех протоколов
  - Документированы все поддерживаемые параметры для каждого протокола
  - Добавлены примеры для каждого формата протокола
  - Обновлена документация Hysteria2 согласно официальной спецификации
  - Добавлен параметр `pinSHA256` для Hysteria2
  - Документирована поддержка формата multi-port

### 🐛 Исправления ошибок

#### Парсер подписок
- **Исправлено сохранение всех ProxySource entries**: Теперь все записи с `connections` (без `source`) сохраняются, а не только первая
- **Исправлено сохранение tag_prefix и tag_postfix**: Исправлена ошибка, при которой терялись `tag_prefix` и `tag_postfix` для ProxySource с `connections` при навигации между окнами визарда
- **Исправлено сохранение плейсхолдеров**: Исправлена ошибка, при которой плейсхолдеры вроде `{$tag}`, `{$scheme}` в `tag_prefix`/`tag_postfix`/`tag_mask` повреждались при обновлении конфигурации
- Добавлена логика сопоставления connections для обновления существующих записей вместо замены
- Сохраняются все существующие connections при отсутствии новых connections из ввода

</details>

<details>
<summary><b>🇬🇧 English Version</b></summary>

### ✨ New Features

#### SSH Protocol Support
- **Added SSH protocol support**: Now you can use SSH tunnels via `ssh://` format
  - Supports URI format: `ssh://user:password@server:port#Tag`
  - Support for private keys via `private_key_path` parameter
  - Support for host keys for server authentication
  - Support for passphrase for private keys
  - Configurable SSH client version

#### macOS Installation Script
- **Added installation script for macOS**: Now you can install the app with a single command
  ```bash
  curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash -s -- 0.6.2
  ```
- Script automatically downloads, extracts, installs, and launches the application
- Automatically fixes macOS quarantine attributes and permissions
- Installs to `~/Applications/Singbox-Launcher/`

### 🔧 Improvements

#### Hysteria2
- **Improved Hysteria2 support**: Added support for short format `hy2://` (official Hysteria 2 specification)
- **Updated documentation**: Added detailed URI format documentation for all protocols
  - Documented all supported parameters for each protocol
  - Added examples for each protocol format
  - Updated Hysteria2 documentation according to official specification
  - Added `pinSHA256` parameter for Hysteria2
  - Documented multi-port format support

### 🐛 Bug Fixes

#### Subscription Parser
- **Fixed preservation of all ProxySource entries**: All entries with `connections` (without `source`) are now preserved, not just the first one
- **Fixed preservation of tag_prefix and tag_postfix**: Fixed bug where `tag_prefix` and `tag_postfix` were lost for ProxySource with `connections` when navigating between wizard windows
- **Fixed placeholder preservation**: Fixed bug where placeholders like `{$tag}`, `{$scheme}` in `tag_prefix`/`tag_postfix`/`tag_mask` were corrupted during configuration update
- Added connection matching logic to update existing entries instead of replacing
- Preserve all existing connections when no new connections from input

</details>

---

См. [docs/release_notes/0-6-0.md](docs/release_notes/0-6-0.md) для подробностей о предыдущем релизе.

See [docs/release_notes/0-6-0.md](docs/release_notes/0-6-0.md) for details about the previous release.
