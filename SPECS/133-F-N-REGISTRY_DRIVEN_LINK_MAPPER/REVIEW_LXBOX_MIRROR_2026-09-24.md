# Ревизия реестра: лаунчер vs зеркало LxBox (24.09.2026)

Запрос владельца: «в реестре были расхождения — проведи ревизию, чтобы мы имели
общий реестр». Сравнение: `origin/develop` лаунчера (contract 1.1.52, последний
коммит `contract/` = 8a915861) против LxBox (`app/contract.lock`,
`app/assets/contract/`, `docs/contract/`, `app/assets/contract_draft/`).

## Итог одной строкой

**Зеркало совпадает байт в байт, отставания по версии нет.** Настоящие
расхождения живут не в зеркале, а рядом: 27 файлов в
`app/assets/contract_draft/` (14 оверлеев + 13 секций `singbox/*`), которые
загрузчик LxBox накладывает поверх реестра, плюс разная семантика двух
примитивов в Dart-движке. Синк и lock-проверка их не видят; раннер корпуса тел
у LxBox сравнивает только состав узлов, не тела.

## 1. Версии и синки

| | |
|---|---|
| Лаунчер `origin/develop` | `contract/VERSION` = 1.1.52; после 8a915861 ни один коммит `contract/` не трогает |
| `app/contract.lock` | source_sha=8a915861, synced_at 2026-09-23T23:11:52Z, sha256 дерева совпадает (1606 файлов, пересчитано) |
| `app/assets/contract/` (registry/** + VERSION) | 1.1.52, коммит 949de22b; 30 файлов побайтно = 8a915861 = HEAD |
| `docs/contract/` | 23 файла = `contract/docs/generated/**` |
| Правки зеркала мимо синка | нет: все 25 коммитов дают дерево, равное какому-то коммиту лаунчера |
| Список для синк-бампа | пустой |

Гард `registry_sync_test.dart` сверяет словари кода с `allowlists.json`, а не
копию; побайтную сверку делает `check_contract_lock.dart`, но в CI `app/contract/`
не восстанавливается — шаг ничего не проверяет. Корпусные тесты в CI пропускаются.

## 2. Разрешённые per-app расхождения (б)

- `transports.uri.ws.ed` без `implies` (D-008) — кейс `uri/trojan/ws_ed_flat_no_eh` с парой `.expected.launcher/.lxbox`, TASKS §47.7, ждёт владельца.
- `ws.eh` без `round_trip:false` — принято §47.1.
- 24 пары `.expected.lxbox.json` / `.expected.launcher.json` — одинаковы у сторон.
- D133-24 (хост Endpoint в метке `.conf`): DELTAS говорит «LxBox снимает оверлеем», оверлея `conf/` нет с 538dceb5 — текст DELTAS устарел.

## 3. Настоящие расхождения (в)

### Грамматика: один реестр исполняется по-разному
1. **`json.key_absent` / `json.type_of`.** Dart: `key_absent` не реализован (неизвестный ключ = истина); `type_of` при отсутствующем пути проходит, Go — нет (`linkmap/detect.go:298` vs `interpreter.dart:874`). Следствие: Xray `hysteria` с `version: 3` у LxBox даёт узел v1 (по D133-28 не должен). Правы мы (PRIMITIVES FROZEN). LxBox: исполнить оба примитива, снять `forms` из `xray/trojan.json`, `xray/vless.json`. Нам: кейсы корпуса (hysteria v3; trojan без `streamSettings`).
2. **Коллизия записей блока и секции (MAPPER_ENGINE §7.1).** Dart изымает запись блока при любом пересечении источников, не учитывая `when`; компенсирует оверлеем `uri/http` `$security_none_https`. LxBox: чинить движок, снять оверлей. Нам: кейс `proxy-https` + `security=none` (в корпусе нет).
3. **Булев на выходе без `emit_as`.** Go пишет словом, Dart цифрой; компенсируется оверлеями `emit_as: raw` у трёх полей xhttp. Нам: закрепить умолчание в PRIMITIVES §0.12a. LxBox: снять оверлеи.

### Вид share-ссылки (решения владельца)
4. **vless `security=reality`.** Мы опускаем (`omit_default`, исключение D133-E1), LxBox пишет. **Прав LxBox**: у Xray-клиентов отсутствие `security` = `none`. Нам: убрать из `omit_default`, поправить D133-E1.
5. **anytls, флаг insecure.** Реестр `insecure` (с 1.1.36), LxBox `allowInsecure`; TASKS §33.1 противоречит реестру и D133-E2. Скорее правы мы (anytls — протокол sing-box). Решение владельца; поправить §33.1.
6. **ss, паддинг userinfo.** У нас `padding:true`, у LxBox `false`; эталон SIP002 без `=`. Склоняемся к LxBox. Решение владельца; обе стороны читают обе формы.
7. **naive `omit_port:443`.** LxBox опускает, мы пишем (§45.3). Решение владельца или дельта.
8. **vmess-контейнер.** `param_order`: LxBox — порядок v2rayN, у нас алфавит (§33.1 мы сами просили сообщить); `json_always`: у нас `ps`,`scy`, `aid` числом, у LxBox `ps`/`scy` нет, `aid` строкой `"0"`; `json_map`: LxBox отказывается. Решение владельца; перечень v2rayN, скорее всего, взять.
9. **wireguard.** `omit_default: server_port` есть только у LxBox — дефолта порта в секции нет, эффект проверить.

### Разбор: тело узла разное (10–13 противоречат общему корпусу)
10. **Xray `wsSettings.ed/eh` плоскими полями** — LxBox читает в тело; кейсы `body/xray/ws_ed_flat_only`, `ws_eh_without_ed` ждут `json_field_unknown`. Прав корпус.
11. **`sockopt` с отрицательным `interval`** — Dart ставит `disable_tcp_keep_alive`; кейс `sockopt_keepalive_negative_interval` ждёт `tcp_keep_alive: 30s`. Прав корпус. У нас дыра: отрицательный `interval` без положительного `idle` не читаем.
12. **`$sni_from_server` в `tls.xray`** — LxBox подставляет адрес сервера в `server_name`; 5 узлов корпуса ждут отсутствия. LxBox снять.
13. **`ws.host` в Xray** — оверлей читает только `headers.Host`, теряет плоский `wsSettings.host`. Прав реестр.
14. **xhttp `sessionPlacement`/`sessionKey`** — оверлей LxBox уже реестра (с 1.1.50 реестр читает плоскую форму). LxBox снять.
15. **Мелкие отличия `tls.xray`/`transports.xray`**: reality → uTLS `chrome` сразу (кейса «reality без fingerprint» нет — завести); `when` у `fp`,`insecure`,`pbk`; `http.path "/"` → отсутствие; `ws.path` `empty: significant`; `$key_share_not_carried` — кандидат D133-C8.
16. **`ech_ignored`** — LxBox показывает имя до `+`, мы полное (§32.4). Только текст предупреждения.
17. **vmess по ссылке — у LxBox есть, у нас нет**: keep-alive из JSON-контейнера (мы теряем при обратном разборе), откат host на адрес сервера для h2, `unknown_key.ignore [v, type]`, `type`/`headerType` по формам. Keep-alive и откат host для h2 — забрать в реестр.
18. **anytls, эвристика SNI** — правило `sni_heuristic_falls_back_to_server` в `tls.json` включает anytls, а секция anytls его не несёт (hysteria2 несёт). Реестр противоречит сам себе, прав LxBox. Нам: `on_invalid` у `sni` anytls.
19. **Xray ss с пустым паролем** — LxBox `empty: significant`, у нас `required`. Ждёт владельца, кейса нет.
20. **`mappers.singbox`** — в реестре только trojan и vless, хотя MAPPER_ENGINE §8a требует секцию для всех. У LxBox сгенерированы 13 секций (anytls, http, hysteria, hysteria2, masque, naive, shadowsocks, socks, ssh, tailscale, tuic, vmess, wireguard). Отстаём мы: забрать механически, LxBox удалить `singbox/`.
21. **`contract_draft/registry_mapper.schema.json`** — старая правленая копия схемы, кодом не используется. Удалить.

Мусор у LxBox без влияния: `uri/trojan` (`omit_default: []`) и `tls.blocks.uri_with_host` = реестру; в `kDraftFiles` есть `hysteria2` без файла; `_isUntranslatedCanon` в `emitter.dart:1867` (по §46 п.7 должен быть снят).

## 4. Механизм: как получить общий реестр

Сабмодуль не поможет — файлы и так доезжают байт в байт; расходится исполнение.
Оставить синк по sha с `contract.lock` и добавить:
1. **Оверлеи только со ссылкой на DELTAS.** Каждая запись `contract_draft` несёт ID дельты; тест LxBox краснеет на записи без ID, наш раннер — на ID без записи в DELTAS. DELTAS = единый реестр расхождений.
2. **CI LxBox восстанавливает `app/contract/`** через `git archive` по `source_sha` — lock-проверка и корпус начинают работать.
3. **Раннер тел LxBox сравнивает тело целиком**, не только состав: пункты 10–13 ловятся сами.
