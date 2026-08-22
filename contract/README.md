# LX Shared Contract

Единый контракт данных между десктопным лаунчером (Go) и LxBox (Flutter/Dart).
Программа и решения: `SPECS/103-F-O-LX_SHARED_CONTRACT/` (SPEC, PLAN, DECISIONS).

**Контракт — это данные и документы, ни строки исполняемого кода.** Раннеры живут
в проектах и читают этот каталог.

## Состав

| Путь | Что это |
|------|---------|
| `VERSION` | semver контракта: major — изменение правил канонизации, minor — новые конструкции/разделы, patch — уточнения |
| `docs/CANON.md` | правила канонизации канонического узла (нормативно) |
| `docs/IDENTITY.md` | алгоритм identity-хеша и ключа (нормативно) |
| `docs/TEMPLATE_LANG.md` | язык шаблонов v1 (нормативно) |
| `docs/BACKUP.md` | семантика LX Backup v1 (нормативно) |
| `registry/protocols/<scheme>.json` | реестр протоколов: по файлу на схему |
| `registry/transports.json`, `registry/tls.json` | общие суб-схемы транспортов и TLS |
| `registry/allowlists.json` | канонические allowlist'ы (uTLS fp, ss-методы, …) |
| `registry/warnings.json` | коды warning'ов (локализация — на стороне приложений) |
| `registry/limits.json` | лимиты (URI, нод, тело, цепочки) |
| `registry/vars.json`, `registry/presets.json` | переносимые имена переменных и preset id |
| `registry/containers.json` | контейнеры тел (`vpn://`, wgconf INI, base64) |
| `schema/*.schema.json` | JSON Schema узла, Направления, бэкапа, реестра |
| `diagrams/*.mmd` | нормативные mermaid-блок-схемы пайплайна |
| `corpus/` | golden-фикстуры (фаза 1+) |

## Правила изменения

1. **Контракт раньше кода.** Новый протокол/параметр/приём начинается с PR сюда
   (реестр + фикстуры), реализации — вторым шагом.
2. Golden-файлы `corpus/**/*.expected*` нормативны: перегенерация раннером
   с `-update` — это осознанный PR с ревью диффа, не рутина.
3. Каждое расхождение проектов либо чинится, либо явно помечается
   `"extension": "desktop"|"mobile"` — бесхозных различий не бывает.
4. Решения протоколируются в `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md`.
5. **Арбитраж неоднозначного парсинга** (D-019): если из кода/тестов обоих
   проектов непонятно, как разбирать параметр подписки — порядок сверки:
   поведение ядра sing-box-lx → как параметр парсит **Happ** → конвенции
   Xray/v2rayN. Вердикт — в note реестра + DECISIONS.md.

## Потребители

- Лаунчер: конформанс-тесты `core/config/subscription/contract_test.go` (фаза 1).
- LxBox: копия каталога через `app/tool/sync_contract.sh` + пин `contract.lock`;
  тесты `app/test/contract/` (фаза 1).

## Версии

| Версия | Что появилось |
|--------|---------------|
| 0.1.0 | Каркас: канонический узел, реестры протоколов/лимитов/warning'ов/переменных, корпус URI (282 фикстуры), язык шаблонов |
| 0.1.1 | `#enable`, рекурсия условий, помеченные ключевые слова (`#and`/`#or`/`#value`/`#else`/`#on_change`/`#set`), раздел `corpus/template/deps/` |
| 0.2.0 | Корпус тел подписок (`corpus/body/`) и бэкапов (`corpus/backup/`); коды деградации на узле; правило `scheme` в CANON §1; закрыты расхождения gecko, выбора vpn://-контейнера и несжатого профиля |
| 0.3.0 | Направления (SPEC 104): `schema/direction.schema.json`, корпус `corpus/direction/`, перенос целей правил в бэкапе (`directions[]`, схема v1.1) |
