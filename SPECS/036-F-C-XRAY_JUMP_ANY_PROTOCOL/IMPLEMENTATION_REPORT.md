# IMPLEMENTATION_REPORT: 036 — XRAY_JUMP_ANY_PROTOCOL

## Статус

Завершено (**036-F-C**).

## Сделано

- **`configtypes.ParsedJump`**: поля **`Scheme`**, **`UUID`**, **`Flow`**; hop не обязан быть только SOCKS.
- **`xrayBuildJumpFromOutbound`**: диспетчер по `protocol` — **`socks`** (как раньше), **`vless`** через **`xrayBuildVLESSFromOutbound`** + тег jump; иные протоколы — ошибка, элемент массива пропускается (**`WarnLog`**).
- **`xray_json_array.go`**: удалена жёсткая проверка «только SOCKS»; вызов **`xrayBuildJumpFromOutbound`**.
- **`outbound_generator.go`**: генерация jump по **`node.Jump.Scheme`** (пустой → `socks`); для SOCKS по-прежнему подставляется **`version`** по умолчанию.
- **`source_loader.go`**: после **`MakeTagUnique`** для jump — **`node.Jump.Outbound["tag"]`** синхронизируется с итоговым тегом.
- Тесты: **`TestParseNodesFromXrayJSONArray_VLESSJump`**, **`TestParseNodesFromXrayJSONArray_UnsupportedJumpProtocolSkipped`**, проверка **`Scheme: socks`** в фикстуре провайдера.

## Не вошло (как в SPEC)

- **VMess / Trojan / …** как jump — по-прежнему не поддерживаются (явная ошибка и пропуск элемента).
- **Несколько** hop’ов подряд — не реализовано.

## Проверки

- `go test ./...`, `go vet ./...`, `go build ./...`
