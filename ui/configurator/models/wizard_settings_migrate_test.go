package models

import "testing"

func TestMigrateSettingsVarsFromConfigParams(t *testing.T) {
	t.Run("skips when tun already in vars", func(t *testing.T) {
		sf := &WizardStateFile{
			Vars: []PersistedSettingVar{{Name: "tun", Value: "false"}},
			ConfigParams: []ConfigParam{
				{Name: "enable_tun_macos", Value: "true"},
			},
		}
		MigrateSettingsVarsFromConfigParams(sf)
		if len(sf.Vars) != 1 {
			t.Fatalf("vars len: %d", len(sf.Vars))
		}
		if sf.Vars[0].Value != "false" {
			t.Fatalf("tun overwritten: %q", sf.Vars[0].Value)
		}
	})

	t.Run("migrates enable_tun_macos", func(t *testing.T) {
		sf := &WizardStateFile{
			ConfigParams: []ConfigParam{
				{Name: "enable_tun_macos", Value: "true"},
			},
		}
		MigrateSettingsVarsFromConfigParams(sf)
		if len(sf.Vars) != 1 || sf.Vars[0].Name != "tun" || sf.Vars[0].Value != "true" {
			t.Fatalf("got %+v", sf.Vars)
		}
	})

	t.Run("nil safe", func(t *testing.T) {
		MigrateSettingsVarsFromConfigParams(nil)
	})
}
