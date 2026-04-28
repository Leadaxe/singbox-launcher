# SPEC 050-F-N — DEBUG_API_STATE_MUTATIONS

**Status:** New (N)
**Type:** Feature (F)
**Depends on:** SPEC 045 (state-config decoupling), SPEC 038 (debug API), SPEC 047 (event bus).

## Идея

После SPEC 045 `state.json` стал каноничным источником истины: Save визарда
пишет туда, BuildConfig читает оттуда. Все мутации внутри одного процесса —
через `StateService` (с dirty-маркерами + EventBus).

Этот SPEC добавляет **внешний канал записи** в state — типизированные
HTTP-эндпоинты в существующем debug-API (`core/debugapi`), bearer-аутентификация,
localhost-only. Цели:

1. **Автоматизация** — CI/скрипты могут править подписки, правила, vars без
   GUI-кликов («batch-импорт rule-set из файла», «прогнать `update-subs` на
   N разных DNS-конфигурациях»).
2. **MCP / agent integration** — Claude/agent редактирует правила голосом
   через стабильный JSON-контракт.
3. **Regression-фикстуры** — `GET /debug/snapshot` уже отдаёт срез; этот
   SPEC даёт обратное действие — *восстановить* state из snapshot'а одной
   командой (бережно: не трогает config.json без явного `update-subs`).
4. **Исследование UX** — быстро прокликивать «а что если 0 правил?», «а что
   если только final?», без перезапуска визарда.

## Не-цели

- **Не** добавляет авторизацию ролей: bearer = full-power. Кто получил
  токен — может всё.
- **Не** заменяет визард: GUI остаётся каноническим путём для рядового
  пользователя. API — для скриптов и инструментов.
- **Не** трогает config.json напрямую. Все мутации — *только* state.json;
  config.json пересобирается обычным `/action/update-subs` или Restart'ом.
- **Не** раскрывает endpoint'ы в публичную сеть: bind остаётся `127.0.0.1`,
  токен сохраняется в `bin/settings.json` рядом с существующим `DebugAPIToken`.

## Контракт endpoint'ов

Все запросы:
- `Authorization: Bearer <token>` — иначе 401.
- `Content-Type: application/json`.
- Локалхост-only (existing middleware).
- Sync-исполнение: ответ возвращается после `state.Save` + `EventBus.Publish`.

| Метод | Путь | Тело | Семантика |
|---|---|---|---|
| `GET` | `/state/full` | — | Полный state.json (RawMessage). |
| `PATCH` | `/state/custom_rules` | `{"mode":"replace\|append","rules":[...]}` | Замена или дозапись custom_rules. |
| `PUT` | `/state/dns/servers` | `[{"tag":"...","type":"...",...},...]` | Полная замена `dns_options.servers`. |
| `PATCH` | `/state/dns/rules` | `{"text":"..."}` | Замена `dns_options.rules` многострочным текстом (тот же формат, что в визарде). |
| `POST` | `/state/vars/{key}` | `{"value":"..."}` | Set одной vars-переменной. |
| `DELETE` | `/state/vars/{key}` | — | Удалить vars-ключ. |
| `PATCH` | `/state/parser_config` | `{"parser_config":{...}}` | Замена секции parser_config (proxies/outbounds). |
| `POST` | `/state/restore` | `<snapshot.files.state>` | Атомарно записать целый state.json (валидация схемы обязательна). |

**Универсальный response (success):**
```json
{
  "ok": true,
  "saved_path": "/Applications/.../bin/wizard_states/state.json",
  "dirty": {"update": true, "restart": true},
  "diff_summary": ["custom_rules: replace, 11 → 13 rules"]
}
```

**Error response:**
```json
{"error": "validation: dns.final must be enabled tag (got 'foo')", "field": "dns.final"}
```

Коды:
- 200 OK — успешно;
- 400 Bad Request — невалидное тело / схема;
- 401 — нет токена;
- 409 Conflict — состояние гонок (другая мутация в полёте, mutex held);
- 422 — семантическая ошибка (например, dns.final ссылается на несуществующий tag);
- 500 — внутренняя ошибка записи / I/O.

## Инварианты

1. **Атомарность записи**: каждый запрос делает 1 `state.Save` (`.tmp + Rename`).
   Никаких половинчатых state'ов на диске.
2. **Mutex**: единый `sync.Mutex` на путь к state.json — параллельные
   PATCH сериализуются (последний в очереди видит дифф предыдущего).
3. **Dirty-маркеры**: каждая мутация (кроме no-op'а с `mode:"replace"` где
   тело идентично текущему) поднимает `UpdateDirty`+`RestartDirty`.
   Семантически точную классификацию (sources → только Update,
   шаблон → только Restart) **не делаем** — better-safe.
4. **Events**: после успешной записи публикуется `events.StateChanged`
   с `Changed: ["api/<endpoint>"]`.
5. **Schema validation**: каждый endpoint проверяет JSON-схему *до* мутации.
   Никакого «частичного применения» — либо вся правка, либо ни одной.
6. **Идемпотентность `PUT`/`POST` с тем же телом**: ok, но всё равно поднимает
   маркер (cheap, чтобы не разводить сложности с диффом).

## Безопасность

- Bearer-токен — **единственный** trust-boundary. Тот же токен, что у `GET
  /debug/snapshot` (не вводим отдельный для записи — упрощает UX).
- Логирование: каждая мутация → `INFO StateMutation: <method> <path> by <ip>
  size=<bytes> dirty=update|restart`. **Тело запроса не логируется** (может
  содержать секреты в vars).
- Rate-limit: 10 req/sec per token, 429 при превышении (защита от
  взбесившихся скриптов, не от атак).

## UX в Diagnostics tab

Чекбокс «Allow state mutations via API» рядом с существующим debug-API
toggle. Default — **выключен**. Включён → endpoints отвечают; выключен →
все state-mutate возвращают 403 Forbidden. Read-endpoint'ы (`/state/full`,
`/debug/snapshot`) НЕ зависят от этого флага.

Дополнительно: при первой мутации показать info-toast «Внешний скрипт
изменил state. Откройте визард для просмотра». Toast гасится через 5 сек,
не блокирующий.

## Acceptance criteria

1. `curl -X PATCH .../state/custom_rules -d '{"mode":"replace","rules":[]}'`
   → state.json без custom_rules; UpdateDirty=true; RestartDirty=true.
2. `curl -X POST .../state/vars/dns_strategy -d '{"value":"prefer_ipv4"}'`
   → `state.vars.dns_strategy = "prefer_ipv4"`.
3. После любой мутации `GET /state` (existing) показывает `subs_last_updated_unix`
   неизменным (мутации state НЕ трогают тайминг подписок).
4. `POST /state/restore` с целым snapshot'ом из `GET /debug/snapshot` →
   побайтовый round-trip (`state` после == `state` до).
5. Toggle «Allow state mutations» в OFF → все endpoint'ы возвращают 403,
   GET endpoints продолжают работать.
6. Race-test: 100 parallel `POST /state/vars/X` → последний выигрывает,
   state.json валиден после, ни одна запись не потеряна посередине
   (final value == one of submitted, not corrupted).
7. Bearer-token misuse — 401 с `WWW-Authenticate: Bearer`, без content body.

## Risks & mitigations

| Риск | Mitigation |
|---|---|
| Скрипт сломал state.json неправильным мутатором | Schema-validate ДО `.tmp + Rename` |
| Параллельные мутации race | Single mutex per state path |
| Токен утёк → удалённая правка | localhost-bind остаётся; токен ротируется через GUI; «Allow state mutations» по умолчанию OFF |
| Юзер не понимает «откуда взялись правила» | Toast при первой API-мутации + лог в debuglog |
| Несовместимость с будущим schema bump | `state.version` валидируется; отказ выписывать в state с другой версии без явного миграционного флага |

## Out of scope (для будущего)

- Подписка на change-stream (Server-Sent Events / WebSocket) — следующий SPEC.
- Diff-based PATCH (RFC 6902 JSON Patch) — overkill для текущих нужд;
  mode:"replace|append" хватит.
- Optimistic concurrency через `If-Match: <state-hash>` — добавляется
  бесплатно после mutex-фазы; включить когда появятся multi-writer'ы.

## Связи

- **SPEC 045**: эта фича — расширение state-канала, добавляет writer'а
  кроме GUI Save.
- **SPEC 038**: добавляет endpoints в существующий `core/debugapi`, не
  создаёт нового сервера.
- **SPEC 047**: каждая мутация публикует `events.StateChanged` через
  существующий MemoryBus.
