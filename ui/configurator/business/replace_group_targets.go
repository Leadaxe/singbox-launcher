// File replace_group_targets.go — группы свёртки источников как цели ссылок
// (SPEC 182).
//
// Одна точка для обоих пикеров — detour и позиций цепочки: какие группы
// свёртки можно предложить целью и какие из них замыкают кольцо. Разойдись
// два списка — и одна и та же группа была бы целью в одном окне и
// отсутствовала в другом (так и было до SPEC 182: цепочка группы видела,
// detour — нет).
//
// Правила списка:
//   - источник включён, имя свёртки непустое — все режимы (manual/auto/both);
//   - у both целью идёт селектор `tag`; двойник `<tag>-auto` — его
//     внутренняя опция, отдельной целью не предлагается (как у Направлений);
//   - группа самого носителя не предлагается: узел не ходит через группу,
//     в которой состоит;
//   - имя, уже объявленное Направлением, его двойником, тегом шаблона,
//     пресета или свёрткой выше по списку, не предлагается: сборка такую
//     группу не строит (replace_tag_conflict), и ссылка ушла бы в чужое имя;
//   - группа, из которой переходы ведут обратно к носителю, предлагается с
//     пометкой Loop — показать можно, выбрать нельзя.
package business

import (
	"strings"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// ReplaceGroupTarget — группа свёртки одного источника как цель ссылки.
type ReplaceGroupTarget struct {
	// Tag — имя свёртки: корневая цель ссылки.
	Tag string
	// TwinTag — двойник `<tag>-auto` режима both; пусто у остальных. Целью
	// не предлагается, но ссылка на него из старого состояния живая.
	TwinTag string
	// Mode — режим свёртки (manual/auto/both).
	Mode string
	// SourceID / SourceName — чей это источник.
	SourceID, SourceName string
	// Loop — выбор этой группы носителем замкнул бы кольцо переходов.
	Loop bool
}

// LinkHolder — кто выбирает цель: источник (SourceID) либо узел контейнера
// (FolderID + RawTag). Пустой носитель — никого не исключать и колец не
// искать.
type LinkHolder struct {
	SourceID string
	FolderID string
	RawTag   string
}

// ownSourceID — источник, чья группа носителю своя.
func (h LinkHolder) ownSourceID() string {
	if h.SourceID != "" {
		return h.SourceID
	}
	return h.FolderID
}

// ReplaceGroupTargets — группы свёртки, которые носитель может видеть целью
// ссылки, в порядке источников.
func ReplaceGroupTargets(model *wizardmodels.WizardModel, holder LinkHolder) []ReplaceGroupTarget {
	if model == nil {
		return nil
	}
	taken := nonReplaceRootNames(model)
	own := holder.ownSourceID()
	var out []ReplaceGroupTarget
	for i := range model.Sources {
		src := &model.Sources[i]
		if src.Replace == nil || !src.Enabled {
			continue
		}
		tag := strings.TrimSpace(src.Replace.Tag)
		if tag == "" {
			continue
		}
		twin := ""
		if src.Replace.Mode == corestate.FolderReplaceBoth {
			twin = tag + "-auto"
		}
		if taken[tag] || (twin != "" && taken[twin]) {
			continue // сборка эту группу не построит
		}
		taken[tag] = true
		if twin != "" {
			taken[twin] = true
		}
		if own != "" && src.ID == own {
			continue // своя группа: имя занято, но целью не идёт
		}
		name := src.Name
		if strings.TrimSpace(name) == "" {
			name = src.Label
		}
		out = append(out, ReplaceGroupTarget{
			Tag: tag, TwinTag: twin, Mode: src.Replace.Mode,
			SourceID: src.ID, SourceName: name,
			Loop: linkGraphReaches(model, i, holder),
		})
	}
	return out
}

// nonReplaceRootNames — корневые имена, объявленные НЕ свёртками: с ними
// свёртка проигрывает спор за имя (PrepareFolderReplaces сборки).
func nonReplaceRootNames(model *wizardmodels.WizardModel) map[string]bool {
	names := map[string]bool{wizardmodels.DefaultOutboundTag: true}
	add := func(tag string) {
		if tag = strings.TrimSpace(tag); tag != "" {
			names[tag] = true
		}
	}
	for i := range model.GlobalOutbounds {
		d := &model.GlobalOutbounds[i]
		add(d.Tag)
		if d.Auto != nil && strings.TrimSpace(d.Tag) != "" {
			add(d.AutoTag())
		}
	}
	for _, tag := range collectActivePresetOutboundTags(model) {
		add(tag)
	}
	if td := model.TemplateData; td != nil {
		add(td.DirectionBlockTag())
		for _, tag := range td.SystemOutboundTags() {
			add(tag)
		}
	}
	return names
}

// linkGraphReaches — ведут ли переходы источника start (его detour, detour
// его узлов, позиции цепочки) обратно к носителю.
//
// Идём только по рёбрам, которые видны в модели: ссылка на группу свёртки —
// это весь её источник, корневая ссылка на узел — источник этого узла,
// папочная — узел папки. Направление не раскрывается: его состав считают
// фильтры на сборке, и кольцо через него рвёт сама сборка
// (detour_group_cycle.go).
func linkGraphReaches(model *wizardmodels.WizardModel, start int, holder LinkHolder) bool {
	if holder.SourceID == "" && holder.FolderID == "" && holder.RawTag == "" {
		return false
	}
	rootOwner := map[string]int{} // корневое имя → индекс источника
	byID := map[string]int{}
	for i := range model.Sources {
		s := &model.Sources[i]
		if s.ID != "" {
			byID[s.ID] = i
		}
		if !s.Enabled {
			continue
		}
		switch s.Kind {
		case corestate.SourceKindServer, corestate.SourceKindChain, corestate.SourceKindAuto:
			if tag := strings.TrimSpace(s.NodeTagOrLabel()); tag != "" {
				if _, dup := rootOwner[tag]; !dup {
					rootOwner[tag] = i
				}
			}
		}
	}
	// Свёртки поверх узлов: имя группы главнее тёзки-узла (узел на сборке
	// получает суффикс).
	for i := range model.Sources {
		s := &model.Sources[i]
		if !s.Enabled || s.Replace == nil {
			continue
		}
		if tag := strings.TrimSpace(s.Replace.Tag); tag != "" {
			rootOwner[tag] = i
			if s.Replace.Mode == corestate.FolderReplaceBoth {
				rootOwner[tag+"-auto"] = i
			}
		}
	}

	// isHolder — попали ли в носителя: idx — источник, raw — узел в нём
	// («» = источник целиком).
	isHolder := func(idx int, raw string) bool {
		s := &model.Sources[idx]
		if holder.SourceID != "" {
			return s.ID == holder.SourceID
		}
		if holder.FolderID != "" {
			return s.ID == holder.FolderID && (raw == "" || raw == holder.RawTag)
		}
		// Узел корня: его адрес — сырой тег.
		return strings.TrimSpace(s.NodeTagOrLabel()) == holder.RawTag
	}

	type entity struct {
		idx int
		raw string
	}
	seen := map[entity]bool{}
	var walk func(e entity) bool
	follow := func(link *corestate.NodeLink) bool {
		if link == nil || strings.TrimSpace(link.Tag) == "" {
			return false
		}
		next := entity{}
		if link.FolderID != "" {
			idx, ok := byID[link.FolderID]
			if !ok {
				return false
			}
			next = entity{idx: idx, raw: link.Tag}
		} else {
			idx, ok := rootOwner[strings.TrimSpace(link.Tag)]
			if !ok {
				return false
			}
			next = entity{idx: idx}
		}
		if isHolder(next.idx, next.raw) {
			return true
		}
		return walk(next)
	}
	walk = func(e entity) bool {
		if seen[e] {
			return false
		}
		seen[e] = true
		s := &model.Sources[e.idx]
		if follow(s.Detour) {
			return true
		}
		for i := range s.Hops {
			if follow(&s.Hops[i]) {
				return true
			}
		}
		for i := range s.Nodes {
			if e.raw != "" && s.Nodes[i].Tag != e.raw {
				continue
			}
			if follow(s.Nodes[i].Detour) {
				return true
			}
		}
		return false
	}
	if isHolder(start, "") {
		return true
	}
	return walk(entity{idx: start})
}
