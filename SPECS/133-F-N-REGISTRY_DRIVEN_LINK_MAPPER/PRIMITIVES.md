# PRIMITIVES — инвентаризация ссылочного входа и её выражение примитивами

SPEC 133. Спутник `SPEC.md` (дизайн движка) и `TASKS.md` (волны).
Адреса — на `5170a169` (develop, 19.09.2026), **после** W2d SPEC 131:
правила ЗНАЧЕНИЙ из парсеров уже вынесены, здесь инвентаризируется
**остаток** — структурный перевод диалекта.

Задача документа: по каждой схеме показать, что парсер делает сверх
плоского `param → maps_to`, и **каким примитивом реестра это выражается**.
Именованных крючков в коде не будет: требование владельца 19.09.2026 —
«дать реестру нужное количество примитивов, в коде остаётся только движок,
общий для всех схем». Каждый пункт инвентаризации обязан закрыться
примитивом; остаток «невыразимого» сведён к нулю (§12).

---

## 0. ЗАМОРОЗКА ГРАММАТИКИ — финальные имена атрибутов

**Статус на 19.09.2026, вечер.** LxBox пишет движок по этому файлу.
`FROZEN` = имя атрибута менять нельзя; дальнейшие правки — только
**добавлением**. Любое переименование FROZEN-атрибута согласуется отдельно.
`DRAFT` = имя может измениться; на реализацию брать в последнюю очередь.

Машиночитаемая форма — `contract/schema/registry_mapper.schema.json`.

### 0.1. Структура секции

| Атрибут | Где | Статус | Значение |
|---|---|---|---|
| `mappers.<kind>` | у протокола | **FROZEN** | секция на вид источника; `kind` ∈ `uri`, `xray`, `singbox`, `conf` |
| `detect` | у секции, у формы, у вида источника | **FROZEN** | признак «этот контент — мой» (§0.2) |
| `body_source` | у секции | **FROZEN** | `uri`\|`singbox`\|`xray`\|`wgconf`\|`amnezia` — на это опирается `except_sources` |
| `forms[]` | у секции | **FROZEN** | оболочка: `id`, `detect`, `decode`, `space`, `base`, `level` |
| `forms[].base` | у формы | **FROZEN** | якорь пути, подставляется вместо `$base` в `source` |
| `forms[].level` | у формы | **FROZEN** | `outbound`\|`endpoint` |
| `userinfo` | у секции | **FROZEN** | `decode`, `split{sep,limit}`, `into[]`, `single_into` |
| `params.<имя>` | у секции | **FROZEN** | таблица записей |
| `include[]` | у секции | **FROZEN** | блок с диалектом: `"transports.ws#uri"`, `"tls#xray"` |
| `scheme_sets` | у секции | **FROZEN** | написание схемы → присваивания |
| `type_synonyms` | у секции | **FROZEN** | тип чужого диалекта → схема реестра |
| `unknown_key{action,code}` | у секции | **FROZEN** | `keep`\|`drop` + код |
| `emit` | у секции | **FROZEN** | `null` = обратного хода нет |
| `blocks` | в `transports.json`/`tls.json` | **FROZEN** | блок → диалект → записи |
| `document.sources[]` | в `sources.json` | **FROZEN** | `kind`, `priority`, `mapper`, `detect`, `unwrap`, `redetect`, `split` |
| `document.max_unwrap_depth` | там же | **FROZEN** | предел рекурсии распаковки |
| `document.on_unrecognized` | там же | **FROZEN** | `code`, `severity`, `include_fragment` |

### 0.2. Предикаты `detect` (одни и те же на обоих уровнях)

| Атрибут | Статус | Значение |
|---|---|---|
| `regex` | **FROZEN** | RE2 ∩ ECMAScript, якоря в самом выражении |
| `json.required_keys` / `any_keys` / `key_absent` | **FROZEN** | пути точечные; числовой сегмент индексирует массив |
| `json.type_of` | **FROZEN** | путь → `object`\|`array`\|`string`\|`number`\|`bool` |
| `json.value_of` / `value_in` | **FROZEN** | путь → значение / набор (строки сравниваются fold-case) |
| `json.array_elem_any_keys` | **FROZEN** | хотя бы один элемент массива несёт путь |
| `ini.sections` / `keys` / `keys_any` | **FROZEN** | имена fold-case |
| `text.prefix_fold` / `line_fold` / `contains` | **FROZEN** | |
| `scheme_in` | **FROZEN** | написания схемы ссылки |
| `in_array` | **FROZEN** | `outbounds`\|`endpoints` |
| `not` / `all` / `any` / `default` | **FROZEN** | композиция; `default` не конкурирует с предикатами |

### 0.3. Запись таблицы (`params.<имя>`)

| Атрибут | Статус | Значение |
|---|---|---|
| `source` | **FROZEN** | строка \| массив (приоритет) \| карта по `id` формы. **Единственный** способ получить значение |
| `maps_to` | **FROZEN** | путь \| `null` (осознанно никуда) \| карта по типу тела |
| `aliases` | **FROZEN** | существующий атрибут |
| `type` | **FROZEN** | `string`,`int`,`bool`,`bool_spelled`,`duration`,`base64`,`list`,`object` |
| `required` | **FROZEN** | |
| `selector` | **FROZEN** | параметр первого прохода |
| `priority` | **FROZEN** | порядок двух записей в один путь (меньше = раньше) — **G3** |
| `merge` | **FROZEN** | `keep_first`\|`overwrite`\|`prepend`\|`append` |
| `value_map` | **FROZEN** | перевод значений; `null` = ключа нет; поддерживает `prefix`/`strip` |
| `sets` | **FROZEN** | значение → присваивания; **`null` в присваивании СНИМАЕТ путь** — **G2** |
| `implies` | **FROZEN** | наличие → присваивания |
| `when` | **FROZEN** | по телу, `$type`, `$form`, и по источнику (`query.X`/`json.X`/`ini.X`) — **G1** |
| `extract{re,into}` / `compose` | **FROZEN** | именованные группы ↔ шаблон |
| `list{sep,item,len,coerce_scalar}` | **FROZEN** | |
| `split_into` | **FROZEN** | список по нескольким полям по предикату |
| `normalize` | **FROZEN** | `base64_std`,`cidr_prefix`,`port_range_spec`,`duration_bare_seconds`,`range_order{swap,strict}`,`trim`,`trim_lower` |
| `decode_extra{mode,passes,max,plus_literal}` | **FROZEN** | §0.4 |
| `default_from` / `default_when` | **FROZEN** | существующие атрибуты |
| `materialize_default` | **FROZEN** | маппер ЗАПИСЫВАЕТ дефолт в тело |
| `coerce{object_to_scalar,scalar_to_list}` | **FROZEN** | приведение ФОРМЫ значения |
| `flatten[]` | **FROZEN** | члены вложенного объекта в плоский слой — **G5** |
| `lift` | **FROZEN** | перенос между уровнями вложенности |
| `sort_keys` | **FROZEN** | детерминированный порядок ключей `type:object` — **G4** |
| `empty` | **FROZEN** | `absent` (дефолт) \| `significant` |
| `on_invalid`/`on_present`/`on_item_invalid`/`on_no_match`/`on_len_gt` | **FROZEN** | `{action, code}` |
| `emit_when` / `omit_default` / `implicit` | **FROZEN** | обратное направление |
| `since` | **FROZEN** | версия контракта, с которой запись действует |

### 0.4. `decode_extra` — режимы percent-декода

| | Значение | Статус |
|---|---|---|
| `mode` | `query` \| `path` | **FROZEN** |
| `passes` | целое \| `"until_stable"` | **FROZEN** |
| `max` | предел при `until_stable` | **FROZEN** |
| `plus_literal` | `+` читается буквально; **по умолчанию выводится из `format: base64*`** | **FROZEN** |

Канон: `alpn` = `{mode:"query", passes:"until_stable", max:16}`;
`path` = `{mode:"path", passes:2}` — **path-семантика на ОБОИХ проходах**.

### 0.5. Метка и тег

| Атрибут | Статус | Значение |
|---|---|---|
| `label.source` | **FROZEN** | цепочка источников метки |
| `label.normalize[]` | **FROZEN** | `strip_control`, `trim` — **G8** |
| `label.value_map` | **FROZEN** | 🇪🇳→🇬🇧 и подобное |
| `label.fallback.template` | **FROZEN** | `"{scheme}-{server}-{server_port}"` |
| `label.fallback.scheme_source` | **FROZEN** | `as_written` \| `singbox_type`. **Решение владельца: `singbox_type`** |

### 0.6. Нормы, не являющиеся атрибутами

| Норма | Статус |
|---|---|
| имена параметров query читаются **регистронезависимо**; при двух написаниях в одной ссылке побеждает **точное совпадение с каноном**, иначе **первое по порядку** | **FROZEN** |
| дубли одного написания (`?fp=a&fp=b`) — берётся первый | **FROZEN** |
| `+` в query = пробел; исключение — поля `format: base64*` (`plus_literal`) | **FROZEN** |
| authority разбирает **лексер движка**, не платформенный парсер URL | **FROZEN** |
| percent-декод один раз, до всего; `decode_extra` — поверх | **FROZEN** |
| пусто = отсутствует, опт-аут `empty: "significant"` | **FROZEN** |
| порядок ключей тела задаёт `body.order` через `Emit`, не порядок примитивов | **FROZEN** |
| маппер не судит значения — судит санитайзер | **FROZEN** |
| эмит: `param_order` = **алфавит** (правило, не перечень) | **FROZEN** |
| эмит: пробел на выходе = `%20` | **FROZEN** |
| эмит: каноническое имя параметра = первое в `aliases` | **FROZEN** |

### 0.7. DRAFT — имена могут измениться

| Атрибут | Почему draft |
|---|---|
| `ini_dialect{key_case,value_case,comment_prefixes,inline_comments,repeated_key,sections.<S>.repeat,on_extra}` | набор ключей устаканится на волне `conf` (W8.6); сегодня выражает сегодняшний разбор дословно |
| источник `ini.$comment.<Section>` | синтаксис `$comment` не сверен с LxBox — **G7** |
| `$base` как подстановка в `source` | механика якоря формы проверяется на xray (W8) |
| служебные записи с `$`-префиксом (`$multiport`, `$plaintext_port`, `$legacy_flat`) | соглашение об имени: `$` помечает запись без `maps_to`, в документацию не идёт |
| `emit.form_from` | форма выбора схемы по телу нужна только эмиту (W7) |
| `round_trip: false` + причина | появляется в W7 вместе с исключениями раннера |
| `unwrap` значения (`base64`, `amnezia_vpn`) | список распаковщиков закрытый, но пополнится при первом новом контейнере |

---

## 1. Словарь примитивов

Ниже — полный набор. Примитивы **общие**: ни один не назван по схеме и ни
один не знает имени схемы. Существующие атрибуты реестра
(`when`, `any_set`, `aliases`, `normalize`, `default_when`, `on_invalid`,
`format`, `values`) переиспользуются как есть — новых языков не заводится.

| № | Примитив | Где объявляется | Что выражает |
|---|---|---|---|
| P1 | `uri.forms[]` | у схемы | оболочка ссылки: как из текста получить пространство источников |
| P2 | `uri.userinfo` | у схемы | разбор userinfo на компоненты |
| P3 | `source` | у параметра | откуда берётся значение (пространство источников) |
| P4 | `value_map` | у параметра | перевод ЗНАЧЕНИЙ диалекта |
| P5 | `selector: true` + `when` | у параметра | двухпроходность: селектор → зависимые |
| P6 | `sets` | у параметра / у формы | значение → набор присваиваний в тело |
| P7 | `implies` | у параметра | наличие параметра → присваивания |
| P8 | `extract` / `compose` | у параметра | регулярка с группами ↔ шаблон сборки |
| P9 | `list` | у параметра | список через разделитель |
| P10 | `default_from` | у параметра | источник значения по умолчанию (эвристики) |
| P11 | `label_fallback` | у схемы / у типа тела | имя узла, когда fragment пуст |
| P12 | `emit_when` / `omit_default` / `emit_form` / `param_order` | у схемы | обратное направление |

Типы значений, которые движок умеет приводить (расширение существующего
`type`): `string`, `int`, `bool_spelled` (общий набор написаний истины),
`duration`, `base64`, `list`, `object`.

### 1.1. Пространство источников (общее для P1/P3)

После того как форма (P1) распаковала текст, движок работает с ОДНИМ
плоским пространством имён. Параметр адресует источник строкой:

```
scheme              host              query.<name>       json.<path>
userinfo            port              fragment           ini.<Section>.<Key>
userinfo.user       path              authority
userinfo.pass
```

`query.<name>` — с учётом `aliases` (имена) и регистронезависимо.
Имя источника — **единственный** способ параметра получить значение;
движок не имеет доступа ни к чему, что параметр не объявил (§11, линтер).

---

## 2. P1 — `uri.forms[]`: оболочка ссылки

Формы пробуются **по порядку**, первая, чей `detect` сработал, выигрывает.
`decode` — конвейер общих декодеров движка: `url`, `percent`, `base64`,
`base64url`, `json`, `ini`. `space` называет итоговое пространство.

```jsonc
"uri": {
  "forms": [
    { "id": "v2rayn", "detect": {"after_scheme": "base64"},
      "decode": ["base64", "json"], "space": "json" },
    { "id": "url", "detect": {"default": true},
      "decode": ["url"], "space": "url" }
  ],
  "emit_form": "v2rayn"
}
```

### Что это закрывает

| Случай | Файл:строка | Форма |
|---|---|---|
| vmess base64(JSON) v2rayN | `node_parser_vmess.go:68-83` | `decode: ["base64","json"]`, `space: json` |
| vmess legacy cleartext `method:uuid@host:port` | `node_parser_vmess.go:117-194` | `decode: ["base64","url"]`, `space: url` |
| ss SIP002 `base64(method:pass)@host:port` | `node_parser_core.go:194-224` | форма `url` + P2 `decode` на userinfo |
| ss legacy `base64(method:pass@host:port)` | `node_parser_core.go:225-250` | `decode: ["base64","url"]` |
| `awg://<base64 .conf>` | `wgconf_text.go:221` | `decode: ["base64","ini"]`, `space: ini` |
| `.conf` текстом (wg-quick) | `wgconf_text.go:64` | форма без схемы: `decode: ["ini"]` |
| hysteria2 base64-обёртка тела | `node_parser_core.go:222-238` | `decode: ["base64","url"]` |
| Xray-JSON объект | `xray_outbound_convert.go` | форма без схемы: `space: json` (§10) |

**Ловушка порядка декодирования (норма для обеих сторон).**
`percent` применяется **один раз и до всего остального**, силами декодера
`url`. Сегодня это соблюдено сознательно (`node_parser_core.go:452-455`,
`node_parser_ssh.go:12-15`: повторный `QueryUnescape` портил `+` в PEM и
паролях), но с двумя узаконенными исключениями, которые в новой грамматике
становятся **свойством параметра**, а не поведением кода:

| Исключение | Сегодня | Примитив |
|---|---|---|
| `alpn` — декод до стабилизации (без ограничения проходов!) | `node_parser_transport.go:24-33`, применение `:840` | `"decode_extra": {"percent": 2}` у параметра |
| `path` — ровно 2 прохода `PathUnescape` | `node_parser_transport.go:759-776` | то же, `{"percent": 2, "mode": "path"}` |
| userinfo ss — percent ДО base64 | `node_parser_core.go:198-200` | порядок задан `decode` в P2 |

**`+` в значении query — ЧЕТЫРЕ заплаты на один корень (риск identity).**
`url.ParseQuery` трактует значение как `application/x-www-form-urlencoded` и
превращает `+` в пробел, ломая base64. Сегодня это обойдено точечно:

| Заплата | Адрес | Чинит |
|---|---|---|
| `queryParamPreservePlus` — ручной разбор `RawQuery` + `PathUnescape` | `node_parser_wireguard.go:360-375` | WG `publickey`, `presharedkey` |
| `queryParamPreservePlusOrGet` | `awg3.go:263-270` | AWG3 `header_protection_key` |
| `strings.ReplaceAll(v, " ", "+")` — обратная замена | `node_parser_wireguard.go:60` | WG `privatekey` из query |
| тот же `queryParamPreservePlus` | `node_parser_masque.go:58` | masque `publickey` |

**Вне этих четырёх параметров `+` в query сегодня становится пробелом** —
в том числе у `pbk` (REALITY), `password`, `obfs-password`. Норма движка
(и просьба LxBox, пункт (к)): принимать сырой текст и делать percent-декод
самому, без замены `+`. Это правильное поведение, но оно **меняет значения
там, где заплат не стояло** → риск identity, порядок действий в `SPEC.md` §9.3.

Отдельно: **пробел в userinfo чинится до `url.Parse`**
(`percentEncodeUserinfoSpaces`, `node_parser_core.go:87-111`) — иначе
`net/url` отбивает весь узел. Это свойство декодера `url`, а не схемы:
в движке — часть реализации `decode: ["url"]`, нормируется в CANON.

---

## 3. P2 — `uri.userinfo`: разбор userinfo

```jsonc
"userinfo": {
  "decode": "base64_if_no_colon",   // опционально
  "split": ":",
  "into": ["method", "password"],    // пути тела по порядку
  "single_into": "password"          // когда разделителя нет
}
```

| Схема | Сегодня | Декларация |
|---|---|---|
| vless/vmess | uuid целиком | `{"into": ["uuid"]}` |
| trojan | пароль = часть ДО `:` (`node_parser_core.go:462-466`) | `{"split": ":", "into": ["password"]}` |
| ss SIP002 | percent → base64 → `method:password` | `{"decode": ["percent","base64"], "split": ":", "into": ["method","password"]}` |
| ss SS2022 | **не поддержан**: открытый `method:key@` уходит в base64-декодер и даёт мусор либо роняет узел (`node_parser_core.go:201`, дроп `:429-435`) | тот же `decode: "base64_if_undecodable"` — при неудаче base64 читается как есть. **Закрывает дефект** |
| tuic | `uuid:password` | `{"split": ":", "into": ["uuid","password"]}` |
| anytls | `Username()` — **режет по первому `:`** (`node_parser_core.go:456`), пароль с двоеточием теряется | `{"into": ["password"]}` без `split` — весь userinfo. **Закрывает дефект** |
| naive | одиночный → password (`node_parser_core.go:466-478`) | `{"split": ":", "into": ["username","password"], "single_into": "password"}` |
| http | три формы `user` / `user:pass` / `:pass` | то же, `single_into: "username"` |
| socks | `user:pass`; base64-userinfo **не поддержан** | `{"decode": "base64_if_no_colon", "split": ":", "into": ["username","password"]}` |
| hysteria2 | пароль | `{"into": ["password"]}` |
| ssh | user; пароль в пару | `{"split": ":", "into": ["user","password"]}` |
| masque | privateKeyDer | `{"into": ["private_key"]}` |
| wireguard | приватный ключ (сырой `/` percent-энкодится до `url.Parse`) | `{"into": ["private_key"]}` + `normalize: base64_std` |

**Метка НИКОГДА не берётся из userinfo** — там секрет
(`node_parser_core.go:518-530`). В грамматике это не запрет, а факт:
`label_fallback` (P11) не содержит источника `userinfo`.

---

## 4. P3/P4 — `source` и `value_map`

`source` делает невозможным дефект «поле знаем, но не читаем»: движок
обходит ВСЕ объявленные параметры, а не те, что вспомнил автор ветки.

**Доказательство необходимости — пять живых дефектов, найденных инвентаризацией:**

| Дефект | Адрес | Реестр знал? |
|---|---|---|
| ss `plugin`/`plugin_opts` (SIP003) не читаются вовсе; слова `plugin` нет во всём `core/` | тело ss пишет только method+password: `node_parser_core.go:920-927` | **Да** — `shadowsocks.json:40-52`, и там прямо написано «Go молча теряет query целиком» |
| SS2022 открытый userinfo уходит в base64-декодер | `node_parser_core.go:194-224` | да, кейс `corpus/uri/shadowsocks/ss2022_blake3` живёт только потому, что ключ оказался валидным base64 |
| `disable_sni` не читается **ни у tuic, ни у masque** | нет в `node_parser_tuic.go` / `node_parser_masque.go`; в Go только эмиттер `outbound_tls_emit.go:84` | **Да** — объявлен у обеих схем с пометкой `ext: mobile` / «Только Dart» |
| masque: свой список написаний `insecure` и своя истинность — **`yes` не работает**, в отличие от всех прочих схем | `node_parser_masque.go:144-148` против общего `uri_params.go:75-81` | да, общий набор из 9 имён — в `tls.json` |
| `socks5h` не распознаётся ни диспетчером, ни таблицей версий | `node_parser_core.go:36-39`, `node_parser_socks.go:27-32` | нет — добавить в `aliases` схемы |

Шестой, того же класса, но в обратную сторону: masque `sni` читается
**точным** `q.Get("sni")` (`node_parser_masque.go:141`) — без реестровых
алиасов, без регистронезависимости, без эвристики и без фолбэка, которые
есть у всех остальных QUIC-схем.

`value_map` — перевод значений чужого диалекта. Первое написание —
каноническое (для эмита).

```jsonc
"type": {
  "source": "query.type", "selector": true, "maps_to": "transport.type",
  "value_map": {"tcp": null, "raw": null, "": null, "h2": "http"}
}
```

| Случай | Файл:строка | `value_map` |
|---|---|---|
| `raw`/`tcp`/пусто = «транспорта нет» | `node_parser_transport.go:162-266` | `→ null` |
| `h2` → `http` (только vmess) | `node_parser_vmess.go:257-271` | `"h2": "http"` |
| uTLS Xray-идентификаторы `HelloChrome_120` → `chrome` | `node_parser_transport.go:53-148` | `value_map` с префиксным режимом: `{"prefix": {"hellochrome": "chrome", …}}` |
| vmess `chacha20-ietf-poly1305` → `chacha20-poly1305` | `node_parser_vmess.go:45-56` | прямое |
| `packetEncoding=none` → ключа нет | `node_parser_core.go:759-768` | `{"none": null}` |
| ss `2022-blake3-*` и 9 legacy-шифров | реестр `allowlists.json` | не `value_map` — судит санитайзер |

**`bool_spelled`** — общий тип, закрывающий семейство `insecure`
(9 написаний, `tls.json`) и все прочие флаги. Истина = `1|true|yes`
(`uri_params.go:70-81`). Три разных правила истинности, живущих сегодня
у vmess (`=="tls"` в JSON-форме `:288`, `∈{1,true,tls}` в legacy `:167-170`,
общее `flagValueTrue`), сводятся к одному типу + `value_map` там, где
значение действительно другое.

---

## 5. P5 — `selector` + `when`: двухпроходность

Владелец назвал три «сложных случая» vless; все три — это селектор.

Движок делает **два прохода**: сперва параметры с `selector: true`
(в порядке объявления), затем остальные, у каждого `when` проверяется по
УЖЕ ПОСТРОЕННОМУ телу и/или по источникам. Грамматика `when` —
существующая (`Condition`/`Relation` в `registry.go:209-225`), расширяется
предикатом `in` и равенством по источнику.

```jsonc
"path": { "source": "query.path", "maps_to": "transport.path",
          "when": {"transport.type": {"in": ["ws","http","httpupgrade","xhttp"]}} },
"serviceName": { "source": "query.serviceName", "maps_to": "transport.service_name",
                 "aliases": ["service_name", "path"],
                 "when": {"transport.type": "grpc"} }
```

| Условная адресация | Файл:строка | `when` |
|---|---|---|
| `path` только у ws/http/httpupgrade/xhttp | `node_parser_transport.go:181-260` | `transport.type in [...]` |
| `serviceName` только у grpc, с фолбэком на `path` | `:212-223` | `transport.type == grpc` + `aliases` |
| ws `host` → `headers.Host`, http `host` → `host[]` (список), httpupgrade `host` → `host` (строка) | `:201-233`, `:257-259` | три записи в `transports.json`, каждая со своим `maps_to` и `when` — **уже так разложено** |
| `mode` только у xhttp | `:342-410` | `transport.type == xhttp` |
| plaintext-порты vless: `security` пуст И порт ∈ {80,8080,8880,2052,2082,2086,2095} | `node_parser_transport.go:150-158`, `:912-914` | `when: {"query.security": "", "port": {"in": [80,…]}}` |
| `sni`/`fp`/`insecure` у http только при https-суффиксе | `node_parser_http.go:131-139` | `when: {"tls.enabled": true}` |
| `mode`/`path`/`host` у xhttp читаются ТОЛЬКО из плоского слоя, остальное из `extra` | `:372-380` | приоритет источников: `source: "query.mode"` без `json.extra.mode` в списке |

**Вариантность транспорта уже разложена** в `transports.json`
(`transports.<type>.params`) — движок читает её как есть, `when` выводится
из положения параметра в дереве. Новой записи не нужно.

---

## 6. P6/P7 — `sets` и `implies`

`sets` — значение параметра (или схемы) даёт **набор присваиваний**.
`implies` — то же, но от одного лишь НАЛИЧИЯ параметра.

```jsonc
"security": {
  "source": "query.security", "selector": true,
  "sets": {
    "tls":     {"tls.enabled": true},
    "reality": {"tls.enabled": true, "tls.reality.enabled": true},
    "none":    {},
    "":        {"tls.enabled": true}
  }
},
"pbk": { "source": "query.pbk", "maps_to": "tls.reality.public_key",
         "implies": {"tls.enabled": true, "tls.reality.enabled": true} }
```

| Случай | Файл:строка | Примитив |
|---|---|---|
| `security=none` → ключа `tls` НЕТ вовсе (не `enabled:false`; явный блок ронял ядра lx.5–lx.18 в SIGSEGV) | `node_parser_transport.go:904-914`, `:1036-1040` | `sets: {"none": {}}` — пустой набор, ключ не появляется |
| REALITY заводится **по наличию `pbk`**, а не по `security=reality` | `:932-948` | `implies` у `pbk` |
| vmess `net=h2` ⇒ TLS принудительно | `node_parser_vmess.go:314-336` | `implies` у `type` при значении `h2` (`sets` на значении) |
| socks: схема → `version` "4"/"4a"/"5" | `node_parser_socks.go:27-32` | `sets` по источнику `scheme` |
| naive: `naive+quic` ⇒ `quic: true` **+ `quic_congestion_control: "bbr"`** (значения в ссылке НЕТ) | `node_parser_core.go:483-492`, `node_parser_naive.go:131-134` | `sets` по `scheme` — оба присваивания сразу |
| http: суффикс схемы `https` ⇒ `tls.enabled` + дефолтный порт 443/80 | `node_parser_http.go:55,69-72` | `sets` по `scheme` (включая `server_port`) |
| `hy2` ⇒ hysteria2, `hy` ⇒ hysteria | `node_parser_core.go:258-263` | `aliases` схемы (уже есть) |
| naive TLS всегда on, урезанный до `enabled`+`server_name` | `node_parser_naive.go:154-160` | `sets` по схеме + `forbidden_for` в `tls.json` (**уже есть**) |
| anytls: `utls.enabled: true` всегда | `node_parser_anytls.go:54-57` | `sets` |

**Асимметрия, которую примитив чинит бесплатно:** naive дописывает
`quic_congestion_control: "bbr"` на входе, а эмиттер его не пишет
(`shareuri_naive.go`) — тело-из-ссылки ≠ тело-из-тела. С `sets` + `omit_default`
обе стороны читают одну запись.

---

## 7. P8/P9 — `extract` / `compose` / `list`

`extract` — регулярка с ИМЕНОВАННЫМИ группами раскладывает одно значение
по нескольким путям тела; `compose` — обратный шаблон для эмита.
Диалект регулярок — общее подмножество RE2 ∩ ECMAScript (как у
существующего `pattern`): без backreference и lookaround.

```jsonc
"path": {
  "source": "query.path",
  "extract": {
    "re": "^(?P<path>[^?]*)(?:\\?ed=(?P<ed>\\d+))?$",
    "into": {
      "path": "transport.path",
      "ed":   {"path": "transport.max_early_data", "type": "int",
               "implies": {"transport.early_data_header_name": "Sec-WebSocket-Protocol"}}
    }
  },
  "compose": "{transport.path}?ed={transport.max_early_data}",
  "when": {"transport.type": "ws"}
}
```

| Случай | Файл:строка | Примитив |
|---|---|---|
| ws `?ed=N` в пути → 2 поля (+ дефолтное имя заголовка) | `node_parser_transport.go:696-751`; обратно `:778-790` | `extract` + `compose`. **Память проекта: `?ed=N` жил в 4 слоях** — теперь в одном |
| httpupgrade: `?ed=` срезается, значение отбрасывается | `:242-256` | тот же `extract`, `into` без `ed` |
| hysteria multi-port в authority `host:1000-2000,3000` | `hysteria2_ports.go:75-138`, хук `node_parser_core.go:383-388` | `extract` по источнику `authority` → `server` + `server_ports` |
| `mport=1000-2000,3000` → `["1000:2000","3000:3000"]` (ядру нужно ДВОЕТОЧИЕ, дефис = фатал) | `hysteria2_ports.go` | `extract` + `normalize: range_order` |
| `upmbps=100 mbps` → `100` | реестр `hysteria.json` mapper | `extract` `^(?P<v>\d+)` |
| tuic `heartbeat=10` → `"10s"` | `node_parser_tuic.go:10-20` | `compose`-подобный `normalize: duration_bare_seconds` |
| http/naive `headers=`/`extra-headers=` строкой → объект | `node_parser_naive.go:81-115`, http `:113-126` | `list: {"sep": "\r\n"}` + `extract` `^(?P<k>[^:]+):\s*(?P<v>.*)$` → `object` |
| IPv6-скобки в authority | `node_parser_vmess.go:86-115` (ручной разбор у vmess-legacy) | свойство декодера `url`; для форм без `url` — `extract` |
| alpn/host_key/host_key_algorithms/address/allowedips через запятую | `:838-846`, `node_parser_ssh.go:43-69` | `list: {"sep": ","}` |
| bare IP → `/32` / `/128` | `wireguard.json` mapper | `normalize: cidr_prefix` |
| `flow=xtls-rprx-vision-udp443` → flow + packet_encoding | `node_parser_core.go:743-758` | `value_map` + `sets` |

---

## 8. P10/P11 — `default_from` и `label_fallback`

### SNI-эвристика (`default_from`)

В коде сегодня **три разных правила** для одного и того же:

| Правило | Файл:строка |
|---|---|
| `sni` → server (простой фолбэк) | `node_parser_transport.go:916-919`, `:1042-1045` (vless/trojan) |
| `sni` принимается ТОЛЬКО если содержит `.` или `:`, иначе server | `:1014-1021` (`tlsServerNameFromQuery`, зовут anytls/QUIC) |
| `sni` → `peer` → server | `node_parser_core.go:889-898` (vmess) |

Одна декларация вместо трёх:

```jsonc
"sni": { "source": "query.sni", "aliases": ["peer"], "maps_to": "tls.server_name",
         "default_from": "server",
         "on_invalid": {"action": "default_from", "when": {"value": {"not_matches": "[.:]"}}} }
```

Третья ступень `host` объявляется только у trojan (у vless `host` — это
Host транспорта) — **уже так и записано** в `trojan.json` через `aliases`.

Конвенционные дефолты (`default_when`, **уже есть** в реестре):
`fp` пустой → `random` у vless/anytls (D-009); `vhttp` пустой → `h3` у masque;
`profile` → `cloudflare`; `allowedips` → `0.0.0.0/0,::/0`; WG-порт → 51820;
ssh user → `root`; hysteria v1 полоса → 100 Мбит.

### `label_fallback` (P11) — ответ на находку LxBox

Сегодня: `generateDefaultTag(scheme, server, port)` = `"%s-%s-%d"`
(`node_parser_core.go:640-642`) — **по СХЕМЕ**. Отсюда `hy2-…` против
`hysteria2-…`, `socks5-…` против `socks-…`: одна и та же нода получает
разные теги в зависимости от написания ссылки, и `socks5` намеренно НЕ
канонизируется именно потому, что канонизация переименовала бы живые узлы
(`node_parser_socks.go:13-15`). Это и есть сдвиг identity, о котором
сообщила сторона LxBox (у них — на `ss://`).

Декларация — по ТИПУ ТЕЛА, с явной фиксацией legacy-написаний:

```jsonc
"label_fallback": {"template": "{scheme}-{server}-{server_port}",
                   "scheme_source": "as_written"}
```

`as_written` фиксирует сегодняшнее поведение (identity живых узлов не
двигается); переключение на `singbox_type` — **отдельное решение владельца**,
оно переименует существующие узлы. Вынесено вопросом в SPEC §13.

Цепочка метки целиком (`node_parser_core.go:494-545`): `fragment`
(`PathUnescape`, не Query — `+` во фрагменте литерален) → `path` без
ведущего `/` → `label_fallback`. У vmess JSON-формы источник — `json.ps`,
и фрагмент игнорируется (`node_parser_vmess.go:237-246`): в грамматике это
`"label": {"source": "json.ps"}` внутри формы `v2rayn`.

---

## 9. P12 — обратное направление (эмит)

Эмит — та же таблица наоборот: `maps_to⁻¹`, `value_map⁻¹`, `sets`
сопоставлением, `compose`, `into⁻¹` у userinfo, `emit_form`, `param_order`.

**Сегодня `param_order` реестра не читает НИКТО**: грепом по `core/`
атрибута нет, порядок даёт `q.Encode()` — алфавитная сортировка `net/url`.
Канон имени параметра (`QueryAliases`, первое имя) читает только
`uri_params.go:59` — и только на ВХОДЕ; эмиттеры vless/trojan/vmess зашивают
имена литералами.

Что эмиттер делает сверх «поле → параметр»:

| Условие эмита | Файл:строка | Примитив |
|---|---|---|
| `fp=random` НЕ пишется у vless, пишется у trojan/vmess | `shareuri_vless.go:49-51,65-67` против `shareuri_trojan.go:31-33` | `omit_default` — асимметрия исчезает |
| `encryption` пишется ВСЕГДА (пусто → `none`) | `shareuri_vless.go:81-87` | `emit_when: "always"` |
| отсутствие блока `tls` ⇒ явный `security=none` | `shareuri_vless.go:18-26`, `shareuri_trojan.go:15-22` | `sets⁻¹` |
| REALITY-ветка пишет `pbk` и НЕ пишет `security=reality` | `shareuri_vless.go:27-56` | `implies⁻¹` |
| схема по телу: `version` → socks4/4a/5; `quic` → naive+quic; `tls` → proxy-https | `node_parser_socks.go:58-66`, `shareuri_naive.go:24-27`, `shareuri_http.go:63-79` | `emit_form` + `sets⁻¹` |
| ssh дефолты дописываются (`root`, 22) | `shareuri_ssh.go:56-58,64-66` | `omit_default` наоборот — вопрос §13 |
| `ErrShareURINotSupported`: нет обязательных полей / ss-метод вне allowlist / `private_key` списком ≥2 | `shareuri_ss.go:16-21`, `shareuri_ssh.go:80-82` | `required` реестра (**уже есть**) |
| **vmess: `httpupgrade` → `net:"ws"`** — транспорт деградирует | `shareuri_vmess.go:70-73` | `value_map⁻¹` обязан быть инъективен; линтер §11 это ловит |
| **vmess xhttp: весь набор XHTTP-полей теряется** (xmux, padding, sc*) | `shareuri_vmess.go:74-83` | таблица одна — потеря невозможна |
| `early_data_header_name` не эмитится (нестандартный заголовок теряется) | `shareuri_helpers.go:113-117` | `compose` |
| лимит длины URI на выходе **не проверяется** (`max_uri_length` только на входе) | `node_parser_core.go:130-132` | `limits.json` на обе стороны |

**Оговорки LxBox (эмит — отдельный осторожный этап):** у них `toUri` — ещё
и форма хранения ручных узлов (`origin.raw`), и корпус знает не-round-trip
случаи: явный `path=/`, >2 уровней percent-кодирования, ссылка длиннее
`max_uri_length`. У нас соответствующие исключения уже перечислены в
раннере (`contract_emit_test.go:190-217`: D-028 путь с `%2F`, ws-Host из sni).
Эти случаи — **данные** (`round_trip: false` с причиной), не код.

---

## 10. Xray-JSON — тот же движок

Xray-вход — **второй рукописный парсер**: ~2100 строк, из них ~530 чистой
per-scheme развилки (`xray_protocols.go:97-112` диспетчер, билдеры
`xray_outbound_convert.go:103-217` + `xray_protocols.go:186-352` +
`xray_hysteria.go:28-128`, switch транспортов `:330-405`, switch TLS `:223-283`).
**Реестр он не читает вообще** — ни один из пяти файлов не импортирует
`core/config/registry`; правила догоняют узел лишь ниже, в `node_materialize.go:56`.
При этом реестр Xray **описывает**: `sources: [… "xray"]` у семи схем и целая
секция `xray_dialect` в `hysteria.json`.

Выразимость: форма без схемы, `space: json`, параметры с `source: "json.<path>"`.

```jsonc
{ "id": "xray", "detect": {"json_has": "protocol"}, "space": "json" }
```

| Xray-путь | Примитив |
|---|---|
| `settings.vnext[0].users[0].id` → `uuid` | `source: "json.settings.vnext.0.users.0.id"` |
| `streamSettings.network` → транспорт | `selector` + `value_map` |
| `streamSettings.security` → tls | `selector` + `sets` |
| `xhttpSettings` ∥ `splithttpSettings` | `aliases` источника |
| `httpSettings.host` скаляр → массив | `list` с `coerce_scalar` |
| hysteria `obfs` строка → объект `{type,password}` | `sets` |
| один `protocol:"hysteria"` → два sing-box type по `version` | `selector` по `json.hysteriaSettings.version` + `sets` |

**Вне движка остаётся не перевод, а СБОРКА документа** — и это правильная
граница: `dialerProxy` → `node.Chain[]` с разрывом колец и лимитом глубины
(`xray_json_array.go:716-781`), два прохода владения (`:340-434`),
`balancers[0]` → узел-группа (`xray_balancer.go:39-86`), дедуп и slug тегов
(`:844-901`). Это работа над МАССИВОМ узлов и связями между ними, а не над
одним узлом; маппер одного узла её не касается. То же и с распаковщиком
Amnezia `vpn://` (сжатый профиль → `.conf`/JSON, `node_parser_amnezia.go:50`):
обёртка над текстом, выдающая вход тому же движку.

Рекомендация: Xray **в объёме** (волна W8), но последней — после того как
ссылочный вход доказал грамматику на 16 схемах.

---

## 11. Линтер полноты (W0, обязателен)

| Проверка | Что ловит |
|---|---|
| у каждого `uri`-параметра есть `source` | «объявлен, но не читается» |
| каждый `source` разрешим в пространстве формы | опечатка в пути |
| каждый `maps_to` существует в `body.fields` | разъезд с телом |
| `value_map` инъективен там, где схема эмитится | vmess `httpupgrade→ws` (§9) |
| каждый `when` ссылается на объявленный путь/источник | висячее условие |
| `extract.re` компилируется в обоих диалектах; группы покрыты `into` | RE2 ∩ ECMAScript |
| `param_order` покрывает все эмитируемые параметры | молчаливая потеря при эмите |
| движок читает только объявленное | дефект «читаем мимо реестра» |

Последний пункт обеспечен **конструктивно**: движок получает значение
только через `source`, доступа к сырому `url.Values` у него нет. Греп-страж
как дополнительная мера — вопрос §13 (владелец ранее отменял сторожевой
тест «на остатки»).

---

## 12. Остаток невыразимого

После разбора — **пусто**. Три случая, которые выглядели невыразимыми, и
чем закрыты:

| Случай | Почему казался особым | Чем закрыт |
|---|---|---|
| `ech=` — параметр без адреса в теле | некуда `maps_to` | `maps_to: null` + `on_invalid: {action: "drop", code: "ech_ignored"}` — **уже есть** в `tls.json` |
| naive `padding=` — снимается с кодом | то же | то же |
| xhttp `extra` — вложенный слой с инверсией приоритета для трёх ключей | два источника одного параметра | `source` — список с приоритетом; для `mode`/`path`/`host` список из одного элемента |

Вне маппера сознательно остаются (§10): сборка массива узлов Xray, связи
`dialerProxy`, дедуп/владение, распаковщик Amnezia, разбор тела подписки.

---

## 13. Сводка по схемам

Доля параметров, выразимых **плоской** записью `source`+`maps_to` (без
`when`/`sets`/`extract`), и число структурных примитивов на схему:

| Схема | Параметров | Плоских | Структурных примитивов | Основные |
|---|---|---|---|---|
| trojan | 7 + userinfo | 4 (57%) | 4 | selector `type`, `sets` security, `when` транспорта, sni `default_from` |
| vless | 6 + общие TLS/транспорт | 3 (50%) | 7 | + plaintext-порты, `implies` pbk, `extract` ed, `value_map` flow |
| vmess | 18 | 10 (56%) | 6 | 2 формы, `implies` h2, `label` из `ps`, `value_map` scy |
| shadowsocks | 4 + userinfo | 2 (50%) | 3 | 3 формы userinfo, `decode`, plugin (**новое**) |
| socks | 0 + userinfo | 0 | 2 | `sets` по схеме, `decode` userinfo |
| http | 7 + userinfo | 4 (57%) | 3 | `sets` по суффиксу схемы, `when` tls, headers `extract` |
| ssh | 6 + userinfo | 4 (67%) | 2 | `list` ×2, взаимоисключение key/path |
| naive | 2 + userinfo | 1 | 4 | `sets` по схеме (+bbr), headers, `single_into` |
| anytls | 10 | 7 (70%) | 3 | duration, sni-эвристика, `implies` pbk |
| hysteria | 11 | 7 (64%) | 4 | mport `extract`, mbps-суффикс, obfs, полоса |
| hysteria2 | 12 | 8 (67%) | 4 | base64-форма, multi-port authority, obfs, mport |
| tuic | 9 | 7 (78%) | 2 | userinfo `split`, duration |
| masque | 11 | 8 (73%) | 3 | userinfo ключ, `default_when` vhttp/profile |
| wireguard/awg | 29 | 22 (76%) | 5 | 2 формы (+ini), `normalize` ключей, cidr, `any_set` род |
| **Итого** | **~135** | **~87 (64%)** | **~52** | |

52 структурных места закрываются **13 группами примитивов** — ни одного
именованного крючка и ни одной схемной функции в коде.

Полные черновики секций `uri` в новой грамматике — `SCHEMES.md`
(по схеме на раздел, JSON целиком, главный артефакт для сверки с LxBox).

---

## 14. Чек-лист выразимости LxBox (фича 480) — где ответ

| Пункт | Ответ |
|---|---|
| (а) vmess: две формы, один набор `maps_to` | `SCHEMES.md` §0.2 (`source` картой по формам) и §3 — JSON целиком |
| (б) ss legacy `base64(authority)`; SS2022 split по ПЕРВОМУ `:` | `SCHEMES.md` §0.3 (`reparse`), §4 (`split{sep,limit:2}`, `base64?`) |
| (в) hy2 base64-обёртка + multi-port в authority | `SCHEMES.md` §0.1 (источник `port_raw`), §7 (`$multiport` через `extract`) |
| (г) scheme несёт тело; socks base64-userinfo с фолбэком | `SCHEMES.md` §5 (`scheme_sets`, `decode: "base64?"`) |
| (д) awg base64-conf и `.conf` — одна таблица `ini.*`; списки с типом | `SCHEMES.md` §9 (`source` картой url/ini), `list{item:"int",len:3}` |
| (е) ssh списки; многострочный PEM | `SCHEMES.md` §6 (`list{sep:","}`), норма `+` — §0.4 |
| (ж) SNI: цепочка источников, не у всех схем | `SCHEMES.md` §1/§6 (`source` списком + `default_from` с предикатом); у masque сегодня её нет вовсе — §8 |
| (з) tag-фолбэк по типу тела | `SCHEMES.md` §0.5; **переключение переименует живые узлы** → вопрос владельцу `SPEC.md` §13 |
| (и) `?ed=N`: неявный заголовок не писать в ссылку | `SCHEMES.md` §11 — `implicit: true` в `implies`, влияет только на эмит |
| (к) `+` в query не пробел | §2 выше (четыре заплаты) + риск identity `SPEC.md` §9.3 |


---

## 15. Примитивы расширенной области (все источники) — 19.09.2026

Добавлены после расширения кампании со ссылок на ВСЕ источники узла и после
сверки с LxBox. Нумерация продолжает §1 (P1–P12).

| № | Примитив | Где | Что выражает | Ради чего |
|---|---|---|---|---|
| P13 | `detect` | у секции, у формы, у вида источника | «этот контент — мой»: `regex` \| `json{required_keys,any_keys,key_absent,type_of,value_in}` \| `ini{sections,keys}` \| `text{prefix_fold,line_fold}` + `not`/`all`/`any`/`default` | убирает рукописный сниффер формата (`body_classify.go`, каскад из 25 схем) |
| P14 | `mappers.<kind>` | у протокола | отдельная таблица на вид источника: `uri`/`xray`/`singbox`/`conf` | решение владельца «мапперы могут быть разные» |
| P15 | `lift` / `flatten` | у записи | перенос между уровнями вложенности: `outbound ↔ endpoint`, `peers[] ↔ плоские`, `extra.xmux.* → плоский слой` | диалекты форков; сегодня `xrayFlattenScalars` вручную |
| P16 | `include` / `$ref` блока | у секции | переиспользуемый блок с ДИАЛЕКТНЫМИ вариантами (`transports.ws#uri` против `#xray`) | не дублировать transport/tls/multiplex между протоколами и диалектами |
| P17 | `unknown_key{action,code}` | у секции | что делать с неперечисленным ключом: `keep`+код (singbox) / `drop`+код (xray) | `uri_param_unknown`, `json_field_unknown` — решение владельца (1 = Б) |
| P18 | `when.source_has` / `when.form` | у записи | условие по ИСТОЧНИКУ и ФОРМЕ, а не по телу | **G1**: род AWG объявляет вход (`hasAWGParams(q)` читает query), в теле awg-ключей может не остаться |
| P19 | `sets: {path: null}` | у записи | `null` = **снять** путь (отличается от «не писать») | **G2**: tuic `disable_sni=1` убирает `tls.server_name` |
| P20 | `priority` + `merge{keep_first,overwrite}` | у записи | явный порядок двух записей в один путь | **G3**: `flow=…-udp443` против `packetEncoding=` |
| P21 | `sort_keys` | у `type: object` | детерминированный порядок ключей объекта | **G4**: заголовки http/naive входят в identity; сегодня порядок — случайность реализации Go-map |
| P22 | `label.normalize` | у метки | `strip_control`, `trim`, `value_map` над НЕПУСТОЙ меткой | **G8**: метка = identity; сегодня только `normalizeFlagTag` (🇪🇳→🇬🇧) |
| P23 | `materialize_default` | у записи | маппер ЗАПИСЫВАЕТ дефолт в тело, даже когда источник молчал | vmess `security` (у нас уже так — `xray_protocols.go:208`; у LxBox опущенный ключ снимал узел) |
| P24 | `body_source` | у секции | каким `source` тело приходит в санитайзер | `except_sources` в правилах реестра; у нас 5 значений, у LxBox — 2 |
| P25 | `unwrap` + `redetect` + `max_unwrap_depth` | у вида источника | оболочка-декодер с рекурсивным передетектом и пределом глубины | base64-подписка, `vpn://` (zlib+base64); предела сегодня нет вовсе |
| P26 | источник `ini.$comment.<Section>` | у записи | значение из комментария секции | **G7**: имя узла из комментария под `[Peer]` (`wgconf_text.go:82-107`) |

### 15.1. Уточнение `decode_extra` (снимает противоречие черновика)

Черновик `SCHEMES.md` писал `{"percent": 2}` и `alpn`, и `path`. **Ошибка**:
по коду это разные режимы, и двойка сломала бы фикстуру.

| | Сегодня | Норма |
|---|---|---|
| `alpn` | `QueryUnescape` **до стабилизации, без предела** (`node_parser_transport.go:24-33`) | `{"mode": "query", "passes": "until_stable", "max": 16}` |
| `path` | ровно **2** прохода `PathUnescape` — **оба** с path-семантикой (`:755-776`) | `{"mode": "path", "passes": 2}` |

**Ответ на предложение LxBox «первый проход — query-семантика, повторный —
только percent»: НЕТ.** У нас оба прохода — `PathUnescape`, то есть `+`
литерален с самого начала. Выбран вариант, который **не двигает identity**:

- query-семантика на первом проходе превратила бы `/ws+v2%2Fdata` в
  `/ws v2/data` — сервер отвечает 404, узел «жив» и молча не работает
  (обоснование прямо в комментарии `:765-768`);
- `%2F` и прочие проценты `PathUnescape` и `QueryUnescape` декодируют
  одинаково, поэтому разница ровно в трактовке `+`;
- битый percent сохраняется как есть — фикстура
  `uri/trojan/ws_path_broken_percent_kept` (`path=%2Fx%25zz`) требует именно
  этого, а не снятия значения.

Норма: `{"mode": "path", "passes": 2}`, где `mode: path` означает
path-семантику на **каждом** проходе.

Фикстура `uri/vless/alpn_multiply_encoded.uri` несёт `http%2525252F1.1` —
**три** уровня, ожидание `["http/1.1"]`. Семантика различается намеренно:
в пути литеральный `+` легален, `QueryUnescape` превращал `/ws+v2` в
`/ws v2` → 404.

### 15.2. Норма разбора authority

Authority разбирает **лексер движка**, не платформенный парсер URL:
`host:443,20000-30000` (multi-port hysteria) `Uri.tryParse` у Dart отвергает
целиком. Сырая строка порта доступна как `port_raw` (§0.1 `SCHEMES.md`).
У нас это уже так — `hysteria2_ports.go` работает с сырым authority через
хук `node_parser_core.go:383-388`.

### 15.3. Инвентаризация прочих входов (адреса)

**Xray-JSON** — `PRIMITIVES.md` §10 и `SPEC.md` §3.4a. Ключевое сверх §10:
диспетчер `xray_protocols.go:97-112` плюс **второй, скрытый**
(`xray_json_array.go:789`, `if protocol == "socks"` — socks только как хоп);
три формы эндпоинта (`vnext[]`, `servers[]`, плоская `settings.{address,port}`
у hysteria), причём выемка vnext **продублирована дословно**
(`xray_outbound_convert.go:104-131` ≡ `xray_protocols.go:117-144`);
`vnext[1..]`/`servers[1..]`/`users[1..]` отбрасываются молча; `mux` и весь
`sockopt` кроме `dialerProxy`/`dialer` не читаются; `network` вне набора и
`security: "xtls"` теряются без кода (`:403-405`, `:223-225`);
`httpSettings.host` как массив даёт мусор `["[a.com b.com]"]` через
`fmt.Sprint` (`:26`, `:363-365`).

**sing-box JSON** — `SPEC.md` §3.4. Формы: одиночный outbound, массив
outbound'ов, целый конфиг, массив конфигов (`body_classify.go:145-196`);
`outbounds` ++ `endpoints` склеиваются в один список
(`singbox_import.go:300-316`); единственное переименование типа —
`shadowsocks → ss` (`:390-409`); `singboxTypeIsAddressless` = `wireguard` ∨
`tailscale` (`:429-431`) — `if type ==` в чистом виде;
`singboxCredentialFromMap` — switch по схеме из 8 литералов (`:437-449`);
`SanitizeSingboxOutboundMap` **всегда возвращает `nil`** (`:71`), то есть ни
одна деградация sing-box-входа не доезжает до узла; reject'ы идут **без
машинного кода** (`add` вместо `addCoded`).

**`.conf` / ini** — `SCHEMES.md` §9. Свой парсер, 36 строк
(`node_parser_amnezia.go:466-501`): ключи lowercase, значения as-is,
комментарии `#` и `;` только целой строкой (inline **не** поддержан),
повторяющийся ключ — **последний выигрывает**, секции кроме
`interface`/`peer` игнорируются. Чтутся поля **только первой** `[Peer]`
(`:494-497`) — прочие молча отброшены. Каждая `[Interface]` даёт отдельный
узел (`wgconf_text.go:26-49`). `Interface.DNS` ставится в query
(`:555-557`) и **не читается никогда** — by-design по
`wireguard.json:124-131`, но молча. Пять независимых реализаций проверки
`[Interface]`/`[Peer]`.

**Amnezia `vpn://`** — обёртка (base64url → qCompress: 4 байта BE размер +
zlib), отдаёт **текст `.conf`** (`:96-164`, `:313-360`); выбор контейнера —
default первым, при нескольких WG-контейнерах код `amnezia_container_choice`
(`:174-206`); множественный путь `ParseAmneziaVPNLinkAll` (`:252-303`)
возвращает все контейнеры.

**Clash YAML — поддержки НЕТ.** Ни парсера, ни зависимости; все вхождения
`clash` в коде — про Clash API ядра. В `sources.json` не объявляется.


### 15.4. Политика `+` в query (рабочее решение 19.09.2026)

Норма **не меняется**: `+` в query = пробел (form-encoding). Так же наш эмит и
**пишет**: все 11 эмиттеров идут через `q.Encode()` (`shareuri_*.go`), который
кодирует пробел как `+`.

Исключение выводится **из реестра, а не из списка имён**: у поля с
base64-форматом (`format: base64`, `base64_32`) движок читает `+` буквально —
атрибут `decode.plus_literal`, по умолчанию выводимый из `format`, с
возможностью явного указания. Одно правило заменяет четыре сегодняшние
заплаты (`PRIMITIVES.md` §2); заплаты удаляются при переходе схемы на движок.

`password` / `obfs-password` / `host` — `+` = пробел, **как сейчас**: тела
рабочих узлов не меняются. Дельта — `DELTAS.md` D133-7.

### 15.5. Приоритет написаний имени параметра

Имена параметров query читаются **регистронезависимо** (`queryGetFold`), и
корпус на это опирается (`AllowInsecure=1`, `SNI=`, `Fp=` — живые формы).

**Дефект, вскрытый сверкой:** `queryGetFold` обходит `url.Values` — Go-map, —
поэтому когда в ОДНОЙ ссылке присутствуют два написания одного имени
(`?sni=a&SNI=b`), победитель **недетерминирован между запусками**.

Норма: **побеждает точное совпадение с каноном; при его отсутствии — первое
по порядку появления в ссылке.** Требует сохранения порядка параметров,
которого `url.Values` не даёт, — ещё один довод за собственный лексер
authority/query в движке (§15.2).

Дубли ОДНОГО написания (`?fp=a&fp=b`): берётся первый — сегодняшнее
поведение (`vs[0]`), фиксируется явно.