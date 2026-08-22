package models

import "testing"

func varsOf(sf *WizardStateFile) map[string]string {
	out := map[string]string{}
	for _, v := range sf.Vars {
		out[v.Name] = v.Value
	}
	return out
}

func TestMigrateTunAddressSplit(t *testing.T) {
	tests := []struct {
		name string
		vars []PersistedSettingVar
		want map[string]string
	}{
		{
			name: "однострочный адрес не трогается",
			vars: []PersistedSettingVar{{Name: "tun_address", Value: "172.16.0.1/30"}},
			want: map[string]string{"tun_address": "172.16.0.1/30"},
		},
		{
			name: "v4+v6 разводятся и включают галку",
			vars: []PersistedSettingVar{{Name: "tun_address", Value: "172.16.0.1/30\nfdfe:dcba:9876::1/126"}},
			want: map[string]string{
				"tun_address":  "172.16.0.1/30",
				"tun_address6": "fdfe:dcba:9876::1/126",
				"ipv6_enabled": "true",
			},
		},
		{
			// Порядок записей в state произвольный: флаг может лежать
			// раньше адреса, и однопроходный разбор включал бы его даже
			// при уже заданном tun_address6.
			name: "заданный tun_address6 важнее унаследованной строки",
			vars: []PersistedSettingVar{
				{Name: "ipv6_enabled", Value: "false"},
				{Name: "tun_address6", Value: "fd00::9/126"},
				{Name: "tun_address", Value: "172.16.0.1/30\nfdfe::1/126"},
			},
			want: map[string]string{
				"tun_address":  "172.16.0.1/30",
				"tun_address6": "fd00::9/126",
				"ipv6_enabled": "false",
			},
		},
		{
			name: "существующий выключенный флаг включается",
			vars: []PersistedSettingVar{
				{Name: "ipv6_enabled", Value: "false"},
				{Name: "tun_address", Value: "10.0.0.1/30\nfd00::1/126"},
			},
			want: map[string]string{
				"tun_address":  "10.0.0.1/30",
				"tun_address6": "fd00::1/126",
				"ipv6_enabled": "true",
			},
		},
		{
			name: "лишние строки сверх первой на семейство отбрасываются",
			vars: []PersistedSettingVar{{Name: "tun_address", Value: "10.0.0.1/30\n10.0.0.5/30\nfd00::1/126\nfd00::2/126"}},
			want: map[string]string{
				"tun_address":  "10.0.0.1/30",
				"tun_address6": "fd00::1/126",
				"ipv6_enabled": "true",
			},
		},
		{
			// Затирать в пустоту нельзя: без адреса TUN не поднимется.
			name: "только IPv6 в поле IPv4 — первая строка остаётся",
			vars: []PersistedSettingVar{{Name: "tun_address", Value: "fd00::1/126\nfd00::2/126"}},
			want: map[string]string{
				"tun_address":  "fd00::1/126",
				"tun_address6": "fd00::1/126",
				"ipv6_enabled": "true",
			},
		},
		{
			name: "пустые строки не порождают адресов",
			vars: []PersistedSettingVar{{Name: "tun_address", Value: "172.16.0.1/30\n\n  \n"}},
			want: map[string]string{"tun_address": "172.16.0.1/30"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sf := &WizardStateFile{Vars: tt.vars}
			MigrateTunAddressSplit(sf)
			got := varsOf(sf)
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("%s = %q, ожидалось %q (все vars: %v)", k, got[k], want, got)
				}
			}
			if _, unexpected := got["tun_address6"]; unexpected && tt.want["tun_address6"] == "" {
				t.Errorf("tun_address6 создан там, где его быть не должно: %v", got)
			}
		})
	}
}

// Идемпотентность: повторная загрузка уже разведённого состояния ничего не
// меняет — иначе миграция дописывала бы дубли на каждом Load.
func TestMigrateTunAddressSplitIdempotent(t *testing.T) {
	sf := &WizardStateFile{Vars: []PersistedSettingVar{
		{Name: "tun_address", Value: "172.16.0.1/30\nfd00::1/126"},
	}}
	MigrateTunAddressSplit(sf)
	first := varsOf(sf)
	countAfterFirst := len(sf.Vars)

	MigrateTunAddressSplit(sf)
	if len(sf.Vars) != countAfterFirst {
		t.Fatalf("повторный вызов изменил число vars: %d → %d", countAfterFirst, len(sf.Vars))
	}
	for k, v := range varsOf(sf) {
		if first[k] != v {
			t.Errorf("%s изменился при повторном вызове: %q → %q", k, first[k], v)
		}
	}
}
