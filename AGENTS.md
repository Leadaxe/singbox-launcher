# Руководство для AI-агентов (Cursor, Codex и др.)

Документ для агентов, работающих с репозиторием **singbox-launch**. Следуй ему при реализации задач, рефакторинге и ответах на вопросы по коду.

---

## 0. Scope агента

**Агент может:**
- читать любые файлы репозитория;
- изменять файлы и создавать новые в рамках **TASKS.md**, **SPEC.md**, **PLAN.md** текущей задачи.

**При выходе за эти рамки** (другие файлы, новые файлы не из SPEC/PLAN, изменение архитектуры, перемещение файлов, изменение public API) — спросить пользователя или дождаться явного разрешения/указания, не делать по своей инициативе.

---

## 1. Что читать в первую очередь

| Документ | Назначение |
|----------|------------|
| **SPECS/CONSTITUTION.md** | Приоритеты, архитектура, логирование, запреты, язык UI и стиль кода |
| **SPECS/IMPLEMENTATION_PROMPT.md** | Философия разработки, DoD, Git/консоль, контракт выхода после реализации |
| **docs/ARCHITECTURE.md** | Структура проекта, зоны ответственности, потоки данных |
| **SPECS/README.md** | Формат папок (NNN-T-S-NAME), Spec Kit и workflow |

Перед реализацией задачи из `SPECS/` обязательно прочитай **SPEC.md**, **PLAN.md**, **TASKS.md** и применяй **SPECS/IMPLEMENTATION_PROMPT.md**. Ограничения по архитектуре, производительности, логированию, ошибкам и запретам — в **SPECS/CONSTITUTION.md**.

---

## 2. Формат задач (Spec Kit)

Все задачи в **SPECS/** — папки `NNN-T-S-NAME`. Внутри: SPEC.md, PLAN.md, TASKS.md, IMPLEMENTATION_REPORT.md. Подробно — **SPECS/README.md**.

Если пользователь просит «реализовать задачу» или «сделать по ТЗ» — уточни или возьми задачу из **TASKS.md** и не выходи за рамки **SPEC** и **PLAN**. Если SPEC/PLAN не определяют поведение однозначно — запроси уточнение, не делай предположений, меняющих архитектуру или UX.

**Язык:** отчёты (IMPLEMENTATION_REPORT), ответы пользователю и пояснения в чате — преимущественно **русский**. Язык кода, UI и комментариев — **SPECS/CONSTITUTION.md** (разделы 8.2, 8.3).

---

## 3. Ограничения (кратко)

- **Git, консоль, минимальный дифф, утилиты** — см. **SPECS/IMPLEMENTATION_PROMPT.md**.
- **Логирование, ошибки пользователю, запреты** — см. **SPECS/CONSTITUTION.md** (разделы 5, 8.2, 11).
- Перед новой функцией проверять: `internal/debuglog`, `internal/constants`, `internal/ctxutil`, `core/network_utils`, `internal/platform`, пакеты в `core/config`, `ui/wizard/business`.
- Без явного запроса пользователя не упоминать агентов (Cursor, Codex, модели) в документах, коммитах и коде.

---

## 4. Контракт выхода и документация при закрытии

**Что вернуть после реализации** — см. **SPECS/IMPLEMENTATION_PROMPT.md** (раздел «Контракт выхода»).

**При закрытии задачи обновлять:**

| Что | Куда |
|-----|------|
| Изменения кода/поведения | **docs/release_notes/upcoming.md** (EN → Highlights, RU → Основное) |
| Архитектура, потоки | **docs/ARCHITECTURE.md** (и др. в docs/ при необходимости) |
| Выжимка для пользователя | **RELEASE_NOTES.md** (при значимых изменениях UX) |

> **Выпуск релиза или пререлиза — отдельная процедура:** см. **[docs/RELEASE_PROCESS.md](docs/RELEASE_PROCESS.md)**. Перед пушем stable-тега обязательно `upcoming.md` → `docs/release_notes/X-Y-Z.md` (+ пустой `upcoming.md`). Для пререлиза — создать `docs/release_notes/<SLUG>.md` с вычисленным локально slug'ом. CI падает, если файла нет.

---

## 5. Где что искать

- **Релизные заметки (черновик):** `docs/release_notes/upcoming.md`
- **Протокол выпуска релиза / пререлиза:** `docs/RELEASE_PROCESS.md`
- **CI (run-modes, команды):** `.github/workflows/README.md`
- **Архитектура:** `docs/ARCHITECTURE.md`
- **Принципы и запреты:** `SPECS/CONSTITUTION.md`
- **Задачи:** `SPECS/README.md`, папки `SPECS/NNN-T-S-NAME/`
- **Правила реализации и DoD:** `SPECS/IMPLEMENTATION_PROMPT.md`
- **Сборка, тесты:** `docs/BUILD_WINDOWS.md`, `docs/TEST_README.md`; GUI-пакеты исключены из `go test` (CONSTITUTION).
- **Конфиг парсера:** `docs/ParserConfig.md`; шаблон визарда — `bin/wizard_template.json`.

---

## 6. Чеклист перед завершением

Полный ритуал закрытия задачи в стиле Spec Kit (порядок шагов, доки, переименование папки **…-C-…**): **[SPECS/README.md — «Закрытие задачи (чеклист)»](SPECS/README.md#closing-task-checklist)**.

- [ ] Сборка и тесты: `go build ./...`, `go test ./...`, `go vet ./...`.
- [ ] Соблюдены **SPECS/IMPLEMENTATION_PROMPT.md** и **SPECS/CONSTITUTION.md** (логирование, ошибки, Git, стиль).
- [ ] Изменены только файлы из TASKS.md; новые файлы — только если указаны в SPEC/PLAN.
- [ ] **docs/release_notes/upcoming.md** обновлён.
- [ ] При изменении архитектуры/потоков — обновлён **docs/ARCHITECTURE.md**.
- [ ] При значимых изменениях UX — пункт в **RELEASE_NOTES.md**.
- [ ] Если задача инициирует выпуск — следовать **[docs/RELEASE_PROCESS.md](docs/RELEASE_PROCESS.md)** (перенос `upcoming.md` → `X-Y-Z.md` для stable / создание `<SLUG>.md` для prerelease).
