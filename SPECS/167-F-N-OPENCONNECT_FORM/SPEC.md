# SPEC 167 — Форма OpenConnect: реестр с wildcard `*`, правила формы, роль `variant`, второй фактор и SSO под демоном

**Тип:** F (фича). **Статус:** N → O при реализации. **Контракт:** 1.1.122.
**Решения владельца:** 15.10.2026 (переписка по форме, разработанной с командой ядра).
**Проект реестра:** `REGISTRY_DESIGN.md` (в этой папке) — норма примитивов, запись `openconnect.json`, план кода и корпуса. SPEC ссылается на него, не дублирует.

## 1. Проблема

1. Узел `openconnect` (endpoint форка `sing-box-lx`, тег сборки `with_openconnect`, с 1.14.0-lx.10) до контракта 1.1.120 уходил в `outbounds[]` и валил весь конфиг; с 1.1.120 он известен реестру, но как `fields_unchecked`: правил нет, формы нет, подпись в списке — голое «openconnect».
2. Реестр знает два состояния тела: всё описано или ничего. Для формы нужны правила на подмножество полей (enum у `flavor`, секреты, конфликты, формат отпечатка, видимость по `flavor`), а описывать все ~60 полей ядра с вложенными `tncc`/`csd`/`mca_*` бессмысленно.
3. Второй фактор и SSO: ядро вешает челлендж и ждёт ответа через gRPC StartedService; в лаунчере UI под это нет. В классическом режиме канала нет вовсе.

## 2. Решения

### 2.1 Реестр — три новых примитива (норма в `REGISTRY_DESIGN.md` §2–§4)

1. **Wildcard `"*"` в `fields` объекта.** Неописанные ключи ЭТОГО объекта проходят как есть, без `unknown_key`; описанные проверяются своими правилами. **Не наследуется:** если в корне есть `*`, а `tls` описан объектом, внутри `tls` строго; нестрогость внутри — только явным `*` в `tls.fields`. Масок нет. Позиция `*` в `order` — слот эмиссии сквозных ключей (по алфавиту). `fields_unchecked` по коду в точности равен `order: ["*"]` и **удаляется**: `openvpn-client.json` и `openconnect.json` переводятся на `*`, ветки `FieldsUnchecked` в санитайзере и эмиттере снимаются. Кейс `openvpn_client_endpoint` не меняется и подтверждает эквивалентность.
2. **Атрибут `form`** у поля и секция `form` тела. Читают только формы обеих сторон; санитайзер, эмиттер, гейт, маппер его не видят. Ключи поля: `block`, `visible_when` (существующая грамматика `condition`), `required`, `required_unless`, `min`/`max`. Секция тела: `blocks[]`, `choices[]` (группа взаимоисключающих вариантов; активен первый, у которого задан путь из `paths`; смена варианта чистит пути остальных и пишет `set`). Скрытое поле из тела **не снимается**: что пришло из JSON, уходит в ядро, ядро его при другом `flavor` игнорирует. Предупреждения формы — коды `advisory` самого поля.
3. **Роль `variant`** (`role: "variant"`): одно поле верхнего уровня, `string` или `enum`; его значение (после trim; иначе `default`; иначе пусто) встаёт во вторую часть подписи узла после `transport` и `vhttp`. Для `flavor: fortinet` подпись — «openconnect·fortinet».

### 2.2 Запись `openconnect.json` (фрагмент в `REGISTRY_DESIGN.md` §5)

Поля формы описаны с правилами ядра, сверенными по исходникам (`core_rejects`): `server` обязателен; неизвестный `flavor` → `drop_node` (без ключа ядро молча берёт anyconnect); `token`: `mode` ∈ {totp,hotp,stoken,oidc}, `secret` обязателен при `mode`, конфликт `secret`/`secret_path`; сертификат и ключ только парой, путь и inline-материал не вместе; `mtu` ≤ 65535 (диапазон 576…1500 — правило формы); `user_agent` без `\r\n\0`; `peer_fingerprint` — регулярка по `parsePeerFingerprint` (без префикса — SHA-1 сертификата hex 4..40; `sha1:` hex ≤40; `sha256:` hex ≤64; `pin-sha256:` base64 ≤44). Wildcard `*` стоит в корне, `tls`, `token`, `fortinet_host_check`.

Новые коды `warnings.json`: `openconnect_flavor_invalid` (warning), `openconnect_fingerprint_invalid` (warning), `openconnect_mtu_invalid` (warning), `openconnect_insecure_crypto` (info).

Решения по открытым вопросам проекта: скрытое поле в теле остаётся; обязательность `username` снимают `cookie` и клиентский сертификат (OIDC не считается); подписи полей и блоков — ключи locale каждой стороны, не `desc_*` реестра.

### 2.3 Форма «Add server → OpenConnect» и правка узла (первая волна)

Три блока. Блок 1 всегда виден, Security раскрыт, Advanced свёрнут. Подписи английские через `locale.T`, перевод в `bin/locale/ru.json`. Макет:

```
Name      [ 🇮🇱 forti ]
Gateway   [ FortiGate (fortinet) ▾ ]      fortinet · anyconnect · gp · f5 · pulse · nc
Server    [ fw2.soft-master.com:42319 ]   host:port or full URL; port 443 if omitted
Username  [ lxinternet ]
Password  [ •••••••• 👁 ]                 If the gateway asks for a code, you will be prompted on connect (daemon mode only)

▾ Security
  Server certificate   (•) System CA   ( ) Pinned fingerprint [ sha256:… ]   ( ) Do not verify ⚠
  SNI            [ ]          only if it differs from the server host
  Group / Realm  [ ]          hidden for FortiGate (auth_group не используется этим flavor)
  [ ] Disable SSO (external auth)

▸ Advanced
  [ ] TCP only (no DTLS)            Use when UDP is blocked and the tunnel keeps dropping
  User-Agent        [ ]             Empty = core default (FortiGate: Mozilla/5.0 SV1)
  Hostcheck         [ ]             FortiGate only
  MTU               [ ]             Empty = from gateway; lower it if HTTPS stalls (576…1500)
  DPD interval      [ ]             Keep-alive probe period, e.g. 30s. Empty = from gateway
  Reconnect timeout [ ]             How long to retry a dropped tunnel, e.g. 5m. Empty = core default (5m)
  [ ] Disable IPv6
  [ ] Allow legacy crypto ⚠         only for old gateways (TLS 1.0)
```

Правила:
- Пустое поле ключ в JSON **не пишет** (дефолт остаётся за шлюзом/ядром). MTU пустой, не 0.
- `visible_when` из реестра: `fortinet_host_check` только при `flavor = fortinet`; `auth_group` скрыт при `fortinet`.
- Группа выбора сертификата — из `form.choices` реестра: `system` (ничего не пишет), `pinned` (`tls.peer_fingerprint`, список через запятую → массив), `insecure` (`tls.insecure: true`, окрашено предупреждением `tls_insecure`).
- Server: принимается `host:port` или URL; `https://` лаунчер не дописывает — ядро делает это само.
- `detour` формой не правится (цепочка — отдельный механизм). Что пришло в JSON, остаётся.
- Форма пишет только поля с `form` в реестре; прочие ключи тела (cookie, token, client cert, mobile, csd/hip/tncc, form_entries, compression_*, system…) не трогает. Правка узла — тот же набор полей из тела.
- Сохранение — как у всех узлов: `sing-box check` кандидата (core-reject) плюс правила реестра (формат отпечатка, конфликты) до записи.
- В классическом режиме на узле подсказка «Second-factor prompts need the daemon mode».

**Не в форме (ручной JSON):** `token.*`, клиентский сертификат, `cookie`, `system`, компрессия, `csd`/`hip`/`tncc`, `form_entries`, `mca_*`. Кнопка «Fetch fingerprint» и импорт из FortiClient — не в этой SPEC.

### 2.4 Подпись узла

`TransportLabel`: `transport` → `vhttp` → роль `variant` → пусто. Везде, где подпись строится из `config.json` (список серверов, Info, фильтр, превью) — одна функция. LxBox повторяет в `_deriveTransport`.

### 2.5 Второй фактор (форма) и SSO (браузер) — только под демоном

Канал: gRPC StartedService (`SubscribeOpenConnectStatus`, `SubmitOpenConnectAuthResponse`, `CancelOpenConnectAuthChallenge`). Доступен у локального lxd (`DaemonBackend`) и у удалённых машин (`LxdRemoteTransport`). В классике (`sing-box run`) канала нет; **сервис `api` в конфиг не добавляем** (решение владельца: лишних портов не открывать).

Факты ядра (ответ команды ядра 15.10.2026): челлендж без таймаута, endpoint в `auth-pending`, бокс стартует, в endpoint трафик не идёт; ядро не логинится повторно, пока висит челлендж; **Cancel** = новый логин через 1 с с бэкоффом до 60 с (новое письмо с кодом); неверный код = то же; неверный пароль — ядро стирает его из кэша и просит в форме заново (цикла нет).

Поведение лаунчера:
1. Подписка на стрим статуса openconnect по образцу Tailscale (локально `superviseOpenConnect`, удалённо `ensureOpenConnectStream`). Строка узла показывает состояние (`auth-pending` → «Waiting for sign-in», `error` → текст ошибки, `connected`).
2. **Форма** (`AuthChallenge.form`): модальный диалог поверх окна Servers с полями по `Form.Fields` как есть (kind text/password/select с `options`), banner/message/error ядра сверху. Кнопки: **Submit**, **Not now** (только закрывает диалог, в ядро ничего не шлёт), **Request a new code** (явный `Cancel…` с предупреждением «the gateway will send a new code»). После двух неверных кодов подряд — предупреждение о повторных логинах. Если форма принесла поле пароля — предложение сохранить новый пароль в узел.
3. **Браузер** (`AuthChallenge.browser`):
   - **callback-режим** (`callbackURLPrefixes` непустой, loopback; так работает FortiGate SAML): диалог «Sign in in your browser» с кнопкой **Open** (`fyne OpenURL`), строкой ссылки для копирования и ожиданием. На время диалога лаунчер слушает `127.0.0.1:<port>`; порт — из префикса, если указан, иначе из JSON-поля узла `lx_sso_callback_port`, по умолчанию 8020 (как FortiClient). Полученный URL редиректа → `BrowserResult{finalURL}` → Submit. Удалённая машина — тот же путь: браузер и слушатель на этом компьютере, ядро получает только строку URL.
   - **cookie/header-режим** (AnyConnect и др.): не поддерживается в этой SPEC; диалог с текстом «SSO for this gateway type is not supported yet», без Cancel.
4. Диалог не всплывает сам при каждом тике: показывается при появлении нового `challenge.id`, повторно — по клику на статус узла. Висящий челлендж при открытии лаунчера показывается сразу (стрим отдаёт текущее состояние).
5. Классический режим: ничего из п. 1–4; подсказка на узле (п. 2.3).

## 3. Контракт

Версия **1.1.122**: примитивы §2.1, запись `openconnect.json`, `openvpn-client.json` на `*`, удаление `fields_unchecked` из схемы, четыре кода, роль `variant` в enum ролей, корпус (см. `REGISTRY_DESIGN.md` §6: 8 кейсов тела + 1 авторский + корпус формы `corpus/form/openconnect_form.json`), `go generate`, `README.md` changelog, `TASKS_LXBOX.md` §118. Поддержка `*` входит в одну версию для обеих сторон: сторона без wildcard снимет сквозные ключи с `unknown_key`.

## 4. Вне scope

SSO в cookie/header-режиме (нужен встроенный браузер); кнопка «Fetch fingerprint»; импорт профилей FortiClient; поля `token.*` и клиентский сертификат в форме; сервис `api` в классике; `system: true`; компрессия в форме.

## 5. Проверки

`go build ./...`; именованные: `TestWildcardNestedStrict` (nodeflow), раннеры корпуса `TestContractCorpusBody`, `TestContractCorpusAuthored`, `TestRegistryBodyStructure`, `TestRegistryBodyCoversCoreSchema`, `TestRegistryWarningsHaveCauseAndFix`, новый раннер корпуса формы по имени. UI — без тестов, показ владельцу. Полный прогон — CI `run_mode=tests` в конце волны.
