package core

import "testing"

// TestClassifyCoreVersion — как установленное ядро соотносится с закреплённой
// версией. Раньше UI сравнивал строки на точное равенство, и кастомная сборка
// новее закреплённой выглядела как «другая версия» с кнопкой «Reinstall»:
// пользователя подталкивали заменить рабочее ядро официальным (SPEC 143).
func TestClassifyCoreVersion(t *testing.T) {
	const required = "1.14.2-lx.4"

	tests := []struct {
		name      string
		installed string
		required  string
		want      CoreVersionRelation
	}{
		{
			name:      "exact pinned version matches",
			installed: "1.14.2-lx.4",
			required:  required,
			want:      CoreVersionSame,
		},
		{
			name:      "pinned version with v prefix still matches",
			installed: "v1.14.2-lx.4",
			required:  required,
			want:      CoreVersionSame,
		},
		{
			// Главный случай: рабочее кастомное ядро новее закреплённого.
			// Замена была бы откатом — «Reinstall» предлагать нельзя.
			name:      "custom core newer than pinned",
			installed: "1.15.0-jiejie-masquerade.5",
			required:  required,
			want:      CoreVersionNewer,
		},
		{
			name:      "fork build newer by lx number",
			installed: "1.15.0-lx.1",
			required:  required,
			want:      CoreVersionNewer,
		},
		{
			name:      "older fork build",
			installed: "1.14.1-lx.12",
			required:  required,
			want:      CoreVersionOlder,
		},
		{
			name:      "older base version",
			installed: "1.13.9-lx.30",
			required:  required,
			want:      CoreVersionOlder,
		},
		{
			// База та же, суффикс другой — не откат, но и не «то же самое».
			name:      "same base different suffix",
			installed: "1.14.2-jiejie.1",
			required:  required,
			want:      CoreVersionNewer,
		},
		{
			name:      "empty installed version is unknown",
			installed: "",
			required:  required,
			want:      CoreVersionUnknown,
		},
		{
			name:      "unparseable installed version is unknown",
			installed: "unknown",
			required:  required,
			want:      CoreVersionUnknown,
		},
		{
			name:      "dev build without numbers is unknown",
			installed: "unnamed-dev",
			required:  required,
			want:      CoreVersionUnknown,
		},
		{
			// Неопознанное ядро не должно выглядеть как устаревшее: иначе
			// пользователю предложат заменить то, что лаунчер не понял.
			name:      "unparseable version never reads as older",
			installed: "custom-build",
			required:  required,
			want:      CoreVersionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyCoreVersion(tt.installed, tt.required); got != tt.want {
				t.Fatalf("ClassifyCoreVersion(%q, %q) = %q, want %q",
					tt.installed, tt.required, got, tt.want)
			}
		})
	}
}

// TestClassifyCoreVersion_CustomCoreIsNotOlderThanPinned — регрессия на
// конкретную жалобу: 1.15.0-jiejie-masquerade.5 не должно приводить к
// предложению заменить ядро официальным 1.14.2-lx.4.
func TestClassifyCoreVersion_CustomCoreIsNotOlderThanPinned(t *testing.T) {
	got := ClassifyCoreVersion("1.15.0-jiejie-masquerade.5", "1.14.2-lx.4")
	if got == CoreVersionOlder {
		t.Fatal("a newer custom core must not be classified as older: the UI would offer to replace a working core")
	}
	if got != CoreVersionNewer {
		t.Fatalf("relation = %q, want %q", got, CoreVersionNewer)
	}
}
