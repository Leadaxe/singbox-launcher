# PLAN 151

1. `internal/platform/privileged_darwin.go`: `privilegedShell` →
   `/bin/bash`, флаг `privilegedShellKeepUID` (`-p`); в
   `privilegedStartBody` — сверка euid с `$8` и причина отказа открытия
   лога; `PrivilegedStartArgs` получает `shellUID`, `StartPrivilegedCore`
   передаёт `0`.
2. `internal/platform/privileged_darwin_test.go`: новый argv, случай
   «прод-argv без root».
3. Документы: SPEC 137 §3, `docs/ARCHITECTURE*.md`,
   `docs/DAEMON_AND_REMOTE*.md`, `docs/release_notes/upcoming.md`,
   `SPECS/README.md`.
