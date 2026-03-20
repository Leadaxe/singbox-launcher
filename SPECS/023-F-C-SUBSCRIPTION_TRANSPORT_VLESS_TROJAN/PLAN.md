# PLAN: Подписки — VLESS/Trojan transport и TLS

## 1. Архитектура

- Логика транспорта и условного TLS для URI вынесена в отдельный файл пакета `subscription`, чтобы не раздувать `node_parser.go`.
- `buildOutbound` заполняет `node.Outbound["transport"]` и `node.Outbound["tls"]` согласно sing-box; `GenerateNodeJSON` только сериализует уже готовые map, включая расширенный `transport` (в т.ч. `service_name`, `host` как массив для `type=http`).

## 2. Изменяемые файлы

| Файл | Изменения |
|------|-----------|
| `core/config/subscription/node_parser_transport.go` | **Новый:** `uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode`, вспомогательные функции |
| `core/config/subscription/node_parser.go` | VLESS/Trojan ветки в `buildOutbound`; VMess `grpc` → `service_name` |
| `core/config/outbound_generator.go` | `appendOutboundTransportParts`; порядок полей; `tls` при `enabled:false`; поддержка `host` string и `[]string` |
| `ui/wizard/tabs/source_tab.go` | `MakeTagUnique` в `fetchAndParseSource` |
| `core/config/subscription/node_parser_test.go` | Новые тесты transport/TLS |
| `core/config/generator_test.go` | Тест JSON без tls при `security=none` |
| `docs/release_notes/upcoming.md` | Пункт релиза EN/RU |

## 3. Порядок работ

1. Реализовать транспорт и TLS в `buildOutbound` + новый файл.
2. Расширить `GenerateNodeJSON` и VMess gRPC.
3. Визард + release notes.
4. Тесты и проверка с документацией sing-box.

## 4. Риски

- `xhttp` в Xray и `httpupgrade` в sing-box не эквивалентны на 100%; параметр `mode` из подписок не попадает в JSON (нет в официальной схеме httpupgrade).
- gRPC в sing-box может требовать отдельной сборки бинарника.
