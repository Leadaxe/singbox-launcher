# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Configurator and Traffic Profiler for Local and for a machine can be open side by side: one Configurator window per target (`Configure` on a machine no longer just raises the local window), and both window titles name the target — `Config Wizard — Local` / `— <machine>`, `Traffic Profiler — <target>`, which stays in the title while recording.
- Fix: the tailnet state directory of a machine no longer leaks into the local `config.json` after the machine's Configurator was opened — the path is now applied only to that machine's build.
- The JSON tab of a Tailscale node has a read-only field below the editor with what the build adds to the stored body, real values included: `disable_taildrop`, `state_directory`, `taildrop_directory`. The tab used to claim it showed exactly what reaches `config.json`.
- Taildrop settings now belong to the node and travel with its backup. If Taildrop is allowed on a node, clipboard and links from your own devices are accepted and received files go to Downloads by default; both can be turned off on the Settings tab of the node. Per-device permissions are gone, old settings are not migrated.
- Double copy goes to one device chosen on the Settings tab of the node; nothing is sent until you choose it.
- A clipboard text or a link sent while acceptance is off no longer disappears silently: a notification names the device and where to turn it on.
- Renaming a Tailscale node no longer registers a new device: the state directory name is now a parameter of the node, and new nodes get a random one.

### Technical / Internal
-

## RU
### Основное
- Конфигуратор и Профайлер трафика для Local и для машины можно держать открытыми одновременно: у каждой цели своё окно Конфигуратора (`Configure` у машины больше не поднимает локальное окно), а заголовки обоих окон называют цель — «Мастер конфигурации — Локально» / «— <машина>», «Профайлер трафика — <цель>», и во время записи тоже.
- Исправлено: каталог состояния tailnet машины больше не попадает в локальный `config.json` после открытия Конфигуратора машины — путь действует только в сборке этой машины.
- На вкладке JSON узла Tailscale под редактором появилось поле только для чтения: что сборка добавит к хранимому телу, с настоящими значениями — `disable_taildrop`, `state_directory`, `taildrop_directory`. Раньше вкладка утверждала, что показывает ровно то, что попадёт в `config.json`.
- Настройки Taildrop теперь принадлежат узлу и переносятся с его бэкапом. Если на узле разрешён Taildrop, буфер и ссылки от своих устройств принимаются, а файлы сохраняются в Downloads; обе настройки можно выключить на вкладке Settings узла. Разрешений по устройствам больше нет, старые настройки не переносятся.
- Двойное копирование уходит на одно устройство, выбранное на вкладке Settings узла; пока устройство не выбрано, ничего не отправляется.
- Текст для буфера или ссылка при выключенном приёме больше не пропадают молча: уведомление называет устройство и место, где включить приём.
- Переименование узла Tailscale больше не регистрирует новое устройство: имя каталога состояния стало параметром узла, у новых узлов оно случайное.

### Техническое / Внутреннее
-
