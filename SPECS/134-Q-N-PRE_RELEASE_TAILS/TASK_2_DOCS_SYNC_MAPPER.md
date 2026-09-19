# ТЗ 2 · Сверка документации с новым устройством разбора источников

**Цель одной фразой:** привести общие документы (`docs/`, `README*`,
архитектурные файлы, `contract/docs`) в соответствие с реальностью — разбор
источников узла ведёт общий движок от таблиц реестра, а рукописные парсеры
ссылок, конвертер Xray и конвертер `.conf` удалены, — и заодно дособрать
черновик релизных заметок по коммитам с последнего тега.

---

## 0. Что изменилось в коде (контекст для того, кто репозиторий видит впервые)

Приложение — десктопный лаунчер VPN-ядра `sing-box`. Пользователь добавляет
«источники» узлов: share-ссылки (`vless://`, `vmess://`, …), подписки, JSON
конфигов Xray, файлы `.conf` WireGuard. Из всего этого строится
`config.json` для ядра.

**Как было.** Под каждую схему ссылки был свой рукописный парсер на Go
(`node_parser_vless.go`, `node_parser_vmess.go`, …), плюс отдельный
рукописный конвертер элементов Xray-JSON, плюс отдельный конвертер `.conf`
в промежуточную ссылку. Одни и те же правила лежали в трёх-четырёх копиях и
успели разойтись: один и тот же битый узел ссылкой принимался, а объектом —
нет.

**Как стало (SPEC 133).** Всё это удалено. Разбор ведёт **один общий движок**
`core/config/linkmap`, а *что* и *куда* класть — описано **данными** в
таблицах `contract/registry`. В коде движка нет ни одного имени схемы или
протокола.

**Источник истины по устройству движка:** `contract/docs/MAPPER_ENGINE.md`.
Все прочие документы обязаны ему не противоречить и при подробностях
ссылаться на него, а не пересказывать.

Дополнительно почитать: `SPECS/133-F-N-REGISTRY_DRIVEN_LINK_MAPPER/SPEC.md`,
`contract/README.md` (таблица секций реестра, строки 69–75) и историю версий
контракта там же (строки 285–315 — это же и есть человеческое описание, что
именно снесли на каждом шаге).

---

## 1. Границы

### Входит

1. Правка `.md`-файлов: `docs/**`, `README.md`, `README.ru.md`,
   `contract/docs/IDENTITY.md`, `contract/README.md`.
2. Актуализация `docs/release_notes/upcoming.md`.

### НЕ трогать

- **Ни строки Go-кода.** Задача чисто документационная. Если по ходу
  обнаружится расхождение, которое лечится кодом, — записать в отчёт, не
  чинить.
- `core/config/linkmap/**`, `contract/registry/**`,
  `core/config/subscription/shareuri_*.go` — сейчас в работе у другого
  исполнителя; **только читать**.
- `contract/docs/MAPPER_ENGINE.md` — источник истины, он актуален; не
  «улучшать».
- `contract/docs/generated/**` — генерируется из реестра
  (`go generate ./contract/...`), руками не править.
- **Исторические релизные заметки** `docs/release_notes/1-*.md`,
  `0-*.md` — это протокол на момент выпуска. Упоминания удалённых функций
  там **законны**, исправлять нельзя.
- `SPECS/**` — журналы реализации, тоже слепки времени.

---

## 2. Разведка (проверено, ссылки точные)

### 2.1 Что именно удалено — как получить список самому

```
git log --diff-filter=D --name-only --pretty=format:'%h %s' -- core/config/subscription | sort -u
git log --diff-filter=D --name-only --pretty=format:'%h %s' -- core/config | sort -u
```

Тело удалённого файла (чтобы вынуть имена функций):

```
git show <commit>^:<путь> | rg '^func '
```

**Удалённые файлы, релевантные докам:**

`node_parser_anytls.go`, `node_parser_http.go`, `node_parser_hysteria.go`,
`node_parser_hysteria2.go`, `node_parser_masque.go`, `node_parser_naive.go`,
`node_parser_socks.go`, `node_parser_ss.go`, `node_parser_ssh.go`,
`node_parser_tuic.go`, `node_parser_vmess.go`, `node_parser_wireguard.go`,
`xray_hysteria.go`, `outbound_tls_emit.go`, `share_uri_encode.go`
(последний не удалён по смыслу, а разъят на `share_uri.go` + `shareuri_*.go`
ещё в SPEC 070).

**Удалённые функции, которые ещё называют доки:** `buildOutbound`,
`wgConfToURI`, `parseWireGuardURI`, `parseVMessDecoded`, `parseVMessJSON`,
`parseVMessLegacyCleartext`, `uriTransportFromQuery`, `vlessTLSFromNode`,
`trojanTLSFromNode`, `xrayBuildVLESSFromOutbound`,
`xrayBuildVMessFromOutbound`, `xrayBuildTrojanFromOutbound`,
`xrayBuildShadowsocksFromOutbound`, `xrayBuildHysteria2FromOutbound`,
`normalizeVMessSecurity`, `emitOutboundTLSJSON`, `IsSocksScheme`.

**Живо и упоминать можно:** `ParseNode` (осталось три ветки),
`node_parser_engine.go`, `node_parser_core.go` (`normalizeFlagTag` там жива),
`node_parser_amnezia.go`, `wgconf_text.go` (`WGConfBodyToURIs`),
`xray_json_array.go`, `xray_balancer.go`, `xray_outbound_convert.go`
(сильно урезан — остался уровень **документа**: какой элемент становится
узлом, какой хопом цепочки, какой группой), `share_uri.go` + `shareuri_*.go`.

### 2.2 Точный список правок по файлам

| Файл | Строка(и) | Что не так |
|------|-----------|------------|
| `docs/ParserConfig.md` | **58** | Строка таблицы «The `core/config/subscription` package» перечисляет `buildOutbound`, `node_parser_vmess.go` (`parseVMessDecoded`/`parseVMessJSON`/`parseVMessLegacyCleartext`), `node_parser_hysteria2.go`, `node_parser_wireguard.go`, `node_parser_ssh.go`, `xray_protocols.go` — всё удалено |
| `docs/ParserConfig.md` | 697 | `normalizeFlagTag` в `node_parser_core.go` — **актуально**, не трогать |
| `docs/ParserConfig.ru.md` | **57** | Зеркало строки 58 |
| `docs/ParserConfig.ru.md` | 696 | Актуально, не трогать |
| `docs/Protocols.md` | **85** | Реализация Xray-массива: `xray_outbound_convert.go` названа как конвертер протоколов — теперь она про уровень документа |
| `docs/Protocols.md` | **133** | Тот же устаревший инвентарь пакета, что в ParserConfig:58 |
| `docs/Protocols.md` | **139** | «`ParseNode` → `buildOutbound`» — второй функции нет |
| `docs/Protocols.md` | **143** | «тот же набор полей, что даёт `parseWireGuardURI`» — функции нет |
| `docs/Protocols.md` | **145** | `uriTransportFromQuery`, `vlessTLSFromNode`, `trojanTLSFromNode` в `node_parser_transport.go` — нет ни одной |
| `docs/Protocols.md` | **164** | «Поля JSON узла согласованы с `parseVMessJSON`» — функции нет |
| `docs/Protocols.md` | 184 | `share_uri_encode_test.go` — **проверить существование файла** перед правкой |
| `docs/Protocols.md` | 199, 230 | `node_parser_amnezia.go` / `wgconf_text.go` живы, но **путь описан неверно**: `.conf` больше не переводится в промежуточную ссылку, его ведёт секция `mappers.conf` реестра |
| `docs/Protocols.ru.md` | **83, 130, 136, 140, 142, 161**, 181, 195, 225 | Зеркало всего перечисленного |
| `docs/ARCHITECTURE_PACKAGES.md` | **180–191** | Таблица файлов пакета `subscription`: строки `node_parser_core.go` (упомянут `buildOutbound`), `node_parser_transport.go`, `node_parser_vmess.go`, `node_parser_ss.go`, `node_parser_ssh.go`, `node_parser_hysteria2.go`, `node_parser_naive.go`, `node_parser_tuic.go`, `node_parser_wireguard.go` — девять строк про удалённые файлы; нет `node_parser_engine.go` и нет пакета `core/config/linkmap` |
| `docs/ARCHITECTURE_PACKAGES.ru.md` | **182–193** | Зеркало |
| `docs/ARCHITECTURE.md` | **316–318** | §6.0: «**Mapper** (`core/config/subscription/*`)» — пакет назван неверно, это `core/config/linkmap` по таблицам `contract/registry/**/mappers.*` |
| `docs/ARCHITECTURE.md` | **358–359** | §6.1, ветка «URI list»: «`node_parser.ParseNode` per line (scheme dispatch → protocol parser → transport/TLS build …)» — диспетчера по схемам и протокольных парсеров больше нет |
| `docs/ARCHITECTURE.md` | 521, 597, 637–639 | Историческая запись про раскол монолитов SPEC 070 — **перечитать и решить**: как *история* это законно, как описание *текущей* раскладки — врёт. Минимум — пометить временем |
| `docs/ARCHITECTURE.md` | **671** | ADR-070-6 «объединить `uriTransportFromQuery` + `xrayTransportFromStreamSettings`» — задача снята самим SPEC 133, отложенный пункт больше не отложен |
| `docs/ARCHITECTURE.ru.md` | 475, **491** | То же: имена монолитов и тот же ADR |
| `contract/docs/IDENTITY.md` | **259** | «строится из схемы (`node_parser_core.go`)» — логика тега уехала в реестр |
| `contract/README.md` | **71, 72, 74** | Строки таблицы секций: у `uri` написано «Go-загрузчик секцию не разбирает, парсеры ссылок написаны руками. Исполняемой делает SPEC 133»; у `mapper` — «ни автопроверки, ни исполнения сегодня нет»; у `emit` — «`share_uri` и `param_order` в Go не читаются ни одной строкой». Все три — **прошедшее время**, SPEC 133 это сделал |
| `README.md`, `README.ru.md` | — | Имён удалённых функций **нет**; но оба ведут читателя в `docs/ParserConfig.md`. Проверить, что общая проза («парсер подписок») после правок не противоречит |

### 2.3 Релизные заметки

- Черновик живёт в **`docs/release_notes/upcoming.md`** (сейчас 78 строк).
- Структура файла: шапка с правилом «не добавлять мелкие правки только UI»,
  затем `## EN` → `### Highlights` / `### Fixes` /
  `### Technical / Internal`, затем зеркальный `## RU` → `### Основное` /
  `### Исправления` / `### Техническое / Внутреннее`.
- Стиль: законченные предложения от лица пользователя — «что было плохо → что
  стало». Не «добавлена функция X», а «один негодный сервер больше не стоит
  вам всего VPN». RU — **не подстрочник EN**, а тот же смысл по-русски.
- Образцы готовых файлов: `docs/release_notes/1-6-3.md` (патч, только
  Fixes + Technical), `docs/release_notes/1-2-7.md` (фича, с Highlights и
  Notes).
- Последний тег: **`v1.6.3`**. Коммитов с него — около 144.

```
git describe --tags --abbrev=0
git log v1.6.3..HEAD --pretty=format:'%h %s'
```

**Важно:** `upcoming.md` уже плотно заполнен и уже покрывает крупные волны
(SPEC 131 — единый конвейер узла, SPEC 132 — страховка «ядро отвергло узел»,
SPEC 133 — движок разбора). **Задача не переписать файл, а найти пробелы**:
пройти список коммитов и отметить, что из пользовательски заметного в него
ещё не попало.

---

## 3. Ловушки

1. **Не «исправлять» историю.** Исторические `docs/release_notes/*.md` и
   журналы в `SPECS/**` описывают мир на свою дату. Упоминание там
   `buildOutbound` — не ошибка.
2. **Русские файлы — отдельные документы, а не перевод.** У пары
   `docs/ARCHITECTURE.md` / `docs/ARCHITECTURE.ru.md` содержание уже
   разошлось: §6.0 есть в EN и отсутствует в RU. Синхронизировать целиком
   **не входит в задачу** — правьте только перечисленные строки, расхождение
   объёма запишите в отчёт.
3. **Не пересказывать `MAPPER_ENGINE.md`.** Подробности устройства движка —
   ссылкой на него. Копия разойдётся так же, как разошлись парсеры.
4. **Проверять факт, а не верить таблице выше.** Перед каждой правкой
   убедиться `rg`-ом, что файл/функция действительно отсутствует: часть имён
   жива (`ParseNode`, `normalizeFlagTag`, `WGConfBodyToURIs`,
   `xray_json_array.go`), и снести их из доков было бы новой ошибкой.
5. **`xray_outbound_convert.go` жив, но означает другое.** Он больше не
   конвертирует протоколы — он решает на уровне **документа**, какой элемент
   становится узлом, какой звеном цепочки, какой группой-балансером.
   Оставить имя, поменять описание.
6. **Про `.conf`.** Раньше `.conf` переводился в промежуточную ссылку
   (`wgConfToURI`) и потом разбирался как ссылка. Теперь его читает секция
   `mappers.conf` напрямую, и промежуточной ссылки нет. Доки, описывающие
   старый двухшаговый путь, врут о поведении, а не только об именах.
7. **`upcoming.md` — не свалка.** В шапке файла прямо написано: не добавлять
   правки только UI (порядок виджетов, выравнивание, стиль кнопок без смены
   действия). Писать новое поведение: данные, форматы, сохранение, заметные
   возможности.
8. **Ссылки между документами.** После правок проверить, что относительные
   ссылки в таблицах не ведут на переименованные якоря.

---

## 4. Шаги

1. Собрать факты самому: два `git log --diff-filter=D` из §2.1 и `rg` по
   каждому имени из списка удалённых функций по `docs/`, `README*.md`,
   `contract/docs/`, `contract/README.md`.
2. Свести результат в рабочий список «файл:строка → что заменить». Сверить
   со списком из §2.2 — расхождения разобрать (документ мог сдвинуться).
3. Прочитать `contract/docs/MAPPER_ENGINE.md` целиком — формулировки
   заимствовать оттуда.
4. Править по файлам, начиная с самых «инвентарных» (они дают наибольший
   эффект): `docs/ARCHITECTURE_PACKAGES.md(.ru.md)` → `docs/ParserConfig.md`
   (`.ru.md`) → `docs/Protocols.md` (`.ru.md`) → `docs/ARCHITECTURE.md`
   (`.ru.md`) → `contract/README.md` → `contract/docs/IDENTITY.md`.
5. `upcoming.md`: пройти `git log v1.6.3..HEAD`, отметить пользовательски
   заметное, не попавшее в файл, дописать пунктами EN и RU в принятом стиле.
6. Перечитать `README.md` / `README.ru.md` на непротиворечивость.

---

## 5. Как проверить

Задача документационная, кода не касается. Тем не менее перед коммитом:

```
go build ./...
```

— чтобы убедиться, что дерево в рабочем состоянии (правки другого
исполнителя тоже лежат в этой копии) и вы ничего не задели случайно.

Тестов по этой задаче **не писать**: тестов на тексты и формат строк в
проекте нет принципиально.

**Запрещено локально:** `go test ./...`, `go vet ./...` по репозиторию,
Win7-сборка с `go.win7.mod`.

**Запрещено вообще:** запускать `sing-box run`, убивать процессы sing-box
(`pkill`, `killall`) — на машине живой VPN.

Если правились `contract/**` — **не** запускать `go generate ./contract/...`:
генератор трогает `contract/docs/generated/**`, а этот каталог в задачу не
входит и сейчас может быть в работе у другого исполнителя.

Полный прогон — в CI, после захода:

```
gh workflow run ci.yml --ref develop -f run_mode=tests
```

---

## 6. Как коммитить

Ветка **только `develop`**. Запрещены `git checkout`/`switch`, `stash`,
`reset`, `rebase`, `clean`, `git add -A`, `git commit -a`. Рабочая копия
разделяемая — чужие незакоммиченные правки не трогать.

Явным списком путей, двумя коммитами (доки и релизные заметки — разное):

```
git commit -m "docs: сверка документации с движком разбора источников" -- \
  docs/ARCHITECTURE.md docs/ARCHITECTURE.ru.md \
  docs/ARCHITECTURE_PACKAGES.md docs/ARCHITECTURE_PACKAGES.ru.md \
  docs/ParserConfig.md docs/ParserConfig.ru.md \
  docs/Protocols.md docs/Protocols.ru.md \
  contract/README.md contract/docs/IDENTITY.md

git commit -m "docs(release-notes): черновик upcoming по коммитам с v1.6.3" -- \
  docs/release_notes/upcoming.md

git push origin develop
```

---

## 7. Формат отчёта (≤ 8 строк)

```
1. Файлов правлено: <N>, строк заменено: <M>
2. Самое существенное: <2 строки — какие утверждения были ложны>
3. upcoming.md: добавлено пунктов EN/RU: <N>/<M>; по коммитам <диапазон>
4. Проверено фактом: <как убеждались, что функция/файл действительно удалены>
5. go build ./...: OK / ошибка
6. Расхождение EN/RU по объёму: <где нашли, или «нет»>
7. Найдено, но не чинилось (лечится кодом): <или «нет»>
8. Требует решения владельца: <или «нет»>
```
