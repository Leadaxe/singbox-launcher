// File nodelink_normalize.go — терпимое чтение ссылок NodeLink
// (contract/docs/NODE_LINK.md §7.3).
//
// # Зачем
//
// Сборки 1.6.0 до выпуска писали ссылки на провайдерские группы и члены групп
// в формах, которые норма не принимает: член группы внутри контейнера без
// folder_id, позицию цепочки на группу ФИНАЛЬНЫМ тегом (форма брала имя из
// конфига). Форма хранения state v8 и файл 1.0 ещё не выпущены, поэтому новой
// версии схемы нет: такие ссылки поднимаются до нормы при ЧТЕНИИ — загрузкой
// состояния, импортом бэкапа и вставкой записи.
//
// Правило одно на все входы и идемпотентно: поднятая ссылка под правило больше
// не подпадает, поэтому повторная загрузка ничего не меняет, и отдельной
// перезаписи файла на загрузке не нужно.
//
// # Правила
//
//   - S1 — член `{tag}` группы ВНУТРИ контейнера C → `{C, tag}`. Сборка так его
//     и трактует (§5.1 № 8), форма просто становится явной.
//   - S3 — пара `{F, T}` в detour или позиции, где F — контейнер, T не сырой
//     тег ни одного узла F и T — финальный тег (политика F без суффикса
//     уникализации) ровно одного узла F, и это группа → `{F, сырой тег
//     группы}`.
//
// Ноль или несколько кандидатов — ссылка остаётся как есть: подставить узел
// «похожий по имени» запрещено (NODE_LINK.md §6 правило 3), а висячую ссылку
// разбирает сборка fail-closed. Ничего не удаляется.
package state

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// NodeLinkFinalTag — финальный тег члена контейнера, каким его выпустит сборка
// БЕЗ суффикса глобальной уникализации: `norm(prefix + сырой тег + postfix)`,
// та же нормализация, что у эмиссионной тег-машины
// (core/config/canonical_emit.go, applyEmissionTagMachine).
//
// ok=false — политика несёт переменные (`{$num}`, `{$label}` …): их раскрывает
// только сборка, и угадывать финальный тег здесь нельзя — такой контейнер
// кандидатов не даёт.
func NodeLinkFinalTag(policy *TagPolicy, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if policy != nil && (strings.Contains(policy.Prefix, "{$") || strings.Contains(policy.Postfix, "{$")) {
		return "", false
	}
	final := textnorm.NormalizeProxyDisplay(strings.TrimSpace(policy.FinalTag(raw)))
	if final == "" {
		return "", false
	}
	return final, true
}

// NodeLinkFinalIndex — финальные теги членов контейнеров → их адреса.
//
// Кандидаты — все собираемые узлы папок и подписок с id, ВКЛЮЧАЯ
// выключенные: выключенный узел слот финального тега потребляет, и ссылка на
// него — ссылка на него, а не на соседа. Неразобранная запись целью ссылки не
// бывает. skip (может быть nil) отсеивает адреса, которые вызывающий знает как
// чужие (импорт: член, переименованный слиянием).
func NodeLinkFinalIndex(sources []Source, skip func(NodeLink) bool) map[string][]NodeLink {
	out := map[string][]NodeLink{}
	for i := range sources {
		src := &sources[i]
		if !isLinkContainer(src) {
			continue
		}
		for j := range src.Nodes {
			n := &src.Nodes[j]
			if n.IsUnsupported() {
				continue
			}
			final, ok := NodeLinkFinalTag(src.TagPolicy, n.Tag)
			if !ok {
				continue
			}
			here := NodeLink{FolderID: src.ID, Tag: n.Tag}
			if skip != nil && skip(here) {
				continue
			}
			out[final] = append(out[final], here)
		}
	}
	return out
}

// NormalizeNodeLinks поднимает dev-формы ссылок до нормы (правила — в шапке
// файла). directions — Направления состояния: их использует правило подъёма
// ссылок на узлы, записанных корневым именем.
//
// Возвращает число поднятых ссылок.
func NormalizeNodeLinks(sources []Source, directions []configtypes.Direction) int {
	nz := newLinkNormalizer(sources, directions)
	if nz == nil {
		return 0
	}
	nz.run()
	if nz.lifted > 0 {
		debuglog.InfoLog("nodelink: %d stored link(s) lifted to the {folder_id, tag} norm", nz.lifted)
	}
	if len(nz.ambiguous) > 0 {
		debuglog.InfoLog("nodelink: %d link(s) left as is — no single candidate: %s",
			len(nz.ambiguous), strings.Join(nz.ambiguous, "; "))
	}
	return nz.lifted
}

// linkNormalizer — один проход нормализации по набору источников.
type linkNormalizer struct {
	sources []Source
	// byID — контейнеры (папки и подписки) по id.
	byID map[string]*Source
	// final — индекс финальных тегов членов (NodeLinkFinalIndex).
	final map[string][]NodeLink

	lifted    int
	ambiguous []string
}

func newLinkNormalizer(sources []Source, directions []configtypes.Direction) *linkNormalizer {
	if len(sources) == 0 {
		return nil
	}
	nz := &linkNormalizer{
		sources: sources,
		byID:    make(map[string]*Source, len(sources)),
	}
	for i := range sources {
		src := &sources[i]
		if !isLinkContainer(src) {
			continue
		}
		if _, dup := nz.byID[src.ID]; !dup {
			nz.byID[src.ID] = src
		}
	}
	nz.final = NodeLinkFinalIndex(sources, nil)
	return nz
}

func (nz *linkNormalizer) run() {
	for i := range nz.sources {
		src := &nz.sources[i]
		nz.node(&src.Node, "")
		if !isLinkContainer(src) {
			continue
		}
		for j := range src.Nodes {
			nz.node(&src.Nodes[j], src.ID)
		}
	}
}

// node — ссылки одного узла (или собственный detour контейнера). space —
// контейнер, в котором лежит узел; "" у корневой записи.
func (nz *linkNormalizer) node(n *Node, space string) {
	if n.Detour != nil {
		nz.pairToGroup(n.Detour)
	}
	for i := range n.Hops {
		nz.pairToGroup(&n.Hops[i])
	}
	if n.Group != nil && space != "" {
		for i := range n.Group.Members {
			nz.memberInContainer(&n.Group.Members[i], space)
		}
	}
}

// memberInContainer — S1: член группы внутри контейнера без folder_id
// адресует свой контейнер.
func (nz *linkNormalizer) memberInContainer(link *NodeLink, space string) {
	if strings.TrimSpace(link.FolderID) != "" || strings.TrimSpace(link.Tag) == "" {
		return
	}
	link.FolderID = space
	nz.lifted++
}

// pairToGroup — S3: пара на провайдерскую группу, записанная финальным тегом.
func (nz *linkNormalizer) pairToGroup(link *NodeLink) {
	folderID := strings.TrimSpace(link.FolderID)
	tag := strings.TrimSpace(link.Tag)
	if folderID == "" || tag == "" {
		return
	}
	container := nz.byID[folderID]
	if container == nil || !hasAutoNode(container) {
		return
	}
	for j := range container.Nodes {
		if container.Nodes[j].Tag == link.Tag {
			return // сырой тег узла — ссылка в норме
		}
	}
	var hits []NodeLink
	for _, h := range nz.final[tag] {
		if h.FolderID == folderID {
			hits = append(hits, h)
		}
	}
	if len(hits) == 1 {
		if n := containerNode(container, hits[0].Tag); n != nil && n.Kind == SourceKindAuto {
			link.FolderID = folderID
			link.Tag = hits[0].Tag
			nz.lifted++
			return
		}
	}
	nz.ambiguous = append(nz.ambiguous, fmt.Sprintf("{%s, %q}", folderID, link.Tag))
}

// isLinkContainer — источник адресуется как контейнер ссылок: папка или
// подписка с id.
func isLinkContainer(src *Source) bool {
	if src == nil || strings.TrimSpace(src.ID) == "" {
		return false
	}
	return src.Kind == SourceKindFolder || src.Kind == SourceKindSubscription
}

// hasAutoNode — в контейнере есть провайдерская группа.
func hasAutoNode(src *Source) bool {
	for i := range src.Nodes {
		if src.Nodes[i].Kind == SourceKindAuto {
			return true
		}
	}
	return false
}

// containerNode — узел контейнера по сырому тегу.
func containerNode(src *Source, tag string) *Node {
	for i := range src.Nodes {
		if src.Nodes[i].Tag == tag {
			return &src.Nodes[i]
		}
	}
	return nil
}
