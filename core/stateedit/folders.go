package stateedit

// Папки (kind=folder) — контейнеры собственных узлов пользователя: завести
// пустую, как «Add folder» Конфигуратора, и удалить. Подписка здесь не
// папка: её состав ведёт провайдер, и заводится и удаляется она не тут.

import (
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
)

// FolderView — строка списка папок.
type FolderView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Nodes — число узлов в папке.
	Nodes int `json:"nodes"`
	// Replace — тег свёртки папки; "" — папка не свёрнута.
	Replace string `json:"replace,omitempty"`
}

func folderView(src *state.Source) FolderView {
	v := FolderView{ID: src.ID, Name: src.Name, Enabled: src.Enabled, Nodes: len(src.Nodes)}
	if src.Replace != nil {
		v.Replace = strings.TrimSpace(src.Replace.Tag)
	}
	return v
}

// ListFolders — папки в порядке состояния, включая пустые (их нет в
// ListServers: там строка — узел).
func ListFolders(st *state.State) []FolderView {
	out := []FolderView{}
	for i := range st.Sources {
		if st.Sources[i].Kind == state.SourceKindFolder {
			out = append(out, folderView(&st.Sources[i]))
		}
	}
	return out
}

// AddFolder заводит пустую папку в конце списка источников — конструктором
// state.NewFolderSource, как «Add folder» Конфигуратора. Имя обязательно и
// не должно совпадать с именем другой папки или подписки: Конфигуратор
// дубли терпит (имя — подпись), но API адресует папку и по имени, и второе
// такое же имя сделало бы адрес `folder=<имя>` неоднозначным.
func AddFolder(st *state.State, name string, enabled bool) (FolderView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FolderView{}, fieldErr("name", "name is required")
	}
	for i := range st.Sources {
		k := st.Sources[i].Kind
		if (k == state.SourceKindFolder || k == state.SourceKindSubscription) && strings.TrimSpace(st.Sources[i].Name) == name {
			return FolderView{}, fieldErr("name", "%q already names a %s", name, k)
		}
	}
	folder := state.NewFolderSource(name)
	folder.Enabled = enabled
	st.Sources = append(st.Sources, folder)
	return folderView(&st.Sources[len(st.Sources)-1]), nil
}

// DeleteFolderResult — исход удаления папки.
type DeleteFolderResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// NodesRemoved — сырые теги узлов, ушедших вместе с папкой.
	NodesRemoved []string `json:"nodes_removed"`
	// LinksRemoved — где сняты NodeLink на узлы папки и на тег её свёртки.
	LinksRemoved []string `json:"links_removed"`
	// DirectionsUpdated — Направления, из `include` которых ушёл тег свёртки.
	DirectionsUpdated []string `json:"directions_updated"`
	// Dangling — ссылки на тег свёртки по имени, оставленные как есть (как у
	// DeleteServer корневого узла).
	Dangling []string `json:"dangling"`
	// TailscaleStateDirs — каталоги состояния tailnet-узлов папки; сносит их
	// вызывающий и только у локального состояния (см. DeleteServerResult).
	TailscaleStateDirs []string `json:"-"`
}

// DeleteFolder удаляет папку. Непустая удаляется только при withNodes —
// узлы уходят вместе с ней («Delete with nodes» Конфигуратора), и ссылки на
// них снимаются так же, как при DeleteServer каждого. Тег свёртки — корневое
// имя: NodeLink и строки `include` на него гаснут, ссылки по имени
// называются.
func DeleteFolder(st *state.State, ref string, withNodes bool) (*DeleteFolderResult, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, fieldErr("folder", "folder is required")
	}
	idx, err := findFolder(st, ref)
	if err != nil {
		return nil, err
	}
	folder := st.Sources[idx]
	if n := len(folder.Nodes); n > 0 && !withNodes {
		return nil, fieldErr("with_nodes", "folder %q holds %d node(s): pass with_nodes=1 to delete them together with the folder", ref, n)
	}
	st.Sources = append(st.Sources[:idx], st.Sources[idx+1:]...)

	res := &DeleteFolderResult{
		ID: folder.ID, Name: folder.Name,
		NodesRemoved: []string{}, LinksRemoved: []string{}, DirectionsUpdated: []string{}, Dangling: []string{},
	}
	fid := strings.TrimSpace(folder.ID)
	for i := range folder.Nodes {
		n := &folder.Nodes[i]
		res.NodesRemoved = append(res.NodesRemoved, n.Tag)
		res.LinksRemoved = append(res.LinksRemoved, dropNodeLinks(st, state.NodeLink{FolderID: fid, Tag: n.Tag})...)
		if isTailscaleNode(n) {
			res.TailscaleStateDirs = append(res.TailscaleStateDirs, config.TailscaleStateDirName(folder.TagPolicy.FinalTag(n.Tag)))
		}
	}
	if folder.Replace != nil {
		if tag := strings.TrimSpace(folder.Replace.Tag); tag != "" {
			for _, name := range []string{tag, tag + "-auto"} {
				links, dirs, dangling := dropRootName(st, name)
				res.LinksRemoved = append(res.LinksRemoved, links...)
				res.DirectionsUpdated = append(res.DirectionsUpdated, dirs...)
				res.Dangling = append(res.Dangling, dangling...)
			}
		}
	}
	return res, nil
}

// findContainer — индекс папки или подписки по ULID, затем по имени: цель
// detour может лежать и в подписке, в отличие от места для нового узла.
func findContainer(st *state.State, ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	for i := range st.Sources {
		if strings.TrimSpace(st.Sources[i].ID) == ref {
			if k := st.Sources[i].Kind; k == state.SourceKindFolder || k == state.SourceKindSubscription {
				return i, nil
			}
			return -1, fieldErr("detour.folder", "%q is not a folder or subscription (kind=%s)", ref, st.Sources[i].Kind)
		}
	}
	for i := range st.Sources {
		k := st.Sources[i].Kind
		if (k == state.SourceKindFolder || k == state.SourceKindSubscription) && strings.TrimSpace(st.Sources[i].Name) == ref {
			return i, nil
		}
	}
	return -1, fieldErr("detour.folder", "no folder or subscription %q", ref)
}

// DetourRef — цель detour в запросе: корневое имя (Folder пуст) или узел
// папки либо подписки (Folder — её ULID или имя, Tag — сырой тег узла).
type DetourRef struct {
	Tag    string
	Folder string
}

// resolveDetour — ссылка модели на существующую цель. Корневое имя
// проверяется тем же составом, что цель правила (KnownRuleTargets), кроме
// тега блокировки: это не дозвон.
func resolveDetour(st *state.State, known map[string]bool, blockTag string, ref DetourRef) (*state.NodeLink, error) {
	tag := strings.TrimSpace(ref.Tag)
	if tag == "" {
		return nil, fieldErr("detour.tag", "detour.tag is required")
	}
	if strings.TrimSpace(ref.Folder) == "" {
		if tag == blockTag {
			return nil, fieldErr("detour.tag", "%q is the block outbound, not a dialer", tag)
		}
		if !known[tag] {
			return nil, fieldErr("detour.tag", "unknown detour target %q", tag)
		}
		return &state.NodeLink{Tag: tag}, nil
	}
	idx, err := findContainer(st, ref.Folder)
	if err != nil {
		return nil, err
	}
	c := &st.Sources[idx]
	for i := range c.Nodes {
		if c.Nodes[i].Tag == tag {
			return &state.NodeLink{FolderID: strings.TrimSpace(c.ID), Tag: tag}, nil
		}
	}
	return nil, fieldErr("detour.tag", "no node %q in %s %q", tag, c.Kind, c.Name)
}

// dropRootName — уход корневого имени tag: NodeLink корня на него снимаются,
// строки `include` Направлений гаснут, ссылки по имени только называются.
func dropRootName(st *state.State, tag string) (links, directions, dangling []string) {
	links = dropNodeLinks(st, state.NodeLink{Tag: tag})
	for i := range st.Directions {
		d := &st.Directions[i]
		kept := d.AddOutbounds[:0:0]
		for _, o := range d.AddOutbounds {
			if strings.TrimSpace(o) != tag {
				kept = append(kept, o)
			}
		}
		if len(kept) != len(d.AddOutbounds) {
			d.AddOutbounds = kept
			directions = append(directions, d.Tag)
		}
	}
	return links, directions, nameRefs(st, tag)
}
