# REVIEW_LXBOX_MIRROR — зеркальные дефекты LxBox (SPEC 134)

Контракт **1.1.44**. Три пункта из pre-release ревью партнёра; воспроизведение на `develop`.

| № | У нас воспроизводится? | Что сделано | Тест |
|---|------------------------|-------------|------|
| 1. CIDR `1.2.3.4/64`, `::::/128` и др. в `address` / `allowed_ips` | **Нет** — `format cidr` (`net.ParseCIDR`) отвергает; узел с негодным `address` — `type_invalid` | Кейс корпуса `uri/wireguard/address_cidr_invalid`; страж `TestCIDRFormatRejectsInvalidPrefixes` | `go test ./core/config/nodeflow -run TestCIDRFormat -count=1`; `go test ./core/config -run 'TestContractCorpusURI/wireguard/address_cidr_invalid' -count=1` |
| 2. Emit WireGuard с двумя `peers` | **Нет** — `emit.refuse_when` отказывает с причиной «ОДИН удалённый сервер» | Стражи на `[]interface{}` и `[]map[string]interface{}` | `go test ./core/config/linkmap -run TestEmitWireGuardRefusesMultiplePeers -count=1`; `go test ./core/config/subscription -run TestShareURIFromWireGuardEndpoint_MultiPeer -count=1` |
| 3. Дробный `port` (443.9) в vmess JSON / Xray | **Нет** — не усекается; `asInt`/`coerce` отбраковывает; `443.0` допустим | Кейсы `uri/vmess/fractional_port`, `body/xray/vmess_fractional_port`; стражи mtu/reserved | `go test ./core/config/nodeflow -run 'TestFractional|TestWholeFloat' -count=1`; `go test ./core/config/subscription -run 'TestVMessJSON|TestXrayJSON' -count=1` |

Код движка не менялся — только корпус, тесты, бамп `contract/VERSION`.
