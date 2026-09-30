# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- macOS install script 0.5: the core now goes to the data folder (`~/Library/Application Support/singbox-launcher/bin`), where v2.3+ looks first — before, an older core there shadowed the fresh one put into the bundle. When upgrading from a pre-2.3 install the old core is no longer carried over. At the end the script offers to create the root-owned core copy for TUN (system-wide VPN) with one `sudo`, or tells the command to run later; with the daemon service installed it runs `lxd --service=install`. Works under `curl | bash` and `sudo bash`.
- The one-time notice on the first start with an empty data folder is now a "Welcome" message: it names the data folder and suggests restoring from an LX Backup only if subscriptions from an earlier version are missing, instead of the alarming "No previous data found" on a clean install.

### Technical / Internal
- Contract 1.1.108: stale registry texts fixed (naive emit note, Tailscale refs and build-tag wording, `body.core` → 1.14.2-lx.11 after re-checking the fork's option structs, MASQUE `idle_timeout` semantics). The Tailscale "core unsupported" warning now names the `with_tailscale` build tag instead of a version.

## RU
### Основное
- Скрипт установки macOS 0.5: ядро кладётся в папку данных (`~/Library/Application Support/singbox-launcher/bin`), где v2.3+ ищет его в первую очередь, — раньше старое ядро там перекрывало свежее, положенное в бандл. При обновлении с установки до v2.3 старое ядро больше не переносится. В конце скрипт предлагает одной командой `sudo` создать защищённую копию ядра для TUN (системный VPN) или показывает команду на потом; при установленной службе демона выполняется `lxd --service=install`. Работает и под `curl | bash`, и под `sudo bash`.
- Одноразовое уведомление при первом запуске с пустой папкой данных стало приветствием «Welcome»: показывает папку данных и предлагает восстановиться из LX Backup, только если пропали подписки прежней версии, — вместо пугающего «No previous data found» на чистой установке.

### Техническое / Внутреннее
- Контракт 1.1.108: устаревшие тексты реестра (note эмита naive, ссылки и формулировка гейта Tailscale по тегу сборки, `body.core` → 1.14.2-lx.11 после сверки структур форка, семантика `idle_timeout` у MASQUE). Предупреждение «ядро не умеет Tailscale» теперь называет тег `with_tailscale`, а не версию.
