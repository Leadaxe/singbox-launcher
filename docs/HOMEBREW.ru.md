# Homebrew Cask (macOS)

**🌐 Язык**: [English](HOMEBREW.md) | Русский

Этот репозиторий также можно использовать как Homebrew tap. Поддерживается **универсальная** сборка для macOS 11+ (Apple Silicon и Intel). Отдельная устаревшая сборка для macOS Catalina в Cask не включена.

## Установка

Поскольку репозиторий называется `singbox-launcher`, а не `homebrew-singbox-launcher`, при добавлении tap нужно явно указать Git URL:

```bash
brew tap leadaxe/singbox-launcher https://github.com/Leadaxe/singbox-launcher.git
brew install --cask leadaxe/singbox-launcher/singbox-launcher
```

Установка Cask по полному имени подтверждает доверие только к этому Cask, а не ко всему tap. Двухкомпонентное имя `leadaxe/singbox-launcher` для Cask не подходит: Homebrew использует формат `user/tap/cask`.

После подключения tap можно использовать короткое имя, предварительно доверив Homebrew только этот Cask:

```bash
brew trust --cask leadaxe/singbox-launcher/singbox-launcher
brew install --cask singbox-launcher
```

Если Cask уже был установлен по полному имени, доверие сохраняется: повторно выполнять `brew trust` не требуется.

Для обновления используются обычные команды Homebrew:

```bash
brew update
brew upgrade --cask singbox-launcher
```

## Отличия от стандартного скрипта установки

- Cask устанавливает **только** `.app`. Он не загружает заранее отдельное ядро sing-box и шаблон конфигурации, а также не настраивает привилегированный компонент для TUN. Для первоначальной настройки или включения TUN приложению может понадобиться доступ в интернет и разрешение администратора. Если нужен первый запуск без дополнительных загрузок, по-прежнему рекомендуется `scripts/install-macos.sh`.
- Приложение пока не прошло notarization Apple для macOS. Gatekeeper может блокировать его запуск. Этот Cask **не** снимает атрибут quarantine и не отключает Gatekeeper. Прежде чем разрешать запуск, ознакомьтесь с [инструкциями по установке на macOS](../README.ru.md#macos) и используйте штатные настройки безопасности macOS, если решите доверять приложению.
- При переходе с версии до v2.3 рекомендуется заранее сделать **LX Backup**: в старых версиях пользовательские данные хранились внутри `.app`, и замена приложения может их удалить.

## Поддержка актуальности Cask

В `Casks/singbox-launcher.rb` зафиксированы последний стабильный релиз GitHub и его SHA-256. Скрипт `scripts/update-homebrew-cask.sh` считывает метаданные релиза (при необходимости — `checksums.txt`) и меняет **только** поля `version` и `sha256`.

[Workflow Update Homebrew Cask](../.github/workflows/update-homebrew-cask.yml) запускается после успешного `CI/CD` на событии `push`, раз в сутки для подстраховки или вручную. Он проверяет синтаксис Ruby, скачивает опубликованный macOS ZIP, сверяет SHA-256 и наличие `.app`, после чего автоматически коммитит в `main` **только Cask**. Для новых релизов ручное принятие PR не требуется. При ошибке проверки Cask не изменяется. Пользователю Homebrew по-прежнему нужно выполнить `brew update` и `brew upgrade --cask singbox-launcher`, чтобы установить обновление.

**Действие для сопровождающего репозиторий:** workflow запрашивает `contents: write` для `GITHUB_TOKEN`. Настройки репозитория и защита ветки должны разрешать GitHub Actions записывать изменения Cask в `main`. Если прямой push запрещён, workflow завершится ошибкой и не будет обходить защиту: понадобится изменить правила или вернуть схему обновления через PR. Разрешение GitHub Actions на создание PR **не требуется**.

Для ручной проверки новой версии используйте `brew audit --cask ./Casks/singbox-launcher.rb` и протестируйте установку на macOS. Сейчас upstream-архив содержит приложение в каталоге `artifacts-darwin/` (каталог артефакта CI релиза), поэтому поле `app` в Cask намеренно использует этот вложенный путь.
