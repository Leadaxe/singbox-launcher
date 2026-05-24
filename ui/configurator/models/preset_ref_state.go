// Package models содержит модели данных конфигуратора.
//
// Файл preset_ref_state.go — модель preset-ref правила (SPEC 053, kind=preset
// в state.json v6). Параллельно RuleState (который покрывает legacy
// kind=inline/srs).
//
// Preset-ref — это тонкая ссылка `{Ref → template.preset.id, Vars → diff от template defaults}`.
// Match-поля живут в template, расширяются при build через preset_expand.go.
package models

// PresetRefState — UI state одного preset-ref правила.
type PresetRefState struct {
	// Ref — id template-preset'а (template.presets[i].id).
	Ref string

	// Enabled — включено ли правило.
	Enabled bool

	// Vars — пользовательские значения переменных, только diff от template defaults.
	// Пустая map = всё дефолтное. Bump RequiredTemplateRef → новые дефолты подтягиваются автоматически.
	Vars map[string]string

	// DNSServerEnabled — per-server toggle для bundled DNS-серверов preset'а
	// (SPEC 056-R-N follow-up). Key = local tag (без preset prefix).
	// Отсутствие ключа = enabled=true (default). Юзер выключил чекбокс → false.
	// При Save конвертится в state.DNS.Servers[kind=preset, ref=<id>:<tag>].Enabled.
	DNSServerEnabled map[string]bool

	// DNSRuleEnabled — toggle для bundled dns_rule preset'а. Один dns_rule
	// на preset. nil или true = enabled. false = выключен.
	// При Save → state.DNS.Rules[kind=preset, ref=<id>].Enabled.
	DNSRuleEnabled *bool
}

// Clone — глубокая копия (для diff/undo сценариев).
func (p *PresetRefState) Clone() *PresetRefState {
	if p == nil {
		return nil
	}
	cp := &PresetRefState{
		Ref:     p.Ref,
		Enabled: p.Enabled,
		Vars:    make(map[string]string, len(p.Vars)),
	}
	for k, v := range p.Vars {
		cp.Vars[k] = v
	}
	if p.DNSServerEnabled != nil {
		cp.DNSServerEnabled = make(map[string]bool, len(p.DNSServerEnabled))
		for k, v := range p.DNSServerEnabled {
			cp.DNSServerEnabled[k] = v
		}
	}
	if p.DNSRuleEnabled != nil {
		b := *p.DNSRuleEnabled
		cp.DNSRuleEnabled = &b
	}
	return cp
}

// IsDNSServerEnabled — default true; false если юзер выключил.
func (p *PresetRefState) IsDNSServerEnabled(localTag string) bool {
	if p == nil || p.DNSServerEnabled == nil {
		return true
	}
	v, has := p.DNSServerEnabled[localTag]
	if !has {
		return true
	}
	return v
}

// IsDNSRuleEnabled — default true.
func (p *PresetRefState) IsDNSRuleEnabled() bool {
	if p == nil || p.DNSRuleEnabled == nil {
		return true
	}
	return *p.DNSRuleEnabled
}

// SetDNSServerEnabled — записать toggle (lazy-init map).
func (p *PresetRefState) SetDNSServerEnabled(localTag string, enabled bool) {
	if p == nil {
		return
	}
	if p.DNSServerEnabled == nil {
		p.DNSServerEnabled = make(map[string]bool)
	}
	p.DNSServerEnabled[localTag] = enabled
}

// SetDNSRuleEnabled — записать toggle (lazy-init pointer).
func (p *PresetRefState) SetDNSRuleEnabled(enabled bool) {
	if p == nil {
		return
	}
	p.DNSRuleEnabled = &enabled
}
