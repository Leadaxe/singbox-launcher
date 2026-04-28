# SUB SPEC 038 — Snapshot endpoint

**Родительский SPEC:** [SPEC 038 — Debug API](SPEC.md). Эта саб-спека добавляет один endpoint в существующий API без изменения базовых инвариантов (loopback, Bearer-auth, off by default).

**Триггер:** SPEC 045 фаза 5.3 (strangler-fig порт `BuildConfig`) требует «снять с dev-машины N реальных сценариев» как golden-data. Сейчас это делается ручным `cp` четырёх файлов, что утомительно. Один HTTP-вызов решает проблему.

---

## 1. Endpoint

| Method | Path              | Auth | Описание |
|--------|-------------------|------|----------|
| GET    | `/debug/snapshot` | ✓    | Возвращает срез четырёх файлов wizard-pipeline'а **как есть** (без редакций). |

### 1.1 Response shape

```json
{
  "captured_at": "2026-04-27T12:34:56Z",
  "launcher_version": "v0.8.8.3",
  "singbox_version": "1.13.11",
  "files": {
    "template": { ... содержимое bin/wizard_template.json как parsed JSON ... },
    "state":    { ... содержимое bin/wizard_states/state.json ... },
    "cache":    { ... содержимое bin/outbounds.cache.json ... },
    "config":   { ... содержимое bin/config.json ... }
  },
  "missing": ["cache"],
  "errors": {
    "config": "invalid character ',' looking for beginning of value"
  }
}
```

**Поля:**

- `captured_at` — UTC RFC3339 момент сборки snapshot'а.
- `launcher_version` / `singbox_version` — для воспроизведения в bug-репорте.
- `files.<name>` — содержимое соответствующего файла как **уже-распарсенный JSON** (не строка). Если файла нет на диске или он битый — поле отсутствует.
- `missing` — список файлов, которых физически нет на диске. Норма: на чистой установке без прогона Update нет `cache`; без Save визарда нет `state`.
- `errors` — map `{file_name: error_message}` для файлов, которые есть, но не парсятся как JSON. Это редкий случай (обычно баг лаунчера или ручная правка).

### 1.2 Никаких редакций

Snapshot возвращает четвёрку файлов **как есть**. Bearer-токен на endpoint'е — это наш trust boundary: кто может аутентифицироваться, тот владелец установки. Маскировать секреты от самого себя не имеет смысла.

**Следствия для пользователя:**
- При ручном шаринге snapshot'а **в публичный bug-репорт** (GitHub issue / chat) — пользователь сам несёт ответственность за чистку sensitive полей. UI рядом с кнопкой «Copy snapshot» в Diagnostics tab показывает warning «Snapshot contains your secrets (Clash secret, WireGuard private keys, subscription URLs). Don't share publicly without redaction».
- Снапшот, использующийся локально для golden-data в `core/build/testdata/golden/<scenario>/` — обезличивается **руками** при копировании в репозиторий проекта (если пользователь захочет залить scenario как тест-кейс на upstream).

### 1.3 Идемпотентность

Read-only. Никаких побочных эффектов.

---

## 2. Архитектура

### 2.1 Пакет `core/debugapi/`

Новый файл `core/debugapi/snapshot.go`:

```go
type snapshotResponse struct {
    CapturedAt      string                     `json:"captured_at"`
    LauncherVersion string                     `json:"launcher_version"`
    SingboxVersion  string                     `json:"singbox_version"`
    Files           map[string]json.RawMessage `json:"files,omitempty"`
    Missing         []string                   `json:"missing,omitempty"`
    Errors          map[string]string          `json:"errors,omitempty"`
}

func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) { ... }
```

Один файл, ~80 LOC.

### 2.2 ControllerFacade

Добавляется один метод:

```go
type ControllerFacade interface {
    // ... existing ...
    GetExecDir() string  // НОВОЕ — для резолва путей через platform.GetWizard*Path / GetConfigPath / GetOutboundsCachePath
}
```

В `core/debugapi_wiring.go`: `func (f *debugAPIFacade) GetExecDir() string { return f.ac.FileService.ExecDir }`.

Snapshot-сборщик читает файлы через канонические helpers `platform.GetWizardTemplatePath` / `GetWizardStatePath` / `GetOutboundsCachePath` / `GetConfigPath` — никаких хардкод-путей (политика v0.8.8.2).

### 2.3 Поведение чтения

Для каждого из четырёх файлов:
- `os.ReadFile(path)`:
  - `os.IsNotExist(err)` → добавить ключ в `missing`, перейти к следующему;
  - другая ошибка → добавить ключ в `errors[name] = err.Error()`;
- `json.RawMessage` (валидируем синтаксис через `json.Valid` — без полного парсинга, чтобы скорость не страдала на 5-МБ outbounds):
  - не валиден → `errors[name] = "invalid JSON: ..."`;
  - валиден → `files[name] = raw`.

`json.RawMessage` как тип в `Files` map'е — он сериализуется как inline-JSON, а не как строка-в-строке. Получается читаемый итоговый объект.

---

## 3. Тесты

`core/debugapi/snapshot_test.go`:

1. **`TestSnapshot_AllFilesPresent`** — fake exec-dir с четырьмя файлами; snapshot собирает все четыре; missing/errors пусты.
2. **`TestSnapshot_MissingCache`** — без `outbounds.cache.json` → `missing == ["cache"]`, остальные три на месте.
3. **`TestSnapshot_AllMissing`** — пустая bin-директория → все четыре в missing, files пуст.
4. **`TestSnapshot_CorruptJSON`** — config — невалидный JSON → попадает в `errors["config"]`, не в missing, остальные три файла норм.
5. **`TestSnapshot_NoRedaction`** — config с `experimental.clash_api.secret = "deadbeef"`; в response **строго** видно `"deadbeef"`, никакого "REDACTED". Регрессионный pin: эта саб-спека намеренно не маскирует.
6. **`TestSnapshot_FilesAreInlineJSON`** — `files.template` в response — JSON-объект, а не строка-в-строке.
7. **`TestSnapshot_AuthRequired`** — GET без `Authorization` → 401.
8. **`TestSnapshot_GETOnly`** — POST → 405.

---

## 4. UI / docs

- **Diagnostics tab** — кнопка «Copy snapshot» рядом с «Copy token». Curl на свой сервер → clipboard. Toast-подтверждение. Под кнопкой — мелкий warning-текст: «Snapshot contains your secrets — don't share publicly without redaction».
- **`docs/release_notes/upcoming.md`** — пункт в EN+RU.

---

## 5. Не-цели

- **Маскирование секретов.** Намеренный no-op (см. §1.2).
- **Бинарная упаковка** (zip/tar) — JSON-объект.
- **Запись snapshot'а на диск со стороны лаунчера.** Только response.
- **Streaming / chunked.** Один request-response.

---

## 6. Acceptance criteria

1. `GET /debug/snapshot` с Bearer-токеном возвращает 200 + JSON-схему §1.1.
2. На пустой установке — 200 + `missing` со всеми отсутствующими, без падения.
3. **Никаких редакций.** Sensitive значения проходят как есть.
4. Битый JSON одного файла не валит снапшот целиком — попадает в `errors`.
5. POST → 405; без auth → 401.
6. Пакет `core/debugapi/` остаётся изолированным от `core/state` / `core/build` / `core/outboundscache` (читает файлы как `[]byte`, без типизации содержимого).
7. golden-harness `core/build/testdata/golden/<scenario>/` принимает в качестве источника либо четвёрку файлов раздельно, либо один `snapshot.json` — split в 4 input'а на лету в test-helper'е.
