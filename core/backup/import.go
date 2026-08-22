package backup

// Импорт LX Backup в состояние лаунчера (SPEC 103, фаза 4).

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/state"
)

// Warning — то, что импортёр не смог применить дословно.
//
// Не ошибка: импорт продолжается. Но и не молчание — пользователь обязан
// узнать, что правило приехало выключенным или что настройка не применилась
// (BACKUP.md §1, инвариант «нет молчаливых потерь»).
type Warning struct {
	// Code — машинный код (contract/registry/warnings.json).
	Code string
	// Detail — что именно затронуто: имя правила, тег, URL.
	Detail string
}

func (w Warning) String() string { return w.Code + ": " + w.Detail }

// Коды предупреждений импорта.
const (
	// WarnBackupUnknownOutbound — цель rules[].outbound не существует:
	// правило импортируется ВЫКЛЮЧЕННЫМ. Молча включить его нельзя —
	// правило с несуществующей целью роняет конфиг ядра.
	WarnBackupUnknownOutbound = "backup_unknown_outbound"
	// WarnBackupFinalDropped — route.final указывает в никуда: не
	// применяется (иначе весь трафик уходит в несуществующий outbound).
	WarnBackupFinalDropped = "backup_final_dropped"
	// WarnBackupUnknownPreset — preset id вне шаблона принимающей стороны.
	WarnBackupUnknownPreset = "backup_unknown_preset"
	// WarnBackupVarSkipped — переменная не в списке переносимых.
	WarnBackupVarSkipped = "backup_var_skipped"
	// WarnBackupUnknownField — ключ вне схемы и вне extensions: default-deny,
	// в состояние не попадает.
	WarnBackupUnknownField = "backup_unknown_field"
	// WarnBackupDirectionExists — Направление с таким тегом уже есть:
	// приехавшее НЕ применяется. Перезапись стёрла бы настройки, сделанные
	// на этой машине, а правила и так найдут цель по тегу (SPEC 104).
	WarnBackupDirectionExists = "backup_direction_exists"
)

// errSkipRule — правило пропущено осознанно (чужой kind), а не сломалось.
// Отдельная ошибка, а не bool: вызывающий обязан различать «пропусти это» и
// «импорт невозможен», иначе одно чужое правило уронит весь файл.
var errSkipRule = errors.New("rule skipped")

// ImportMode — как сочетать с текущим состоянием.
type ImportMode int

const (
	// ImportReplace — заменить разделы целиком.
	ImportReplace ImportMode = iota
	// ImportMerge — дописать к существующему, не трогая совпадающее.
	ImportMerge
)

// ImportOptions — контекст принимающей стороны.
type ImportOptions struct {
	Mode ImportMode
	// KnownOutbounds — теги, на которые правилу разрешено ссылаться.
	// Пустой список означает «проверять нечем» — тогда ссылки не режутся:
	// выключить всё подряд хуже, чем импортировать как есть.
	KnownOutbounds []string
	// KnownPresets — id пресетов шаблона принимающей стороны.
	KnownPresets []string
}

// ImportResult — что получилось.
type ImportResult struct {
	Warnings []Warning
	// AppliedRules / AppliedSources / AppliedDirections — сколько записей
	// реально применено.
	AppliedRules      int
	AppliedSources    int
	AppliedDirections int
}

// Import применяет бэкап к state.
//
// Порядок разделов значим: источники раньше правил, потому что правило может
// ссылаться на тег, который приезжает вместе с источником.
func Import(s *state.State, b *Backup, opts ImportOptions) (*ImportResult, error) {
	if s == nil {
		return nil, fmt.Errorf("nil state")
	}
	if b == nil {
		return nil, fmt.Errorf("nil backup")
	}
	if b.LxBackup > FormatVersion {
		return nil, fmt.Errorf("формат бэкапа v%d новее поддерживаемого v%d — обновите приложение",
			b.LxBackup, FormatVersion)
	}

	res := &ImportResult{}

	if opts.Mode == ImportReplace {
		s.Connections.Sources = nil
		s.Rules = nil
	}

	for _, sub := range b.Subscriptions {
		s.Connections.Sources = append(s.Connections.Sources, importSubscription(sub))
		res.AppliedSources++
	}
	for _, srv := range b.Servers {
		s.Connections.Sources = append(s.Connections.Sources, importServer(srv))
		res.AppliedSources++
	}

	// SPEC 104: Направления импортируются ДО правил и пополняют список
	// известных целей — иначе правило, чья цель приехала в этом же файле,
	// импортировалось бы выключенным.
	//
	// Существующий тег не трогаем: у принимающей стороны своё Направление с
	// этим именем, и перезапись стёрла бы его настройки.
	existing := make(map[string]bool, len(s.Connections.Outbounds))
	for _, d := range s.Connections.Outbounds {
		existing[d.Tag] = true
	}
	knownTags := append([]string(nil), opts.KnownOutbounds...)
	for _, in := range b.Directions {
		if in.Tag == "" {
			continue
		}
		if existing[in.Tag] {
			res.Warnings = append(res.Warnings, Warning{WarnBackupDirectionExists, in.Tag})
			knownTags = append(knownTags, in.Tag)
			continue
		}
		s.Connections.Outbounds = append(s.Connections.Outbounds, importDirection(in))
		existing[in.Tag] = true
		knownTags = append(knownTags, in.Tag)
		res.AppliedDirections++
	}

	known := newTagSet(knownTags)
	presets := newTagSet(opts.KnownPresets)

	for _, r := range b.Rules {
		rule, warns, err := importRule(r, known, presets)
		res.Warnings = append(res.Warnings, warns...)
		if errors.Is(err, errSkipRule) {
			continue // правило не наше — пропущено с warning, импорт живёт
		}
		if err != nil {
			return nil, fmt.Errorf("правило %q: %w", ruleLabel(r), err)
		}
		s.Rules = append(s.Rules, rule)
		res.AppliedRules++
	}

	// Ось порядка перенумеровывается: абсолютные номера у сторон свои, важен
	// лишь относительный порядок (BACKUP.md §2).
	renumberImportedRules(s.Rules)

	if b.Route != nil && b.Route.Final != "" {
		if known.empty() || known.has(b.Route.Final) {
			setConfigParam(s, "final", b.Route.Final)
		} else {
			res.Warnings = append(res.Warnings, Warning{WarnBackupFinalDropped, b.Route.Final})
		}
	}

	res.Warnings = append(res.Warnings, importVars(s, b.Vars)...)

	// Чужой блоб extensions сохраняем нетронутым до следующего экспорта.
	for app, blob := range b.Extensions {
		if app == AppLauncher {
			continue // своё применяется полями выше
		}
		if s.ForeignBackupExtensions == nil {
			s.ForeignBackupExtensions = map[string]json.RawMessage{}
		}
		s.ForeignBackupExtensions[app] = append(json.RawMessage(nil), blob...)
	}

	return res, nil
}

func importSubscription(sub Subscription) state.Source {
	src := state.Source{
		Type:     state.SourceTypeSubscription,
		URL:      sub.URL,
		Label:    sub.Label,
		MaxNodes: sub.MaxNodes,
		Enabled:  sub.Enabled == nil || *sub.Enabled,
	}
	if sub.Tag != nil {
		src.Tag = &state.TagSpec{Prefix: sub.Tag.Prefix, Postfix: sub.Tag.Postfix, Mask: sub.Tag.Mask}
	}
	if sub.Update != nil {
		src.Update = &state.UpdateSpec{IntervalHours: sub.Update.IntervalHours, AutoRefresh: sub.Update.Auto}
	}
	if len(sub.Disabled) > 0 {
		src.DisabledNodes = make(map[string]int64, len(sub.Disabled))
		for hash, ts := range sub.Disabled {
			src.DisabledNodes[hash] = ts
		}
	}
	applyLauncherSourceExtensions(&src, sub.Extensions)
	return src
}

func importServer(srv Server) state.Source {
	src := state.Source{
		Type:    state.SourceTypeServer,
		URI:     srv.URI,
		Label:   srv.Label,
		Enabled: srv.Enabled == nil || *srv.Enabled,
	}
	if len(srv.ConfigJSON) > 0 {
		src.ConfigJSON = append(json.RawMessage(nil), srv.ConfigJSON...)
	}
	applyLauncherSourceExtensions(&src, srv.Extensions)
	return src
}

// applyLauncherSourceExtensions возвращает собственные непереносимые поля.
// Чужие блобы здесь не трогаются — их место в ForeignBackupExtensions.
func applyLauncherSourceExtensions(src *state.Source, ext Extensions) {
	blob, ok := ext[AppLauncher]
	if !ok || len(blob) == 0 {
		return
	}
	var own struct {
		ID                      string              `json:"id"`
		Skip                    []map[string]string `json:"skip"`
		ExcludeFromGlobal       bool                `json:"exclude_from_global"`
		ExposeGroupTagsToGlobal bool                `json:"expose_group_tags_to_global"`
		DetourTag               string              `json:"detour_tag"`
		DetourNodeHash          string              `json:"detour_node_hash"`
		DetourNodeLabel         string              `json:"detour_node_label"`
	}
	if err := json.Unmarshal(blob, &own); err != nil {
		return // битое расширение не повод терять источник
	}
	src.ID = own.ID
	src.Skip = own.Skip
	src.ExcludeFromGlobal = own.ExcludeFromGlobal
	src.ExposeGroupTagsToGlobal = own.ExposeGroupTagsToGlobal
	src.DetourTag = own.DetourTag
	src.DetourNodeHash = own.DetourNodeHash
	src.DetourNodeLabel = own.DetourNodeLabel
}

func importRule(r Rule, known, presets tagSet) (state.Rule, []Warning, error) {
	var warns []Warning
	enabled := r.Enabled == nil || *r.Enabled

	// Символическая ссылка в никуда: правило приезжает выключенным, а не
	// теряется. Ядро отвергает конфиг с несуществующим outbound целиком,
	// поэтому «оставить включённым» здесь означало бы сломать пользователю
	// весь VPN одним импортом (BACKUP.md §3).
	if r.Outbound != "" && !known.empty() && !known.has(r.Outbound) {
		enabled = false
		warns = append(warns, Warning{WarnBackupUnknownOutbound, ruleLabel(r) + " → " + r.Outbound})
	}

	out := state.Rule{
		Kind:    state.RuleKind(r.Kind),
		Enabled: enabled,
	}
	if r.Num != nil {
		n := int(*r.Num)
		out.OrderNum = &n
	}

	switch RuleKind(r.Kind) {
	case RulePreset:
		out.Ref = r.Ref
		if !presets.empty() && !presets.has(r.Ref) {
			out.Enabled = false
			warns = append(warns, Warning{WarnBackupUnknownPreset, r.Ref})
		}
		body := state.PresetBody{Vars: r.Vars}
		raw, err := json.Marshal(body)
		if err != nil {
			return state.Rule{}, warns, err
		}
		out.Body = raw
	case RuleInline:
		var match map[string]interface{}
		if len(r.Match) > 0 {
			if err := json.Unmarshal(r.Match, &match); err != nil {
				return state.Rule{}, warns, fmt.Errorf("match: %w", err)
			}
		}
		raw, err := json.Marshal(state.InlineBody{Name: r.Name, Match: match, Outbound: r.Outbound})
		if err != nil {
			return state.Rule{}, warns, err
		}
		out.Body = raw
	case RuleSRS:
		raw, err := json.Marshal(state.SrsBody{Name: r.Name, SrsURL: r.Ref, Outbound: r.Outbound})
		if err != nil {
			return state.Rule{}, warns, err
		}
		out.Body = raw
	case RuleJSON:
		// kind=json — сырое правило другой стороны: применять вслепую нельзя
		// (структура чужая). Но и ронять весь импорт из-за одного правила
		// нельзя — пользователь потеряет всё остальное. Правило
		// пропускается, факт называется.
		return state.Rule{}, append(warns, Warning{WarnBackupUnknownField, "rules[].kind=json: " + ruleLabel(r)}), errSkipRule
	default:
		return state.Rule{}, append(warns, Warning{WarnBackupUnknownField, "rules[].kind=" + string(r.Kind)}), errSkipRule
	}

	return out, warns, nil
}

func ruleLabel(r Rule) string {
	if r.Name != "" {
		return r.Name
	}
	if r.Ref != "" {
		return r.Ref
	}
	return string(r.Kind)
}

// renumberImportedRules перенумеровывает ось порядка, сохраняя относительный
// порядок: у сторон свои диапазоны, а важен лишь порядок следования.
func renumberImportedRules(rules []state.Rule) {
	idx := make([]int, 0, len(rules))
	for i, r := range rules {
		if r.OrderNum != nil {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return *rules[idx[a]].OrderNum < *rules[idx[b]].OrderNum
	})
	for pos, i := range idx {
		n := state.UserRuleNumStart + pos
		rules[i].OrderNum = &n
	}
}

func importVars(s *state.State, vars map[string]string) []Warning {
	if len(vars) == 0 {
		return nil
	}
	var warns []Warning
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !IsPortableVar(name) {
			// Непереносимое имя на этой машине значит другое (путь,
			// интерфейс, платформенный флаг) — применять нельзя.
			warns = append(warns, Warning{WarnBackupVarSkipped, name})
			continue
		}
		setVar(s, name, vars[name])
	}
	return warns
}

func setVar(s *state.State, name, value string) {
	for i := range s.Vars {
		if s.Vars[i].Name == name {
			s.Vars[i].Value = value
			return
		}
	}
	s.Vars = append(s.Vars, state.SettingVar{Name: name, Value: value})
}

func setConfigParam(s *state.State, name, value string) {
	for i := range s.ConfigParams {
		if s.ConfigParams[i].Name == name {
			s.ConfigParams[i].Value = value
			return
		}
	}
	s.ConfigParams = append(s.ConfigParams, state.ConfigParam{Name: name, Value: value})
}

// tagSet — множество известных тегов с нормализацией регистра.
type tagSet map[string]struct{}

func newTagSet(items []string) tagSet {
	if len(items) == 0 {
		return nil
	}
	out := make(tagSet, len(items))
	for _, it := range items {
		out[strings.ToLower(strings.TrimSpace(it))] = struct{}{}
	}
	return out
}

func (t tagSet) empty() bool { return len(t) == 0 }

func (t tagSet) has(tag string) bool {
	if t.empty() {
		return false
	}
	// Зарезервированные литералы существуют всегда: их не нужно объявлять.
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "direct", "block", "reject", "drop", "dns-out":
		return true
	}
	_, ok := t[strings.ToLower(strings.TrimSpace(tag))]
	return ok
}
