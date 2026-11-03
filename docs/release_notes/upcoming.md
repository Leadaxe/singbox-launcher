# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Configurator and Traffic Profiler for Local and for a machine can be open side by side: one Configurator window per target (`Configure` on a machine no longer just raises the local window), and both window titles name the target — `Config Wizard — Local` / `— <machine>`, `Traffic Profiler — <target>`, which stays in the title while recording.
- Fix: the tailnet state directory of a machine no longer leaks into the local `config.json` after the machine's Configurator was opened — the path is now applied only to that machine's build.
- The JSON tab of a Tailscale node says what the build adds to the stored body: `disable_taildrop: true`, `state_directory`, `taildrop_directory`. The tab used to claim it showed exactly what reaches `config.json`.

### Technical / Internal
-

## RU
### Основное
- Конфигуратор и Профайлер трафика для Local и для машины можно держать открытыми одновременно: у каждой цели своё окно Конфигуратора (`Configure` у машины больше не поднимает локальное окно), а заголовки обоих окон называют цель — «Мастер конфигурации — Локально» / «— <машина>», «Профайлер трафика — <цель>», и во время записи тоже.
- Исправлено: каталог состояния tailnet машины больше не попадает в локальный `config.json` после открытия Конфигуратора машины — путь действует только в сборке этой машины.
- Вкладка JSON узла Tailscale называет, что сборка добавит к хранимому телу: `disable_taildrop: true`, `state_directory`, `taildrop_directory`. Раньше вкладка утверждала, что показывает ровно то, что попадёт в `config.json`.

### Техническое / Внутреннее
-
