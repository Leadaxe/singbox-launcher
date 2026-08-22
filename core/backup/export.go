package backup

// Экспорт состояния лаунчера в переносимый LX Backup (SPEC 103, фаза 4).

import (
	"encoding/json"
	"fmt"
	"time"

	"singbox-launcher/core/state"
)

// ExportOptions — что подмешать в шапку файла.
type ExportOptions struct {
	// AppVersion — версия лаунчера (exported_by.version).
	AppVersion string
	// Platform — GOOS, для диагностики односторонних полей.
	Platform string
	// Now — момент экспорта; ноль означает time.Now(). Параметр существует
	// ради воспроизводимых тестов, а не ради «настраиваемости».
	Now time.Time
}

// Export переносит state в формат бэкапа.
//
// Что НЕ едет в переносимую часть и почему:
//   - runtime-данные подписок (Meta: когда обновлялась, сколько нод) —
//     они принадлежат машине, а не пользовательской настройке;
//   - local outbounds источника, exclude_from_global и прочее, чему нет
//     места в схеме, — уезжает в extensions.launcher, чтобы round-trip
//     остался lossless.
//
// Чужой блоб extensions.lxbox, если он пришёл с импортом, возвращается в
// файл нетронутым: бэкап, побывавший на десктопе, не должен вернуться на
// телефон обеднённым (BACKUP.md §1).
func Export(s *state.State, opts ExportOptions) (*Backup, error) {
	if s == nil {
		return nil, fmt.Errorf("nil state")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	b := &Backup{
		LxBackup: FormatVersion,
		ExportedBy: ExportedBy{
			App:      AppLauncher,
			Version:  opts.AppVersion,
			Platform: opts.Platform,
		},
		ExportedAt: now.UTC().Format(time.RFC3339),
	}

	// SPEC 104: Направления едут вместе с правилами — иначе правило,
	// сославшееся на `vpn-3`, приезжало бы в никуда.
	for _, d := range s.Connections.Outbounds {
		if d.Tag == "" {
			continue
		}
		b.Directions = append(b.Directions, exportDirection(d))
	}

	for _, src := range s.Connections.Sources {
		switch src.Type {
		case state.SourceTypeSubscription:
			b.Subscriptions = append(b.Subscriptions, exportSubscription(src))
		case state.SourceTypeServer:
			b.Servers = append(b.Servers, exportServer(src))
		}
	}

	for _, r := range s.Rules {
		rule, err := exportRule(r)
		if err != nil {
			return nil, fmt.Errorf("правило %s: %w", r.Kind, err)
		}
		b.Rules = append(b.Rules, rule)
	}

	if vars := exportVars(s.Vars); len(vars) > 0 {
		b.Vars = vars
	}
	if final := routeFinal(s); final != "" {
		b.Route = &Route{Final: final}
	}
	if dns := exportDNS(s); dns != nil {
		b.DNS = dns
	}

	// Чужие расширения, сохранённые прошлым импортом.
	if len(s.ForeignBackupExtensions) > 0 {
		b.Extensions = Extensions{}
		for app, blob := range s.ForeignBackupExtensions {
			b.Extensions[app] = append(json.RawMessage(nil), blob...)
		}
	}

	return b, nil
}

func exportSubscription(src state.Source) Subscription {
	out := Subscription{
		URL:      src.URL,
		Label:    src.Label,
		MaxNodes: src.MaxNodes,
	}
	if !src.Enabled {
		out.Enabled = boolPtr(false)
	}
	if src.Tag != nil && !src.Tag.IsZero() {
		out.Tag = &TagPolicy{Prefix: src.Tag.Prefix, Postfix: src.Tag.Postfix, Mask: src.Tag.Mask}
	}
	if src.Update != nil {
		out.Update = &UpdatePolicy{IntervalHours: src.Update.IntervalHours, Auto: src.Update.AutoRefresh}
	}
	if len(src.DisabledNodes) > 0 {
		out.Disabled = make(map[string]int64, len(src.DisabledNodes))
		for hash, ts := range src.DisabledNodes {
			out.Disabled[hash] = ts
		}
	}
	if ext := launcherSourceExtensions(src); ext != nil {
		out.Extensions = Extensions{AppLauncher: ext}
	}
	return out
}

func exportServer(src state.Source) Server {
	out := Server{
		URI:   src.URI,
		Label: src.Label,
	}
	if len(src.ConfigJSON) > 0 {
		out.ConfigJSON = append(json.RawMessage(nil), src.ConfigJSON...)
	}
	if !src.Enabled {
		out.Enabled = boolPtr(false)
	}
	if ext := launcherSourceExtensions(src); ext != nil {
		out.Extensions = Extensions{AppLauncher: ext}
	}
	return out
}

// launcherSourceExtensions собирает непереносимые поля источника.
//
// Без этого round-trip терял бы detour, skip-фильтры и локальные outbound'ы:
// экспорт-импорт на одной машине обязан быть тождественным (BACKUP.md §1).
func launcherSourceExtensions(src state.Source) json.RawMessage {
	ext := map[string]any{}
	if src.ID != "" {
		ext["id"] = src.ID
	}
	if len(src.Skip) > 0 {
		ext["skip"] = src.Skip
	}
	if len(src.Outbounds) > 0 {
		ext["outbounds"] = src.Outbounds
	}
	if src.ExcludeFromGlobal {
		ext["exclude_from_global"] = true
	}
	if src.ExposeGroupTagsToGlobal {
		ext["expose_group_tags_to_global"] = true
	}
	if src.DetourTag != "" {
		ext["detour_tag"] = src.DetourTag
	}
	if src.DetourNodeHash != "" {
		ext["detour_node_hash"] = src.DetourNodeHash
		ext["detour_node_label"] = src.DetourNodeLabel
	}
	if len(ext) == 0 {
		return nil
	}
	raw, err := json.Marshal(ext)
	if err != nil {
		return nil
	}
	return raw
}

func exportRule(r state.Rule) (Rule, error) {
	out := Rule{
		Kind:    RuleKind(r.Kind),
		Enabled: boolPtr(r.Enabled),
	}
	if r.OrderNum != nil {
		out.Num = f64Ptr(float64(*r.OrderNum))
	}
	if !r.Enabled {
		out.Enabled = boolPtr(false)
	} else {
		out.Enabled = nil // true — умолчание схемы
	}

	switch r.Kind {
	case state.RuleKindPreset:
		out.Ref = r.Ref
		var body state.PresetBody
		if len(r.Body) > 0 {
			if err := json.Unmarshal(r.Body, &body); err != nil {
				return Rule{}, fmt.Errorf("preset body: %w", err)
			}
		}
		if len(body.Vars) > 0 {
			out.Vars = body.Vars
		}
	case state.RuleKindInline:
		var body state.InlineBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			return Rule{}, fmt.Errorf("inline body: %w", err)
		}
		out.Name = body.Name
		out.Outbound = body.Outbound
		if len(body.Match) > 0 {
			raw, err := json.Marshal(body.Match)
			if err != nil {
				return Rule{}, fmt.Errorf("inline match: %w", err)
			}
			out.Match = raw
		}
	case state.RuleKindSrs:
		var body state.SrsBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			return Rule{}, fmt.Errorf("srs body: %w", err)
		}
		out.Name = body.Name
		out.Ref = body.SrsURL
		out.Outbound = body.Outbound
	default:
		return Rule{}, fmt.Errorf("неизвестный kind %q", r.Kind)
	}
	return out, nil
}

// exportVars отдаёт только переносимые имена переменных.
//
// Реестр (registry/vars.json) помечает portable-имена: те, что означают одно
// и то же на обеих сторонах. Непереносимое (пути, интерфейсы, платформенные
// флаги) на другой машине значит другое, и переносить его — значит молча
// сломать чужую настройку.
func exportVars(vars []state.SettingVar) map[string]string {
	if len(vars) == 0 {
		return nil
	}
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		if !IsPortableVar(v.Name) {
			continue
		}
		out[v.Name] = v.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func routeFinal(s *state.State) string {
	for _, p := range s.ConfigParams {
		if p.Name == "final" || p.Name == "route.final" {
			return p.Value
		}
	}
	return ""
}

// exportDNS переносит секцию DNS.
//
// Записи kind=template/preset едут ССЫЛКАМИ (ref/tag), а не телом: тело
// приедет из шаблона принимающей стороны. Переносить развёрнутое тело значило
// бы зафиксировать чужие умолчания навсегда — обновление шаблона перестало бы
// доходить до пользователя.
func exportDNS(s *state.State) *DNS {
	out := &DNS{
		Final:    s.DNS.Final,
		Strategy: s.DNS.Strategy,
	}
	for _, srv := range s.DNS.Servers {
		out.Servers = append(out.Servers, dnsRefFrom(string(srv.Kind), srv.Tag, srv.Ref, srv.Enabled, srv.Body))
	}
	for _, rule := range s.DNS.Rules {
		out.Rules = append(out.Rules, dnsRefFrom(string(rule.Kind), "", rule.Ref, rule.Enabled, rule.Body))
	}
	if out.Final == "" && out.Strategy == "" && len(out.Servers) == 0 && len(out.Rules) == 0 {
		return nil
	}
	return out
}

func dnsRefFrom(kind, tag, ref string, enabled bool, body map[string]interface{}) DNSRef {
	out := DNSRef{Kind: kind, Name: tag, Ref: ref}
	if !enabled {
		out.Enabled = boolPtr(false)
	}
	// Тело переносится только у пользовательских записей: у template/preset
	// оно принадлежит шаблону принимающей стороны.
	if kind == "user" && len(body) > 0 {
		if raw, err := json.Marshal(body); err == nil {
			out.Value = raw
		}
	}
	return out
}
