package platform

import (
	"encoding/base64"
	"encoding/binary"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"
)

// TestPowerShellEncodedDialogScript — скрипты файловых диалогов Windows
// (file_dialog_windows.go, лаунчер там работает с правами администратора)
// с враждебной подписью и именем файла: все кавычки, которые PowerShell
// считает кавычками (' " ‘ ’ ‚ ‛), подстановка $(calc), обратный апостроф,
// разделители команд. В исполняемой части скрипта значений нет: каждый
// строковый литерал — base64 и раскодируется ровно в исходное значение;
// -EncodedCommand — base64 от UTF-16LE и переживает обратное
// преобразование байт в байт (в том числе символы вне BMP).
func TestPowerShellEncodedDialogScript(t *testing.T) {
	hostile := "it's \"x\" ‘a’ ‚b‛ $(calc) `whoami` ; & | $env:USERNAME 😀"
	hostileName := "o'brien’s ‛backup‛ $(calc).json"
	filter := "Configs (*.conf;*.vpn)|*.conf;*.vpn|All files (*.*)|*.*"
	literal := regexp.MustCompile(`'([^']*)'`)
	base64Only := regexp.MustCompile(`^[A-Za-z0-9+/=]*$`)

	for name, tc := range map[string]struct {
		script string
		values []string
	}{
		"open":     {psOpenFileScript(hostile, filter, false), []string{hostile, filter}},
		"open-all": {psOpenFileScript(hostile, filter, true), []string{hostile, filter}},
		"save":     {psSaveFileScript(hostile, hostileName, filter), []string{hostile, hostileName, filter}},
		"empty":    {psSaveFileScript("", "", ""), []string{"", "", ""}},
	} {
		t.Run(name, func(t *testing.T) {
			for _, bad := range []string{"calc", "whoami", "‘", "’", "‚", "‛", "`", "\"", "USERNAME", "😀", "*.vpn"} {
				if strings.Contains(tc.script, bad) {
					t.Fatalf("value text %q leaked into the script:\n%s", bad, tc.script)
				}
			}
			// Каждый литерал в одинарных кавычках — base64, и вместе они
			// раскодируются ровно в значения, в том же порядке.
			var decoded []string
			for _, m := range literal.FindAllStringSubmatch(tc.script, -1) {
				if !base64Only.MatchString(m[1]) {
					t.Fatalf("non-base64 literal %q in the script:\n%s", m[1], tc.script)
				}
				raw, err := base64.StdEncoding.DecodeString(m[1])
				if err != nil {
					t.Fatalf("literal %q: %v", m[1], err)
				}
				decoded = append(decoded, string(raw))
			}
			if strings.Join(decoded, "\x00") != strings.Join(tc.values, "\x00") {
				t.Fatalf("decoded values %q, want %q", decoded, tc.values)
			}

			// -EncodedCommand: base64 от UTF-16LE, обратно — тот же скрипт.
			args := psDialogArgs(tc.script)
			if len(args) != 5 || args[3] != "-EncodedCommand" || args[0] != "-NoProfile" || args[1] != "-NonInteractive" || args[2] != "-STA" {
				t.Fatalf("unexpected powershell args %q", args)
			}
			if !base64Only.MatchString(args[4]) {
				t.Fatalf("-EncodedCommand is not base64: %q", args[4])
			}
			raw, err := base64.StdEncoding.DecodeString(args[4])
			if err != nil || len(raw)%2 != 0 {
				t.Fatalf("-EncodedCommand decode: %v (len %d)", err, len(raw))
			}
			units := make([]uint16, len(raw)/2)
			for i := range units {
				units[i] = binary.LittleEndian.Uint16(raw[2*i:])
			}
			if got := string(utf16.Decode(units)); got != tc.script {
				t.Fatalf("UTF-16LE round trip:\n got %q\nwant %q", got, tc.script)
			}
		})
	}

	// Известный вектор: "a€😀" в UTF-16LE — 61 00, AC 20, 3D D8 00 DE.
	if got, want := psEncodedCommand("a€😀"), base64.StdEncoding.EncodeToString([]byte{0x61, 0x00, 0xAC, 0x20, 0x3D, 0xD8, 0x00, 0xDE}); got != want {
		t.Fatalf("psEncodedCommand(\"a€😀\") = %s, want %s", got, want)
	}
}
