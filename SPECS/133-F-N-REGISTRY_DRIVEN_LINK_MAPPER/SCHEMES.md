# SCHEMES — черновики секций `uri` в грамматике примитивов

SPEC 133. Главный артефакт для сверки с LxBox (их фича 480).
Грамматика — `SPEC.md` §3, инвентаризация и обоснования — `PRIMITIVES.md`.

Черновики **не финальные**: это предложение формы. Значения и коды берутся
из сегодняшнего реестра без изменений — кампания переносит РЕШЕНИЯ в данные,
а не меняет поведение (критерий приёмки: корпус зелёный без единого
изменённого ожидания, `SPEC.md` §8).

Сокращения: опущены `desc_en`/`desc_ru`/`impl` (переносятся из текущего
реестра дословно) и параметры, полностью выразимые плоской парой
`source`+`maps_to` — они перечислены строкой «плоские».

---

## 0. Общие определения движка

### 0.1. Пространство источников

```jsonc
// Заполняется формой (P1). Параметр адресует источник строкой.
{
  "scheme": "vless",              // как написано в ссылке
  "authority": "user@host:443",   // сырая authority, до разбора
  "userinfo": "…", "userinfo.user": "…", "userinfo.pass": "…",
  "host": "example.com", "port": 443, "port_raw": "443",
  "path": "/ws", "fragment": "label",
  "query.<name>": "…",            // с aliases, регистронезависимо
  "json.<path>": …,               // форма со space: json
  "ini.<Section>.<Key>": "…"      // форма со space: ini
}
```

`port_raw` — **сырая** строка порта до приведения к int: из неё читается
multi-port hysteria (`443,8443-8450`), который в `port` не помещается
(ответ на пункт (в) чек-листа LxBox).

### 0.2. Несколько источников у одного параметра

Пункт (а) чек-листа LxBox: у vmess две формы с разными пространствами, но
ОДИН набор `maps_to`. Форма записи — список, первый найденный выигрывает;
ключ формы допустим, когда источники по формам разные:

```jsonc
"uuid": { "source": ["json.id", "userinfo.user"], "maps_to": "uuid" }
// эквивалентно, когда нужна явная привязка к форме:
"uuid": { "source": {"v2rayn": "json.id", "url": "userinfo.user"}, "maps_to": "uuid" }
```

Тот же механизм закрывает приоритетные цепочки, живущие сегодня в коде:
hysteria `auth → auth_str → authStr → password → userinfo`
(`node_parser_hysteria.go:81-88`), WG `privatekey` userinfo→query
(`node_parser_wireguard.go:43-61`), grpc `serviceName → service_name → path`.

### 0.3. `decode` и `reparse`

Пункт (б): у ss legacy в base64 лежит ВСЯ authority. Шаг декодера,
возвращающий текст, помечается `reparse` — текст проходит разбор заново:

```jsonc
{ "id": "legacy", "detect": {"after_scheme": "base64", "decoded_has": "@"},
  "decode": ["base64", {"reparse": "authority"}], "space": "url" }
```

`"base64?"` — try-decode с фолбэком «не base64 → как есть» (пункт (г),
socks userinfo; он же чинит SS2022, `PRIMITIVES.md` §3).

### 0.4. Норма percent-декодирования и `+`

Пункт (к) — **риск identity, описан отдельно** в `SPEC.md` §9.3.
Норма: движок принимает сырой ТЕКСТ и делает percent-декод сам, **без
замены `+` на пробел**. Сегодня у нас `url.Values` (`ParseQuery`) заменяет,
и это обойдено ТРЕМЯ разными заплатами в трёх местах:

| Заплата | Адрес | Что чинит |
|---|---|---|
| `queryParamPreservePlus` — ручной разбор `RawQuery` + `PathUnescape` | `node_parser_wireguard.go:360-375` | `publickey`, `presharedkey` |
| `queryParamPreservePlusOrGet` | `awg3.go:263-270` | AWG3 `header_protection_key` |
| `strings.ReplaceAll(v, " ", "+")` — обратная замена | `node_parser_wireguard.go:60` | WG `privatekey` из query |
| тот же `queryParamPreservePlus` | `node_parser_masque.go:58` | masque `publickey` |

Вне этих четырёх параметров `+` в значении query **сегодня становится
пробелом**. Единый декодер без замены — правильное поведение, но он меняет
значения там, где заплаты не стояли. Порядок действий — `SPEC.md` §9.3.

### 0.5. `label_fallback` — по типу тела

Пункт (з): шаблон берёт **тип тела**, не схему. Объявляется один раз в
общем разделе, схемы не переопределяют:

```jsonc
// registry/common.json
"label_fallback": { "template": "{scheme}-{server}-{server_port}",
                    "scheme_source": "as_written" }
```

`as_written` = сегодняшнее поведение (`generateDefaultTag(scheme, …)`,
`node_parser_core.go:640-642`): `hy2-…` ≠ `hysteria2-…`, `socks5-…` ≠ `socks-…`.
Переключение на `"scheme_source": "singbox_type"` — то, чего просит LxBox, —
**переименует живые узлы**, поэтому вынесено вопросом владельцу
(`SPEC.md` §13, вопрос 3).

---

## 1. trojan — пилот (W1)

Простейшая схема с полным набором: селектор транспорта, `sets` у TLS,
фолбэк-цепочка SNI, общие суб-схемы.

```jsonc
"uri": {
  "forms": [ {"id": "url", "detect": {"default": true},
              "decode": ["url"], "space": "url"} ],
  "userinfo": { "split": {"sep": ":", "limit": 2}, "into": ["password"] },
  "fragment": "label",
  "defaults": { "server_port": 443 },
  "query": {
    "type": { "source": "query.type", "selector": true,
              "maps_to": "transport.type",
              "value_map": {"": null, "tcp": null, "raw": null, "h2": "http"} },
    "headerType": { "source": "query.headerType", "selector": true,
                    "maps_to": "transport.type",
                    "when": {"query.type": {"in": ["tcp", "raw", ""]}},
                    "value_map": {"http": "http", "none": null, "": null} },
    "security": { "source": "query.security", "selector": true,
                  "sets": { "tls":  {"tls.enabled": true},
                            "none": {},
                            "":     {"tls.enabled": true} } },
    "sni":  { "source": ["query.sni", "query.peer", "query.host"],
              "maps_to": "tls.server_name", "default_from": "host" },
    "fp":   { "source": "query.fp", "maps_to": "tls.utls.fingerprint",
              "value_map": {"$ref": "tls.fp_dialect"},
              "implies": {"tls.utls.enabled": true} },
    "alpn": { "source": "query.alpn", "maps_to": "tls.alpn",
              "list": {"sep": ","}, "decode_extra": {"percent": 2} },
    "insecure": { "source": "query.insecure", "type": "bool_spelled",
                  "maps_to": "tls.insecure" }
  },
  "emit": { "form": "url", "param_order": ["alpn","fp","host","insecure",
            "path","security","serviceName","sni","type"],
            "omit_default": ["security"] }
}
```

Транспортные `path`/`host`/`serviceName` не перечисляются: они приходят из
`transports.json`, где уже разложены по типам с нужными `maps_to`
(`transports.ws.host → transport.headers.Host`, `transports.http.host →
transport.host` списком). Движок берёт их по `transport.type`, выставленному
селектором.

**Почему пилот именно trojan:** 66 кейсов корпуса, все четыре класса
примитивов (selector, sets, цепочка источников, list), но нет ни форм, ни
`extract`, ни `implies`-REALITY — грамматика проверяется без экзотики.

---

## 2. vless (W2) — три «сложных случая» владельца

```jsonc
"uri": {
  "forms": [ {"id": "url", "detect": {"default": true},
              "decode": ["url"], "space": "url"} ],
  "userinfo": { "into": ["uuid"], "required": true },
  "fragment": "label",
  "defaults": { "server_port": 443 },
  "query": {
    // (1) type → блок транспорта
    "type": { "source": "query.type", "selector": true,
              "maps_to": "transport.type",
              "value_map": {"": null, "tcp": null, "raw": null} },
    "headerType": { "source": "query.headerType", "selector": true,
                    "maps_to": "transport.type",
                    "when": {"query.type": {"in": ["tcp", "raw", ""]}},
                    "value_map": {"http": "http", "none": null, "": null} },

    // (2) security=reality + plaintext-порты
    "security": { "source": "query.security", "selector": true,
                  "sets": { "tls":     {"tls.enabled": true},
                            "reality": {"tls.enabled": true,
                                        "tls.reality.enabled": true},
                            "none":    {},
                            "":        {"tls.enabled": true} } },
    "$plaintext_port": { "source": "query.security", "selector": true,
                         "when": {"query.security": "",
                                  "port": {"in": [80,8080,8880,2052,2082,2086,2095]}},
                         "sets": {"": {}} },
    "pbk": { "source": "query.pbk", "maps_to": "tls.reality.public_key",
             "decode_extra": {"preserve_plus": true},
             "implies": {"tls.enabled": true, "tls.reality.enabled": true} },
    "sid": { "source": "query.sid", "maps_to": "tls.reality.short_id",
             "when": {"tls.reality.enabled": true} },
    "key_share": { "source": "query.key_share",
                   "maps_to": "tls.reality.key_share",
                   "when": {"tls.reality.enabled": true} },

    // (3) ?ed=N — один параметр, два поля тела
    // объявлен в transports.json (общий для vless/vmess/trojan), см. §12

    "flow": { "source": "query.flow", "maps_to": "flow",
              "value_map": {"xtls-rprx-vision-udp443": "xtls-rprx-vision"},
              "sets": {"xtls-rprx-vision-udp443": {"packet_encoding": "xudp"}} },
    "packetEncoding": { "source": "query.packetEncoding",
                        "maps_to": "packet_encoding",
                        "value_map": {"none": null} },
    "encryption": { "source": "query.encryption", "maps_to": "encryption",
                    "normalize": "trim", "value_map": {"": null, "none": null} },
    "fp": { "source": ["query.fp", "query.fingerprint"],
            "maps_to": "tls.utls.fingerprint",
            "value_map": {"$ref": "tls.fp_dialect"},
            "default_when": {"absent": true, "value": "random"},
            "implies": {"tls.utls.enabled": true} },
    "sni": { "source": ["query.sni", "query.peer"],
             "maps_to": "tls.server_name", "default_from": "host" },
    "alpn": { "source": "query.alpn", "maps_to": "tls.alpn",
              "list": {"sep": ","}, "decode_extra": {"percent": 2} },
    "insecure": { "source": "query.insecure", "type": "bool_spelled",
                  "maps_to": "tls.insecure" }
  },
  "emit": { "form": "url", "param_order": [ /* как сегодня */ ],
            "omit_default": ["fp", "security"],
            "emit_when": {"encryption": "always"} }
}
```

`$plaintext_port` — запись-условие без `maps_to`: её единственная работа —
перебить дефолт `security: ""→ tls.enabled` пустым `sets`. Имя с `$`
помечает служебную запись (в документацию не идёт). Альтернатива —
`sets` с `when` прямо у `security`; выбор формы — вопрос сверки с LxBox.

---

## 3. vmess (W3) — две формы, одна таблица

Пункт (а) чек-листа: разные пространства, один `maps_to`.

```jsonc
"uri": {
  "forms": [
    { "id": "v2rayn", "detect": {"after_scheme": "base64", "decoded_is": "json"},
      "decode": ["base64", "json"], "space": "json" },
    { "id": "legacy", "detect": {"after_scheme": "base64"},
      "decode": ["base64", {"reparse": "url"}], "space": "url" }
  ],
  "emit_form": "v2rayn",
  "userinfo": { "split": {"sep": ":", "limit": 2},
                "into": ["security", "uuid"] },   // legacy: method:uuid@
  "query": {
    "id":   { "source": {"v2rayn": "json.id", "legacy": "userinfo.pass"},
              "maps_to": "uuid", "required": true },
    "add":  { "source": {"v2rayn": "json.add", "legacy": "host"},
              "maps_to": "server", "required": true },
    "port": { "source": {"v2rayn": "json.port", "legacy": "port"},
              "maps_to": "server_port", "type": "int", "required": true },
    "ps":   { "source": {"v2rayn": "json.ps", "legacy": "fragment"},
              "maps_to": "$label" },
    "aid":  { "source": {"v2rayn": "json.aid", "legacy": "query.aid"},
              "maps_to": "alter_id", "type": "int", "omit_default": 0 },
    "scy":  { "source": {"v2rayn": ["json.scy", "json.security"],
                         "legacy": "userinfo.user"},
              "maps_to": "security",
              "value_map": {"chacha20-ietf-poly1305": "chacha20-poly1305",
                            "": "auto", "null": "auto", "undefined": "auto"},
              "emit_when": "always" },
    "net":  { "source": {"v2rayn": "json.net", "legacy": "query.type"},
              "selector": true, "maps_to": "transport.type",
              "value_map": {"": null, "tcp": null, "h2": "http"},
              "sets": {"h2": {"tls.enabled": true}} },
    "tls":  { "source": {"v2rayn": "json.tls", "legacy": "query.tls"},
              "selector": true,
              "sets": {"tls": {"tls.enabled": true}, "": {}, "none": {}} },
    "sni":  { "source": ["json.sni", "json.host"], "maps_to": "tls.server_name",
              "default_from": "host" }
    // host/path/mode/alpn/fp/insecure — плоские, источник по форме
  }
}
```

`"maps_to": "$label"` — путь не в тело, а в метку узла: так выражается
«у JSON-формы имя берётся из `ps`, а фрагмент игнорируется»
(`node_parser_vmess.go:237-246`).

`net=h2 ⇒ TLS` (`node_parser_vmess.go:314-336`) — `sets` на значении
селектора, без отдельной ветки.

**Риск при эмите:** сегодня `httpupgrade → net:"ws"`
(`shareuri_vmess.go:70-73`) — `value_map` неинъективен, транспорт
деградирует. Линтер W0 это ловит (`SPEC.md` §7); лечение — отдельным
решением, тело живых узлов не трогая.

---

## 4. shadowsocks (W4) — три формы userinfo

Пункт (б) чек-листа. Закрывает два живых дефекта (`PRIMITIVES.md` §4).

```jsonc
"uri": {
  "forms": [
    { "id": "sip002", "detect": {"has": "@"},
      "decode": ["url"], "space": "url" },
    { "id": "legacy", "detect": {"default": true},
      "decode": ["base64", {"reparse": "authority"}], "space": "url" }
  ],
  "emit_form": "sip002",
  "userinfo": { "decode": ["percent", "base64?"],
                "split": {"sep": ":", "limit": 2},
                "into": ["method", "password"] },
  "fragment": "label",
  "query": {
    "plugin":      { "source": "query.plugin", "maps_to": "plugin" },
    "plugin_opts": { "source": "query.plugin_opts", "maps_to": "plugin_opts",
                     "aliases": ["pluginOpts"] }
  },
  "emit": { "form": "sip002", "param_order": ["plugin", "plugin_opts"] }
}
```

- `"base64?"` (try-decode) чинит **SS2022**: открытый
  `2022-blake3-aes-128-gcm:key@host` сегодня уходит в безусловный base64-декод
  и даёт мусор либо роняет узел (`node_parser_core.go:194-224`, дроп `:429-435`).
- `limit: 2` чинит пароль с двоеточием.
- `plugin`/`plugin_opts` — **сегодня не читаются вовсе**, хотя реестр их
  объявляет и прямо фиксирует «Go молча теряет query целиком»
  (`shadowsocks.json:40-52`). Это тот самый класс дефекта, ради которого
  затеяна кампания.

Оба исправления **меняют тела живых узлов** → вопрос владельцу
(`SPEC.md` §13, вопрос 2).

---

## 5. socks / http / naive (W4) — схема несёт тело

Пункт (г): `sets` по источнику `scheme`.

```jsonc
// socks
"uri": {
  "scheme_aliases": ["socks", "socks5", "socks4", "socks4a", "socks5h"],
  "userinfo": { "decode": ["base64?"], "split": {"sep": ":", "limit": 2},
                "into": ["username", "password"] },
  "defaults": { "server_port": 1080 },
  "scheme_sets": { "socks":   {"version": "5"}, "socks5":  {"version": "5"},
                   "socks5h": {"version": "5"}, "socks4":  {"version": "4"},
                   "socks4a": {"version": "4a"} },
  "emit": { "form_from": {"version": {"4": "socks4", "4a": "socks4a",
                                      "*": "socks5"}} }
}

// http
"uri": {
  "scheme_aliases": ["proxy-http","proxy-https","proxy+http","proxy+https"],
  "userinfo": { "split": {"sep": ":", "limit": 2},
                "into": ["username","password"], "single_into": "username" },
  "scheme_sets": {
    "proxy-https": {"tls.enabled": true, "$default_port": 443},
    "proxy+https": {"tls.enabled": true, "$default_port": 443},
    "proxy-http":  {"$default_port": 80},
    "proxy+http":  {"$default_port": 80} },
  "query": {
    "path":    { "source": "query.path", "maps_to": "path" },
    "headers": { "source": "query.headers", "maps_to": "headers",
                 "list": {"sep": "\r\n"},
                 "extract": {"re": "^(?P<k>[^:]+):\\s*(?P<v>.*)$",
                             "into": {"k": "$key", "v": "$value"}},
                 "type": "object" },
    "sni": { "source": "query.sni", "maps_to": "tls.server_name",
             "when": {"tls.enabled": true} },
    "fp":  { "source": "query.fp", "maps_to": "tls.utls.fingerprint",
             "when": {"tls.enabled": true} },
    "insecure": { "source": "query.insecure", "type": "bool_spelled",
                  "maps_to": "tls.insecure", "when": {"tls.enabled": true} }
  },
  "emit": { "form_from": {"tls.enabled": {"true": "proxy-https",
                                          "*": "proxy-http"}} }
}

// naive
"uri": {
  "scheme_aliases": ["naive+https", "naive+quic"],
  "userinfo": { "split": {"sep": ":", "limit": 2},
                "into": ["username","password"], "single_into": "password" },
  "defaults": { "server_port": 443 },
  "scheme_sets": {
    "naive+https": {"tls.enabled": true, "tls.server_name": "$host"},
    "naive+quic":  {"tls.enabled": true, "tls.server_name": "$host",
                    "quic": true, "quic_congestion_control": "bbr"} },
  "query": {
    "extra-headers": { "source": "query.extra-headers", "maps_to": "headers",
                       "list": {"sep": "\r\n"},
                       "extract": {"re": "^(?P<k>[^:]+):\\s*(?P<v>.*)$",
                                   "into": {"k": "$key", "v": "$value"}},
                       "on_item_invalid": {"action": "skip",
                                           "code": "naive_extra_headers_invalid"} },
    "padding": { "source": "query.padding", "maps_to": null,
                 "on_present": {"action": "drop", "code": "naive_padding_ignored"} }
  }
}
```

`quic_congestion_control: "bbr"` — значение, которого в ссылке нет
(`node_parser_naive.go:131-134`); сегодня эмиттер его не пишет, из-за чего
тело-из-ссылки ≠ тело-из-тела. Одна запись — обе стороны согласованы.

Урезанный TLS у naive (только `enabled`+`server_name`) выражен **уже
существующим** `forbidden_for` в `tls.json` — новой записи не нужно.

---

## 6. ssh / anytls / tuic (W4–W5)

Пункт (е).

```jsonc
// ssh
"userinfo": { "split": {"sep": ":", "limit": 2}, "into": ["user","password"] },
"defaults": { "server_port": 22, "user": "root" },   // default_when + код ssh_user_default
"query": {
  "host_key":            { "source": "query.host_key", "maps_to": "host_key",
                           "list": {"sep": ","} },
  "host_key_algorithms": { "source": "query.host_key_algorithms",
                           "maps_to": "host_key_algorithms", "list": {"sep": ","} },
  "private_key":         { "source": "query.private_key", "maps_to": "private_key" },
  "private_key_path":    { "source": "query.private_key_path",
                           "maps_to": "private_key_path",
                           "when": {"private_key": {"present": false}} },
  "private_key_passphrase": { "source": "query.private_key_passphrase",
                              "maps_to": "private_key_passphrase" },
  "client_version":      { "source": "query.client_version", "maps_to": "client_version" }
}
```

Многострочный PEM: `url`-декодер раскрывает `%0A` один раз и **не трогает
`+`** (§0.4) — сегодня это обеспечено запретом повторного декода
(`node_parser_ssh.go:12-15`), в грамматике — нормой движка.

```jsonc
// anytls — пароль = ВЕСЬ userinfo (сегодня режется по первому ':')
"userinfo": { "into": ["password"] },
"query": {
  "idle_session_check_interval": { "source": "query.idle_session_check_interval",
    "maps_to": "idle_session_check_interval", "normalize": "duration_bare_seconds" },
  "idle_session_timeout": { "source": "query.idle_session_timeout",
    "maps_to": "idle_session_timeout", "normalize": "duration_bare_seconds" },
  "min_idle_session": { "source": "query.min_idle_session",
    "maps_to": "min_idle_session" },
  "sni": { "source": ["query.sni","query.peer"], "maps_to": "tls.server_name",
           "default_from": {"source": "host",
                            "when": {"value": {"not_matches": "[.:]"}}} },
  "fp":  { "source": "query.fp", "maps_to": "tls.utls.fingerprint",
           "default_when": {"absent": true, "value": "random"},
           "implies": {"tls.utls.enabled": true} },
  "pbk": { "source": "query.pbk", "maps_to": "tls.reality.public_key",
           "implies": {"tls.reality.enabled": true} }
},
"scheme_sets": {"anytls": {"tls.enabled": true}}

// tuic
"userinfo": { "split": {"sep": ":", "limit": 2}, "into": ["uuid","password"] },
"query": {
  "heartbeat": { "source": "query.heartbeat", "maps_to": "heartbeat",
                 "normalize": "duration_bare_seconds" },
  "reduce_rtt": { "source": "query.reduce_rtt", "type": "bool_spelled",
                  "maps_to": "zero_rtt_handshake", "omit_default": false },
  "allow_insecure": { "source": "query.allow_insecure", "type": "bool_spelled",
                      "maps_to": "tls.insecure" },
  "disable_sni": { "source": "query.disable_sni", "type": "bool_spelled",
                   "maps_to": "tls.disable_sni" }
},
"scheme_sets": {"tuic": {"tls.enabled": true}}
```

**Две находки, закрываемые `source`:** `disable_sni` объявлен в реестре у
tuic и у masque, но **Go не читает его ни там, ни там** (пометка
`ext: mobile` / «Только Dart»). С общим циклом параметр читается по факту
объявления — узел перестаёт терять поле на десктопе.

`anytls`: `security` сегодня не читается вовсе (`enabled:true` безусловно,
`node_parser_anytls.go:46-74`) — в отличие от trojan/http. Через
`scheme_sets` поведение сохраняется дословно; вопрос «читать ли
`security=none`» — к владельцу, не к кампании.

---

## 7. hysteria v1 / hysteria2 (W5)

Пункт (в): base64-обёртка + multi-port из сырого порта.

```jsonc
// hysteria2
"uri": {
  "scheme_aliases": ["hysteria2", "hy2"],
  "forms": [
    { "id": "wrapped", "detect": {"after_scheme": "base64", "decoded_has": "@"},
      "decode": ["base64", {"reparse": "url"}], "space": "url" },
    { "id": "url", "detect": {"default": true}, "decode": ["url"], "space": "url" }
  ],
  "emit_form": "url",
  "userinfo": { "into": ["password"] },
  "query": {
    "$multiport": { "source": "port_raw", "selector": true,
      "extract": { "re": "^(?P<first>\\d+)(?P<rest>[,\\-:].*)?$",
                   "into": { "first": {"path": "server_port", "type": "int"},
                             "rest":  {"path": "server_ports",
                                       "normalize": "port_range_spec"} } } },
    "mport": { "source": ["query.mport", "query.ports"],
               "maps_to": "server_ports", "merge": "prepend",
               "normalize": "port_range_spec" },
    "obfs": { "source": "query.obfs", "selector": true, "maps_to": "obfs.type" },
    "obfs-password": { "source": "query.obfs-password", "maps_to": "obfs.password",
                       "trim": false, "when": {"obfs.type": {"present": true}} },
    "obfs-min-packet-size": { "source": "query.obfs-min-packet-size",
                              "maps_to": "obfs.min_packet_size", "type": "int",
                              "when": {"obfs.type": {"present": true}} },
    "obfs-max-packet-size": { "source": "query.obfs-max-packet-size",
                              "maps_to": "obfs.max_packet_size", "type": "int",
                              "when": {"obfs.type": {"present": true}} },
    "upmbps":   { "source": "query.upmbps", "aliases": ["up_mbps","up"],
                  "maps_to": "up_mbps" },
    "downmbps": { "source": "query.downmbps", "aliases": ["down_mbps","down"],
                  "maps_to": "down_mbps" },
    "pinSHA256": { "source": "query.pinSHA256",
                   "maps_to": "tls.certificate_public_key_sha256",
                   "list": {"sep": ",", "coerce_scalar": true} }
  },
  "scheme_sets": {"*": {"tls.enabled": true}}
}
```

`normalize: "port_range_spec"` — один общий нормализатор
`"1000-2000,3000"` → `["1000:2000","3000:3000"]` (ядру нужно ДВОЕТОЧИЕ;
дефис даёт фатал «bad port range»). Обратно — при эмите
(`hysteria2_ports.go:175-191`). `merge: "prepend"` выражает сегодняшнюю
конкатенацию authority-портов с query-`mport` (`node_parser_core.go:419-425`).

hysteria **v1** отличается тремя записями:

```jsonc
"auth": { "source": ["query.auth","query.auth_str","query.authStr",
                     "query.password","userinfo.user"], "maps_to": "auth_str" },
"obfs": { "source": ["query.obfsParam","query.obfs-password","query.obfs"],
          "maps_to": "obfs", "value_map": {"xplus": null} },
"upmbps": { "source": "query.upmbps", "maps_to": "up_mbps", "type": "int",
            "extract": {"re": "^(?P<v>\\d+)", "into": {"v": "up_mbps"}},
            "default_when": {"absent": true, "value": 100} }
```

`value_map: {"xplus": null}` — голое `obfs=xplus` это объявление ТИПА, а не
секрет (`node_parser_hysteria.go:56`); иначе ядро шифровало бы по слову
«xplus». При эмите обратный `compose` даёт пару
`obfs=xplus&obfsParam=<секрет>` (`shareuri_hysteria.go:76-79`).

**Поправка к постановке задачи:** `fp` у hysteria/hysteria2/tuic
**читается** (`node_parser_hysteria.go:132` → `applyTLSCamouflageFromQuery`,
`node_parser_transport.go:971-977`); реестровая запись помечена как
исправленная в контракте 1.1.4. Снимает блок реестр
(`forbidden_for` QUIC, код `tls_not_applicable_quic`), а не парсер.

---

## 8. masque (W5)

Самая «своевольная» схема: собственный парсер мимо `ParseNode`, свой список
`insecure` (где `yes` не работает), `sni` без алиасов и без эвристики,
собственная сборка метки. Всё это исчезает как класс.

```jsonc
"uri": {
  "userinfo": { "into": ["private_key"] },
  "defaults": { "server_port": 443 },
  "query": {
    "private_key": { "source": ["userinfo.user", "query.private_key"],
                     "aliases": ["privatekey"], "maps_to": "private_key",
                     "required": true },
    "publickey": { "source": "query.publickey", "maps_to": "public_key",
                   "decode_extra": {"preserve_plus": true}, "required": true },
    "address": { "source": "query.address", "list": {"sep": ","},
                 "normalize": "cidr_prefix",
                 "split_into": {"ip": {"when": {"item": {"not_matches": ":"}},
                                       "take": "first"},
                                "ipv6": {"when": {"item": {"matches": ":"}},
                                         "take": "first"}},
                 "required": true },
    "profile": { "source": "query.profile", "maps_to": "profile",
                 "default_when": {"absent": true, "value": "cloudflare"} },
    "vhttp":   { "source": "query.vhttp", "maps_to": "vhttp",
                 "default_when": {"absent": true, "value": "h3"},
                 "emit_when": "always" },
    "sni":     { "source": "query.sni", "maps_to": "tls.server_name" },
    "disable_sni": { "source": "query.disable_sni", "type": "bool_spelled",
                     "maps_to": "tls.disable_sni" },
    "insecure": { "source": "query.insecure", "type": "bool_spelled",
                  "maps_to": "tls.insecure" },
    "mtu": { "source": "query.mtu", "maps_to": "mtu", "type": "int",
             "default_when": {"absent": true, "value": 1280} },
    "idle_timeout": { "source": "query.idle_timeout", "maps_to": "idle_timeout" },
    "keep_alive":   { "source": "query.keep_alive",
                      "aliases": ["keep_alive_period"],
                      "maps_to": "keep_alive_period" }
  }
}
```

`split_into` — единственный примитив, добавленный ради masque: список
разбрасывается по двум полям по предикату на элементе
(`node_parser_masque.go:67-81`). Он общий (не назван по схеме) и при эмите
работает обратно — склейка `ip`+`ipv6` в `address=`
(`shareuri_masque.go:24-37`).

**Что чинится само:** `insecure` получает общий набор из 9 написаний и
общую истинность (`yes` заработает); `sni` — реестровые алиасы; метка —
общий конвейер.

---

## 9. wireguard / awg (W6) — четыре формы, одно ini-пространство

Пункт (д): **одна таблица `ini.*`** и на ссылку `awg://<base64 .conf>`, и на
`.conf` как файл.

```jsonc
"uri": {
  "scheme_aliases": ["wireguard", "awg"],
  "forms": [
    { "id": "conf_b64", "detect": {"after_scheme": "base64",
                                   "not_has": ["@", ":", "?"],
                                   "decoded_has": "[Interface]"},
      "decode": ["base64", "ini"], "space": "ini" },
    { "id": "conf_text", "detect": {"text_has": "[Interface]"},
      "decode": ["ini"], "space": "ini" },
    { "id": "url", "detect": {"default": true}, "decode": ["url"], "space": "url" }
  ],
  "emit_form": "url",
  "userinfo": { "into": ["private_key"] },
  "defaults": { "server_port": 51820 },
  "query": {
    "privatekey": { "source": {"url": ["userinfo.user","query.privatekey"],
                               "ini": "ini.Interface.PrivateKey"},
                    "aliases": ["private_key"], "maps_to": "private_key",
                    "normalize": "base64_std", "required": true,
                    "decode_extra": {"preserve_plus": true} },
    "publickey":  { "source": {"url": "query.publickey",
                               "ini": "ini.Peer.PublicKey"},
                    "aliases": ["public_key"], "maps_to": "peers[].public_key",
                    "normalize": "base64_std", "required": true,
                    "decode_extra": {"preserve_plus": true} },
    "presharedkey": { "source": {"url": "query.presharedkey",
                                 "ini": "ini.Peer.PresharedKey"},
                      "maps_to": "peers[].pre_shared_key",
                      "normalize": "base64_std",
                      "decode_extra": {"preserve_plus": true} },
    "address":    { "source": {"url": "query.address",
                               "ini": "ini.Interface.Address"},
                    "maps_to": "address", "list": {"sep": ","},
                    "normalize": "cidr_prefix", "required": true },
    "allowedips": { "source": {"url": "query.allowedips",
                               "ini": "ini.Peer.AllowedIPs"},
                    "aliases": ["allowed_ips"], "maps_to": "peers[].allowed_ips",
                    "list": {"sep": ","}, "normalize": "cidr_prefix",
                    "default_when": {"absent": true, "value": "0.0.0.0/0,::/0"} },
    "$endpoint":  { "source": {"ini": "ini.Peer.Endpoint"}, "selector": true,
                    "extract": {"re": "^(?P<h>\\[[^\\]]+\\]|[^:]+):(?P<p>\\d+)$",
                                "into": {"h": "peers[].address",
                                         "p": {"path": "peers[].port", "type": "int"}}} },
    "reserved":   { "source": "query.reserved", "aliases": ["client_id"],
                    "maps_to": "peers[].reserved",
                    "list": {"sep": ",", "item": "int", "len": 3} },
    "keepalive":  { "source": {"url": "query.keepalive",
                               "ini": ["ini.Peer.PersistentKeepalive",
                                       "ini.Interface.PersistentKeepalive"]},
                    "maps_to": "peers[].persistent_keepalive_interval" },
    "mtu":        { "source": {"url": "query.mtu", "ini": "ini.Interface.MTU"},
                    "maps_to": "mtu", "type": "int" },
    "name":       { "source": "query.name", "maps_to": "name",
                    "omit_default": "singbox-wg0" }
    // jc/jmin/jmax/s1-s4/h1-h4/i1-i5/id/ip/ib — плоские; имя ключа .conf в
    // lower === имя query-параметра, поэтому одна запись обслуживает обе формы
  }
}
```

Ключевые замечания:

- **MTU-правил в парсере уже нет** — кламп 1280 и дефолт AWG перенесены в
  `wireguard.body.fields.mtu` (`default_when`/`max_when`) волной W2d, и род
  узла там выражен **по телу**, а не по query. Кампания это не трогает.
- **Род узла AWG** (`hasAWGParams`, `node_parser_wireguard.go:406-415`) —
  существующий `any_set`, он уже в реестре.
- `h1-h4` с диапазоном и **перестановкой границ** при `hi<lo`
  (`:553-567`), против AWG3-диапазонов, где границы **не переставляются**
  намеренно (`awg3.go:136-139`), — два режима одного `normalize`:
  `range_order: "swap"` и `range_order: "strict"`.
- Дропы узла (пересечение h1–h4, AWG3-ключ, `s1-s4 < 12`) — **правила тела**
  в реестре, не маппера: они судят значения, а маппер значения не судит.
- Имя узла из комментария после `[Peer]` (`wgconf_text.go:82-107`) —
  `label` с источником `ini.$comment.Peer` и предикатом «нет `=`».

**Amnezia `vpn://`** в грамматику не входит и входить не должен: это
распаковщик (zlib+base64, рекурсивный поиск INI, выбор контейнера,
плейсхолдеры DNS — `node_parser_amnezia.go:50-460`). Он остаётся обёрткой
над текстом, выдающей `.conf` тому же движку.

---

## 10. Xray-JSON (W8, если в объёме)

```jsonc
{ "id": "xray", "detect": {"json_has": "protocol"}, "space": "json" }
```

Таблица соответствий и граница «перевод против сборки документа» —
`PRIMITIVES.md` §10. Кратко: перевод одного outbound выразим целиком;
`dialerProxy`→цепочки, дедуп/владение, `balancers`→группы остаются вне
маппера, потому что работают над МАССИВОМ узлов.

---

## 11. Транспорты и TLS — уже разложены

`transports.json` и `tls.json` менять почти не нужно: параметры уже лежат
по типам (`transports.<type>.params`) и несут `maps_to`. Добавляется
`source` и переводятся в примитивы три места, живущие сегодня прозой в
`aliases`/`impl`:

```jsonc
// transports.ws.path — пункт (и) чек-листа LxBox
"path": {
  "source": "query.path", "decode_extra": {"percent": 2, "mode": "path"},
  "extract": { "re": "^(?P<path>[^?]*)(?:\\?ed=(?P<ed>\\d+))?$",
               "into": { "path": "transport.path",
                         "ed": {"path": "transport.max_early_data", "type": "int",
                                "implies": {"transport.early_data_header_name":
                                            {"value": "Sec-WebSocket-Protocol",
                                             "implicit": true}}} } },
  "compose": "{transport.path}?ed={transport.max_early_data}"
}
```

`"implicit": true` — ответ на пункт (и): заголовок подставлен конвенцией,
а не ссылкой, поэтому **в ссылку он не пишется**, хотя в теле присутствует.
Признак живёт в данных и влияет только на эмит.

```jsonc
// transports.ws.host — фолбэк-цепочка, сегодня проза в aliases
"host": { "source": ["query.host", "query.sni", "query.obfsParam"],
          "maps_to": "transport.headers.Host" }

// tls.fp_dialect — именованная таблица value_map, на неё ссылаются схемы
"fp_dialect": { "prefix": {"hellochrome": "chrome", "hellofirefox": "firefox",
                           "helloedge": "edge", "hellosafari": "safari",
                           "helloios": "ios", "helloandroid": "android",
                           "hello360": "360", "helloqq": "qq",
                           "hellorandomized": "randomized",
                           "hellorandom": "random"},
                "strip": ["_", "-", " "] }
```

---

## 12. Сводка новых примитивов

Из всего перечисленного **новыми** (не выводимыми из сегодняшнего реестра)
оказываются:

| Примитив | Ради чего |
|---|---|
| `uri.forms[]` + `decode` + `reparse` + `space` | vmess ×2, ss ×2, hy2-обёртка, awg-conf, xray |
| `source` (строка / список / карта по формам) | ядро кампании; убирает «объявлен, но не читается» |
| `userinfo.{decode,split{sep,limit},into,single_into}` | 14 схем, три живых дефекта |
| `value_map` (+ `prefix`/`strip`) | диалекты значений; при эмите — обратно |
| `selector` + расширение `when` (`in`, равенство по источнику) | транспорт, TLS, plaintext-порты |
| `sets` / `scheme_sets` / `implies` | security, схема-несёт-тело, REALITY по pbk |
| `extract` / `compose` / `split_into` / `merge` | `?ed=N`, multi-port, headers, masque address |
| `list{sep,item,len,coerce_scalar}` | alpn, host_key, address, reserved |
| `default_from` (+ цепочка и предикат) | SNI-эвристика ×3 → одна запись |
| `label_fallback{template,scheme_source}` | identity тега |
| `decode_extra{percent,preserve_plus,mode}` | alpn ×∞, path ×2, `+` в base64 |
| `emit_when` / `omit_default` / `emit_form` / `form_from` / `implicit` | эмит от той же таблицы |
| `normalize`: `base64_std`, `cidr_prefix`, `port_range_spec`, `duration_bare_seconds`, `range_order{swap,strict}` | нормализаторы, сегодня функции |

Тринадцать групп. Ни одна не названа по схеме; каждая используется
минимум двумя схемами, кроме `split_into` (masque) — он оставлен общим
осознанно, потому что обратная операция (склейка) нужна эмиттеру.


---

## 13. Секции по ВИДАМ ИСТОЧНИКА — черновики

Расширение области 19.09.2026: `mappers.uri` (§1–§11 выше), плюс три
независимые секции. Форма записи одна (`SPEC.md` §3.2), различаются
`detect`, `forms` и написание `source`.

### 13.1. `registry/sources.json` — уровень документа

Полный черновик — `SPEC.md` §3A.3. Кратко: упорядоченный список видов
источника, у каждого `detect` + `priority` + `mapper` + оболочка
(`unwrap`/`redetect`). Переводит в данные сегодняшний
`ClassifySubscriptionBody` (`body_classify.go:90-124`) вместе с
обоснованиями порядка.

### 13.2. `mappers.xray` — черновики по протоколам

```jsonc
// vless
"mappers": { "xray": {
  "detect": {"json": {"value_of": {"protocol": "vless"}}},
  "body_source": "xray",
  "unknown_key": {"action": "drop", "code": "json_field_unknown"},
  "forms": [
    { "id": "vnext", "detect": {"json": {"type_of": {"settings.vnext": "array"}}},
      "space": "json", "base": "settings.vnext.0" }
  ],
  "params": {
    "address": { "source": "json.$base.address", "maps_to": "server", "required": true },
    "port":    { "source": "json.$base.port", "maps_to": "server_port",
                 "type": "int", "required": true },
    "id":      { "source": "json.$base.users.0.id", "maps_to": "uuid", "required": true },
    "flow":    { "source": "json.$base.users.0.flow", "maps_to": "flow",
                 "value_map": {"xtls-rprx-vision-udp443": "xtls-rprx-vision"},
                 "sets": {"xtls-rprx-vision-udp443": {"packet_encoding": "xudp"}},
                 "priority": 10 },
    "encryption": { "source": "json.$base.users.0.encryption", "maps_to": "encryption",
                    "value_map": {"none": null, "": null} },
    "$extra_users": { "source": "json.$base.users", "maps_to": null,
                      "on_len_gt": {"n": 1, "action": "note",
                                    "code": "xray_extra_entries_dropped"} }
  },
  "include": ["transports#xray", "tls#xray"],
  "emit": null
} }

// hysteria — версия выбирает ТИП ТЕЛА
"mappers": { "xray": {
  "detect": {"json": {"value_of": {"protocol": "hysteria"}}},
  "forms": [ { "id": "flat", "detect": {"json": {"any_keys": ["settings.address"]}},
               "base": "settings" },
             { "id": "servers", "detect": {"default": true},
               "base": "settings.servers.0" } ],
  "params": {
    "version": { "source": ["json.streamSettings.hysteriaSettings.version",
                            "json.settings.version"],
                 "selector": true, "type": "int",
                 "default_when": {"absent": true, "value": 1},
                 "sets": {"1": {"$type": "hysteria"}, "2": {"$type": "hysteria2"}},
                 "on_invalid": {"action": "drop_node", "code": "protocol_unsupported"} },
    "auth": { "source": ["json.streamSettings.hysteriaSettings.auth",
                         "json.streamSettings.hysteriaSettings.auth_str",
                         "json.streamSettings.hysteriaSettings.authStr",
                         "json.streamSettings.hysteriaSettings.password",
                         "json.settings.auth", "json.settings.auth_str",
                         "json.settings.authStr", "json.settings.password"],
              "maps_to": {"hysteria": "auth_str", "hysteria2": "password"} },
    "obfs": { "source": ["json.$hy.obfs", "json.$hy.obfsParam", "json.$hy.obfs_password"],
              "maps_to": {"hysteria": "obfs",
                          "hysteria2": "obfs.password"},
              "sets": {"$present": {"obfs.type": "salamander"}},
              "when": {"$type": "hysteria2"} }
  }
} }
```

`$base` — якорь формы: одна таблица параметров обслуживает `vnext[0]`,
`servers[0]` и плоскую форму, различающиеся только префиксом пути. Это
снимает дословный дубль выемки (`xray_outbound_convert.go:104-131` ≡
`xray_protocols.go:117-144`).

`maps_to` картой по типу тела — выражение «один вход, два целевых поля в
зависимости от версии» без единой ветки в коде.

**Транспорты xray** (`transports#xray`) — где формы `host` различаются:

```jsonc
"blocks": { "transports": { "xray": {
  "ws":          { "$settings": "wsSettings",
                   "path": {"source": "json.wsSettings.path",
                            "extract": {"$ref": "ws_early_data"}},
                   "host": {"source": "json.wsSettings.host",
                            "maps_to": "transport.headers.Host"} },
  "http":        { "host": {"source": "json.httpSettings.host",
                            "maps_to": "transport.host",
                            "list": {"coerce_scalar": true}} },
  "httpupgrade": { "host": {"source": "json.httpupgradeSettings.host",
                            "maps_to": "transport.host"} },
  "grpc":        { "service_name": {"source": "json.grpcSettings.serviceName",
                                    "aliases": ["service_name"],
                                    "maps_to": "transport.service_name"} },
  "xhttp":       { "$settings": ["xhttpSettings", "splithttpSettings"],
                   "flatten": ["extra", "xmux"] } } } }
```

`list.coerce_scalar` у `httpSettings.host` чинит сегодняшний баг: массив
`["a.com","b.com"]` уходит через `fmt.Sprint` и даёт `["[a.com b.com]"]`
(`xray_outbound_convert.go:26`, `:363-365`).

### 13.3. `mappers.singbox` — диалекты форков

Полный разбор — `SPEC.md` §3.4. Черновик записи:

```jsonc
"mappers": { "singbox": {
  "detect": {"json": {"value_in": {"type": ["shadowsocks", "ss"]}}},
  "body_source": "singbox",
  "unknown_key": {"action": "keep", "code": "json_field_unknown"},
  "forms": [
    { "id": "endpoint", "detect": {"in_array": "endpoints"}, "level": "endpoint" },
    { "id": "outbound", "detect": {"default": true},         "level": "outbound" }
  ],
  "type_synonyms": {"shadowsocks": "ss"},
  "params": { /* только исключения: см. ниже */ }
} }
```

Что обязано быть перечислено (сегодня — код или ничего):

| Запись | Закрывает |
|---|---|
| `$masque_flat` (`network`/`sni`/`skip_cert_verify`, `since: "0.8.0"`, drop + код) | `singbox_sanitize.go:84-96` — сегодня стрип **без кода** |
| `obfs` с `coerce.object_to_scalar: "password"`, `when: {$type: "hysteria"}` | `:147-173` — сегодня три ветки, все **только WarnLog** |
| `tls` с проверкой ФОРМЫ (не объект → снять; `enabled:false` → снять целиком) | `:109-132`, SPEC 045 (SIGSEGV ядер lx.5–lx.18) |
| `xmux.*` с `source` списком (snake ∥ camel ∥ `extra.xmux.*`) | реестр уже объявляет оба написания, код читает одно |
| `$addressless` (`when.$type in [wireguard, tailscale]` — не требовать server/port) | `singbox_import.go:429-431`, `if type ==` |
| `$credential` (какое поле несёт секрет по типу) | `:437-449`, switch по схеме из 8 литералов |

### 13.4. `mappers.conf` — ini напрямую

`SCHEMES.md` §9 уже записан в форме `ini.<Section>.<Key>`. Добавляется:

```jsonc
"mappers": { "conf": {
  "detect": {"ini": {"sections": ["Interface"]}},
  "body_source": "wgconf",
  "forms": [
    { "id": "awg3", "detect": {"ini": {"keys_any": ["H1","I1","HeaderProtectionKey"]}} },
    { "id": "awg",  "detect": {"ini": {"keys_any": ["Jc","Jmin","Jmax","S1"]}} },
    { "id": "wg",   "detect": {"default": true} }
  ],
  "ini_dialect": { "key_case": "lower", "value_case": "preserve",
                   "comment_prefixes": ["#", ";"], "inline_comments": false,
                   "repeated_key": "last_wins",
                   "sections": {"Peer": {"repeat": "first_only",
                                         "on_extra": {"code": "wgconf_extra_peer_dropped"}}} },
  "params": {
    "label":    { "source": "ini.$comment.Peer",
                  "when": {"value": {"not_matches": "="}} },
    "endpoint": { "source": "ini.Peer.Endpoint", "selector": true,
                  "extract": {"re": "^(?:\\[(?P<h6>[^\\]]+)\\]|(?P<h>[^:]+(?::[^:]+)*?)):(?P<p>\\d+)$",
                              "into": {"h6": "peers[].address", "h": "peers[].address",
                                       "p": {"path": "peers[].port", "type": "int"}}},
                  "on_no_match": {"action": "take_all",
                                  "into": "peers[].address",
                                  "defaults": {"peers[].port": 51820}} },
    "dns":      { "source": "ini.Interface.DNS", "maps_to": null,
                  "on_present": {"action": "note", "code": "wgconf_dns_ignored"} }
  }
} }
```

Три места, где черновик исправлен по факту кода:

- **`repeat: "first_only"` + код** — сегодня вторая `[Peer]` отбрасывается
  **молча** (`node_parser_amnezia.go:494-497`), хотя счётчик уже посчитан;
- **`Endpoint` с голым IPv6** — регулярка черновика `^(\[[^\]]+\]|[^:]+):(\d+)$`
  не матчит `2001:db8::1:51820`; код это умеет (`LastIndex(":")` +
  `Trim("[]")`, `wgconf_text.go:111-118`), грамматика обязана уметь тоже;
- **`Interface.DNS`** — by-design лоссы (`wireguard.json:124-131`), но
  сегодня **молча**; получает info-код, раз параметр объявлен.

### 13.5. Что НЕ переводится (границы)

| Остаётся | Почему |
|---|---|
| `xray_json_array.go`, `xray_balancer.go` | сборка ДОКУМЕНТА: владение, дедуп, `dialerProxy`→цепочки, `balancers[0]`→группа — работа над массивом узлов и связями |
| распаковщик Amnezia `vpn://` (`node_parser_amnezia.go:96-360`) | обёртка над текстом: zlib+base64, выбор контейнера, плейсхолдеры DNS. Отдаёт `.conf` секции `conf` |
| `body_classify.go` | **переводится** в `sources.json` (§13.1) — единственное исключение из «классификатор остаётся» |
| разбор тела подписки (`parse_body.go`) | лимиты, баннеры, построчность — свойство подписки, не узла |
