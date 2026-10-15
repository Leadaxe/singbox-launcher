# SPEC 168 — Формы узлов по реестру: первая очередь (VLESS, Hysteria2, Trojan, WireGuard (AWG), naive, masque, SOCKS5, HTTP)

**Тип:** F (фича). **Статус:** O. **Контракт:** 1.1.123.
**Решения владельца:** 15.10.2026 (переписка: список протоколов и очереди, подписи «WireGuard (AWG)» и «OpenConnect (SSL VPN)»).
**База:** SPEC 167 — атрибут `form`, `FormState`, общая по сути модель `openconnect_form_model.go`. Эта SPEC её обобщает, а не пишет вторую.

## 1. Проблема

1. В «Add server» руками заводятся только SOCKS5, HTTP, WireGuard, Tailscale, OpenConnect. Самые ходовые протоколы (VLESS, Hysteria2, Trojan) — только вставкой ссылки или JSON.
2. SOCKS5/HTTP/WireGuard собраны вручную: свои виджеты, свой сборщик share-URI (`buildProxyURI`, `buildWireGuardURI`), свои проверки. Полей AmneziaWG в форме нет совсем, хотя реестр их описывает (`wireguard.json`: `jc`…`i5`, уровни `awg`/`awg1.5`/`awg2`).
3. Модель формы SPEC 167 привязана к схеме константой `OpenConnectScheme` и своей таблицей подписей; общих блоков (TLS, транспорт), которые делят VLESS/Trojan/HTTP, реестр для формы не описывает.

## 2. Решения

### 2.1 Очереди (решение владельца)

| Очередь | Протоколы |
|---|---|
| Уже есть | OpenConnect, Tailscale (ручная, не трогаем), Source |
| **Первая (эта SPEC)** | VLESS, Hysteria2, Trojan, WireGuard (AWG), naive, masque; SOCKS5 и HTTP переводятся на модель |
| Вторая (отдельная SPEC) | Shadowsocks, VMess, SSH |
| Не поддерживаем формой | TUIC, AnyTLS, Hysteria (v1) — только Source/JSON; chain, group — не серверы; openvpn-client — `.ovpn` через Source |

Порядок в списке типов: **VLESS · Hysteria2 · Trojan · WireGuard (AWG) · naive · masque · SOCKS5 · HTTP · OpenConnect (SSL VPN) · Tailscale · Source**. Подписи `WireGuard (AWG)` и `OpenConnect (SSL VPN)` — новые ключи locale с переводом в `ru.json`.

### 2.2 Одна модель, одна вьюха

- `ui/configurator/business/openconnect_form_model.go` → **`node_form_model.go`**: `NodeFormModel` строится по `(scheme, reg)`; типы `OpenConnectField*` → `NodeFormField*` (переименование, не копия). Подписи и подсказки — таблица по **пути тела** (`server`, `tls.server_name`, `transport.path`, …), общая для всех схем, плюс переопределения по схеме (`openconnect`: `server` → «Server» с подсказкой URL). Таблица — отдельный файл `node_form_labels.go`. Поле с `form`, у которого нет подписи, не рисуется (правило SPEC 167 сохраняется).
- `ui/configurator/dialogs/add_server_openconnect.go` → **`node_form_view.go`**: виджет формы по модели (блоки, выбор, видимость, обязательность, секреты с глазком, enum → Select, bool → Check, list → строка через запятую). Первый блок раскрыт, остальные — аккордеон; `advanced` свёрнут.
- Вкладка правки готового узла `tabs/source_openconnect_edit.go` → **`source_form_edit.go`**: та же вьюха для любой схемы первой очереди и OpenConnect. У `wireguard` блок `amnezia` в окне узла НЕ рисуется: обфускацию там правит существующий `source_awg_edit.go` (снятие при detour, перечитывание после пересборки) — один редактор на поле.
- Ручные ветки `modeSocks`/`modeHTTP`/`modeWireGuard`, `buildProxyURI`, `buildWireGuardURI`, `wgFieldHint`, `numEntry`-поля хоста/порта, `defaultPort` — **удаляются** вместе с тестами на них. Остаются `modeTailscale`, `modeSource` и один `modeForm` со схемой.

### 2.3 Выход формы — тело узла

Форма отдаёт тело (как OpenConnect, `ConfigJSON`), а не share-URI. Превью вкладки JSON — `ParseNodeDocument → RenderNodeDocument` (как у OpenConnect). Проверка при Add — `nodeflow.Sanitize` по реестру с показом кодов; ядро — на сборке, как сегодня. Пустое поле ключ не пишет. Форма меняет только свои пути; прочее тело (при правке) остаётся.

Следствие: узел из формы хранится как ручной JSON, а не как ссылка. Ссылку по-прежнему даёт эмиттер реестра там, где он есть.

### 2.4 Реестр — три примитива (контракт 1.1.123)

1. **`form` в общих секциях.** Атрибут `form` разрешён у полей `tls.json` и вариантов `transports.json`, а также у ref-полей схемы (`server`/`server_port` → `dialer.common`, `tls`, `transport`): `form` ref-поля переносится через `resolveSection` так же, как `required`/`forbidden_for`. Поле общей секции входит в форму схемы, только если:
   - у **ref-поля схемы** есть `form` (`"tls": {"type":"ref","ref":"tls","form":{"block":"tls"}}`) — схема подключает секцию явно;
   - и схема не в `forbidden_for` этого поля (Reality/uTLS у Hysteria2 и naive отпадают сами).
   `block` поля общей секции наследуется от `form.block` ref-поля, если свой не задан.
2. **Варианты (транспорт).** У ref-поля на вариантную секцию с `form` форма рисует дискриминатор (`transport.type`) как Select: пусто («None / TCP» — блок `transport` не пишется) + `values` секции, кроме перечисленных в `form.exclude_variants` ref-поля. Поля варианта видимы, только когда дискриминатор равен его имени. `FormState`/`walkForm` обходят `Variants` по текущему значению дискриминатора.
3. **`form.fixed` у тела схемы:** `{путь: значение}` — форма всегда пишет значение и поле не рисует. Hysteria2, naive, Trojan: `{"tls.enabled": true}`.

Плюс по месту (не примитивы):
- `transports.json` ws/httpupgrade: `headers` получает описанное поле `Host` и `"*"` (wildcard 1.1.122) — чтобы форма правила заголовок Host, а прочие заголовки шли насквозь.
- **Массив объектов — первый элемент.** Атрибут `form` у поля `type: array` с `items: object` значит: форма правит элемент `[0]` (создаёт его, если массива нет), остальные элементы не трогает. Пути формы внутри — `peers.0.address`. Нужен для WireGuard (`peers`). Грамматика пути с числовым сегментом — в `form.go` (`bodyValue`/запись) и в корпусе формы.

### 2.5 Раскладка форм (блоки и поля)

Блоки: `connection` (раскрыт), затем специфичные схемы, `tls`, `transport`, `advanced` (свёрнут). Порядок полей — `order` реестра.

| Схема | connection | свой блок | tls | transport | advanced |
|---|---|---|---|---|---|
| **vless** | server*, server_port*, uuid*, flow | — | Security: None / TLS / Reality (группа выбора `security`), server_name, utls.fingerprint, alpn, insecure; Reality: public_key*, short_id | type + поля варианта | packet_encoding, encryption |
| **hysteria2** | server*, server_port*, password | Obfuscation: obfs.type, obfs.password (видим при obfs.type) | server_name, alpn, insecure (fixed enabled) | — | server_ports, hop_interval, up_mbps, down_mbps |
| **trojan** | server*, server_port*, password* | — | server_name, utls.fingerprint, alpn, insecure (fixed enabled) | type + поля варианта | — |
| **wireguard** | peers.0.address*, peers.0.port*, private_key*, peers.0.public_key*, address* | Amnezia (свёрнут): jc, jmin, jmax, s1–s4, h1–h4, i1–i5 | — | — | peers.0.pre_shared_key, peers.0.allowed_ips, peers.0.persistent_keepalive_interval, mtu |
| **naive** | server*, server_port*, username, password | — | server_name (fixed enabled) | — | quic, quic_congestion_control (видим при quic) |
| **masque** | server*, server_port*, profile, private_key, public_key | — | server_name, insecure | — | ip, ipv6, vhttp, mtu, uri |
| **socks** | server*, server_port*, version, username, password | — | — | — | udp_over_tcp.enabled |
| **http** | server*, server_port*, username, password | — | TLS-галка (`tls.enabled`), server_name, insecure | — | path |

`*` — `form.required`. Группа выбора `security` у VLESS (`body.form.choices`): `none` (paths `[]`), `tls` (paths `["tls.enabled"]`, set `{tls.enabled: true}`), `reality` (paths `["tls.reality.enabled"]`, set `{tls.enabled: true, tls.reality.enabled: true}`). Активный вариант — **по самому длинному совпадению**: среди вариантов с заданным путём побеждает тот, у которого задано больше путей из `paths ∪ ключей set`; при равенстве — первый. Уточнение нормы `ActiveFormOption` (OpenConnect не меняется: его варианты не пересекаются).

AWG3 (`header_protection_key`, `id`/`ip`/`ib`, `content_padding_addition`, rekey-ключи, `random_trailers`, `disable_cookies`) — в первой волне только JSON. DNS у WireGuard формой не пишется: реестр его игнорирует (`wgconf_dns_ignored`).

### 2.6 Подписи узла и прочее

Подпись узла (`TransportLabel`) не меняется. Подсказка порта по умолчанию — из реестра (дефолт порта схемы, если описан), иначе пусто; своих чисел у формы нет.

## 3. Контракт 1.1.123

Схема `registry_body.schema.json` (form в общих секциях, `fixed`, `exclude_variants`, form у массива), записи `vless`/`hysteria2`/`trojan`/`wireguard`/`naive`/`masque`/`socks`/`http` (`body.form` + `form` полей), `tls.json`/`transports.json` (form у полей), норма активного варианта, корпус формы `contract/corpus/form/<scheme>_form.json` на каждую схему, `go generate ./contract/...`, `contract/README.md` changelog, `contract/TASKS_LXBOX.md` §120, `contract/docs/PARSING_PRINCIPLES.md` (если норма формы там описана). Санитайзер/эмиттер/гейт `form` по-прежнему не читают — корпус тела не меняется.

## 4. Вне scope

Вторая очередь; AWG3 в форме; multiplex, ECH, fragment, сертификаты клиента в форме; Tailscale на модели; импорт ссылки в форму (вставка ссылки остаётся в Source).

## 5. Проверки

`go build ./...`; `l10n_check --strict`; раннер корпуса формы по имени (`TestContractCorpusForm`), `TestRegistryBodyStructure`. UI — без тестов, показ владельцу. Полный прогон — CI `run_mode=tests` в конце волны.
