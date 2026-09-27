# SPEC 148 — Tailscale: псевдо-направление NETWORKS и вкладка Network

Статус: **C** (решение владельца 2026-09-27, реализовано 2026-09-27).
Тип: Feature. Контракт: не затрагивается (форма конфига и реестр не меняются,
`contract/` 1.1.97 без правок).

Норма поведения — спеки LxBox:
`LxBox/docs/spec/tasks/579-networks-pseudo-direction.md` (NETWORKS) и
`LxBox/docs/spec/tasks/581-tailscale-network-tab.md` (вкладка Network).
Здесь — только отличия и устройство лаунчера. Спека уточняет SPEC 130: секция
«Tailscale» вкладки Details заменена вкладкой Network, запрет на
`SetTailscaleExitNode` из UI (SPEC 130 §3) снят решением владельца.

## 1. Опись (до изменений)

- **Экран выбора узла.** Вкладка Servers — `ui/clash_api_tab.go`,
  `CreateProxyListPanel`. Перечень направлений — `groupSelect`
  (`widget.Select`, значение пункта = тег `selector`-группы) из
  `config.GetSelectorGroupsFromConfig` (Local) или gRPC (Remote), перечитывание
  — `selectorReloader` (`clash_api_tab_selector_reload.go`). Узлы группы —
  `transport.GroupProxies` → `ac.SetProxiesList` (общий список: трей, пинги);
  видимый срез — `proxiesForListView`. Строка — `createItem`/`updateItem`
  (имя, подзаголовок, кликабельная задержка, кнопка ▶), меню —
  `serversProxyContextMenu` (`clash_api_tab_render.go`).
- **Окно узла.** `showNodeInfoWindow` (`ui/servers_node_info.go`): вкладки
  Details и Outbound JSON; у tailscale была секция «Tailscale» в Details
  (`servers_node_info_tailscale.go`, SPEC 130).
- **Демон.** `DaemonBackend.grpcClient()` → `daemonpb.StartedServiceClient`
  (`core/backend_daemon.go`); стрим `SubscribeTailscaleStatus` уже держит
  `superviseTailscale` с коннекта (SPEC 130), кеш —
  `services.TailscaleStatusCache`, диспетчер — `core/tailscale_status.go`.
  Legacy-движок (без демона) статуса tailnet не получает.
- **Узел Tailscale без выхода** (`endpoints[]`, без `exit_node`) в группы не
  входит (`IsExitCapable`) и на вкладке Servers не виден; виден только в окне
  «⇄» (список endpoint'ов, слово состояния) — SPEC 130.

## 2. NETWORKS

- Отбор — `config.NetworksNodeTags` (`core/config/config_loader.go`):
  запись в `endpoints[]`, тип `tailscale`, `registry.ExitCapable` ложно (при
  нечитаемом реестре узел считается выходом, как в сборке).
- Показ: только панель Local, ядро работает (`RunningState.IsRunning`), узлы
  есть. Пункт `NETWORKS` последним в `groupSelect`. Опрос —
  `watchNetworksDirection` (`ui/networks_direction.go`): раз в секунду,
  config.json перечитывается при смене размера или mtime; тот же тик
  перерисовывает состояния.
- Только вид: `networksOpen` в панели; `selectedGroup`, выбор группы в
  `APIService`, общий список прокси и трей не меняются. Пункты Select —
  строки, поэтому при направлении пользователя с тегом `NETWORKS` к пункту
  добавляется неразрывный пробел (`networksOptionLabel`).
- Строка: тег, подзаголовок как у узла, на месте задержки состояние
  (`networksRowState`: `starting` / `running` зелёным / `sign-in needed`,
  `stopped` цветом предупреждения / иначе `StateText` ядра); кнопки ▶ и
  замера нет; нажатие и «Node info…» открывают окно узла; в меню нет
  «Re-test now». Кнопки сортировки, фильтров, копирования ссылок, замера и
  его настроек скрыты; `pingProxy` и «ping all» при NETWORKS ничего не делают.
  Строка статуса: `NETWORKS: N nodes` — счётчик узлов NETWORKS.
- Узлы пропали (VPN выключен, узел удалён или стал выходом) — снова виден
  выбранный настоящий список.

## 3. Вкладка Network

Файл `ui/servers_node_network_tab.go`. Видна у узла типа `tailscale`, когда
источник отдаёт статус tailnet (`ac.TailscaleAvailable`), между Details и
Outbound JSON. Перерисовка — не чаще раза в секунду и только при новом
снимке или смене записанного `exit_node`; обновление останавливается, когда
окно закрыто.

- Без данных (`tailscaleNetworkViewFor`): VPN выключен — «Start VPN to see
  the network.»; стрим не дал снимка — индикатор ожидания; снимок есть, узла
  в нём нет — «The node is not in the running config.».
- **Status**: состояние (слова §2), имя сети, «signed in with a key», Sign in
  (NeedsLogin и `AuthURL`), Log out (Running, после подтверждения, текст §3
  LxBox §581) → `TailscaleLogout`.
- **This device**: имя, MagicDNS-имя, адреса (нажатие копирует), срок ключа
  (`KeyExpiry`, добавлен в `services.TailscalePeer`).
- **Exit node**: Select «None» + устройства с `ExitNodeOption`; выбор →
  `SetTailscaleExitNode(tag, StableID)` на ходу. Записанное — `exit_node`
  тела в собранном конфиге; сравнение — `services.CompareExitNode` (совпадение
  по адресу, имени или DNS-имени: ядро принимает IP или имя). Различие — знак
  предупреждения, один из трёх текстов §5 LxBox §581 и Save choice.
- **Save choice** — `ac.SaveTailscaleExitNode` (`core/tailscale_exit_save.go`):
  только свой узел (сервер в корне или член папки, не подписка, только
  Local); у узла с источником-телом правится поле в тексте источника и тело
  материализуется из него (`MaterializeServerNode`, как Apply вкладки JSON),
  у узла из ссылки или INI — поле в теле с сохранением происхождения
  (`MaterializeEditedBody`). Порядок ключей сохраняется
  (`SetBodyTopLevelString`); пишется первый адрес tailnet устройства; «None»
  убирает поле. Затем `MarkConfigStale` и `RebuildConfigIfDirty`.
- **Devices**: сначала в сети, затем остальные, внутри по имени
  (`services.SortDevices`); группы по владельцам при числе групп больше
  одного (заголовок `DisplayName`, иначе `LoginName`); отметки online / last
  seen, key expired, shared (`ShareeNode`), exit node. Меню «…»: Copy name,
  Copy address, Ping. Трафика по устройствам нет.
- **Ping**: `StartTailscalePing` в диалоге; строка ответа — задержка, direct
  с адресом или relay с регионом; до закрытия или пяти ответов.
- **Журнал**: имена, адреса, имя сети, владельцы и ссылка входа не пишутся;
  ошибки команд логируются только с тегом узла. В Debug API лаунчера данных
  tailnet нет.

## 4. Проверка через внешний адрес (LxBox §581 раздел 8)

Узел Tailscale Local с состоянием Running без действующего exit node
(`tailscaleHasNoExit`): в строке списка вместо задержки «no exit», клик замер
не запускает; в Details вместо «Last delay» — «This node has no exit. Check
devices on the Network tab.». С действующим выходом — как у обычного узла.

## 5. Проверено по исходникам ядра (sing-box-lx, `protocol/tailscale/endpoint.go`)

1. Старт без `exit_node` в конфиге: `editPrefs` ставит маску `ExitNodeIPSet`
   с пустым адресом, `ExitNodeID` маской не затрагивается. Выбор на ходу
   (`SetTailscaleExitNode` пишет `ExitNodeID` в prefs каталога состояния),
   по коду, переживает перезапуск; поведение `LocalBackend` на стенде не
   проверялось.
2. `exit_node` в конфиге — `MaskedPrefs.SetExitNodeIP(value, status)`: адрес
   или имя устройства, не `StableID`. В тело пишется адрес.

## 6. Тесты

- `core/services/tailscale_network_test.go`: разбор полей вкладки, четыре
  строки таблицы Exit node, значение для записи, порядок устройств, список
  выходов.
- `core/config/networks_direction_test.go`: отбор NETWORKS (с `exit_node`,
  без, пустой `exit_node`, WireGuard, запись в `outbounds[]`, без тега,
  пустой итог).
- `core/tailscale_exit_save_test.go`: поле появляется, меняется, убирается;
  свой узел в корне и в папке, узел подписки не свой.
- `ui/networks_direction_test.go`: строки таблицы состояния, пункт при
  совпадении тега, вид вкладки без данных, три текста, отметки устройства.

## 7. Не сделано

- Удалённая машина: NETWORKS и команды tailnet (выбор выхода, Log out, Ping)
  только у локального демона; у Remote вкладка показывает состояние без
  команд, Save choice скрыта.
- Legacy-движок (Windows 386 и прочие без демона): вкладки Network и
  NETWORKS нет — статуса tailnet там нет (SPEC 130 §3).
- Список устройств строится целиком, не лениво; перерисовка только по новому
  снимку.
