# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **AmneziaWG 2.0 header ranges (`H1-H4 = lo-hi`) are now imported and passed to the core.** Real AWG 2.0 exports randomize magic headers as ranges; previously the parser silently dropped all four fields — the node imported without them and the handshake never matched. Ranges now flow 1:1 into the endpoint (`"h1": "N-M"`), and the core picks a fresh in-range value on every handshake. Applies to `awg://` links, `vpn://` profiles and pasted `.conf` text. **Requires core `1.13.13-lx.6`** (the pin is bumped) — update via Core Dashboard → Download/Reinstall. (SPEC 073.2)

### Technical / Internal
- `parseAWGHeaderRange` in `node_parser_wireguard.go`: plain uint32 → int64 (JSON number) as before; `lo-hi` on `h1`–`h4` → normalized range string (reversed bounds reordered); ranges on other numeric AWG fields and invalid values keep the skip-with-debug-log policy. Share-URI emits the range unchanged (`awgNumericString` already passes strings through) — lossless round-trip. Tests: `awg_range_test.go`.
- `chore(core)`: `RequiredCoreVersion` 1.13.13-lx.5 → **1.13.13-lx.6** (native `"hN": "lo-hi"` support; earlier cores reject the string form).

## RU
### Основное
- **Диапазоны заголовков AmneziaWG 2.0 (`H1-H4 = lo-hi`) теперь импортируются и пробрасываются в ядро.** Реальные экспорты AWG 2.0 рандомизируют magic-заголовки диапазонами; раньше парсер молча выбрасывал все четыре поля — узел импортировался без них, и handshake не сходился. Теперь диапазон идёт в endpoint один в один (`"h1": "N-M"`), и ядро само выбирает свежее значение внутри диапазона на каждый handshake. Работает для `awg://`-ссылок, `vpn://`-профилей и вставленного `.conf`-текста. **Требуется ядро `1.13.13-lx.6`** (пин забампен) — обновите через Core Dashboard → Download/Reinstall. (SPEC 073.2)

### Техническое / Внутреннее
- `parseAWGHeaderRange` в `node_parser_wireguard.go`: одиночный uint32 → int64 (JSON-число) как раньше; `lo-hi` на `h1`–`h4` → нормализованная строка диапазона (перевёрнутые границы упорядочиваются); диапазоны на прочих числовых AWG-полях и невалидные значения — прежняя политика skip + debug-log. Share-URI отдаёт диапазон без изменений (`awgNumericString` уже пропускал строки) — round-trip без потерь. Тесты: `awg_range_test.go`.
- `chore(core)`: `RequiredCoreVersion` 1.13.13-lx.5 → **1.13.13-lx.6** (нативная поддержка `"hN": "lo-hi"`; старые ядра отвергают строковую форму).
