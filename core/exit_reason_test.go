package core

import "testing"

// TestClassifyExitText — причины завершения ядра по тексту лога.
// Детерминированные ошибки должны отличаться от транзиентных: иначе
// авто-перезапуск либо жжёт попытки впустую, либо отключается на
// временном сетевом сбое (SPEC 143).
func TestClassifyExitText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want exitReason
	}{
		{
			name: "unknown field in config is deterministic",
			text: `FATAL decode config at config.json: outbounds[0].version: json: unknown field "version"`,
			want: exitReasonConfigInvalid,
		},
		{
			name: "json parse error is deterministic",
			text: `FATAL decode config at config.json: json: cannot unmarshal string into Go value`,
			want: exitReasonConfigInvalid,
		},
		{
			name: "tun permission denied is deterministic",
			text: `FATAL start inbound/tun[tun-in]: Create: Connect: operation not permitted`,
			want: exitReasonPermission,
		},
		{
			name: "explicit permission denied",
			text: `FATAL open /Library/Logs/sing-box-lxd/classic.log: permission denied`,
			want: exitReasonPermission,
		},
		{
			name: "port in use is deterministic",
			text: `FATAL start inbound/mixed[mixed-in]: listen tcp 127.0.0.1:7890: bind: address already in use`,
			want: exitReasonPortInUse,
		},
		{
			// Порт важнее прав: `bind: address already in use` не про права,
			// хотя строка и содержит «use».
			name: "port in use wins over generic wording",
			text: `FATAL listen tcp 127.0.0.1:7890: bind: address already in use`,
			want: exitReasonPortInUse,
		},
		{
			name: "missing file is deterministic",
			text: `FATAL load rule-set: open ./geosite.db: no such file or directory`,
			want: exitReasonMissingResource,
		},
		{
			name: "api auth failure is deterministic",
			text: `FATAL clash api: authentication failed`,
			want: exitReasonAPIUnavailable,
		},
		{
			name: "no route to host is transient",
			text: `WARN open UDP connection: connect: no route to host`,
			want: exitReasonTransient,
		},
		{
			name: "connection refused is transient",
			text: `ERROR outbound/vless: connection refused`,
			want: exitReasonTransient,
		},
		{
			name: "i/o timeout is transient",
			text: `ERROR outbound/naive: dial tcp: i/o timeout`,
			want: exitReasonTransient,
		},
		{
			// Пустой лог — не доказательство отсутствия ошибки, но и не
			// повод отключать авто-восстановление.
			name: "empty log is unknown",
			text: "",
			want: exitReasonUnknown,
		},
		{
			name: "whitespace only is unknown",
			text: "   \n\n  ",
			want: exitReasonUnknown,
		},
		{
			name: "unrelated noise is unknown",
			text: `INFO router: loaded 12 rules`,
			want: exitReasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyExitText(tt.text); got != tt.want {
				t.Fatalf("classifyExitText(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// TestExitReasonDeterministic — какие причины прекращают авто-перезапуск.
func TestExitReasonDeterministic(t *testing.T) {
	tests := []struct {
		reason exitReason
		want   bool
	}{
		{exitReasonConfigInvalid, true},
		{exitReasonPermission, true},
		{exitReasonPortInUse, true},
		{exitReasonMissingResource, true},
		{exitReasonAPIUnavailable, true},
		{exitReasonTransient, false},
		// Неизвестная причина остаётся транзиентной: сдаваться без
		// доказательств нельзя.
		{exitReasonUnknown, false},
	}
	for _, tt := range tests {
		if got := tt.reason.Deterministic(); got != tt.want {
			t.Fatalf("%v.Deterministic() = %v, want %v", tt.reason, got, tt.want)
		}
	}
}

// TestDecideCrashActionReason_DeterministicStopsRetry — детерминированная
// ошибка прекращает авто-перезапуск СРАЗУ, а не после трёх одинаковых
// попыток. Это и есть жалоба «три раза повторяет одну и ту же ошибку».
func TestDecideCrashActionReason_DeterministicStopsRetry(t *testing.T) {
	deterministic := []exitReason{
		exitReasonConfigInvalid,
		exitReasonPermission,
		exitReasonPortInUse,
		exitReasonMissingResource,
		exitReasonAPIUnavailable,
	}
	for _, reason := range deterministic {
		t.Run(reason.String(), func(t *testing.T) {
			// Даже на первой попытке — сразу отказ, счётчик обнулён.
			action, attempts := decideCrashActionReason(false, false, false, 0, 3, reason)
			if action != actionDeterministicFailure {
				t.Fatalf("action = %v, want actionDeterministicFailure", action)
			}
			if attempts != 0 {
				t.Fatalf("attempts = %d, want 0", attempts)
			}
			// И повторный вызов с уже накопленным счётчиком — тоже отказ.
			action, attempts = decideCrashActionReason(false, false, false, 2, 3, reason)
			if action != actionDeterministicFailure || attempts != 0 {
				t.Fatalf("second call: action=%v attempts=%d, want deterministic/0", action, attempts)
			}
		})
	}
}

// TestDecideCrashActionReason_TransientStillRetries — временная причина
// по-прежнему перезапускается, с прежним лимитом и счётчиком.
func TestDecideCrashActionReason_TransientStillRetries(t *testing.T) {
	action, attempts := decideCrashActionReason(false, false, false, 0, 3, exitReasonTransient)
	if action != actionCrashRestart || attempts != 1 {
		t.Fatalf("first transient: action=%v attempts=%d, want restart/1", action, attempts)
	}
	action, attempts = decideCrashActionReason(false, false, false, 2, 3, exitReasonTransient)
	if action != actionCrashRestart || attempts != 3 {
		t.Fatalf("third transient: action=%v attempts=%d, want restart/3", action, attempts)
	}
	// Лимит исчерпан — прекращаем.
	action, attempts = decideCrashActionReason(false, false, false, 3, 3, exitReasonTransient)
	if action != actionMaxAttempts || attempts != 0 {
		t.Fatalf("over limit: action=%v attempts=%d, want maxAttempts/0", action, attempts)
	}
}

// TestDecideCrashActionReason_UserIntentWins — намерение пользователя
// сильнее классификации: Stop и Restart не должны блокироваться тем, что
// в логе осталась старая детерминированная ошибка.
func TestDecideCrashActionReason_UserIntentWins(t *testing.T) {
	// Пользователь остановил — не перезапускаем, что бы ни было в логе.
	action, attempts := decideCrashActionReason(true, false, false, 0, 3, exitReasonPortInUse)
	if action != actionStoppedByUser || attempts != 0 {
		t.Fatalf("stopped by user: action=%v attempts=%d", action, attempts)
	}
	// Пользователь нажал Restart — перезапускаем даже при детерминированной
	// причине: он мог только что устранить её.
	action, attempts = decideCrashActionReason(false, true, false, 0, 3, exitReasonConfigInvalid)
	if action != actionUserRestart || attempts != 0 {
		t.Fatalf("restart requested: action=%v attempts=%d", action, attempts)
	}
	// Чистый выход — тоже не крэш.
	action, attempts = decideCrashActionReason(false, false, true, 0, 3, exitReasonPermission)
	if action != actionClean || attempts != 0 {
		t.Fatalf("clean exit: action=%v attempts=%d", action, attempts)
	}
}

// TestDecideCrashAction_DefaultKeepsOldBehaviour — прежняя функция без
// причины сохраняет поведение: неизвестная причина = транзиентная.
func TestDecideCrashAction_DefaultKeepsOldBehaviour(t *testing.T) {
	action, attempts := decideCrashAction(false, false, false, 0, 3)
	if action != actionCrashRestart || attempts != 1 {
		t.Fatalf("action=%v attempts=%d, want restart/1", action, attempts)
	}
}

// TestLastLines — берём хвост: причина падения в конце лога.
func TestLastLines(t *testing.T) {
	text := "line1\nline2\nline3\nline4"
	if got := lastLines(text, 2); got != "line3\nline4" {
		t.Fatalf("lastLines = %q", got)
	}
	if got := lastLines("only", 5); got != "only" {
		t.Fatalf("lastLines short = %q", got)
	}
	if got := lastLines("", 5); got != "" {
		t.Fatalf("lastLines empty = %q", got)
	}
}

// TestDeterministicExitText — у каждой детерминированной причины есть текст
// с конкретным действием (иначе диалог покажет пустоту).
func TestDeterministicExitText(t *testing.T) {
	for _, r := range []exitReason{
		exitReasonConfigInvalid, exitReasonPermission, exitReasonPortInUse,
		exitReasonMissingResource, exitReasonAPIUnavailable,
	} {
		if deterministicExitText(r) == "" {
			t.Fatalf("%v has no user-facing text", r)
		}
	}
	if deterministicExitText(exitReasonTransient) != "" {
		t.Fatal("transient must not have a deterministic-failure text")
	}
	if deterministicExitText(exitReasonUnknown) != "" {
		t.Fatal("unknown must not have a deterministic-failure text")
	}
}
