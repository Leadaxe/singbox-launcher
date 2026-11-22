# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **Remote Linux machine: "TUN auto redirect" no longer blocks Save** (issue #161). A Windows or macOS launcher checked the config of a remote Linux machine with its own core, which rejects `auto_redirect` as Linux-only. The check now ignores this host-only key; the machine still gets it.
- **TUN: "LAN interfaces" apply only in gateway mode** (issue #160). The `include_interface` list of the TUN inbound was written into the config even with **Gateway mode** off — a value left in the greyed-out field (for example an IP address instead of an interface name) broke TUN on the machine. The list is now emitted only when gateway mode is on.
- **Rule-sets (.srs) update themselves** (issue #157). A downloaded `.srs` file was never refreshed: block and bypass lists went stale for months while the rule showed ✔️. Rule-sets are now refreshed by the same hourly loop that updates subscriptions — a file older than its interval (a week by default) is downloaded again, checked, and swapped in; the running core picks it up without a restart. A missing file is downloaded when the config is built and before the core starts; if that fails, the rule is left out and the core still starts instead of failing with `FATAL initialize rule_set`. A remote machine gets fresh files on Deploy.
- **Rule-set settings: Local / Remote, interval, download route.** Settings has a new **Rule-sets** group, and the window of an SRS rule or of a preset with rule-sets has a **Rule-set updates** block that overrides it for that rule. **Remote** hands the download to the core (`type: remote` with `update_interval` and `download_detour`); note that the core will not start if its first download fails and it has no cached copy. The srs button tooltip shows when the files were last downloaded.

### Technical / Internal
- Contract 1.1.177: `rules[].update_interval_hours` is shared by both apps and valid for presets; new launcher-side rule fields `srs_mode`, `download_detour`; vars `srs_mode`, `srs_update_interval_hours`, `srs_download_detour` (SPEC 185).
-

## RU
### Основное
- **Удалённая Linux-машина: «TUN auto redirect» больше не ломает Save** (issue #161). Лаунчер на Windows и macOS проверял конфиг удалённой Linux-машины своим ядром, а оно отвергает `auto_redirect` как ключ только для Linux. Проверка теперь этот ключ хоста не учитывает; на машину он уходит как был.
- **TUN: «LAN interfaces» действуют только в режиме шлюза** (issue #160). Список `include_interface` у TUN-инбаунда попадал в конфиг и при выключенном **Gateway mode** — значение, оставшееся в погашенном поле (например IP-адрес вместо имени интерфейса), ломало TUN на машине. Теперь список пишется только при включённом режиме шлюза.
- **Наборы правил (.srs) обновляются сами** (issue #157). Скачанный `.srs` не обновлялся никогда: списки блокировок и обхода устаревали на месяцы, а у правила горело ✔️. Теперь наборы обновляет тот же ежечасный цикл, что и подписки: файл старше своего периода (по умолчанию неделя) скачивается заново, проверяется и подменяется; работающее ядро подхватывает его без перезапуска. Отсутствующий файл докачивается при сборке конфига и перед стартом ядра; если не вышло — правило не попадает в конфиг, а ядро стартует, вместо `FATAL initialize rule_set`. Удалённая машина получает свежие файлы при Deploy.
- **Настройки наборов: Local / Remote, период, маршрут загрузки.** В Settings — новая группа **Rule-sets**, а в окне правила SRS и пресета с наборами — блок **Rule-set updates**, который переопределяет её для этого правила. **Remote** отдаёт загрузку ядру (`type: remote` с `update_interval` и `download_detour`); учтите: ядро не запустится, если первая загрузка не удалась и сохранённой копии нет. Подсказка кнопки srs показывает, когда файлы скачаны в последний раз.

### Техническое / Внутреннее
- Контракт 1.1.177: `rules[].update_interval_hours` — общее поле обоих приложений, действует и у пресетов; новые поля правила стороны лаунчера `srs_mode`, `download_detour`; переменные `srs_mode`, `srs_update_interval_hours`, `srs_download_detour` (SPEC 185).
-
