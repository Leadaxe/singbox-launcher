# Реестр: wildcard `*`, правила формы и роль `variant` (openconnect)

Проект к контракту 1.1.122. База — develop `50deef63`, контракт 1.1.121. Код и реестр не менялись.
Ядро сверено по `sing-box-lx/option/openconnect.go`, `protocol/openconnect/{client,endpoint}.go` и `sing-openconnect@cbd68c45` (`client.go`, `tls_config.go`).

## 1. Чего не хватает реестру

Почти вся таблица формы выражается существующими примитивами:

| Что нужно | Чем выражается |
|---|---|
| enum | `type: enum` + `values` |
| секрет | `secret` |
| длительность | `type: duration` |
| вложенные пути | `object` с `order`/`fields` |
| формат отпечатка | `pattern` + `item_pattern` + `on_item_invalid: drop_item` |
| пара сертификат/ключ | `requires` + `unless_set` |
| конфликт secret/secret_path | `conflicts` |
| обязательность внутри `token` | `required`, `required_unless` |
| отказ ядра | `core_rejects` |
| предупреждения | `advisory` → `tls_insecure` и новый info-код |
| detour | `__dialer` (`managed`) |
| условие по значению | `condition`: `any_set`, `путь: значение`, `in` / `not_in` |

Новых примитивов три:

1. **Wildcard `*` в `fields`** — неописанные ключи объекта проходят насквозь (§2).
2. **Атрибут `form`** — правила формы, которые санитайзер не читает: видимость, обязательность и диапазон формы, блоки, группа выбора (§3).
3. **Роль `variant`** — значение поля во второй части подзаголовка узла (§4).

## 2. Wildcard `*`

**Норма.**
- Запись `"*"` в `fields` объекта значит: ключи этого объекта, которых нет в `fields`, копируются как есть, без `unknown_key` и без проверки. Описанные поля проверяются своими правилами как сегодня.
- **Wildcard не наследуется.** Если в корне есть `*`, а `tls` описан объектом, внутри `tls` действует строгий режим. Чтобы внутри было нестрого, wildcard ставится явно на этом уровне (`tls.*`, то есть `"*"` в `tls.fields`).
- Масок нет: только целый `*`, одно правило на объект.
- Объект, который не описан вовсе (`mobile`, `csd`, `form_entries`), — это значение сквозного ключа родителя. Он уходит целиком.

**Запись.** Тип и правила у записи не ставятся, разрешены только `desc_en`, `desc_ru`, `impl`. `"*"` обязан стоять в `order`; его позиция — слот эмиссии.
Ключ ядра не может называться `*`, коллизии нет. В путях связей и условий `*` запрещён.

**Санитайзер.**
- `tag`/`type` в корне и `managed` (detour) снимаются молча, как сегодня.
- Сквозные ключи видны связям: `pathPresent` читает `srcRoot`, поэтому `unless_set: ["tls.client_key"]` работает и на неописанном пути.
- Правило на неописанный ключ не ставится. Если ядро что-то отвергает в неописанном поле, поле описывается (без формы).
- Объект, снятый целиком (`absent_when`, отказ обязательного вложенного поля), уходит вместе со сквозными ключами.
- Коды возможны только у описанных полей и от узлового гейта. `unknown_key` под `*` не бывает.
- Авторское тело (PARSING_PRINCIPLES §10) работает без изменений.

**Эмиттер.** Описанные ключи идут по `order`. В слоте `"*"` идут сквозные ключи по алфавиту, через `writeJSON`. Корпус не затронут: `entry` сравнивается с отсортированными ключами (§2.3).

**`fields_unchecked`.** Точный эквивалент — `order: ["*"], fields: {"*": {…}}`. Проверено по коду: на пустой схеме предпроходы пусты, обход копирует всё, кроме tag/type, а алфавитная эмиссия совпадает с `writeJSON`.
- `openvpn-client.json` и `openconnect.json` в 1.1.122 переводятся на `*`.
- Атрибут удаляется из схемы, ветки `FieldsUnchecked` в `SanitizeFromKind`/`Emit` удаляются.
- Кейс `openvpn_client_endpoint` не меняется и подтверждает эквивалентность.

**Dart.** `fields["*"]` — не поле: все обходы полей его пропускают (санитайзер, эмиттер, валидатор, формы, роли, гейт, поиск по пути). При наличии `*` неизвестный ключ объекта копируется. Эмиссия в слоте — по алфавиту.
Сторона, которая wildcard не знает, снимет сквозные ключи с `unknown_key`. Поэтому поддержка на обеих сторонах входит в одну версию контракта.

**Схема `registry_body.schema.json`.**

```json
"wildcardField": { "type": "object", "additionalProperties": false,
  "description": "Запись \"*\" в fields (1.1.122): неописанные ключи ЭТОГО объекта проходят как есть, без unknown_key. Не наследуется. Позиция в order — слот эмиссии (по алфавиту).",
  "properties": { "desc_en": {"type":"string"}, "desc_ru": {"type":"string"}, "impl": {"type":"string"} } },
"bodySection.properties.fields": { "type": "object",
  "patternProperties": { "^\\*$": { "$ref": "#/definitions/wildcardField" } },
  "additionalProperties": { "$ref": "#/definitions/field" } },
"field.properties.fields": "так же",
"bodySection.properties.fields_unchecked": "удалить",
"bodySection.properties.form": { "$ref": "#/definitions/formSection" },
"field.properties.form": { "$ref": "#/definitions/fieldForm" },
"field.properties.role": { "enum": ["credential", "private_key", "variant"] }
```

## 3. Атрибут `form`

**Норма.** `form` читают только формы обеих сторон. Санитайзер, эмиттер, гейт и маппер его не читают. Поле с `form` есть в форме. Описанное поле без `form` проверяется санитайзером, но правится только руками в JSON.

| Ключ поля `form` | Семантика |
|---|---|
| `block` | id блока из `body.form.blocks`. Порядок внутри блока задаёт `body.order`. |
| `visible_when` | Существующий `condition` (без `source_kind`), считается по текущему состоянию формы. Ложно — поле не рисуется и не валидируется. У объекта скрывает всё поддерево. |
| `required` | Форма не сохраняет, пока видимое поле пусто. С обязательностью ядра не связано. |
| `required_unless` | Та же форма `{set, absent}`, что у санитайзера. Пути из `set` могут быть неописанными. |
| `min` / `max` | Диапазон формы. Пусто означает «ключ не пишется». |

Ключи секции тела `form`:
- `blocks` — список id блоков по порядку.
- `choices[]` — группа взаимоисключающих вариантов `{id, block, options: [{id, paths, set?}]}`.
  - **Активный вариант** — первый, у которого задан хоть один путь из `paths`; если не задан ни один, активен вариант с пустыми `paths`.
  - **Смена варианта** очищает `paths` остальных вариантов и пишет `set` выбранного.

Форма показывает рядом с полем коды его `advisory`, когда значение под них попадает. Отдельного атрибута предупреждения нет.

`visible_when` — только UI. Санитайзер скрытые поля не снимает и кодов не ставит: ядро `fortinet_host_check` при другом flavor игнорирует. Скрытое значение форма оставляет в теле.

## 4. Роль `variant`

- Ставится у поля верхнего уровня тела протокола, не больше одного на схему, тип `string` или `enum`.
- Метка — значение поля после trim; если его нет, `default` поля; если нет и его, пусто. Значение не переводится.
- Метка встаёт в слот транспорта подзаголовка. Порядок в `TransportLabel`: поле `transport` → `vhttp` (как сейчас) → роль `variant` → пусто.
- Примеры: `flavor: fortinet` → «openconnect·fortinet»; без flavor → «openconnect·anyconnect».
- LxBox: `_deriveTransport` в `config_node.dart`, тот же порядок.

## 5. Запись `openconnect.json` (ключевое)

```json
"body": {
  "core": "1.14.3-lx.17", "lx_only": true, "min_core": "1.14.0-lx.10", "build_tag": "with_openconnect",
  "on_core_unsupported": { "action": "drop_node", "code": "openconnect_core_unsupported" },
  "order": ["server", "flavor", "username", "password", "auth_group", "token", "user_agent", "fortinet_host_check",
            "no_udp", "ipv6_disabled", "external_auth_disabled", "mtu", "dpd_interval", "reconnect_timeout",
            "allow_insecure_crypto", "tls", "__dialer", "*"],
  "form": {
    "blocks": ["connection", "security", "advanced"],
    "choices": [{ "id": "server_cert_check", "block": "security", "options": [
      { "id": "system",   "paths": [] },
      { "id": "pinned",   "paths": ["tls.peer_fingerprint"] },
      { "id": "insecure", "paths": ["tls.insecure"], "set": { "tls.insecure": true } } ] }]
  },
  "fields": {
    "server": { "type": "string", "required": true, "core_rejects": true,
      "form": { "block": "connection", "required": true } },
    "flavor": { "type": "enum", "values": ["fortinet", "anyconnect", "gp", "f5", "pulse", "nc"],
      "default": "anyconnect", "role": "variant",
      "on_invalid": { "action": "drop_node", "code": "openconnect_flavor_invalid", "core_rejects": true },
      "form": { "block": "connection", "required": true } },
    "username": { "type": "string",
      "form": { "block": "connection", "required": true,
        "required_unless": { "set": ["tls.client_certificate_path", "tls.client_certificate", "cookie"] } } },
    "password": { "type": "string", "secret": true, "form": { "block": "connection" } },
    "auth_group": { "type": "string", "form": { "block": "security" } },
    "token": { "type": "object", "order": ["mode", "secret", "secret_path", "counter", "*"], "fields": {
      "mode": { "type": "enum", "values": ["totp", "hotp", "stoken", "oidc"], "required": true, "core_rejects": true,
        "form": { "block": "security" } },
      "secret": { "type": "string", "secret": true, "required": true, "core_rejects": true,
        "required_unless": { "set": ["token.secret_path"] },
        "form": { "block": "security", "required": true, "visible_when": { "any_set": ["token.mode"] } } },
      "secret_path": { "type": "string",
        "conflicts": [{ "with": "token.secret", "code": "field_conflict", "core_rejects": true }] },
      "counter": { "type": "int", "min": 0,
        "form": { "block": "security", "visible_when": { "token.mode": "hotp" } } },
      "*": { "impl": "pin, password, device_id" } } },
    "user_agent": { "type": "string", "pattern": "^[^\\r\\n\\u0000]*$", "core_rejects": true,
      "on_invalid": { "action": "drop", "code": "type_invalid" }, "form": { "block": "advanced" } },
    "fortinet_host_check": { "type": "object", "order": ["hostcheck", "*"],
      "fields": { "hostcheck": { "type": "string", "form": { "block": "advanced" } },
                  "*": { "impl": "check_virtual_desktop" } },
      "form": { "block": "advanced", "visible_when": { "flavor": "fortinet" } } },
    "no_udp": { "type": "bool", "form": { "block": "advanced" } },
    "ipv6_disabled": { "type": "bool", "form": { "block": "advanced" } },
    "external_auth_disabled": { "type": "bool", "form": { "block": "security" } },
    "mtu": { "type": "int", "max": 65535, "core_rejects": true,
      "on_invalid": { "action": "drop", "code": "openconnect_mtu_invalid" },
      "form": { "block": "advanced", "min": 576, "max": 1500 } },
    "dpd_interval": { "type": "duration", "form": { "block": "advanced" } },
    "reconnect_timeout": { "type": "duration", "form": { "block": "advanced" } },
    "allow_insecure_crypto": { "type": "bool",
      "advisory": [{ "values": [true], "code": "openconnect_insecure_crypto" }], "form": { "block": "advanced" } },
    "tls": { "type": "object",
      "order": ["insecure", "server_name", "peer_fingerprint", "certificate_authority", "certificate_authority_path",
                "client_certificate", "client_certificate_path", "client_key", "client_key_path",
                "client_key_password", "*"],
      "fields": {
        "insecure": { "type": "bool", "advisory": [{ "values": [true], "code": "tls_insecure" }], "form": {} },
        "server_name": { "type": "string", "form": { "block": "security" } },
        "peer_fingerprint": { "type": "listable_string", "core_rejects": true,
          "pattern": "^(?:[0-9A-Fa-f]{4,40}|sha1:[0-9A-Fa-f]{4,40}|sha256:[0-9A-Fa-f]{4,64}|pin-sha256:(?:[A-Za-z0-9+/]{4,43}|[A-Za-z0-9+/]{3,42}=|[A-Za-z0-9+/]{2,41}==|[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=))$",
          "item_pattern": "(та же)",
          "on_invalid": { "action": "drop", "code": "openconnect_fingerprint_invalid" },
          "on_item_invalid": { "action": "drop_item", "code": "openconnect_fingerprint_invalid" },
          "form": {} },
        "certificate_authority": { "type": "listable_string" },
        "certificate_authority_path": { "type": "string", "form": { "block": "security" },
          "conflicts": [{ "with": "tls.certificate_authority", "code": "field_conflict", "core_rejects": true }] },
        "client_certificate": { "type": "listable_string" },
        "client_certificate_path": { "type": "string", "form": { "block": "security" },
          "conflicts": [{ "with": "tls.client_certificate", "code": "field_conflict", "core_rejects": true }],
          "requires": [{ "path": "tls.client_key_path", "unless_set": ["tls.client_key"], "code": "field_requires", "core_rejects": true }] },
        "client_key": { "type": "listable_string", "secret": true },
        "client_key_path": { "type": "string", "form": { "block": "security" },
          "conflicts": [{ "with": "tls.client_key", "code": "field_conflict", "core_rejects": true }],
          "requires": [{ "path": "tls.client_certificate_path", "unless_set": ["tls.client_certificate"], "code": "field_requires", "core_rejects": true }] },
        "client_key_password": { "type": "string", "secret": true,
          "form": { "block": "security", "visible_when": { "any_set": ["tls.client_key_path", "tls.client_key"] } } },
        "*": { "impl": "mca_*, system_trust_disabled" } } },
    "__dialer": { "type": "ref", "ref": "dialer", "inline": true },
    "*": { "desc_en": "Other core keys pass through as written.", "desc_ru": "Прочие ключи ядра проходят как написаны.",
           "impl": "cookie, mobile, csd/hip/tncc, form_entries, compression_*, trojan_interval, queue_length, system, …" }
  }
}
```

Основания правил в ядре:

| Правило | Где в ядре |
|---|---|
| server обязателен | `NewClient`: `ErrMissingServer` |
| неизвестный flavor — отказ | `newFlavorFrontend`. Без ключа ядро берёт anyconnect, поэтому `drop_node`, а не `drop` |
| token без secret, неизвестный mode — отказ | `validateClientOptions` |
| secret + secret_path — отказ | `NewClient` |
| сертификат и ключ только парой | `loadOptionalCertificate` |
| путь и inline-материал не вместе | `Material.Validate` |
| формат отпечатка | `parsePeerFingerprint`: без префикса hex 4..40; `sha1:` hex 4..40; `sha256:` hex 4..64; `pin-sha256:` base64 4..44, при длине 44 строгий base64 на 32 байта |
| mtu > 65535 — отказ | `cstpMaximumMTU`; ниже 576 ядро молча поднимает до 576, 576..1500 — правило формы |
| `\r\n\0` в user_agent — отказ | `validateClientOptions` |

Новые коды в `warnings.json`:
- `openconnect_flavor_invalid` — warning;
- `openconnect_fingerprint_invalid` — warning;
- `openconnect_mtu_invalid` — warning;
- `openconnect_insecure_crypto` — info.

## 6. Код и тесты

| Файл | Что |
|---|---|
| `core/config/registry/registry.go` | `section`/`Field`/`BodySchema`: распознать `"*"` (флаг `Wildcard`, без `type`), типы `FieldForm`/`FormSection` и доступ `Form(scheme)`; `resolveSection` и inline-ref не вливают `*` суб-схемы; `Field()`/`fieldsAtBodyPath` не находят `*`; удалить `FieldsUnchecked`; видимость считать существующим `Condition.HoldsOn` |
| `core/config/nodeflow/sanitize.go` | `SanitizeFromKind` — удалить ветку; `object` — пропуск `*` в цикле `order`, копия `deepCopyValue` вместо `unknown_key` при `fields["*"]`; `markSchemeForbidden`, `markAbsentObjects`, `minWhenAbsent` — пропуск `*` |
| `core/config/nodeflow/emit.go` | `Emit` — удалить ветку; `emitObject` в слоте `*` пишет ключи, которых нет в `fields`, по алфавиту |
| `gate.go`, `coerce.go`, `core/config/linkmap/*`, `contract/tools/gendocs/{scheme,sections,warnings}.go` | все обходы `Fields` пропускают `*`; генератор выводит `*` строкой «прочие ключи — насквозь», правила `form`, роль `variant` |
| `ui/configurator/business/config_node_labels.go` | `TransportLabel`: ветка роли `variant` через `FieldWithRole` |
| `ui/configurator/tabs/source_openconnect_edit.go` (новый, по SPEC) | ручная вёрстка; видимость, обязательность, диапазоны, выбор и блоки — из `reg.Form("openconnect")`; пишет только поля с `form`, остальное в теле не трогает |
| `core/config/registry_body_test.go` | `TestRegistryBodyStructure`: `*` только с desc/impl и обязательно в `order`, `*` в путях запрещён, `fields_unchecked` запрещён; ключи `form` только внутри `form`, пути `visible_when`/`choices` описаны, `blocks` объявлены. `checkFieldRoles`: `variant`, тип string или enum. `walkFields` пропускает `*`. `TestRegistryBodyCoversCoreSchema`: для тел с `*` проверяется только «описанное ⊆ структура ядра» |
| `core/config/nodeflow/fields_unchecked_test.go` | один тест `TestWildcardNestedStrict`: корень с `*` и `tls` без `*` даёт `unknown_key` на `tls.x`, с `tls.*` — проход; sanitize + emit; запуск по имени |

Корпус `contract/corpus/body/singbox/`:

| Кейс | Ожидание |
|---|---|
| `openconnect_endpoint` | без изменений |
| `openvpn_client_endpoint` | без изменений |
| `openconnect_wildcard_pass_through` | `tls.mca_key_path`, `token.pin`, `mobile{…}`, `form_entries[…]`, `x_future` на месте, кодов нет |
| `openconnect_flavor_invalid_dropped` | `dropped`, `openconnect_flavor_invalid` |
| `openconnect_fingerprint_item_invalid` | плохой элемент снят, годный остался |
| `openconnect_token_secret_conflict` | `field_conflict`, снят `secret_path` |
| `openconnect_token_without_secret` | `token` снят, `field_missing` |
| `openconnect_client_cert_without_key` | `field_requires` |
| `openconnect_tls_insecure_advisory` | `tls_insecure`, тело без правки |
| `openconnect_hostcheck_other_flavor_kept` | при `flavor: gp` поле остаётся, кодов нет |

Корпус авторского тела: `authored/hard_openconnect_token_without_secret`.
Корпус формы: `contract/corpus/form/openconnect_form.json` — тело → видимые пути, обязательные пути, активный вариант выбора, нарушения диапазона.

## 7. `contract/TASKS_LXBOX.md` (новый §, контракт 1.1.122)

1. Wildcard `*` по норме §2: не поле; копирование неописанных ключей только в своём объекте; эмиссия в слоте по алфавиту. `fields_unchecked` удалён — `openvpn-client` и `openconnect` переведены на `*`.
2. Описание `openconnect`: правила `core_rejects`, четыре новых кода, кейсы корпуса тела и авторского тела.
3. Роль `variant` в `_deriveTransport`.
4. `form`: вычисление `visible_when` (существующий `condition`), `required`/`required_unless`/`min`/`max` формы, `blocks`, `choices`, показ `advisory`. Форма openconnect пишет только поля с `form`, скрытые значения не трогает. Раннер `corpus/form`.

## 8. Открытые вопросы

1. Сохранять ли в теле значение скрытого поля (принято в проекте) или очищать его при сохранении формы?
2. Достаточно ли `cookie` и клиентского сертификата, чтобы снять обязательность `username`, или нужен ещё `token.mode = oidc`?
3. Подписи полей и блоков — ключи locale каждой стороны (`openconnect.<path>`, `openconnect.block.<id>`; принято в проекте) или `desc_*` реестра?
