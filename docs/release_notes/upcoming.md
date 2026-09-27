# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Nodes no longer carry their own routing and DNS entries ("node sections"). A pasted config or node document now yields only the node; its `dns`/`route` are not kept, and the JSON tab says so. Existing node sections are dropped on load; a backup that carries them imports with a warning. A Tailscale node is added without the MagicDNS server and tailnet route until the template preset for Tailscale lands.

### Technical / Internal
-

## RU
### Основное
- Узел больше не несёт собственных правил маршрутов и DNS («секции узла»). Из вставленного конфига или документа узла берётся только узел; `dns`/`route` не сохраняются, вкладка JSON об этом сообщает. Секции существующих узлов снимаются при загрузке; бэкап с ними импортируется с предупреждением. Узел Tailscale добавляется без сервера MagicDNS и маршрута в tailnet до появления пресета шаблона для Tailscale.

### Техническое / Внутреннее
-
