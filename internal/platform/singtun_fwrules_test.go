package platform

import "testing"

// TestParseSingTunFirewallRule — таблица истинности парсера значений
// FirewallRules для правил sing-tun.
func TestParseSingTunFirewallRule(t *testing.T) {
	const exe = `C:\PROG\VPN\Singbox-launcher\bin\sing-box.exe`

	cases := []struct {
		name     string
		data     string
		wantName string
		wantApp  string
		wantOK   bool
	}{
		{
			name:     "typical sing-tun rule",
			data:     `v2.33|Action=Allow|Active=TRUE|Dir=In|Protocol=6|App=` + exe + `|Name=sing-tun (` + exe + `)|`,
			wantName: `sing-tun (` + exe + `)`,
			wantApp:  exe,
			wantOK:   true,
		},
		{
			name:     "no App field: path derived from rule name",
			data:     `v2.33|Action=Allow|Dir=In|Name=sing-tun (` + exe + `)|`,
			wantName: `sing-tun (` + exe + `)`,
			wantApp:  exe,
			wantOK:   true,
		},
		{
			name:   "foreign rule name",
			data:   `v2.33|Action=Allow|Dir=In|App=` + exe + `|Name=sing-box core|`,
			wantOK: false,
		},
		{
			name:   "sing-tun prefix without closing paren",
			data:   `v2.33|Name=sing-tun (broken|`,
			wantOK: false,
		},
		{
			name:   "no Name field at all",
			data:   `v2.33|Action=Allow|App=` + exe + `|`,
			wantOK: false,
		},
		{
			name:   "empty parens and no App",
			data:   `v2.33|Name=sing-tun ()|`,
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotApp, gotOK := parseSingTunFirewallRule(tc.data)
			if gotOK != tc.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if gotName != tc.wantName {
				t.Errorf("ruleName = %q, want %q", gotName, tc.wantName)
			}
			if gotApp != tc.wantApp {
				t.Errorf("appPath = %q, want %q", gotApp, tc.wantApp)
			}
		})
	}
}
