# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Nodes no longer carry their own routing and DNS entries ("node sections"). A pasted config or node document now yields only the node; its `dns`/`route` are not kept, and the JSON tab says so. Existing node sections are dropped on load; a backup that carries them imports with a warning. Tailscale nodes get their tailnet route and MagicDNS from the new template preset "Tailscale networks": it serves every Tailscale node, subscription nodes included, and turns on by itself once for existing users. A node can opt out with "Skip presets" in its form. On the Rules and DNS tabs the preset row lists the nodes it serves, and its DNS servers appear in the DNS server list.

### Technical / Internal
-

## RU
### Основное
- Узел больше не несёт собственных правил маршрутов и DNS («секции узла»). Из вставленного конфига или документа узла берётся только узел; `dns`/`route` не сохраняются, вкладка JSON об этом сообщает. Секции существующих узлов снимаются при загрузке; бэкап с ними импортируется с предупреждением. Маршрут в tailnet и MagicDNS узлы Tailscale получают из нового пресета шаблона «Tailscale networks»: он обслуживает каждый узел Tailscale, включая узлы подписки, и у существующих пользователей включается сам, один раз. Узел можно исключить переключателем «Skip presets» в его форме. На вкладках Rules и DNS строка пресета перечисляет обслуживаемые узлы, а его DNS-серверы видны в списке DNS-серверов.

### Техническое / Внутреннее
-
