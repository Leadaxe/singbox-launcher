# SPEC 149 — Типы endpoint из реестра; `openvpn-client` без описания полей

Статус: **C** (решение владельца 2026-09-28, реализовано 2026-09-28).
Тип: Feature. Контракт: 1.1.99. Встречная задача LxBox — §586
(`LxBox/docs/spec/tasks/586-endpoint-types-from-registry.md`).

## Решение владельца (28.09.2026)

«Делай в реестр, попроще, без правил, без проверок».

1. Знание «ядро считает этот тип endpoint, а не outbound» живёт в реестре
   контракта, один источник на обе системы; перечней в коде нет.
2. `openvpn-client` — известный тип: место в конфиге `endpoints[]`, ядру
   нужен тег сборки `with_openvpn`. Правил для полей и проверок значений нет,
   тело уходит в ядро как написано.
3. Полного импорта OpenVPN нет: ни формы, ни разбора `.ovpn`, ни ссылок, ни
   модели полей.
4. Тип вне реестра ведёт себя как прежде.
5. Новых правил для полей и признаков `core_rejects` не добавляется.

## Типы endpoint по ядру

sing-box-lx 1.14.2-lx.6 регистрирует `endpoint.Register` для `wireguard`,
`tailscale`, `openvpn-client`, `openvpn-server`, `openconnect`
(`protocol/*/endpoint.go`, `include/*_stub.go`). Записи реестра есть у
`wireguard`, `tailscale` и теперь `openvpn-client`; `openvpn-server` (серверная
роль) и `openconnect` в реестр не вносились — решение владельца касалось
только `openvpn-client`.

## Как устроено

- Раздел конфига узла задаёт поле `kind` записи протокола (`endpoint` /
  `outbound`), его читает `core/config/endpoint_schemes.go:IsEndpointScheme`.
  Перечня в коде лаунчера не было и до этой спеки (SPEC 142 A9) — менять
  пришлось только реестр.
- `contract/registry/protocols/openvpn-client.json`: `kind: endpoint`,
  источник `singbox` (маппер: `type` = `openvpn-client`), тело
  `fields_unchecked: true`, `order`/`fields` пустые, `build_tag: with_openvpn`,
  `min_core: 1.14.0-lx.10`, `on_core_unsupported: drop_node` с кодом
  `openvpn_core_unsupported` (warning).
- Атрибут тела `fields_unchecked` (схема `registry_body`): санитайзер
  (`nodeflow.SanitizeFromKind`) снимает только `tag` и `type` и копирует
  остальное без кодов; эмиттер (`nodeflow.Emit`) пишет тело ключами по
  алфавиту без экранирования HTML. `unknown_key` маппера у записи нет, поэтому
  `json_field_unknown` тоже не выдаётся.
- Гейт ядра — общий `nodeflow.NodeCoreRefusal`, как у `tailscale`.

## Проверка

- Корпус: `body/singbox/openvpn_client_endpoint` — тело с вложенными
  объектами, списком и незнакомым ключом проходит без изменений и без
  предупреждений, `kind` узла — `endpoint`.
- `core/config/nodeflow/fields_unchecked_test.go`: тело как написано,
  предупреждений нет; ядро без `with_openvpn` снимает узел кодом
  `openvpn_core_unsupported`, с тегом — нет.
