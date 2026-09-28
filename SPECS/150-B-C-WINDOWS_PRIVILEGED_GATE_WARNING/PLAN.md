# PLAN 150 — Windows: предупреждение вместо запрета повышенного старта

1. `internal/platform/winservice_windows.go`: тип `ProtectionError`,
   `describeFileRights` (порт из ядра), `aclViolation` возвращает
   `*ProtectionError` с командой `icacls`; флаг `aceInherited`.
2. `internal/platform/privileged_windows.go`: `OpenPrivilegedCoreLog`
   проверяет `root` и `logs\` через `checkProtectedPath`, без предков.
3. `core/classic_privileged.go`: `privilegedCopyCheck.Err`; гейт при флаге
   сессии (`privilegedCopyRunAnyway`) отдаёт ядро лаунчера с WARN.
4. `core/process_service.go`: флаги `runAnywayCopy`, `runAnywayLog`.
5. `core/classic_privileged_windows.go`: общий диалог отказа
   (`showElevatedRefusalDialog`) для копии и лога, кнопка Run anyway,
   фоллбэк лога в `elevatedClassicStart`; тексты.
6. `core/classic_privileged_darwin.go`: заглушка `privilegedCopyRunAnyway`.
7. `bin/locale/ru.json`: переводы новых и изменённых ключей.
8. Документы: SPEC 141 §8, §13; `docs/release_notes/upcoming.md`;
   `SPECS/README.md`.
