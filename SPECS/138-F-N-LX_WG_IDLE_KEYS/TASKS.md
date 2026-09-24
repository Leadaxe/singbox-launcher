# SPEC 138 · Задачи

Отмечать по факту коммита. Этапы — PLAN.md.

## Этап 1 · Разведка и спека
- [x] Grep по репо (сборщик, шаблон, state/vars, UI, running-config, контракт, доки): ключей сна WG нет, простоя MASQUE глобально нет.
- [x] Имена ключей сверены с `option/lx.go` / `option/route.go` форка (`origin/lx` `7de943642`).
- [x] Десктопный `LX_TAGS` без `with_lx_idle_suspend`, заглушка lx.12 отвергает `route.lx_idle_*` — подтверждено по форку.
- [x] SPEC.md, PLAN.md, TASKS.md.

## Этап 2 · Тест
- [x] `TestBuildConfigPassesRootLXBlock` (`core/build/lx_root_block_test.go`), прогон по имени.

## Этап 3 · Доки
- [ ] `docs/ARCHITECTURE.md` / `.ru.md` — сквозной путь корневого `lx`.
- [ ] `docs/release_notes/upcoming.md` EN/RU.
- [ ] Запись в `SPECS/README.md`.

## Проверки
- [ ] `go build ./...`.
- [ ] `l10n_check --strict`, `hardcoded_check --strict`, `paths_guard`, `win7guard`.

## Ждёт
- [ ] Релиз ядра `v1.14.1-lx.13`, затем бамп `RequiredCoreVersion` (SPEC §3.4).
- [ ] Решения владельца по развилкам SPEC §6.
- [ ] CI `run_mode=tests` после слияния волны.
