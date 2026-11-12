# IMPLEMENTATION REPORT 180

**Итог:** поле записи `own_routes` и пресет `tunnel-routes` реализованы, контракт 1.1.161. Интерфейс владельцу не показан, вживую не проверено, полный CI не запускался.

## Что сделано

- **Модель.** `configtypes.NodeRecordFlags.OwnRoutes` (`own_routes`), в `Any()` и `MergeTrue()`; состояние, бэкап и кэш сборки несут поле через встроенную структуру, отдельного кода нет. `template.PresetNodeRecordFields` — `@node.own_routes`.
- **Шаблон.** `bin/wizard_template.json`: пресет `tunnel-routes` (943). `state.LateDefaultPresetIDs` дополнен — у существующих установок правило и его DNS-слот появляются один раз, без поднятого поля в конфиг ничего не попадает.
- **Интерфейс.** `tunnelRoutesBlock` в `ui/configurator/tabs/source_tunnel_access.go`, вызывается на вкладке Settings окна узла перед блоком входящих соединений. Предупреждение о маршруте по умолчанию — по `peers[].allowed_ips` тела WireGuard.
- **Контракт.** `TEMPLATE_LANG.md` §6.5.3, §6.6, §6.7.5, §9; `BACKUP.md`; схема; `registry/presets.json`; корпус `tunnel_routes_*` (7 кейсов); `TASKS_LXBOX.md` §158.

## Проверки

- `go build ./...` — собирается.
- `go test ./core/build/ -run 'TestContractCorpusNodeLoops|TestLatePresetDNSRuleTakesAxisPosition|TestTunnelRoutesDNSRuleBeforeFakeIP' -count=1` — зелёные.
- `TestLatePresetDNSRuleTakesAxisPosition` поправлен: поздних дефолтов теперь три, в `dns.rules` состояния четыре слота.

## Наблюдения по ядру

Записаны в SPEC §3 и в `TASKS_LXBOX.md` §158 п. 4–5: поведение DNS-сервера без присланного DNS, один DNS-сервер на endpoint, DNS туннеля работает только для объявленных сервером доменов.

## Не сделано

- Живая проверка и показ интерфейса (T8), полный CI (T9).
- Проверка конфликта со своим DNS-сервером типа `openvpn` / `openconnect` на тот же узел (SPEC §4).
