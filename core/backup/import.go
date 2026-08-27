package backup

// Импорт LX Backup в состояние лаунчера (контракт 0.11.0).

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// Warning — то, что импортёр не смог применить дословно.
//
// Не ошибка: импорт продолжается. Но и не молчание — пользователь обязан
// узнать, что правило приехало выключенным или что настройка не применилась
// (BACKUP_PRINCIPLES.md П6, «молчаливых потерь нет»).
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
	// WarnBackupUnknownField — ключ вне схемы: в состояние не попадает (П3).
	// Detail называет и ключ, и сущность, в которой он встретился.
	WarnBackupUnknownField = "backup_unknown_field"
	// WarnBackupFieldTypeMismatch — ключ модели пришёл ЧУЖОГО ТИПА: поле
	// отбрасывается, разбор файла продолжается. Отдельный код, а не
	// backup_unknown_field: ключ-то знакомый, разошёлся его тип
	// (`subscriptions[].skip` — boolean у LxBox 0.10.x, список фильтров
	// отсева у launcher), и пользователю важно различать «такого поля тут
	// нет» и «поле есть, но значение записано по-другому». Detail называет
	// полный путь: subscriptions[https://…].skip.
	WarnBackupFieldTypeMismatch = "backup_field_type_mismatch"
	// WarnBackupExtensionsDropped — файл схемы 0.10.x с механизмом
	// extensions. Один warning на файл с перечнем затронутых записей: пока
	// extensions существовал, он был не «одним лишним ключом», а карманом с
	// произвольным содержимым, и перечислять его внутренности по одной
	// значило бы утопить пользователя в списке.
	WarnBackupExtensionsDropped = "backup_extensions_dropped"
	// WarnBackupDirectionExists — Направление с таким тегом уже есть:
	// приехавшее НЕ применяется. Перезапись стёрла бы настройки, сделанные
	// на этой машине, а правила и так найдут цель по тегу (SPEC 104).
	WarnBackupDirectionExists = "backup_direction_exists"
	// WarnBackupChainExists — цепочка с таким тегом уже есть: приехавшая НЕ
	// применяется, своя сильнее. Warning ставится ВСЕГДА, даже когда «своя
	// победила» — молчание скрыло бы случайных тёзок: две несвязанные
	// цепочки, одинаково названные на разных устройствах, склеиваются в
	// одну, и пользователь обязан узнать об этом (BACKUP.md §4).
	WarnBackupChainExists = "backup_chain_exists"
)

// errSkipRule — правило пропущено осознанно (чужой kind), а не сломалось.
// Отдельная ошибка, а не bool: вызывающий обязан различать «пропусти это» и
// «импорт невозможен», иначе одно чужое правило уронит весь файл.
var errSkipRule = errors.New("rule skipped")

// ImportOptions — контекст принимающей стороны.
//
// Режим импорта один — replace (BACKUP.md §9): разделы источников и правил
// замещаются приехавшими. Прежний ImportMerge удалён — он не был достижим
// ни из одного UI и потому не проверялся ничем, кроме собственных тестов, а
// его перенумерация оси переписывала номера уже стоявших правил, включая
// якорные зоны шаблона. Возвращать merge следует не флагом, а вместе с
// инвариантом «файл = ось» из SPEC 113-C.
type ImportOptions struct {
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
// Состояние после импорта неотличимо от настроенного руками (П1): теневых
// полей «на провоз» нет, непонятое отброшено и названо warning'ом (П3).
// Warning'и о неизвестных ключах и об extensions выдаёт Parse — он один видит
// сырой JSON; здесь они не дублируются.
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
		return nil, fmt.Errorf("backup format v%d is newer than supported v%d — update the app",
			b.LxBackup, FormatVersion)
	}

	res := &ImportResult{}

	s.Connections.Sources = nil
	s.Rules = nil

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

	// Цепочки — ПОСЛЕ Направлений (позиция может ссылаться на Направление) и
	// ДО правил (правило может метить в цепочку как в цель — тег пополняет
	// список известных). Порядок записей нормативен и сохраняется как есть.
	// Достижимость hops здесь не проверяется: хоп — чаще всего узел подписки,
	// которого до её обновления не существует; рубеж у обеих сторон один —
	// сборка (chain_hop_missing).
	existingChains := map[string]bool{}
	for _, src := range s.Connections.Sources {
		if src.Type == state.SourceTypeChain {
			existingChains[src.NodeTagOrLabel()] = true
		}
	}
	for _, in := range b.Chains {
		if in.Tag == "" || in.Chain == nil {
			continue
		}
		if existingChains[in.Tag] {
			res.Warnings = append(res.Warnings, Warning{WarnBackupChainExists, in.Tag})
			knownTags = append(knownTags, in.Tag)
			continue
		}
		s.Connections.Sources = append(s.Connections.Sources, importChain(in))
		existingChains[in.Tag] = true
		knownTags = append(knownTags, in.Tag)
		res.AppliedSources++
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
			return nil, fmt.Errorf("rule %q: %w", ruleLabel(r), err)
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

	importDNS(s, b.DNS)
	importWarp(s, b.Warp)

	return res, nil
}

// importDirections — обратная сторона exportDirections.
func importDirections(list []Direction) []configtypes.Direction {
	var out []configtypes.Direction
	for _, in := range list {
		if in.Tag == "" {
			continue
		}
		out = append(out, importDirection(in))
	}
	return out
}

// importSourceRef восстанавливает ссылку источника на цель дозвона.
func importSourceRef(src *state.Source, ref SourceRef) {
	src.DetourTag = ref.DetourTag
	src.DetourNodeSourceID = ref.DetourNodeSourceID
	src.DetourNodeTag = ref.DetourNodeTag
	src.DetourNodeLabel = ref.DetourNodeLabel
}

func importSubscription(sub Subscription) state.Source {
	src := state.Source{
		ID:                      sub.ID,
		Type:                    state.SourceTypeSubscription,
		URL:                     sub.URL,
		Label:                   sub.Label,
		MaxNodes:                sub.MaxNodes,
		Enabled:                 sub.Enabled == nil || *sub.Enabled,
		Skip:                    sub.Skip,
		Outbounds:               importDirections(sub.Outbounds),
		Fold:                    sub.Fold,
		ExcludeFromGlobal:       sub.ExcludeFromGlobal,
		ExposeGroupTagsToGlobal: sub.ExposeGroupTagsToGlobal,
	}
	importSourceRef(&src, sub.SourceRef)
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
	return src
}

func importServer(srv Server) state.Source {
	src := state.Source{
		ID:                srv.ID,
		Type:              state.SourceTypeServer,
		URI:               srv.URI,
		Label:             srv.Label,
		NodeTag:           srv.NodeTag,
		Enabled:           srv.Enabled == nil || *srv.Enabled,
		ExcludeFromGlobal: srv.ExcludeFromGlobal,
	}
	importSourceRef(&src, srv.SourceRef)
	if len(srv.ConfigJSON) > 0 {
		src.ConfigJSON = append(json.RawMessage(nil), srv.ConfigJSON...)
	}
	return src
}

// importChain переводит каноническую запись chains[] во внутренний источник.
//
// Тег записи едет в NodeTag, отображаемое имя — в Label: обе роли имеют своё
// поле. Раньше тег клался в Label (другого места не было), из-за чего импорт
// чужого label разъехался бы со ссылками правил, route.final и позиций других
// цепочек.
func importChain(in Chain) state.Source {
	src := state.Source{
		ID:                in.ID,
		Type:              state.SourceTypeChain,
		NodeTag:           in.Tag,
		Label:             in.Label,
		Enabled:           in.Enabled == nil || *in.Enabled,
		Chain:             in.Chain,
		ExcludeFromGlobal: in.ExcludeFromGlobal,
	}
	importSourceRef(&src, in.SourceRef)
	return src
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
//
// SPEC 113-C §1: перенумерация заканчивается пересортировкой массива. Иначе
// импорт оставлял бы файл, где номера говорят одно, а порядок записей другое —
// а закон оси запрещает читать позицию в слайсе как самостоятельный смысл.
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

	// Неразмеченные (бэкап без num) уезжают в хвост, сохраняя взаимный
	// порядок: разметку им раздаст MarkRuleOrder на первой загрузке, и она
	// пойдёт от конца занятой части — иначе они перебили бы перенумерованных.
	sort.SliceStable(rules, func(a, b int) bool {
		return importedAxisNum(rules[a]) < importedAxisNum(rules[b])
	})
}

// importedAxisNum — номер для сортировки импортированных: неразмеченное
// правило считается стоящим за всеми размеченными.
func importedAxisNum(r state.Rule) int {
	if r.OrderNum == nil {
		return state.UserRuleNumEnd + 1
	}
	return *r.OrderNum
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

// importDNS применяет секцию DNS: списки замещаются приехавшими.
//
// Тела переносятся только у kind=user (симметрия с exportDNS: тело
// template/preset принадлежит шаблону принимающей стороны).
func importDNS(s *state.State, dns *DNS) {
	if dns == nil {
		return
	}
	s.DNS.Servers = nil
	s.DNS.Rules = nil

	serverKey := func(kind, tag, ref string) string { return kind + "\x00" + tag + "\x00" + ref }
	haveServers := map[string]bool{}
	for _, ref := range dns.Servers {
		key := serverKey(ref.Kind, ref.Name, ref.Ref)
		if haveServers[key] {
			continue
		}
		srv := state.DNSServer{
			Kind:    state.DNSServerKind(ref.Kind),
			Tag:     ref.Name,
			Ref:     ref.Ref,
			Enabled: ref.Enabled == nil || *ref.Enabled,
		}
		if ref.Kind == "user" && len(ref.Value) > 0 {
			var body map[string]interface{}
			if json.Unmarshal(ref.Value, &body) == nil {
				srv.Body = body
			}
		}
		s.DNS.Servers = append(s.DNS.Servers, srv)
		haveServers[key] = true
	}

	ruleKey := func(kind, ref string, body map[string]interface{}) string {
		raw, _ := json.Marshal(body)
		return kind + "\x00" + ref + "\x00" + string(raw)
	}
	haveRules := map[string]bool{}
	for _, ref := range dns.Rules {
		var body map[string]interface{}
		if ref.Kind == "user" && len(ref.Value) > 0 {
			_ = json.Unmarshal(ref.Value, &body)
		}
		key := ruleKey(ref.Kind, ref.Ref, body)
		if haveRules[key] {
			continue
		}
		s.DNS.Rules = append(s.DNS.Rules, state.DNSRule{
			Kind:    state.DNSRuleKind(ref.Kind),
			Ref:     ref.Ref,
			Enabled: ref.Enabled == nil || *ref.Enabled,
			Body:    body,
		})
		haveRules[key] = true
	}
	if dns.Final != "" {
		s.DNS.Final = dns.Final
	}
	if dns.Strategy != "" {
		s.DNS.Strategy = dns.Strategy
	}
}

// importWarp восстанавливает WG/MASQUE-регистрации из warp[].
func importWarp(s *state.State, warp []json.RawMessage) {
	for _, raw := range warp {
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &head) != nil {
			continue
		}
		switch head.Type {
		case "wg":
			var acc state.WarpWGAccount
			if json.Unmarshal(raw, &acc) != nil || acc.PrivateKey == "" {
				continue
			}
			if s.WarpAccounts == nil {
				s.WarpAccounts = &state.WarpAccountsSection{}
			}
			s.WarpAccounts.WG = &acc
		case "masque":
			var acc state.WarpMasqueAccount
			if json.Unmarshal(raw, &acc) != nil || acc.PrivateKeyDER == "" {
				continue
			}
			if s.WarpAccounts == nil {
				s.WarpAccounts = &state.WarpAccountsSection{}
			}
			s.WarpAccounts.Masque = &acc
		}
	}
}
