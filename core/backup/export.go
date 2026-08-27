package backup

// Экспорт состояния лаунчера в переносимый LX Backup (контракт 0.11.0).

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"singbox-launcher/core/config/configtypes"
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
// Экспорт — ЧИСТАЯ ФУНКЦИЯ СОСТОЯНИЯ (П1): всё, что пишется в файл, читается
// из полей state, и ничего кроме. Ни блобов «на провоз», ни следов того,
// откуда состояние взялось: два неотличимых состояния обязаны давать
// байт-идентичные файлы, а состояние, приехавшее импортом, — тот же файл,
// что и настроенное руками.
//
// Что НЕ едет и почему:
//   - runtime-данные подписок (Meta: когда обновлялась, сколько нод) —
//     они принадлежат машине, а не пользовательской настройке;
//   - финальные конфиговые теги (П5) — они вычисляются каждой сборкой на
//     принимающей стороне, и хранимый приехал бы мёртвым;
//   - упразднённый detour_node_hash — его нет в схеме 0.11.0.
//
// Пустые и нулевые поля опускаются: значение по умолчанию, записанное явно, —
// это лишний шум, из-за которого «одно и то же состояние» перестаёт давать
// один и тот же файл.
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

	// Цепочки — корневая секция chains[], у обеих сторон общая. Порядок
	// записей нормативен (вложенная цепочка объявлена раньше использующей) —
	// сохраняется порядок списка источников.
	for i, src := range s.Connections.Sources {
		switch src.Type {
		case state.SourceTypeSubscription:
			b.Subscriptions = append(b.Subscriptions, exportSubscription(src))
		case state.SourceTypeServer:
			b.Servers = append(b.Servers, exportServer(src))
		case state.SourceTypeChain:
			b.Chains = append(b.Chains, exportChain(src, i))
		}
	}

	for _, r := range s.Rules {
		rule, err := exportRule(r)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", r.Kind, err)
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

	// Warp-аккаунты (BACKUP.md §2): канонические snake_case-поля плюс
	// дискриминатор type. Без этого регистрация не переезжала вовсе, и на
	// новой машине «Add WARP» плодил лишние device-записи в Cloudflare.
	b.Warp = exportWarp(s)

	return b, nil
}

// exportDirections — список Направлений в канонической форме.
func exportDirections(list []configtypes.Direction) []Direction {
	var out []Direction
	for _, d := range list {
		if d.Tag == "" {
			continue
		}
		out = append(out, exportDirection(d))
	}
	return out
}

// exportSourceRef — ссылка источника на цель дозвона.
//
// Упразднённый detour_node_hash не пишется никогда: в схеме 0.11.0 его нет.
// Источник, ещё не прошедший миграцию, отдаёт пустую ссылку — она
// восстановится на приёмнике первой же сборкой из своего состояния, а вывозить
// протухающий хеш значило бы вернуть в формат ровно то, ради чего он снесён.
func exportSourceRef(src state.Source) SourceRef {
	ref := SourceRef{
		DetourTag:          src.DetourTag,
		DetourNodeSourceID: src.DetourNodeSourceID,
		DetourNodeTag:      src.DetourNodeTag,
	}
	if ref.DetourNodeTag != "" || ref.DetourNodeSourceID != "" {
		ref.DetourNodeLabel = src.DetourNodeLabel
	}
	return ref
}

// exportChain — запись секции chains[] (SPEC 110).
//
// Tag — эффективный тег outbound'а цепочки (NodeTagOrLabel), на который
// ссылаются правила и позиции других цепочек; Label — отображаемое имя.
// index — позиция источника в общем списке: у безымянной цепочки тег на
// сборке получается позиционным (chainSourceTag), и в файл обязан уехать тот
// же эффективный тег — схема требует tag, а импорт зафиксирует его именем
// (нормализация той же категории, что перенумерация правил).
func exportChain(src state.Source, index int) Chain {
	out := Chain{
		ID:                src.ID,
		Tag:               src.NodeTagOrLabel(),
		Label:             src.Label,
		Chain:             src.Chain,
		ExcludeFromGlobal: src.ExcludeFromGlobal,
		SourceRef:         exportSourceRef(src),
	}
	// Подпись, совпадающая с тегом, — это не подпись, а прежнее состояние
	// без разделения ролей: писать её отдельным полем значит плодить шум,
	// который на импорте не несёт информации.
	if out.Label == out.Tag {
		out.Label = ""
	}
	if out.Tag == "" {
		out.Tag = "chain-" + strconv.Itoa(index+1)
	}
	if !src.Enabled {
		out.Enabled = boolPtr(false)
	}
	return out
}

// exportWarp — WG/MASQUE-регистрации в переносимую форму warp[].
func exportWarp(s *state.State) []json.RawMessage {
	if s.WarpAccounts == nil {
		return nil
	}
	var out []json.RawMessage
	appendAcc := func(typ string, acc any) {
		m := map[string]any{}
		raw, err := json.Marshal(acc)
		if err != nil || json.Unmarshal(raw, &m) != nil {
			return
		}
		m["type"] = typ
		if enc, err := json.Marshal(m); err == nil {
			out = append(out, enc)
		}
	}
	if s.WarpAccounts.WG != nil {
		appendAcc("wg", s.WarpAccounts.WG)
	}
	if s.WarpAccounts.Masque != nil {
		appendAcc("masque", s.WarpAccounts.Masque)
	}
	return out
}

func exportSubscription(src state.Source) Subscription {
	out := Subscription{
		ID:                      src.ID,
		URL:                     src.URL,
		Label:                   src.Label,
		MaxNodes:                src.MaxNodes,
		Skip:                    src.Skip,
		Outbounds:               exportDirections(src.Outbounds),
		Fold:                    src.Fold,
		ExcludeFromGlobal:       src.ExcludeFromGlobal,
		ExposeGroupTagsToGlobal: src.ExposeGroupTagsToGlobal,
		SourceRef:               exportSourceRef(src),
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
	return out
}

func exportServer(src state.Source) Server {
	out := Server{
		ID:                src.ID,
		URI:               src.URI,
		Label:             src.Label,
		NodeTag:           src.NodeTag,
		ExcludeFromGlobal: src.ExcludeFromGlobal,
		SourceRef:         exportSourceRef(src),
	}
	if len(src.ConfigJSON) > 0 {
		out.ConfigJSON = append(json.RawMessage(nil), src.ConfigJSON...)
	}
	if !src.Enabled {
		out.Enabled = boolPtr(false)
	}
	return out
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
		return Rule{}, fmt.Errorf("unknown kind %q", r.Kind)
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
