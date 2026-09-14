package backup

// Писатель формата 1.0 (SPEC 127 §6.1 W2.1).
//
// Export10 — ЧИСТАЯ ФУНКЦИЯ СОСТОЯНИЯ (П1): всё, что попадает в файл,
// прочитано из полей state, и ничего кроме. Два неотличимых состояния дают
// байт-идентичные файлы; состояние, приехавшее импортом, даёт тот же файл,
// что настроенное руками.
//
// Отличие от 0.12-писателя не в дисциплине, а в объёме работы: раскладывать
// записи по чужой форме больше не нужно — записи состояния сериализуются
// своими типами. Поэтому здесь нет ни exportRule, ни dnsRefFrom: rules[] и
// dns — срезы состояния как есть. Остаётся ровно две обязанности: снять то,
// что в файл не едет (кэш подписки, рантайм), и перевести три поля в форму
// контракта (fold, disabled, directions).
//
// Предупреждения: backup_local_only_dropped в 1.0 НЕ эмитится — полей без
// дома в схеме больше не осталось. WarnBackupReplaceTagDerived остаётся: он
// про то, что явное имя группы свёртки на приёмнике выведется формулой и
// может не совпасть.

import (
	"encoding/json"
	"fmt"
	"time"

	"singbox-launcher/core/state"
)

// Export10 переносит состояние v8 в файл контракта 1.0.
//
// Второй возврат — предупреждения экспорта: то, что состояние несёт, а файл
// выражает иначе. Молчаливое выпадение запрещено (П6).
func Export10(s *state.State, opts ExportOptions) (*Backup10, []Warning, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("nil state")
	}
	var warnings []Warning
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	b := &Backup10{
		LxBackup: FormatVersion10,
		ExportedBy: ExportedBy{
			App:      AppLauncher,
			Version:  opts.AppVersion,
			Platform: opts.Platform,
		},
		ExportedAt: now.UTC().Format(time.RFC3339),
	}

	// Направления едут вместе с правилами (SPEC 104): правило, сославшееся на
	// `vpn-3`, без них приезжало бы в никуда.
	for _, d := range s.Directions {
		if d.Tag == "" {
			continue
		}
		b.Directions = append(b.Directions, exportDirection(d))
	}

	// Тег свёртки формат 1.0 везёт ЯВНО (`fold_tag`, см. Source10), поэтому
	// WarnBackupReplaceTagDerived здесь не нужен и не эмитится: он говорил о
	// потере, которой в этом формате больше нет. У 0.12-писателя он остался —
	// там теряется по-прежнему.
	for _, src := range s.Sources {
		out, ok := export10Source(src)
		if !ok {
			// Вид, которого union `sources[]` не выражает. Молча выронить
			// запись нельзя (П6): корневая провайдерская группа — законная
			// форма состояния (state.NewAutoSource, normalizeSourceShape
			// принимает её наравне с server/chain), и пользователь обязан
			// узнать, что она в файл не поехала. Код и поля — те же, что у
			// писателя 0.12 (legacy_write_012.go): вид и объём идут отдельно,
			// потому что группа уезжает НЕ ОДНА, а со своим составом.
			warnings = append(warnings, Warning{
				Code:   WarnBackupSourceKindUnsupported,
				Detail: sourceExportName(src),
				Kind:   string(src.Kind),
				Nodes:  len(src.Nodes),
			})
			continue
		}
		b.Sources = append(b.Sources, out)
	}

	// Правила и DNS — срезы состояния; маппера нет, только копии (чтобы
	// правка состояния после экспорта не меняла уже собранный файл).
	b.Rules = export10Rules(s.Rules)
	b.DNS = export10DNS(s)

	if vars := exportVars(s.Vars); len(vars) > 0 {
		b.Vars = vars
	}
	if final := routeFinal(s); final != "" {
		b.Route = &Route{Final: final}
	}
	b.Warp = exportWarp(s)

	return b, warnings, nil
}

// export10Source — запись sources[] из источника состояния.
//
// Второй возврат — едет ли запись вообще. Корневая провайдерская группа
// (kind=auto) в состоянии законна, но union `sources[]` её не выражает и
// слияние применить не умеет, поэтому она не едет — и вызывающий обязан
// сказать об этом предупреждением, а не промолчать (П6).
func export10Source(src state.Source) (Source10, bool) {
	switch src.Kind {
	case state.SourceKindServer, state.SourceKindChain,
		state.SourceKindFolder, state.SourceKindSubscription:
	default:
		return Source10{}, false
	}

	node := cloneNode(src.Node)
	out := Source10{
		Kind:     node.Kind,
		Tag:      node.Tag,
		Enabled:  node.Enabled,
		Origin:   node.Origin,
		Body:     node.Body,
		Detour:   node.Detour,
		Hops:     node.Hops,
		Group:    node.Group,
		Service:  node.Service,
		Reason:   node.Reason,
		Sections: node.Sections,

		ID:        src.ID,
		Name:      src.Name,
		TagPolicy: cloneTagPolicy(src.TagPolicy),
		Nodes:     cloneNodes(src.Nodes),

		URL:                src.URL,
		Identity:           src.Identity.Clone(),
		RelaysInDirections: src.RelaysInDirections,
		Skip:               cloneSkip(src.Skip),
		MaxNodes:           src.MaxNodes,
		Update:             cloneUpdateSpec(src.Update),

		// Свёртка — формой контракта; в состоянии её имя `replace`, и оба
		// имени в одном файле были бы двумя источниками правды. Имя группы
		// в эту форму не влезает (там его нет вовсе) и едет рядом — иначе
		// приёмник выводил бы его формулой и подменял явное имя молча.
		Fold:    exportFold(src.Replace),
		FoldTag: foldTagOf(src.Replace),
	}

	if src.Kind != state.SourceKindSubscription {
		return out, true
	}

	// Подписка: кэш выдачи провайдера в файл не едет — он принадлежит машине
	// и наполнится первым же обновлением на приёмнике. Отметки выключенных
	// узлов, наоборот, едут: это решение пользователя, а не кэш.
	out.Nodes = nil
	out.Disabled = exportDisabledMap(src)
	return out, true
}

// foldTagOf — явное имя группы свёртки; пусто, если свёртки нет.
//
// Отдельная мелкая функция, чтобы у писателя и у сверочного теста был один
// ответ на вопрос «какое имя уехало в файл».
func foldTagOf(r *state.FolderReplace) string {
	if r == nil {
		return ""
	}
	return r.Tag
}

// export10Rules — копии записей правил.
//
// Копии, а не срез состояния: тело — json.RawMessage, и общий массив байт
// означал бы, что правка правила в UI после экспорта меняет уже собранный
// файл. Сама форма записи не трогается — это и есть смысл 1.0.
func export10Rules(rules []state.Rule) []state.Rule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]state.Rule, 0, len(rules))
	for _, r := range rules {
		out = append(out, state.CloneRule(r))
	}
	return out
}

// export10DNS — секция dns состояния копией.
//
// Пустая секция в файл не пишется: значение по умолчанию, записанное явно, —
// шум, из-за которого «одно и то же состояние» перестаёт давать один файл.
func export10DNS(s *state.State) *state.DNSOptions {
	if s.DNS.IsEmpty() {
		return nil
	}
	out := &state.DNSOptions{
		Strategy:              s.DNS.Strategy,
		Final:                 s.DNS.Final,
		DefaultDomainResolver: s.DNS.DefaultDomainResolver,
	}
	for _, srv := range s.DNS.Servers {
		out.Servers = append(out.Servers, state.CloneDNSServer(srv))
	}
	for _, r := range s.DNS.Rules {
		out.Rules = append(out.Rules, state.CloneDNSRule(r))
	}
	return out
}

// ── копии полей источника ──────────────────────────────────────────
//
// Общих указателей у файла и у состояния быть не должно: экспорт — снимок
// момента, а не окно в живые данные. Каждая функция мелкая намеренно: список
// полей, требующих копии, читается глазами и не прячется внутри одной
// большой.

func cloneTagPolicy(in *state.TagPolicy) *state.TagPolicy {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneUpdateSpec(in *state.UpdateSpec) *state.UpdateSpec {
	if in == nil {
		return nil
	}
	out := *in
	if in.AutoRefresh != nil {
		v := *in.AutoRefresh
		out.AutoRefresh = &v
	}
	return &out
}

func cloneSkip(in []map[string]string) []map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(in))
	for _, m := range in {
		c := make(map[string]string, len(m))
		for k, v := range m {
			c[k] = v
		}
		out = append(out, c)
	}
	return out
}

func cloneNodes(in []state.Node) []state.Node {
	if len(in) == 0 {
		return nil
	}
	out := make([]state.Node, 0, len(in))
	for _, n := range in {
		out = append(out, cloneNode(n))
	}
	return out
}

// cloneNode — копия узла вместе с телом, ссылками и секциями.
func cloneNode(n state.Node) state.Node {
	out := n
	if len(n.Body) > 0 {
		out.Body = append(json.RawMessage(nil), n.Body...)
	}
	if n.Origin != nil {
		o := *n.Origin
		out.Origin = &o
	}
	if n.Detour != nil {
		d := *n.Detour
		out.Detour = &d
	}
	if len(n.Hops) > 0 {
		out.Hops = append([]state.NodeLink(nil), n.Hops...)
	}
	if n.Group != nil {
		g := *n.Group
		g.Members = append([]state.NodeLink(nil), n.Group.Members...)
		out.Group = &g
	}
	out.Sections = n.Sections.Clone()
	return out
}
