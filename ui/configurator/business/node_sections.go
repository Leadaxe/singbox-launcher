// File node_sections.go — секции узлов для UI (SPEC 121 §5.3).
//
// Вкладка DNS показывает серверы и правила, которые узлы носят с собой, — а
// чтобы их показать, нужен ФИНАЛЬНЫЙ тег узла: он и подставляется вместо
// `@self`, и префиксует теги DNS-серверов. Финальный тег знает ровно одна
// точка — эмиссионная тег-машина (config.EmitCanonicalSource), та же, которой
// считает превью состава источника.
//
// Пул узлов (model.NodePool) здесь НЕ используется: он строится по всем
// источникам целиком и на подписке в 500+ узлов стоит секунды, а секции живут
// у горстки свободных узлов. Эмитим только те источники, у которых секции
// есть.
package business

import (
	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// NodeSectionSetsFromModel — наборы секций узлов модели с финальными тегами,
// в порядке источников.
//
// Выключенные узлы и выключенные источники пропускаются: их в конфиге нет, а
// значит нет и их фрагментов — показывать их среди действующих значило бы
// обещать работающий DNS там, где его не будет.
func NodeSectionSetsFromModel(model *wizardmodels.WizardModel) []build.NodeSectionSet {
	if model == nil {
		return nil
	}
	var out []build.NodeSectionSet
	// tagCounts — свой на каждый источник, как у превью состава
	// (source_folder_drilldown.go): общий счётчик уникализации принадлежит
	// полной сборке, и одалживать его здесь значило бы показывать теги,
	// зависящие от порядка обхода.
	for i := range model.Sources {
		src := &model.Sources[i]
		if !sourceCarriesNodeSections(src) {
			continue
		}
		ps := src.ToProxySourceV4()
		if ps.Disabled {
			continue
		}
		emitted := config.EmitCanonicalSource(ps, i, map[string]int{})
		if emitted == nil {
			continue
		}
		for _, n := range emitted.Nodes {
			if n == nil || n.Sections.IsEmpty() {
				continue
			}
			decoded := corestate.NodeSectionsFromConfigTypes(n.Sections)
			if decoded == nil {
				continue
			}
			out = append(out, build.NodeSectionSet{
				FinalTag: n.Tag,
				Link:     build.NodeLink{FolderID: n.SectionsLink.FolderID, Tag: n.SectionsLink.Tag},
				Sections: decoded,
			})
		}
	}
	return out
}

// sourceCarriesNodeSections — есть ли у источника хоть один узел с секциями.
func sourceCarriesNodeSections(src *corestate.Source) bool {
	if src == nil {
		return false
	}
	if src.Kind == corestate.SourceKindServer && !src.Node.Sections.IsEmpty() {
		return true
	}
	for j := range src.Nodes {
		if !src.Nodes[j].Sections.IsEmpty() {
			return true
		}
	}
	return false
}

// NodeSectionDNSServer — один DNS-сервер узла, готовый к показу.
type NodeSectionDNSServer struct {
	// FinalTag — финальный тег УЗЛА (не сервера): подпись строки называет его
	// первым, чтобы было видно, чей это сервер.
	FinalTag string
	// LocalTag — тег самого сервера, уже с подставленным `@{self}`.
	LocalTag string
	// Body — тело сервера с подставленным плейсхолдером.
	Body map[string]interface{}
}

// NodeSectionDNSRule — одно DNS-правило узла, готовое к показу.
type NodeSectionDNSRule struct {
	FinalTag string
	Body     map[string]interface{}
}

// NodeSectionDNSForModel — DNS-записи всех узлов модели с подставленным
// финальным тегом.
//
// Подстановка — та же функция, что на сборке (state.SubstituteSelf через
// NodeSectionSet): показывать пользователю иначе подставленное тело значило бы
// завести вторую реализацию правил подстановки (и разойтись с ней на первой же
// правке).
func NodeSectionDNSForModel(model *wizardmodels.WizardModel) ([]NodeSectionDNSServer, []NodeSectionDNSRule) {
	sets := NodeSectionSetsFromModel(model)
	if len(sets) == 0 {
		return nil, nil
	}
	var servers []NodeSectionDNSServer
	var rules []NodeSectionDNSRule
	for _, set := range sets {
		for _, srv := range set.DNSServersWithSelf() {
			body := map[string]interface{}{}
			for k, v := range srv.Body {
				body[k] = v
			}
			if srv.Tag != "" {
				body["tag"] = srv.Tag
			}
			servers = append(servers, NodeSectionDNSServer{
				FinalTag: set.FinalTag,
				LocalTag: srv.Tag,
				Body:     body,
			})
		}
		for _, r := range set.DNSRulesWithSelf() {
			rules = append(rules, NodeSectionDNSRule{FinalTag: set.FinalTag, Body: r.Body})
		}
	}
	return servers, rules
}
