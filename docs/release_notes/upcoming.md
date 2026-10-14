# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- **macOS install script:** refuses to run on anything but macOS before touching the network, instead of failing later on a missing tool.
- **Tailscale node → Network:** the device list now looks like the server list — compact rows with the name and IPv4 on top, OS and IPv6 below, and the connection path (green bolt = direct, orange cloud = relay) on the third line. Devices with more than two addresses get a "+N" expander, and "Copy address" offers every address. The tab now asks the core for the live status every 3 s while open, so the path is current instead of the last streamed snapshot.

### Technical / Internal
-

## RU
### Основное
- **Скрипт установки на macOS:** отказывается работать не на macOS ещё до обращения к сети, а не падает позже на отсутствующей утилите.
- **Узел Tailscale → Network:** список устройств оформлен как список серверов — компактные строки: имя и IPv4 сверху, ОС и IPv6 ниже, путь подключения третьей строкой (зелёная молния — напрямую, оранжевое облако — через relay). У устройства с адресами сверх двух — раскрытие «+N», «Copy address» предлагает каждый адрес. Пока вкладка открыта, она раз в 3 с спрашивает у ядра живой статус, так что путь актуален, а не взят из последнего снимка потока.

### Техническое / Внутреннее
-
