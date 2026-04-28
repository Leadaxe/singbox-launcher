package models

// MigrateSettingsVarsFromConfigParams переносит enable_tun_macos → vars.tun при отсутствии tun (идемпотентно).
func MigrateSettingsVarsFromConfigParams(sf *WizardStateFile) {
	if sf == nil {
		return
	}
	for _, v := range sf.Vars {
		if v.Name == "tun" {
			return
		}
	}
	for _, p := range sf.ConfigParams {
		if p.Name == "enable_tun_macos" {
			sf.Vars = append(sf.Vars, PersistedSettingVar{Name: "tun", Value: p.Value})
			return
		}
	}
}
