# SPEC 123 · AmneziaWG 3.0/3.1: разбор, эмиссия, гейт ядра

Статус: N (в работе). Ветка develop. Ядро: sing-box-lx `v1.14.0-lx.33` (пин; поля AWG3 есть с lx.32, lx.33 чинит приём data-пакетов при random_trailers)
(релиз собран 2026-09-05, ассеты и SHA256SUMS на месте).

Справочник полей ядра: `/Users/macbook/projects/sing-box-lx/docs-lx/lx-protocols-transports.ru.md`
§2.1, §2.6, §2.7, §2.9, §2.10. Спека ядра: `/Users/macbook/projects/sing-box-lx/SPECS/TASKS/080-AWG3_HEADER_PROTECTION_TIMINGS/SPEC.md`.

Общие таблицы полей и предикат уже лежат в
`core/config/subscription/awg3.go` (`awg3RangeFields`, `awg3BoolFields`,
`awg3HeaderKeyField`, `AWG3RootKeys()`, `HasAWG3Fields()`). Оба потока
работ обязаны опираться на них, а не на свои списки.

## 1. Проблема

Amnezia экспортирует AWG 3.x сервер контейнером `amnezia-awg2` с
`awg.protocol_version: "3.1"`. В его `.conf` (`awg.last_config` → JSON-строка
→ `config`) кроме AWG2-набора стоят:

```ini
HeaderProtectionKey = <base64 32 байта>
ContentPaddingAddition = 10-100
RekeyAfterTime = 100-120
RekeyTimeout = 3-7
RejectAfterTime = 150-180
KeepaliveTimeout = 5-15
MaxHandshakeAttempts = 15-20
RandomTrailers = on
DisableCookies = on
[Peer] PersistentKeepalive = 25-35
```

Что делает лаунчер сегодня (проверено на экспорте владельца
`~/Downloads/Telegram Desktop/amnezia_config_seliv.vpn`):

- `wgConfToURI` (`core/config/subscription/node_parser_amnezia.go:404-478`)
  кладёт в `wireguard://` только известные ключи — все AWG3-ключи
  ВЫБРАСЫВАЮТСЯ молча. Узел выглядит настроенным, а хендшейк с сервером,
  шифрующим заголовок, не пройдёт никогда.
- `PersistentKeepalive = 25-35` проходит в URI как `keepalive=25-35`, а
  `parseWireGuardURI` делает `strconv.Atoi` (`node_parser_wireguard.go:176-180`)
  → поле молча теряется.
- `MTU` в экспорте AWG3 лежит НЕ в `[Interface]`, а рядом — в JSON
  `last_config.mtu` (`"1376"`). `findWGIniText` (`:288-330`) возвращает только
  текст → MTU не виден, узел получает кламп `awgMaxMTU=1280`
  (`node_parser_wireguard.go:140-153`). Для AWG3 задача требует брать MTU из
  экспорта.
- `DNS = $PRIMARY_DNS, $SECONDARY_DNS` — плейсхолдеры Amnezia; реальные
  адреса лежат в корне профиля `dns1`/`dns2`. Сейчас плейсхолдеры уезжают в
  URI как есть (`dns=%24PRIMARY_DNS…`).
- Ядро ≤ lx.31 отвергает конфиг с любым AWG3-ключом ЦЕЛИКОМ («невалидный
  JSON») — один узел лишает пользователя VPN. Нужен гейт по образцу
  tailscale (`core/core_capabilities.go:114-188`,
  `core/config/outbound_generator.go:1234-1310`).
- `ShareURIFromWireGuardEndpoint` (`shareuri_wireguard.go:17-110`) не знает
  AWG3-ключей, а `mapGetInt(peer,"persistent_keepalive_interval")` (`:67`)
  на строке-диапазоне даёт 0 → round-trip теряет и keepalive.
- Бейдж уровня `deriveAWGLevel` (`ui/configurator/business/config_node_labels.go:104-120`)
  знает только awg/awg1.5/awg2.
- `clearAWGSettings` (`ui/configurator/tabs/source_awg_edit.go:296-320`)
  снимает AWG2-поля, но оставит AWG3 — «обычный WireGuard» с
  `header_protection_key` ядро отвергнет.

## 2. Решения (приняты, не обсуждаются)

- **Имена URI-параметров** = ключ `.conf` в нижнем регистре
  (как `jc`, `presharedkey`, `listenport`): `headerprotectionkey`,
  `contentpaddingaddition`, `rekeyaftertime`, `rekeytimeout`,
  `rejectaftertime`, `keepalivetimeout`, `maxhandshakeattempts`,
  `randomtrailers`, `disablecookies`; `keepalive=25-35`. Это контракт с
  LxBox — фиксируется в `contract/registry/containers.json:66` и корпусе.
- **Формы значений.** Диапазонное поле: `N` → JSON-число (int64, как AWG2
  numerics), `N-M` → JSON-строка `"N-M"` (нормализованная: без пробелов).
  Булево: `on`/`true`/`1` → `true`; `off`/`false`/`0`/пусто → ключ не
  пишется; иное → поле снято + warning. Ключ защиты: строка base64
  дословно.
- **Политика ошибок** (та же, что у AWG2):
  - поле-диапазон с мусором или `N > M` → ПОЛЕ снято, узел живёт, код
    предупреждения на узле (`awg3_field_invalid`, severity warning,
    params field,value). Не свопать границы (в отличие от h1–h4): тайминги
    клиентские, ядро с дефолтом работает, а перевёрнутый диапазон — опечатка,
    которую человек должен увидеть.
  - `header_protection_key` не base64 / не 32 байта / все нули → УЗЕЛ
    выброшен с ошибкой (как `awgHeaderOverlap`, `node_parser_wireguard.go:225-228`):
    без верного ключа хендшейк невозможен, а ядро отвергает конфиг целиком.
    Код `awg3_header_key_invalid` (error, dropped).
  - `header_protection_key` задан и хоть один из `s1`–`s4` < 12 (или не
    задан = 0) → узел выброшен, код `awg3_padding_too_short` (error, dropped).
    Формулировка ошибки называет поле и минимум.
  - `random_trailers: true` при хоть одном ДИАПАЗОННОМ `h1`–`h4` с шириной
    `hi-lo >= 65536` → info-код `awg3_random_trailers_wide_headers` на
    узле; ничего не снимать (свойство протокола, см. §2.10 доков).
- **MTU.** (пересмотрено 2026-09-05 после приёмки: на 1376 из экспорта у
  владельца данные не шли, на 1280 заработало) — AWG3-узел клампится до
  `awgMaxMTU` = 1280 так же, как AWG2; AWG3-маркер без AWG2-полей тоже
  считается AWG. AWG2 ведёт себя байт в байт как раньше (тест `TestParseNode_AmneziaVPN_AWG`
  и корпус `amnezia_vpn_awg` не меняются).
- **Бейдж** структурный, версия в теле не хранится (как у awg2):
  `awg3.1` — есть `random_trailers` или `disable_cookies`; `awg3` — любой
  другой AWG3-ключ или диапазонный keepalive; суффикс `+` при masquerade
  как сейчас. Подзаголовок в списке серверов покажет `wireguard·awg3.1`.
- **Редактор.** Форма обфускации (`source_awg_edit.go`) остаётся
  masquerade-формой, AWG3-поля в неё НЕ добавляются: они серверные/копируются
  из экспорта и правятся на вкладке JSON узла. Меняется только
  `clearAWGSettings`: снимает все `AWG3RootKeys()`, а диапазонный
  `persistent_keepalive_interval` у пиров заменяет нижней границей (число).
  `applyAWGSettings` AWG3-поля не трогает (сахар `id/ip/ib` совместим с ними).
- **Гейт ядра.** Проба `CoreSupportsAWG3()` по выводу `sing-box version`:
  строка `Tags:` без `with_awg` → не поддерживает; базовая версия
  (`CompareVersions`, `core/core_version.go:194`) > 1.14.0 → поддерживает;
  = 1.14.0 → суффикс `-lx.N` (regexp `-lx\.(\d+)`) с N ≥ 32 → поддерживает,
  иначе нет; нет строки версии / непарсибельный вывод → поддерживает (не
  деградировать на догадках — как у tailscale). Причина для UI:
  `sing-box core %s does not support AmneziaWG 3.x fields (need 1.14.0-lx.32 or newer) — update the core in Core Dashboard`.
  Генератор снимает узлы с `HasAWG3Fields(n.Outbound)` (только
  `n.Scheme == "wireguard"`), считает `SkippedAWG3Nodes/Reason`, отчёт
  сборки получает `BuildReportAWG3Degraded` («%d AmneziaWG 3.x node(s)
  skipped: %s»). Код реестра `awg3_core_unsupported` (warning, build).
- **RequiredCoreVersion** → `1.14.0-lx.33` (пин; порог гейта остаётся lx.32) (`internal/constants/constants.go:131`).
  Ядро в бандле и в `bin/` меняет владелец задачи руками, не агент.

## 3. Поток А — разбор, эмиссия, бейдж, контракт

Файлы: `core/config/subscription/node_parser_wireguard.go`,
`node_parser_amnezia.go`, `shareuri_wireguard.go`, `awg3.go` (дописывать
можно), `ui/configurator/business/config_node_labels.go`,
`ui/configurator/tabs/source_awg_edit.go`, `contract/registry/warnings.json`,
`contract/registry/containers.json`, `contract/corpus/uri/wireguard/*`,
`contract/TASKS_LXBOX.md`, тесты рядом.

### 3.1 `node_parser_wireguard.go`

- `hasAWGParams` (`:471-480`): учитывать AWG3-параметры (`awg3.go`) и
  `keepalive` вида `N-M`.
- `applyAWGFields` (`:493-535`): после AWG2-полей разобрать AWG3:
  ключ защиты (валидация через `normalizeWGKey`-подобную проверку: base64
  любой из четырёх кодировок → 32 байта → не все нули; хранить ДОСЛОВНО как
  пришло, не перекодировать — ядро декодирует std base64, а Amnezia пишет
  именно его; если пришёл url-safe вариант — привести к std, как ключи),
  диапазоны (`parseAWG3Range` → int64 | string), булевы. Возвращать коды.
  Ошибки, роняющие узел, — отдельной функцией `validateAWG3(endpoint) error`,
  зовётся из `parseWireGuardURI` сразу после `awgHeaderOverlap` (`:225`).
- keepalive (`:176-180`): `N` → int как сейчас; `N-M` → строка через тот же
  `parseAWG3Range`; мусор — как сейчас (пропуск).
- MTU (`:140-153`): кламп только если `isAWG && !isAWG3`. `isAWG3`
  считать по query до сборки endpoint (наличие любого AWG3-параметра или
  `keepalive` с дефисом).
- Ловушка: `q.Get` уже URL-декодирует; `+` в base64 ключа защиты превратится
  в пробел — брать через `queryParamPreservePlus` (`:349`), как
  `publickey`/`presharedkey`.

### 3.2 `node_parser_amnezia.go`

- `wgConfToURI` (`:404`): прокидывать AWG3-ключи `[Interface]` по таблицам
  `awg3.go` (`iface[f.Param]` → `q.Set(f.Param, v)`), ключ защиты и булевы
  тоже. `persistentkeepalive` уже прокидывается дословно.
- Извлечение `last_config.mtu` и подстановка DNS: заменить `findWGIniText`
  на вариант, который возвращает и текст, и карту `last_config` (если текст
  найден внутри JSON-строки с ключом `config`). Далее одна функция
  `amneziaPrepareConf(text string, lastConfig map[string]interface{}, profile map[string]interface{}) string`:
  - если в `[Interface]` нет `MTU`, а `lastConfig["mtu"]` (строка или число)
    парсится в int > 0 → дописать строку `MTU = N` в `[Interface]` (текст,
    не URI: `wgConfToURI` остаётся единственной точкой конвертации);
  - `$PRIMARY_DNS` → `profile.dns1`, `$SECONDARY_DNS` → `profile.dns2`;
    неразрешённый плейсхолдер (нет dns1/dns2) — запись выбросить из списка
    DNS; если список опустел — строку `DNS` не писать.
  Использовать в обоих путях: `amneziaWGConfText` (`:173`) и
  `amneziaAllWGConfTexts` (`:215`) — оба должны отдать уже подготовленный
  текст. Ловушка: `findWGIniText` при обходе карты пробует ключи
  `config/last_config/awg/wireguard` первыми — сохранить детерминизм.
- Метка узла и выбор контейнера не меняются.

### 3.3 `shareuri_wireguard.go`

- После masquerade-блока (`:95-100`) эмитить AWG3: ключ защиты и диапазоны —
  по присутствию (`awgNumericString` умеет int64/float64/string), булевы —
  `q.Set(param,"on")` только при `true` (после JSON-раундтрипа `bool`).
- keepalive (`:67`): вместо `mapGetInt` — `awgNumericString(peer[...])`,
  чтобы строка `"25-35"` доехала.
- Цель: endpoint → URI → endpoint для AWG3-узла даёт равный endpoint
  (тест 3.6).

### 3.4 Бейдж и редактор

- `deriveAWGLevel` (`config_node_labels.go:105`): ветки `awg3.1`/`awg3`
  ВЫШЕ `awg2` в switch; использовать `subscription.HasAWG3Fields` для «awg3»
  и прямую проверку двух булевых ключей для «3.1». Пакет `business` уже
  импортирует `subscription`? — проверить, иначе импорт добавить (цикла
  нет: `subscription` не зависит от `ui`).
- `clearAWGSettings` (`source_awg_edit.go:296`): см. §2 «Редактор».
  Ловушка: `peers` после `json.Unmarshal` — `[]interface{}` из
  `map[string]interface{}`.

### 3.5 Контракт

- `contract/registry/warnings.json`: добавить `awg3_field_invalid`
  (warning; params field,value), `awg3_header_key_invalid` (error, dropped),
  `awg3_padding_too_short` (error, dropped; params field,min),
  `awg3_random_trailers_wide_headers` (info), `awg3_core_unsupported`
  (warning; сборка). Поля `dart: null`, `go: <файл:функция>`, `desc` по
  образцу соседей (`:277-299`, `:458-466`). Константы `Warn…` в Go — там,
  где лежат `WarnAWGHeaderInvalid`/`WarnTailscaleCoreUnsupported`
  (`grep -rn "WarnAWGHeaderInvalid ="`). Есть sync-тест реестра — прогнать.
- `contract/registry/containers.json:66`: дописать AWG3-ключи в перечень
  optional.
- Корпус `contract/corpus/uri/wireguard/`: `awg3_full_params.uri` (все
  AWG3-поля + `keepalive=25-35` + `mtu=1376`; ключ защиты — СИНТЕТИЧЕСКИЙ
  валидный 32-байтный base64, не из экспорта владельца) и
  `amnezia_vpn_awg3.uri` (профиль как у владельца по структуре: контейнер
  `amnezia-awg2`, `awg.protocol_version "3.1"`, `last_config` с `mtu`,
  `DNS = $PRIMARY_DNS, $SECONDARY_DNS`, `dns1/dns2` в корне; ключи
  синтетические). Expected генерировать
  `go test ./core/config -run TestContractCorpusURI -update`, затем
  проверить руками: `mtu: 1280` (1376 клампится), `persistent_keepalive_interval: "25-35"`,
  `random_trailers: true`, диапазоны строками, `content_padding_addition`
  строкой, DNS-плейсхолдеров нет. Плюс негативный кейс
  `awg3_header_key_short_dropped.uri` (ключ 16 байт → узел выброшен).
- `contract/TASKS_LXBOX.md`: добавить задачу для LxBox (пин AAR
  `libbox-1.14.0-lx.32.aar`, разбор тех же URI-параметров и `.conf`-ключей,
  тот же маппинг в JSON, гейт «обновите ядро» не нужен — ядро в AAR; бейдж
  `awg3/awg3.1`; корпус — источник истины). Формат — по соседним записям
  файла.

### 3.6 Тесты (в конце, один прогон)

Не россыпь юнитов — по одному комплексному на слой:

- `node_parser_amnezia_test.go`: `TestParseNode_AmneziaVPN_AWG3` — фикстура
  по структуре экспорта владельца (см. выше), проверить: все AWG3-поля на
  корне с нужными типами, `mtu == 1280` (1376 из `last_config` клампится), keepalive `"25-35"`,
  `random_trailers == true`, `disable_cookies == true`, отсутствие
  `$PRIMARY_DNS` в `node.Query.Get("dns")` (там `172.29.172.254,1.0.0.1`),
  `HasAWG3Fields(node.Outbound)`; и что `ParseAmneziaVPNLinkAll` отдаёт
  тот же узел.
- `wireguard_robustness_test.go` или новый `awg3_test.go`: таблица
  негативов — короткий ключ (dropped), нулевой ключ (dropped), `s4=8` при
  ключе (dropped), `rekeyaftertime=180-150` (поле снято + код), `randomtrailers=maybe`
  (снято + код), `random_trailers` + `h1=1-100000` (info-код); плюс
  round-trip `ShareURIFromWireGuardEndpoint(parse(uri).Outbound)` →
  `parse` → `reflect.DeepEqual` endpoint'ов для полного AWG3-набора.
- Проверить, что `TestParseNode_AmneziaVPN_AWG` (AWG2, кламп 1280) зелёный
  без правок.
- Приёмка на реальном файле (НЕ в тесты, ключи секретные): временная
  программа в scratchpad, которая читает
  `~/Downloads/Telegram Desktop/amnezia_config_seliv.vpn`, зовёт
  `subscription.ParseNode`, печатает `GenerateEndpointJSON`, и проверка
  `sing-box check` бинарём lx.32 из
  `/private/tmp/claude-501/-Users-macbook-projects-singbox-launcher/eb3d281c-de64-414b-80ed-cb434599481d/scratchpad/core32/` (найти `sing-box` через `find`)
  на минимальном конфиге `{ "endpoints": [<endpoint>], "outbounds": [{"type":"direct","tag":"direct"}] }`.
  Затем тот же конфиг с inbound `{"type":"socks","tag":"in","listen":"127.0.0.1","listen_port":18999}`
  и `route.final = <тег endpoint>` запустить `sing-box run -c` В ФОНЕ
  (PID сохранить!), `curl -s --socks5-hostname 127.0.0.1:18999 https://api.ipify.org`
  должен дать `77.239.123.44`, затем убить ТОЛЬКО свой PID. Ни в коем
  случае не `pkill sing-box` — живой VPN и демон lxd (root) трогать нельзя.
  Google через этот сервер не отвечает — не критерий.

## 4. Поток Б — гейт ядра, отчёт, версия, заметки

Файлы: `core/core_capabilities.go` (+ `_test.go`), `core/controller.go`,
`core/config/outbound_generator.go`, `core/config/build_report.go`,
`core/build_report_feed.go`, `ui/configurator/tabs/final_report_model.go`,
`internal/constants/constants.go`, `bin/locale/ru.json`,
`docs/release_notes/upcoming.md`, `docs/Protocols.md` (если там описан
wireguard:// — дописать параметры).

### 4.1 Проба

- `core/core_capabilities.go`: по образцу tailscale (`:114-188`) —
  `awg3SupportVerdict` кэш по (mtime,size), `CoreSupportsAWG3()`,
  `probeAWG3Support(path)`, чистая `awg3VerdictFromVersionOutput(out) (bool, string)`
  по правилам §2 «Гейт ядра». Версию брать regexp'ом
  `sing-box version\s+(\S+)` (как `GetInstalledCoreVersion`,
  `core/core_version.go:44`); теги — `versionTagsRegex` (`:84`) +
  `splitBuildTags`. Константы: `awg3MinLxRelease = 32`, `awgBuildTag = "with_awg"`.
- Тест `TestAWG3VerdictFromVersionOutput` в `core_capabilities_test.go`
  по образцу `:71-100`: lx.32 → ok; lx.31 с with_awg → нет (причина
  содержит `1.14.0-lx.32`); lx.32-rc.1 → ok; `1.15.0` upstream без with_awg
  → нет (причина содержит `with_awg`); мусор → ok.
- `core/controller.go:263`: `config.AWG3SupportProbe = ac.CoreSupportsAWG3`.

### 4.2 Генератор и отчёт

- `outbound_generator.go`: `var AWG3SupportProbe func() (bool, string)`
  рядом с `TailscaleSupportProbe` (`:254-258`); в
  `GenerateOutboundsFromParserConfig` третья ветка снятия после tailscale
  (`:1290-1310`): `n.Scheme == "wireguard" && subscription.HasAWG3Fields(n.Outbound)`.
  Учесть `skippedAWG3Here` в условии «все узлы источника сняты → источник
  успешен» (`:1312-1316`). Поля результата `SkippedAWG3Nodes/Reason`
  рядом с tailscale (`:76-81`, `:1367`). Лог с кодом
  `subscription.WarnAWG3CoreUnsupported` (константу заводит поток А в
  том же файле, где `WarnTailscaleCoreUnsupported`; если её ещё нет при
  сборке — завести самому там же с тем же именем, поток А не продублирует:
  перед добавлением grep).
- `build_report.go:52-54`: `BuildReportAWG3Degraded = "awg3_degraded"`;
  `build_report_feed.go:124-131`: запись с Subject `"amneziawg3"`.
- `final_report_model.go:100,142`: приоритет и текст
  `"%d AmneziaWG 3.x node(s) skipped: %s"`.
- Ловушка: HasAWG3Fields читает `n.Outbound` парсера — там `peers` это
  `[]map[string]interface{}`, а после JSON — `[]interface{}`; предикат
  уже умеет оба (через `wireGuardPeerMaps`).

### 4.3 Версия, локаль, заметки

- `RequiredCoreVersion = "1.14.0-lx.33"`. Core Dashboard сам покажет
  «Reinstall v1.14.0-lx.33» (`ui/core_dashboard_tab_status.go:326-345`).
- Локаль: новые `locale.T`/`Tf` ключи — в `bin/locale/ru.json`; проверить
  инструмент `tools/l10n` (README/скрипт) на предмет извлечения ключей и
  запустить его проверку, если она есть.
- `docs/release_notes/upcoming.md`: пункт EN (Highlights) и RU (Основное)
  про поддержку AmneziaWG 3.0/3.1 (импорт vpn:// и .conf с новыми полями,
  MTU из экспорта, DNS из профиля, бейдж awg3/awg3.1, гейт ядра с понятной
  причиной, требуется ядро lx.32); Technical — гейт и правило MTU.
  Стиль — как у соседних пунктов, без упоминания агентов.

## 5. Общие правила для обоих потоков

- Рабочая копия разделяемая: НИКАКИХ `git checkout/stash/reset/switch`,
  веток не переключать, чужие файлы не откатывать. Коммиты не делать —
  их делает владелец.
- Только `go build ./... && go vet ./...` и целевые `go test` пакетов,
  которых касались; полный `go test ./...` — один раз в конце потока А.
  GUI-пакеты из `go test` исключены (CONSTITUTION).
- Комментарии кода — по `SPECS/IMPLEMENTATION_PROMPT.md` («Язык
  комментариев», минимальный дифф). Русские строки в UI — только через
  `locale.T`.
- Секреты экспорта владельца (ключи из `amnezia_config_seliv.vpn`) в репо
  не попадают ни в тесты, ни в корпус, ни в доки.
- Не трогать `bin/sing-box` и бандл в `/Applications` — это делает владелец.
- Запрещено убивать процессы sing-box по имени.

## 6. Приёмка

- Импорт `amnezia_config_seliv.vpn` через «Add from file» → узел
  `wireguard·awg3.1`, `sing-box check` на lx.32 зелёный, соединение через
  socks-прогон даёт `77.239.123.44`.
- AWG2-профиль и обычный WireGuard — прежние expected в корпусе без
  изменений.
- AWG3-узел на ядре lx.30 (бандл до обновления) — узел снят, в отчёте
  сборки строка с причиной и «need 1.14.0-lx.32 or newer», конфиг
  собирается.
