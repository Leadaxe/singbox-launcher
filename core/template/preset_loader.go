package template

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// PresetWarning — non-fatal warning при загрузке/валидации preset'а.
// Скип-семантика per-warning: некоторые warning'и оставляют preset (strip
// невалидное поле), другие пропускают preset целиком. См. Action.
type PresetWarning struct {
	// PresetID — preset.id если известен, иначе "" (например duplicate id).
	PresetID string

	// Message — человеко-читаемое описание.
	Message string

	// Action — что сделал loader:
	//   "strip"  — невалидное поле/значение стерто, остальное сохранено
	//   "skip"   — весь preset пропущен (не появится в [] Preset)
	Action string
}

func (w PresetWarning) String() string {
	prefix := ""
	if w.PresetID != "" {
		prefix = fmt.Sprintf("preset %q: ", w.PresetID)
	}
	return fmt.Sprintf("[%s] %s%s", w.Action, prefix, w.Message)
}

var presetIDPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// LoadPresets парсит и валидирует template.presets[] секцию.
//
// Возвращает []Preset (только валидные, без skip'нутых) и []PresetWarning
// (warning'и обоих типов — strip и skip).
//
// Параметр globalVarsNames — имена глобальных template.vars (TemplateVar.Name)
// — для проверки collision'ов с preset.vars[i].Name.
func LoadPresets(raw json.RawMessage, globalVarsNames map[string]bool) ([]Preset, []PresetWarning) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var rawList []json.RawMessage
	if err := json.Unmarshal(raw, &rawList); err != nil {
		return nil, []PresetWarning{{
			Message: fmt.Sprintf("presets[] is not a JSON array: %v", err),
			Action:  "skip",
		}}
	}

	var (
		result    []Preset
		warnings  []PresetWarning
		seenIDs   = make(map[string]bool, len(rawList))
	)

	for i, rawPreset := range rawList {
		var p Preset
		if err := json.Unmarshal(rawPreset, &p); err != nil {
			warnings = append(warnings, PresetWarning{
				Message: fmt.Sprintf("presets[%d] unmarshal: %v", i, err),
				Action:  "skip",
			})
			continue
		}

		// preset.id required + format
		if p.ID == "" {
			warnings = append(warnings, PresetWarning{
				Message: fmt.Sprintf("presets[%d] missing id", i),
				Action:  "skip",
			})
			continue
		}
		if !presetIDPattern.MatchString(p.ID) {
			warnings = append(warnings, PresetWarning{
				PresetID: p.ID,
				Message:  fmt.Sprintf("id %q does not match [a-z0-9_-]+", p.ID),
				Action:   "skip",
			})
			continue
		}
		if seenIDs[p.ID] {
			warnings = append(warnings, PresetWarning{
				PresetID: p.ID,
				Message:  fmt.Sprintf("duplicate preset id %q (keep first)", p.ID),
				Action:   "skip",
			})
			continue
		}

		// Validate per-preset
		presetWarns, ok := validatePreset(&p, globalVarsNames)
		warnings = append(warnings, presetWarns...)
		if !ok {
			continue
		}

		seenIDs[p.ID] = true
		result = append(result, p)
	}

	return result, warnings
}

// validatePreset запускает все проверки для одного preset'а.
// Возвращает warnings и ok=false если preset надо целиком skip'нуть.
//
// При strip'ах модифицирует p in-place (удаляет невалидные элементы).
func validatePreset(p *Preset, globalVars map[string]bool) ([]PresetWarning, bool) {
	var warns []PresetWarning

	// vars[].name uniqueness + collision с globals
	if ws := validateVarsNames(p, globalVars); len(ws) > 0 {
		warns = append(warns, ws...)
	}

	// rule_set[].tag uniqueness
	if dup := findDuplicateTag(p.RuleSet); dup != "" {
		warns = append(warns, PresetWarning{
			PresetID: p.ID,
			Message:  fmt.Sprintf("duplicate rule_set tag %q", dup),
			Action:   "skip",
		})
		return warns, false
	}

	// dns_servers[].tag uniqueness
	if dup := findDuplicateDNSTag(p.DNSServers); dup != "" {
		warns = append(warns, PresetWarning{
			PresetID: p.ID,
			Message:  fmt.Sprintf("duplicate dns_servers tag %q", dup),
			Action:   "skip",
		})
		return warns, false
	}

	// vars validation (type-specific) — skip preset если любой warning имеет Action=skip
	skipPreset := false
	for i := range p.Vars {
		v := &p.Vars[i]
		if ws := validateVar(p.ID, v); len(ws) > 0 {
			warns = append(warns, ws...)
			for _, w := range ws {
				if w.Action == "skip" {
					skipPreset = true
				}
			}
		}
	}
	if skipPreset {
		return warns, false
	}

	// if/if_or references на bool vars того же preset'а
	if ws := validateIfRefs(p); len(ws) > 0 {
		warns = append(warns, ws...)
	}

	// rule/dns_rule rule_set references
	ruleSetTags := collectRuleSetTags(p.RuleSet)
	if ws := validateRuleSetRefs(p, ruleSetTags); len(ws) > 0 {
		warns = append(warns, ws...)
	}

	// preset.outbounds[] (SPEC 056) — нормализация Mode, валидация Tag/Type
	// уникальности и if/if_or ссылок. Невалидные entries stripped, preset
	// остаётся (loader-level — не roundtrip-failure).
	if ws := validatePresetOutbounds(p); len(ws) > 0 {
		warns = append(warns, ws...)
	}

	return warns, true
}

// validatePresetOutbounds — нормализует Mode, валидирует Tag/Type, уникальность
// тегов и if/if_or ссылки. Невалидные entries удаляются (strip), preset
// остаётся. SKIP preset'а через outbounds не делается — фича опциональная,
// лучше потерять одну entry чем целый preset.
func validatePresetOutbounds(p *Preset) []PresetWarning {
	if len(p.Outbounds) == 0 {
		return nil
	}
	var warns []PresetWarning

	allVars := make(map[string]bool, len(p.Vars))
	boolVars := make(map[string]bool, len(p.Vars))
	for _, v := range p.Vars {
		allVars[v.Name] = true
		if v.Type == "bool" {
			boolVars[v.Name] = true
		}
	}

	seenTags := make(map[string]bool, len(p.Outbounds))
	kept := make([]PresetOutbound, 0, len(p.Outbounds))

	for i := range p.Outbounds {
		ob := p.Outbounds[i]

		// 1) Normalize/validate mode (empty → "add"; unknown → strip entry).
		switch ob.Mode {
		case "":
			ob.Mode = "add"
		case "add", "update":
			// ok
		default:
			warns = append(warns, PresetWarning{
				PresetID: p.ID,
				Message: fmt.Sprintf(
					"outbounds[%d] (tag=%q): unknown mode %q (must be \"add\" or \"update\"; entry stripped)",
					i, ob.Tag, ob.Mode,
				),
				Action: "strip",
			})
			continue
		}

		// 2) Tag required.
		if strings.TrimSpace(ob.Tag) == "" {
			warns = append(warns, PresetWarning{
				PresetID: p.ID,
				Message:  fmt.Sprintf("outbounds[%d]: empty tag (entry stripped)", i),
				Action:   "strip",
			})
			continue
		}

		// 3) Per-mode field validation.
		switch ob.Mode {
		case "add":
			if strings.TrimSpace(ob.Type) == "" {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message: fmt.Sprintf(
						"outbounds[%d] (tag=%q): mode=add requires non-empty type (entry stripped)",
						i, ob.Tag,
					),
					Action: "strip",
				})
				continue
			}
		case "update":
			if strings.TrimSpace(ob.Type) != "" {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message: fmt.Sprintf(
						"outbounds[%d] (tag=%q): mode=update cannot change type (field %q dropped)",
						i, ob.Tag, ob.Type,
					),
					Action: "strip",
				})
				ob.Type = ""
			}
		}

		// 4) if/if_or references → bool vars того же preset'а.
		checkRefs := func(loc string, list []string) {
			for _, ref := range list {
				if !allVars[ref] {
					warns = append(warns, PresetWarning{
						PresetID: p.ID,
						Message: fmt.Sprintf(
							"outbounds[%d] (tag=%q) %s reference %q is unknown var (kept but won't match)",
							i, ob.Tag, loc, ref,
						),
						Action: "strip",
					})
				} else if !boolVars[ref] {
					warns = append(warns, PresetWarning{
						PresetID: p.ID,
						Message: fmt.Sprintf(
							"outbounds[%d] (tag=%q) %s reference %q is not a bool var (kept but won't match)",
							i, ob.Tag, loc, ref,
						),
						Action: "strip",
					})
				}
			}
		}
		checkRefs("if", ob.If)
		checkRefs("if_or", ob.IfOr)

		// 5) Tag uniqueness in preset (warning + skip dup, keep first).
		// Намеренно общая семантика для add+update: дважды update на одно tag
		// в одном preset'е — почти всегда копипаст-баг (для multi-preset
		// update это нормально и обрабатывается RuleOrder'ом в Phase 3).
		if seenTags[ob.Tag] {
			warns = append(warns, PresetWarning{
				PresetID: p.ID,
				Message: fmt.Sprintf(
					"outbounds[%d]: duplicate tag %q in same preset (keep first, drop later)",
					i, ob.Tag,
				),
				Action: "strip",
			})
			continue
		}
		seenTags[ob.Tag] = true

		kept = append(kept, ob)
	}

	p.Outbounds = kept
	return warns
}

// validateVarsNames — уникальность имён + collision с globals.
func validateVarsNames(p *Preset, globalVars map[string]bool) []PresetWarning {
	var warns []PresetWarning
	seen := make(map[string]bool, len(p.Vars))
	dedup := make([]PresetVar, 0, len(p.Vars))
	for _, v := range p.Vars {
		if v.Name == "" {
			warns = append(warns, PresetWarning{
				PresetID: p.ID,
				Message:  "var with empty name (stripped)",
				Action:   "strip",
			})
			continue
		}
		if seen[v.Name] {
			warns = append(warns, PresetWarning{
				PresetID: p.ID,
				Message:  fmt.Sprintf("duplicate var name %q (keep first)", v.Name),
				Action:   "strip",
			})
			continue
		}
		if globalVars[v.Name] {
			// Collision: предупреждаем, но preset.vars wins в его scope.
			warns = append(warns, PresetWarning{
				PresetID: p.ID,
				Message: fmt.Sprintf(
					"var %q shadows global template var (preset-local wins in this scope)",
					v.Name,
				),
				Action: "strip", // marker only; var остаётся
			})
		}
		seen[v.Name] = true
		dedup = append(dedup, v)
	}
	p.Vars = dedup
	return warns
}

// validateVar — type-specific проверки одной var'ы.
func validateVar(presetID string, v *PresetVar) []PresetWarning {
	var warns []PresetWarning

	// type обязателен
	switch v.Type {
	case "outbound", "dns_server", "enum", "text", "number", "bool":
		// ok
	default:
		warns = append(warns, PresetWarning{
			PresetID: presetID,
			Message:  fmt.Sprintf("var %q has unknown type %q (skip preset)", v.Name, v.Type),
			Action:   "skip",
		})
		// Сигнал — caller увидит "skip" Action и должен прервать preset.
		// Простоты ради не делаем early return, остальные warning'и тоже могут быть полезны.
	}

	// default required
	if v.Type != "bool" && v.Default == "" {
		// Для не-bool пустой default допустим только в редких случаях; warning информативный.
		warns = append(warns, PresetWarning{
			PresetID: presetID,
			Message:  fmt.Sprintf("var %q has empty default", v.Name),
			Action:   "strip",
		})
	}

	// select — только для dns_server
	if v.Select != "" {
		if v.Type != "dns_server" {
			warns = append(warns, PresetWarning{
				PresetID: presetID,
				Message: fmt.Sprintf(
					"var %q has 'select' but type is %q (only dns_server supports select; stripped)",
					v.Name, v.Type,
				),
				Action: "strip",
			})
			v.Select = ""
		} else if v.Select != "local" && v.Select != "global" {
			warns = append(warns, PresetWarning{
				PresetID: presetID,
				Message: fmt.Sprintf(
					"var %q has invalid select=%q (must be 'local' or 'global'; falling back to global)",
					v.Name, v.Select,
				),
				Action: "strip",
			})
			v.Select = ""
		}
	}

	// select + options collision
	if v.Select != "" && len(v.Options) > 0 && string(v.Options) != "null" {
		warns = append(warns, PresetWarning{
			PresetID: presetID,
			Message: fmt.Sprintf(
				"var %q has both 'select' and 'options' (options wins, select stripped)",
				v.Name,
			),
			Action: "strip",
		})
		v.Select = ""
	}

	// options decode + default ∈ options
	enum, tags, ok := v.DecodeOptions()
	if !ok {
		warns = append(warns, PresetWarning{
			PresetID: presetID,
			Message:  fmt.Sprintf("var %q has malformed options (stripped)", v.Name),
			Action:   "strip",
		})
		v.Options = nil
	}

	if enum != nil {
		// type=enum: default обязан быть среди value'ов
		found := false
		for _, e := range enum {
			if e.Value == v.Default {
				found = true
				break
			}
		}
		if !found {
			warns = append(warns, PresetWarning{
				PresetID: presetID,
				Message: fmt.Sprintf(
					"var %q (enum) default %q not in options",
					v.Name, v.Default,
				),
				Action: "skip",
			})
		}
	}
	if tags != nil {
		// type=dns_server/outbound с whitelist: default обязан быть в whitelist
		found := false
		for _, t := range tags {
			if t == v.Default {
				found = true
				break
			}
		}
		if !found {
			warns = append(warns, PresetWarning{
				PresetID: presetID,
				Message: fmt.Sprintf(
					"var %q (%s) default %q not in options whitelist",
					v.Name, v.Type, v.Default,
				),
				Action: "skip",
			})
		}
	}

	return warns
}

// validateIfRefs — if/if_or ссылается на bool vars того же preset'а.
func validateIfRefs(p *Preset) []PresetWarning {
	var warns []PresetWarning
	boolVars := make(map[string]bool, len(p.Vars))
	allVars := make(map[string]bool, len(p.Vars))
	for _, v := range p.Vars {
		allVars[v.Name] = true
		if v.Type == "bool" {
			boolVars[v.Name] = true
		}
	}

	check := func(loc string, ifList, ifOrList []string) {
		for _, ref := range ifList {
			if !allVars[ref] {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message:  fmt.Sprintf("%s: if reference %q is unknown var", loc, ref),
					Action:   "strip",
				})
			} else if !boolVars[ref] {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message:  fmt.Sprintf("%s: if reference %q is not a bool var", loc, ref),
					Action:   "strip",
				})
			}
		}
		for _, ref := range ifOrList {
			if !allVars[ref] {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message:  fmt.Sprintf("%s: if_or reference %q is unknown var", loc, ref),
					Action:   "strip",
				})
			} else if !boolVars[ref] {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message:  fmt.Sprintf("%s: if_or reference %q is not a bool var", loc, ref),
					Action:   "strip",
				})
			}
		}
	}

	for _, v := range p.Vars {
		check(fmt.Sprintf("var %q", v.Name), v.If, v.IfOr)
	}
	for _, rs := range p.RuleSet {
		check(fmt.Sprintf("rule_set %q", rs.Tag), rs.If, rs.IfOr)
	}
	for _, ds := range p.DNSServers {
		check(fmt.Sprintf("dns_servers %q", ds.Tag), ds.If, ds.IfOr)
	}

	return warns
}

// collectRuleSetTags — local rule_set tag'и (для validation ссылок).
func collectRuleSetTags(rs []PresetRuleSet) map[string]bool {
	out := make(map[string]bool, len(rs))
	for _, r := range rs {
		out[r.Tag] = true
	}
	return out
}

// validateRuleSetRefs — `rule.rule_set` / `dns_rule.rule_set` ссылаются на
// существующие local tag'и.
func validateRuleSetRefs(p *Preset, validTags map[string]bool) []PresetWarning {
	var warns []PresetWarning

	checkRef := func(loc string, val interface{}) {
		switch v := val.(type) {
		case string:
			if v != "" && !validTags[v] {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message:  fmt.Sprintf("%s: rule_set ref %q is unknown local tag", loc, v),
					Action:   "strip",
				})
			}
		case []interface{}:
			for i, t := range v {
				if s, ok := t.(string); ok && !validTags[s] {
					warns = append(warns, PresetWarning{
						PresetID: p.ID,
						Message: fmt.Sprintf(
							"%s: rule_set[%d] ref %q is unknown local tag",
							loc, i, s,
						),
						Action: "strip",
					})
				}
			}
		}
	}

	if p.Rule != nil {
		if ref, ok := p.Rule["rule_set"]; ok {
			checkRef("rule", ref)
		}
	}
	if p.DNSRule != nil {
		if ref, ok := p.DNSRule["rule_set"]; ok {
			checkRef("dns_rule", ref)
		}
		// dns_rule.server — литерал может ссылаться на bundled DNS-сервер.
		// Проверяем что либо @var (не валидируем здесь), либо существующий dns_server tag.
		if srv, ok := p.DNSRule["server"].(string); ok && srv != "" && !strings.HasPrefix(srv, "@") {
			dnsTags := make(map[string]bool, len(p.DNSServers))
			for _, ds := range p.DNSServers {
				dnsTags[ds.Tag] = true
			}
			if !dnsTags[srv] {
				warns = append(warns, PresetWarning{
					PresetID: p.ID,
					Message: fmt.Sprintf(
						"dns_rule.server %q is unknown bundled dns_server tag",
						srv,
					),
					Action: "strip",
				})
			}
		}
	}

	return warns
}

func findDuplicateTag(rs []PresetRuleSet) string {
	seen := make(map[string]bool, len(rs))
	for _, r := range rs {
		if seen[r.Tag] {
			return r.Tag
		}
		seen[r.Tag] = true
	}
	return ""
}

func findDuplicateDNSTag(ds []PresetDNSServer) string {
	seen := make(map[string]bool, len(ds))
	for _, d := range ds {
		if seen[d.Tag] {
			return d.Tag
		}
		seen[d.Tag] = true
	}
	return ""
}
