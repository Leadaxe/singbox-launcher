# Homebrew Cask (macOS)

**🌐 Язык**: [English](HOMEBREW.md) | Русский

Этот репозиторий также можно использовать как Homebrew tap. Поддерживается **универсальная** сборка для macOS 11+ (Apple Silicon и Intel). Отдельная устаревшая сборка для macOS Catalina в Cask не включена.

## Установка

Поскольку репозиторий называется `singbox-launcher`, а не `homebrew-singbox-launcher`, при добавлении tap нужно явно указать Git URL:

```bash
brew tap leadaxe/singbox-launcher https://github.com/Leadaxe/singbox-launcher.git
brew install --cask leadaxe/singbox-launcher/singbox-launcher
```

В версиях Homebrew с проверкой доверия к сторонним tap установка Cask по полному имени должна запрашивать доверие только для этого Cask. Если Homebrew запросит подтверждение, сначала изучите содержимое Cask.

Для обновления используются обычные команды Homebrew:

```bash
brew update
brew upgrade --cask leadaxe/singbox-launcher/singbox-launcher
```

## Отличия от стандартного скрипта установки

- Cask устанавливает **только** `.app`. Он не загружает заранее отдельное ядро sing-box и шаблон конфигурации, а также не настраивает привилегированный компонент для TUN. Для первоначальной настройки или включения TUN приложению может понадобиться доступ в интернет и разрешение администратора. Если нужен первый запуск без дополнительных загрузок, по-прежнему рекомендуется `scripts/install-macos.sh`.
- Приложение пока не прошло notarization Apple для macOS. Gatekeeper может блокировать его запуск. Этот Cask **не** снимает атрибут quarantine и не отключает Gatekeeper. Прежде чем разрешать запуск, ознакомьтесь с [инструкциями по установке на macOS](../README.ru.md#macos) и используйте штатные настройки безопасности macOS, если решите доверять приложению.
- При переходе с версии до v2.3 рекомендуется заранее сделать **LX Backup**: в старых версиях пользовательские данные хранились внутри `.app`, и замена приложения может их удалить.

## Поддержка актуальности Cask

В `Casks/singbox-launcher.rb` зафиксированы версия последнего стабильного релиза и опубликованная контрольная сумма SHA-256. Скрипт `scripts/update-homebrew-cask.sh` считывает метаданные GitHub Release (при необходимости использует `checksums.txt`) и меняет **только** версию и SHA-256. [Workflow Update Homebrew Cask](../.github/workflows/update-homebrew-cask.yml) запускается после завершения существующего workflow `CI/CD`, раз в сутки в качестве резервной проверки, а также вручную. Он создаёт или обновляет **черновик PR (Draft)** для проверки изменений.

**Действие для сопровождающего репозиторий:** в GitHub необходимо включить **Settings → Actions → General → Workflow permissions → Allow GitHub Actions to create and approve pull requests**. Иначе создание автоматических PR завершится ошибкой. Workflow запрашивает разрешения `contents: write` и `pull-requests: write`.

Для ручной проверки новой версии используйте `brew audit --cask ./Casks/singbox-launcher.rb` и протестируйте установку на macOS. Сейчас upstream-архив содержит приложение в каталоге `artifacts-darwin/` (каталог артефакта CI релиза), поэтому поле `app` в Cask намеренно использует этот вложенный путь.
