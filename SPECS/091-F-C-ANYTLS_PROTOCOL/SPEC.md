# 091-F-C — AnyTLS протокол (парсер подписок + share-URI)

**Тип:** Feature · **Статус:** C (реализовано) · **Дата:** 2026-07-12 · **Ядро:** rc.17 (`C.TypeAnyTLS`, `option/anytls.go`) — есть.

## Проблема
Ядро rc.17 поддерживает AnyTLS-outbound, но парсер подписок лаунчера не читал `anytls://` —
узлы AnyTLS в подписках молча терялись. AnyTLS — растущий протокол (сессионный пул поверх TLS).

## Решение
Добавлен парсер по образцу TUIC/Trojan (единый credential в userinfo, обязательный TLS):

- `node_parser_anytls.go`
  - `buildAnyTLSOutbound` — `password` из userinfo (`node.UUID`, как Trojan; при отсутствии —
    WARN, но не фатально). Session-pool tuning (все опциональны): `idle_session_check_interval`,
    `idle_session_timeout` — bare-int трактуется как **секунды** (конвенция `normalizeTuicHeartbeat`);
    `min_idle_session` — валидируется как **non-negative int**, иначе дропается с WARN.
  - `buildAnyTLSTLS` — TLS **всегда включён**. `server_name`: из `sni`, фолбэк на `peer`, затем на
    адрес сервера; значение-заглушка `🔒` и строки без `.`/`:` игнорируются. `insecure`: истинно при
    любом из `insecure` / `allow_insecure` / `skip-cert-verify` / `skipCertVerify`. uTLS-`fingerprint`:
    из `fp`, фолбэк `fingerprint`, нормализуется через `NormalizeUTLSFingerprint`. `alpn` —
    comma-split с trim.
- `node_parser_core.go` — `anytls://` в `IsDirectLink`; scheme-dispatch (порт по умолчанию 443,
  TLS-валидация userinfo); build-dispatch.
- `shareuri_anytls.go` + `share_uri.go` — обратный encode. Round-trip **по смыслу, не byte-exact**:
  требует password/server/server_port; `insecure` эмитится в каноничном виде `insecure=1`;
  `min_idle_session` пишется только при `>0`; detour-literal и tag→fragment сохраняются.

## Формат URI
Читаемые ключи (не все обязательны):
```
anytls://password@host:port
  ?sni= &peer= &insecure=1 &allow_insecure= &skip-cert-verify= &skipCertVerify=
  &fp= &fingerprint= &alpn=
  &idle_session_check_interval= &idle_session_timeout= &min_idle_session=
  #name
```
Каноничный emit (share round-trip): `anytls://password@host:port?sni=&insecure=1&alpn=&min_idle_session=&idle_session_timeout=#name`

## Проверка (DoD)
- `go test ./core/config/subscription/ -run AnyTLS` — OK (parse canonical, build с TLS/fp/idle-tuning,
  missing-userinfo reject, share round-trip по смыслу). Альт. написания insecure/фолбэки sni·fp
  реализованы, но пока без прямого юнит-теста.
- Ядро rc.17 принимает эмит: `sing-box check` — OK.
- `go vet` — OK.
