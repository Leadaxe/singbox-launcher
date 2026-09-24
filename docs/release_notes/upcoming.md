# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
-

### Technical / Internal
- **Ready for core `lx.13`'s root `lx` block (SPEC 138).** Core lx.13 moves the WireGuard idle-suspend keys from `route.lx_idle_*` to `lx.wg.*`. The launcher never wrote them (desktop core builds lack idle-suspend and refuse them at start), so the pin bump to lx.13 needs no config or state migration. A root `lx` block in the template (e.g. `lx.masque.idle_timeout`) is passed through to `config.json` as-is; a test now pins that.

## RU
### Основное
-

### Техническое / Внутреннее
- **Готовность к корневому блоку `lx` ядра `lx.13` (SPEC 138).** Ядро lx.13 переносит ключи сна WireGuard из `route.lx_idle_*` в `lx.wg.*`. Лаунчер их никогда не писал: десктопные сборки ядра собраны без сна WG и отвергают эти ключи на старте. Поэтому бамп пина на lx.13 не требует миграции конфига и состояния. Корневой блок `lx` из шаблона (например, `lx.masque.idle_timeout`) уходит в `config.json` как есть, теперь это закреплено тестом.
