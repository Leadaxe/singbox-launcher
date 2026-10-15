# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Homebrew on macOS:** `brew install --cask leadaxe/tap/singbox-launcher`. The Cask lives in [Leadaxe/homebrew-tap](https://github.com/Leadaxe/homebrew-tap) and follows stable releases on its own; the install script stays the recommended option.
- **macOS release ZIPs keep the executable bit.** The launcher binary was packed as `0644`, so an app extracted by hand did not start without `chmod +x`.

### Technical / Internal
-

## RU
### Основное
- **Homebrew на macOS:** `brew install --cask leadaxe/tap/singbox-launcher`. Cask живёт в [Leadaxe/homebrew-tap](https://github.com/Leadaxe/homebrew-tap) и сам следует за стабильными релизами; install-скрипт остаётся рекомендуемым способом.
- **В macOS-архивах релиза у бинаря есть бит исполнения.** Бинарь лаунчера упаковывался как `0644`, и распакованное вручную приложение не запускалось без `chmod +x`.

### Техническое / Внутреннее
-
