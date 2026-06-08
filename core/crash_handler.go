package core

// crash_handler.go — SPEC 070: единая точка принятия решения «что делать после
// того как sing-box завершился». До этого решение дублировалось между
// ProcessService.Monitor (обычный путь, cmd.Wait) и
// ProcessService.onPrivilegedScriptExited (privileged путь, WaitForPrivilegedExit).
//
// Здесь живёт ТОЛЬКО чистая логика выбора действия по флагам и счётчику.
// Side-effects (RunningState, dialogs, sleep, Start, stability-goroutine) и
// различия путей (PID-check, наличие err) остаются у вызывающих — см. комментарии
// в decideCrashAction.

// crashAction — решение, что делать после exit sing-box.
type crashAction int

const (
	// actionStoppedByUser — пользователь сам остановил процесс (StoppedByUser).
	// Перезапуск не нужен; счётчик крэшей обнуляется.
	actionStoppedByUser crashAction = iota

	// actionUserRestart — пользователь нажал Restart (RestartRequestedByUser).
	// Поднимаем процесс заново; счётчик крэшей обнуляется.
	actionUserRestart

	// actionCrashRestart — sing-box упал, лимит попыток ещё не исчерпан.
	// Авто-перезапуск; счётчик инкрементирован (см. newAttempts).
	actionCrashRestart

	// actionMaxAttempts — лимит авто-перезапусков исчерпан.
	// Показываем ошибку, перезапуск прекращаем; счётчик обнуляется.
	actionMaxAttempts

	// actionClean — sing-box завершился штатно (exit code 0), не по запросу
	// пользователя. Перезапуск не нужен; счётчик обнуляется.
	//
	// Возвращается только когда вызывающий передал cleanExit=true. Privileged
	// путь (onPrivilegedScriptExited) такого сигнала не имеет (нет cmd.Wait и
	// нет err), поэтому всегда передаёт cleanExit=false и actionClean не получит.
	actionClean
)

// decideCrashAction — чистое решение по флагам завершения процесса.
//
// Приоритет проверок (важно: повторяет порядок обоих исходных call-site'ов):
//  1. stoppedByUser   → actionStoppedByUser, счётчик → 0
//  2. restartRequested→ actionUserRestart,   счётчик → 0
//  3. cleanExit       → actionClean,         счётчик → 0
//     (только Monitor передаёт cleanExit = (err == nil); privileged путь — false)
//  4. иначе — крэш: counter++; если превысил maxAttempts → actionMaxAttempts,
//     счётчик → 0; иначе actionCrashRestart с инкрементированным счётчиком.
//
// newAttempts — значение, которое вызывающий должен записать в
// ac.ConsecutiveCrashAttempts. Для actionCrashRestart это
// consecutiveCrashAttempts+1; для всех остальных — 0.
//
// Что НЕ делается здесь (намеренно остаётся в вызывающих, т.к. пути различаются):
//   - PID-check (Monitor: «это мой процесс?») — до вызова;
//   - err==nil разбор — Monitor передаёт через cleanExit; privileged не имеет err;
//   - RunningState.Set(false) — privileged ставит один раз сверху, Monitor
//     по веткам;
//   - dialogs / sleep / Start / stability-goroutine — после вызова.
func decideCrashAction(
	stoppedByUser bool,
	restartRequested bool,
	cleanExit bool,
	consecutiveCrashAttempts int,
	maxAttempts int,
) (action crashAction, newAttempts int) {
	switch {
	case stoppedByUser:
		return actionStoppedByUser, 0
	case restartRequested:
		return actionUserRestart, 0
	case cleanExit:
		return actionClean, 0
	default:
		inc := consecutiveCrashAttempts + 1
		if inc > maxAttempts {
			return actionMaxAttempts, 0
		}
		return actionCrashRestart, inc
	}
}
