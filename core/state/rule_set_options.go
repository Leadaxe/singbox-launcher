package state

// Режимы набора правил (SPEC 185 §2.4): кто качает .srs.
const (
	// SrsModeLocal — качает лаунчер, ядру уходит type:local с путём к файлу.
	SrsModeLocal = "local"
	// SrsModeRemote — качает ядро, ему уходит type:remote с url.
	SrsModeRemote = "remote"
)

// Переменные шаблона — общие значения настроек наборов (SPEC 185 §2.4).
const (
	VarSrsMode                = "srs_mode"
	VarSrsUpdateIntervalHours = "srs_update_interval_hours"
	VarSrsDownloadDetour      = "srs_download_detour"
)

// DefaultSrsUpdateIntervalHours — встроенное умолчание периода обновления
// набора (неделя; совпадает с умолчанием LxBox, BACKUP.md).
const DefaultSrsUpdateIntervalHours = 168

// RuleSetOptions — настройки наборов одного правила (kind srs|preset). Нулевое
// значение поля = «взять общее».
type RuleSetOptions struct {
	// Mode — SrsModeLocal | SrsModeRemote | "" (общее).
	Mode string
	// IntervalHours — период обновления, часы; 0 — не обновлять; nil — общее.
	IntervalHours *int
	// DownloadDetour — тег выхода, через который набор качает ядро (remote).
	DownloadDetour string
}

// IsZero — у правила нет своих настроек.
func (o RuleSetOptions) IsZero() bool {
	return o.Mode == "" && o.IntervalHours == nil && o.DownloadDetour == ""
}

// Clone — глубокая копия.
func (o RuleSetOptions) Clone() RuleSetOptions {
	if o.IntervalHours != nil {
		v := *o.IntervalHours
		o.IntervalHours = &v
	}
	return o
}

// RuleSetOptions — настройки наборов правила из полей записи. Незнакомый
// режим читается как «общее»; отрицательный период — как «общее».
func (r Rule) RuleSetOptions() RuleSetOptions {
	o := RuleSetOptions{DownloadDetour: r.DownloadDetour}
	if r.SrsMode == SrsModeLocal || r.SrsMode == SrsModeRemote {
		o.Mode = r.SrsMode
	}
	if r.UpdateIntervalHours != nil && *r.UpdateIntervalHours >= 0 {
		v := *r.UpdateIntervalHours
		o.IntervalHours = &v
	}
	return o
}

// SetRuleSetOptions пишет настройки наборов в поля записи. У inline-правила
// наборов нет — поля очищаются.
func (r *Rule) SetRuleSetOptions(o RuleSetOptions) {
	if r.Kind != RuleKindSrs && r.Kind != RuleKindPreset {
		o = RuleSetOptions{}
	}
	o = o.Clone()
	r.SrsMode = o.Mode
	r.UpdateIntervalHours = o.IntervalHours
	r.DownloadDetour = o.DownloadDetour
}
