package state

// Diff описывает «что изменилось» между двумя State.
//
// Логика разделения на CacheStale / ConfigStale:
//   - **CacheStale** — изменения, которые требуют **перепарса подписок**
//     и **перегенерации config.json** (выполняется кнопкой Update либо
//     auto-update'ом). Это правки в parser-конфиге: список / URL'ы /
//     skip / tag_prefix подписок, локальные ноды.
//   - **ConfigStale** — изменения, которые требуют **перезапуска sing-box**
//     (config.json пересобирается, но работающий процесс должен прочитать
//     новый файл). Это правки шаблонной части: tun, dns, rules,
//     log_level, vars шаблона.
//
// На один Save оба флага могут подняться независимо.
type Diff struct {
	// Поля → CacheStale
	ProxiesChanged   bool // ParserConfig.Proxies список изменился (added/removed/modified)
	OutboundsChanged bool // ParserConfig.Outbounds (локальные ноды) изменились

	// Поля → ConfigStale
	VarsChanged            bool // SettingsVars изменились (tun_stack, log_level, dns_*, urltest_*…)
	ConfigParamsChanged    bool // route.final и т.п.
	SelectableRulesChanged bool
	CustomRulesChanged     bool
	DNSOptionsChanged      bool

	// Метаданные правок (не влияют на dirty-флаги)
	IDChanged      bool
	CommentChanged bool
}

// IsEmpty — никаких реальных изменений не зафиксировано.
func (d Diff) IsEmpty() bool {
	return !d.ProxiesChanged &&
		!d.OutboundsChanged &&
		!d.VarsChanged &&
		!d.ConfigParamsChanged &&
		!d.SelectableRulesChanged &&
		!d.CustomRulesChanged &&
		!d.DNSOptionsChanged &&
		!d.IDChanged &&
		!d.CommentChanged
}

// AffectsParser — true, если есть изменения, требующие повторного
// фетча подписок и пересборки config.json.
func (d Diff) AffectsParser() bool {
	return d.ProxiesChanged || d.OutboundsChanged
}

// AffectsTemplate — true, если есть изменения шаблонной части,
// требующие перезапуска работающего sing-box.
func (d Diff) AffectsTemplate() bool {
	return d.VarsChanged ||
		d.ConfigParamsChanged ||
		d.SelectableRulesChanged ||
		d.CustomRulesChanged ||
		d.DNSOptionsChanged
}
