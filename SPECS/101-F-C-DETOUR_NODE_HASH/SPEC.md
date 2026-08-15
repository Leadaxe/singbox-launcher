# SPEC 101-F-C — DETOUR НА КОНКРЕТНЫЙ УЗЕЛ ПО IDENTITY-ХЕШУ

## Цель

Снять ограничение SPEC 077 «target detour — только группа»: дать пользователю выбрать **один конкретный узел** (например, WARP MASQUE endpoint) как хоп для другого источника, без создания группы-обёртки. Кейс-мотиватор: Proton WG → через WARP, чтобы скрыть WG-handshake и/или сменить геометку egress.

## Проблема

Теги узлов генерятся в рантайме (`tag_prefix`/`tag_mask`/уникализация `-2`), поэтому ссылка по тегу на узел протухает при любом переименовании. SPEC 077 сознательно исключил одиночные узлы из пикера. Но у проекта уже есть стабильная адресация узла — identity-хеш (`config.NodeIdentityHash`, SPEC 094 D4, ключи `DisabledNodes`): переживает переименования провайдера, префиксы источника и перестановки.

## Решение (симметрично LxBox)

Ссылка на узел-хоп хранится **по хешу**, резолвится в финальный тег **на этапе генерации**, когда все источники загружены и теги финальны.

### Модель

- `state.Source` / `configtypes.ProxySource`: `DetourNodeHash string` + `DetourNodeLabel string` (display-снимок, в резолве не участвует). Взаимоисключимо с `DetourTag`; при ручной правке state, где выжили оба, хеш побеждает.
- Проброс: `ToProxySourceV4`, `sync_to_legacy`, `sync_to_connections`, `applyProxyEditToSource` — оба направления, оба типа источника.

### Генерация

- `resolveNodeHashDetours` (`core/config/outbound_generator.go`) — после загрузки всех источников, до `sanitizeNodeDetours` (штампованные теги попадают в существующую валидацию циклов/self):
  - индекс `hash → node` по `allNodes` (группы-узлы исключены: цепочка через селектор — это `DetourTag`);
  - найден → `node.Outbound["detour"] = target.Tag` на все ноды источника (eligibility как в SPEC 077: пропуск Xray Jump, пропуск wg+`listen_port`; сам хоп не штампуется);
  - **не найден → fail-closed**: ноды источника ДРОПАЮТСЯ из конфига с WarnLog. Отличие от fail-open у `DetourTag` намеренное: потерянный групповой target деградирует в прямой dial безопасно, а потерянный хоп-узел — нет (пользователь прятал трафик источника за хопом; молчаливый прямой dial раскрывает его).
- `LoadNodesFromSource`: при непустом `DetourNodeHash` теговый штамп SPEC 077 пропускается (хеш-путь штампует в генераторе).

### Хеш wireguard-узлов (сопутствующий фикс)

`NodeIdentityHash` шёл через `GenerateNodeJSON`, у которого нет ветки wireguard → все WG-узлы одного `server:port` схлопывались в один хеш (ломало и `DisabledNodes`). Теперь wireguard хешируется через `GenerateEndpointJSON` (полная карта endpoint). Существующие WG-хеши изменились один раз; старые disabled-отметки на WG отваливаются (они и были некорректны — одна отметка накрывала все WG-ноды сервера).

### UI

- `DetourOptionsWithNodes` (`ui/configurator/business/detour.go`): опции SPEC 077 + все server-источники (`state.Source.URI`), кроме собственных URI редактируемого источника. Отображение с маркером `» `; выбор возвращается через `map[display]DetourChoice{Tag|NodeHash+NodeLabel}`.
- Подписочные узлы в пикер **не** попадают (сотни нестабильных нод — для них группа).
- Висячий выбор (узел удалён/URI изменился) остаётся видимым по сохранённому label и очищаемым — зеркально dangling-тегу.

## Вне объёма

- Хоп из ноды подписки (только server-источники).
- Multi-hop редактор (достижимо каскадом detour у источников).
- UI-индикация fail-closed дропа (сейчас — WarnLog); follow-up при необходимости.
- Per-source preview не показывает хеш-detour (резолв глобальный, преview per-source); финальный конфиг корректен.

## Тесты

- `core/config/detour_node_hash_test.go` — штамп финального тега, fail-closed дроп (включая чистку `nodesBySource`), wg-chained + wg+listen_port пропуск.
- `core/config/node_hash_test.go` — WG-узлы с разными ключами не схлопываются.
- `ui/configurator/business/detour_test.go` — опции с узлами, исключение own-URI, selected по хешу, dangling.
