// File adapter_source.go — проекция canonical v7 Source → сборочная форма
// configtypes.ProxySource.
//
// SPEC 118 W5: моста больше нет. Проекция ничего не деривирует из легаси —
// она ПЕРЕКЛАДЫВАЕТ канон в форму, которую понимает конвейер сборки:
//
//   - узлы (server/chain/auto) и контейнеры (folder/subscription) едут
//     Canonical-проекцией — из неё эмиссия строит outbound'ы, не читая тел;
//   - в самой ProxySource остаётся ровно то, что каноном не описывается:
//     вход fetch (URL, skip) и тексты диагностики (Label, announce).
//
// Индексный инвариант жив: Proxies[i] строится из Sources[i] один к одному,
// без фильтрации и переупорядочивания.
package state

import (
	"singbox-launcher/core/config/configtypes"
)

// ToProxySourceV4 — конвертит Source (v7) в сборочную configtypes.ProxySource.
//
//   - subscription / folder → контейнер: Canonical.Nodes + tag-политика,
//     replace и общий detour папки;
//   - server / chain / auto → одиночный узел канона под своим тегом.
func (s *Source) ToProxySourceV4() configtypes.ProxySource {
	if s == nil {
		return configtypes.ProxySource{}
	}
	switch s.Kind {
	case SourceKindSubscription:
		ps := configtypes.ProxySource{
			ID:     s.ID,            // адресат NodeLink.FolderID
			Label:  s.displayName(), // только для текстов диагностики
			Source: s.URL,
			Skip:   s.Skip,
			// SPEC 115: сообщение провайдера — тоже только для диагностики.
			// Провозится ЗДЕСЬ, потому что дальше по конвейеру метаданных
			// источника уже нет: разбору достаётся только сборочная форма.
			ProviderAnnounce: s.announceMessage(),
			Disabled:         !s.Enabled,
		}
		if s.TagPolicy != nil {
			ps.TagPrefix = s.TagPolicy.Prefix
			ps.TagPostfix = s.TagPolicy.Postfix
		}
		ps.Canonical = s.canonicalProjection()
		return ps

	case SourceKindFolder:
		ps := configtypes.ProxySource{
			ID:       s.ID,
			Label:    s.displayName(),
			Disabled: !s.Enabled,
		}
		if s.TagPolicy != nil {
			ps.TagPrefix = s.TagPolicy.Prefix
			ps.TagPostfix = s.TagPolicy.Postfix
		}
		ps.Canonical = s.canonicalProjection()
		return ps

	case SourceKindServer, SourceKindChain, SourceKindAuto:
		ps := configtypes.ProxySource{
			ID:       s.ID,
			Label:    s.displayName(),
			Disabled: !s.Enabled,
		}
		ps.Canonical = s.canonicalProjection()
		if ps.Canonical == nil {
			// Узел без тела/хопов/группы собирать не из чего: в конфиг он не
			// едет, но позицию в индексном инварианте держит.
			ps.Disabled = true
		}
		return ps
	}

	// Неизвестный kind сюда не доходит (normalizeSourceShape отвергает файл),
	// но позиция обязана существовать в любом случае.
	return configtypes.ProxySource{
		ID:       s.ID,
		Label:    s.displayName(),
		Disabled: true,
	}
}

// canonicalProjection — проекция канона v7 в сборочную форму.
//
// У контейнера (папка/подписка) проекция есть ВСЕГДА, даже когда nodes[]
// пуст: «подписку ещё ни разу не обновляли» — это состояние источника, а не
// повод собирать его каким-то другим путём (warning отчёта сборки — SPEC Т3).
// nil бывает только у узла, которому нечего эмитить.
func (s *Source) canonicalProjection() *configtypes.CanonicalSource {
	if s == nil {
		return nil
	}
	switch s.Kind {
	case SourceKindFolder, SourceKindSubscription:
		cs := &configtypes.CanonicalSource{
			FolderID:     s.ID,
			IsContainer:  true,
			FolderDetour: canonicalLink(s.Detour),
			Replace:      canonicalReplace(s.Replace),
		}
		if s.TagPolicy != nil {
			cs.TagPrefix = s.TagPolicy.Prefix
			cs.TagPostfix = s.TagPolicy.Postfix
		}
		cs.Nodes = make([]configtypes.CanonicalNode, 0, len(s.Nodes))
		for i := range s.Nodes {
			// Неразобранная запись (kind=unsupported) в сборочную форму не
			// едет ВООБЩЕ — SPEC 116 W11. Пропуск ровно здесь, ДО тег-машины:
			// пройди она эмиссию хотя бы «выключенным узлом», она съела бы
			// номер {$num} и слот глобальной уникализации, и у соседей
			// поменялись бы финальные теги — то есть протухли бы выборы в
			// кэше ядра и ссылки, адресующие финальный тег. Старый движок
			// такую запись узлом не считал вовсе, и этот пропуск — ровно то
			// же поведение.
			if s.Nodes[i].IsUnsupported() {
				continue
			}
			cs.Nodes = append(cs.Nodes, canonicalNodeProjection(&s.Nodes[i]))
		}
		return cs

	case SourceKindServer, SourceKindChain, SourceKindAuto:
		if !s.Node.materialized() {
			return nil
		}
		n := canonicalNodeProjection(&s.Node)
		// Тег корневого узла — тот, под которым его знает конфиг.
		n.Tag = s.NodeTagOrLabel()
		return &configtypes.CanonicalSource{
			Nodes: []configtypes.CanonicalNode{n},
		}
	}
	return nil
}

// materialized — узел несёт данные, из которых его можно собрать.
func (n *Node) materialized() bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case SourceKindServer:
		return len(n.Body) > 0
	case SourceKindChain:
		return len(n.Hops) > 0
	case SourceKindAuto:
		return n.Group != nil
	}
	return false
}

// canonicalNodeProjection — Node (канон) → сборочная форма.
func canonicalNodeProjection(n *Node) configtypes.CanonicalNode {
	out := configtypes.CanonicalNode{
		Kind:    string(n.Kind),
		Tag:     n.Tag,
		Enabled: n.Enabled,
		Body:    n.Body,
		Detour:  canonicalLink(n.Detour),
	}
	if n.Origin != nil {
		out.OriginKind = n.Origin.Kind
		out.OriginRaw = n.Origin.Raw
	}
	if len(n.Hops) > 0 {
		out.Hops = make([]configtypes.NodeLink, 0, len(n.Hops))
		for _, h := range n.Hops {
			out.Hops = append(out.Hops, configtypes.NodeLink{FolderID: h.FolderID, Tag: h.Tag})
		}
	}
	if n.Group != nil {
		g := &configtypes.CanonicalAutoGroup{
			GroupType: n.Group.GroupType,
			Default:   n.Group.Default,
			Options:   AutoStrategyOptions(n.Group.Strategy),
		}
		g.Members = make([]configtypes.NodeLink, 0, len(n.Group.Members))
		for _, m := range n.Group.Members {
			g.Members = append(g.Members, configtypes.NodeLink{FolderID: m.FolderID, Tag: m.Tag})
		}
		out.Group = g
	}
	return out
}

// canonicalLink — NodeLink канона → сборочная форма.
func canonicalLink(l *NodeLink) *configtypes.NodeLink {
	if l == nil {
		return nil
	}
	return &configtypes.NodeLink{FolderID: l.FolderID, Tag: l.Tag}
}

// canonicalReplace — FolderReplace канона → сборочная форма.
func canonicalReplace(r *FolderReplace) *configtypes.FolderReplace {
	if r == nil {
		return nil
	}
	return &configtypes.FolderReplace{
		Mode:     r.Mode,
		Tag:      r.Tag,
		Strategy: r.Strategy.Clone(),
	}
}

// AutoStrategyOptions разворачивает AutoStrategy провайдерской группы в опции
// sing-box-группы — тем же allowlist'ом, каким миграция/fetch их собирали
// (config.autoStrategyFromGroupOptions, обратная сторона).
//
// Живёт здесь, а не в config: проекция обязана быть рядом с типом, из
// которого читает, иначе пакет config получил бы вторую точку знания о форме
// AutoStrategy.
func AutoStrategyOptions(a AutoStrategy) map[string]interface{} {
	opts := make(map[string]interface{}, 5)
	if a.URL != "" {
		opts["url"] = a.URL
	}
	if a.Interval != "" {
		opts["interval"] = a.Interval
	}
	if a.IdleTimeout != "" {
		opts["idle_timeout"] = a.IdleTimeout
	}
	if v := a.Tolerance.Value(); v != nil {
		opts["tolerance"] = v
	}
	if a.InterruptExistConnections != nil {
		opts["interrupt_exist_connections"] = *a.InterruptExistConnections
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

// displayName — отображаемое имя источника: канонический Name папки/подписки,
// иначе Label узловых kind'ов.
func (s *Source) displayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Label
}

// announceMessage — сообщение провайдера из метаданных источника, обрезанное
// общим правилом (AnnounceMessage); пусто, если провайдер молчал.
//
// Только для подписок: у источника-сервера и цепочки метаданных нет.
func (s *Source) announceMessage() string {
	if s == nil || s.Meta == nil {
		return ""
	}
	return s.Meta.ProviderAnnounce.AnnounceMessage()
}
