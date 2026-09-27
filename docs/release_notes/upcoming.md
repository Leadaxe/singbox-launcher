# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Nodes no longer carry their own routing and DNS entries ("node sections"). A pasted config or node document now yields only the node; its `dns`/`route` are not kept, and the JSON tab says so. Existing node sections are dropped on load; a backup that carries them imports with a warning. Tailscale nodes get their tailnet route and MagicDNS from the new template preset "Tailscale networks": it serves every Tailscale node, subscription nodes included, and turns on by itself once for existing users. A node can opt out with "Skip presets" in its form. On the Rules and DNS tabs the preset row lists the nodes it serves, and its DNS servers appear in the DNS server list; the preset's JSON tab shows its entries for each of those nodes, or "No matching nodes." when there are none.
- A node you wrote by hand as sing-box JSON (your own server or a folder member) now goes to the core as written. The app still lists every rule that would normally fix it, marked "the app changed nothing"; only rules whose violation stops the whole config from starting are still applied. Subscription nodes are fixed as before, including the AmneziaWG MTU cap for sing-box JSON subscriptions.
- Editing the JSON of your own server or a folder member that came from a share link or a WireGuard config now turns it into a hand-written node: the JSON replaces the link, and the app asks before doing so. The JSON tab also accepts an array of nodes and keeps the first one; if there is more, it says the rest is not kept (the same message as for a node document). A node document with several nodes is accepted too: the first node that is not a service outbound or a group is kept, and the tab says the rest is not kept.

### Technical / Internal
- Contract 1.1.87: registry attribute `core_rejects`, warning flag `applied`, bare-body node source (SPEC 146).
- Contract 1.1.88: an invalid VLESS `flow` is removed even on a hand-written node (the core refuses to start with it); new corpus section `node_edit`; a source test keeps build steps that change a node body behind the single decision point.
- Contract 1.1.89: node document with several nodes in the node JSON tab keeps the first node; import still creates one record per node.
- Contract 1.1.90: the DNS rule of the "Tailscale networks" preset names the node's DNS server in `preferred_by`, not the node itself; the old value kept the core from starting.
- Contract 1.1.91: 29 more registry rules are marked `core_rejects` (the core refuses to start with such a value), so they are applied to hand-written nodes too: VLESS `encryption`, Shadowsocks `method`, WireGuard keys and peer port, REALITY keys, xhttp placements and others.

## RU
### Основное
- Узел больше не несёт собственных правил маршрутов и DNS («секции узла»). Из вставленного конфига или документа узла берётся только узел; `dns`/`route` не сохраняются, вкладка JSON об этом сообщает. Секции существующих узлов снимаются при загрузке; бэкап с ними импортируется с предупреждением. Маршрут в tailnet и MagicDNS узлы Tailscale получают из нового пресета шаблона «Tailscale networks»: он обслуживает каждый узел Tailscale, включая узлы подписки, и у существующих пользователей включается сам, один раз. Узел можно исключить переключателем «Skip presets» в его форме. На вкладках Rules и DNS строка пресета перечисляет обслуживаемые узлы, а его DNS-серверы видны в списке DNS-серверов; вкладка JSON пресета показывает его записи для каждого из этих узлов или «Нет подходящих узлов.», если их нет.
- Узел, написанный вручную в форме sing-box JSON (свой сервер или член папки), уходит в ядро как написан. Приложение по-прежнему показывает каждое правило, которое обычно его поправило бы, с пометкой «приложение ничего не изменило»; применяются только правила, нарушение которых не даёт стартовать всему конфигу. Узлы подписок правятся как раньше, включая потолок MTU AmneziaWG у подписок в sing-box JSON.
- Правка JSON своего сервера или члена папки, пришедшего из ссылки или конфига WireGuard, делает узел написанным вручную: JSON заменяет ссылку, и приложение спрашивает об этом заранее. Вкладка JSON принимает и массив узлов и сохраняет первый; если есть ещё, сообщает, что остальное не сохранено (то же сообщение, что у документа узла). Принимается и документ с несколькими узлами: сохраняется первый узел, который не служебный и не группа, и вкладка сообщает, что остальное не сохранено.

### Техническое / Внутреннее
- Контракт 1.1.87: атрибут реестра `core_rejects`, признак предупреждения `applied`, источник узла — голое тело (SPEC 146).
- Контракт 1.1.88: недопустимый `flow` VLESS снимается и у узла, написанного вручную (ядро с ним не стартует); новый раздел корпуса `node_edit`; тест по исходникам держит шаги сборки, меняющие тело узла, за единой точкой решения.
- Контракт 1.1.89: документ с несколькими узлами во вкладке JSON узла сохраняет первый узел; импорт по-прежнему создаёт запись на каждый узел.
- Контракт 1.1.90: DNS-правило пресета «Tailscale networks» указывает в `preferred_by` DNS-сервер узла, а не сам узел; прежнее значение не давало ядру стартовать.
- Контракт 1.1.91: ещё 29 правил реестра помечены `core_rejects` (с таким значением ядро не стартует) и применяются и к узлу, написанному вручную: `encryption` VLESS, `method` Shadowsocks, ключи и порт пира WireGuard, ключи REALITY, placement-поля xhttp и другие.
