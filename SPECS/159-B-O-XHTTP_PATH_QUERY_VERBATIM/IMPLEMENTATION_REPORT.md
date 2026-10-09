# IMPLEMENTATION REPORT 159

Дата: 09.10.2026. Статус: реализовано, в develop (контракт — коммит 4c995c41); ждёт CI и приёмки владельцем.

## Что сделано

- `contract/registry/transports.json`: у записей `path` блоков `uri/xhttp` и `xray/xhttp` `extract` заменён на `maps_to: transport.path`; переписаны `desc_*`/`impl` параметра ссылки и поля тела xhttp. Код движка не менялся.
- Контракт 1.1.115: `contract/VERSION`, строка таблицы «Версии» в `contract/README.md`, `contract/TASKS_LXBOX.md` §112, `contract/docs/generated/**` (`go generate ./contract/...`: index, `_transports`, vless, trojan, vmess — только тексты описаний и версия).
- Корпус: `uri/vless/xhttp_path_query_tail_trimmed` → `xhttp_path_query_tail_kept`, `uri/vless/xhttp_path_ed_tail_stripped` → `xhttp_path_ed_tail_kept` (ожидание `path: /GaMeOpTiMiZeR?ed=2048`, ожидания правлены по норме вручную); новые `uri/vless/xhttp_path_proxyip_query_kept` (ожидание написано вручную) и `body/xray/xhttp_path_query_kept` (ожидание снято `-update` только этого кейса и сверено с нормой).
- Тесты: новый `core/config/xhttp_path_query_test.go` (`TestXHTTPPathQueryVerbatim`); `TestXHTTPv2_PathQueryTailTrimmed` → `TestXHTTPv2_PathQueryTailKept` в `core/config/subscription/xhttp_v2_test.go`.
- `docs/release_notes/upcoming.md` (EN/RU: пункт в Highlights/Основное; в Technical/Техническое — условие выпуска только с ядром, где есть sing-box-lx SPEC 119, `RequiredCoreVersion` пока `1.14.2-lx.12`), `SPECS/README.md`.

## Проверки

- `TestXHTTPPathQueryVerbatim` на реестре до правки падает на `/?proxyip=…` и `/base?x=1` у ссылки и Xray-JSON (`"/"`, `"/base"`), после — зелёный.
- Зелёные: `go test ./core/config -run 'TestXHTTPPathQueryVerbatim|TestGenerateNodeJSON_WS|TestContractCorpusURI|TestContractCorpusBody|TestContractCorpusEmitRoundTrip|TestCorpusBodiesPassSingboxCheck|TestRegistry|TestContractMapper'`, `go test ./core/config/subscription -run 'TestXHTTP|TestShareURI'`, `go test ./core/config/linkmap -run 'Corpus|Emit|Snapshot|RoundTrip'`.
- `gofmt -l` по двум тестовым файлам — пусто; `go vet ./core/config/ ./core/config/subscription/` — чисто; `go build ./...` — успешно.
- Повторный `go generate ./contract/...` дифф не меняет.

## Что выяснилось

- Ссылка «Поделиться» отдельной правки не потребовала: `queryEscape` кодирует хвост внутри `path=` (`path=%2F%3Fproxyip%3D192.0.2.10%26a%3Db`).
- Дедуп подписки различает узлы по эмиссии без тега: две ссылки, различающиеся только `proxyip`, до правки давали один узел, после — два.
- Тело узла в `state.json` из происхождения не перечитывается: исправление доходит до узлов подписки при обновлении, до узлов, добавленных ссылкой вручную, — при повторном добавлении.
- `core/config/linkmap/testdata/emit_snapshot.json` хранит записи под старыми именами кейсов; это снимок рукописного эмиттера с собственными телами без хвоста, раннеры по нему зелёные, правка не нужна.
