// Package stateedit — правка состояния по образу Конфигуратора: добавить и
// удалить сервер, правило, DNS-сервер и DNS-правило (SPEC 160).
//
// Операции работают со state.State, а не с моделью визарда: их зовёт Debug
// API, которому ui/* недоступен (цикл импортов), а правка состояния обязана
// вести себя так же, как одноимённая кнопка Конфигуратора. Разбор ввода,
// уникализация тегов и чистка ссылок здесь повторяют ветки UI
// (ui/configurator/business: parseSourceInput, AppendURLsToSources,
// AppendNodesToFolder, editNodeLinks, editRootNameRefs) поверх тех же
// core-функций разбора — второго разбора ссылок здесь нет.
//
// Пакет не сохраняет состояние и не пересобирает конфиг: загрузку, нормы
// записи, сохранение и пересборку делает вызывающий под своим мьютексом.
// Отказы — значения трёх видов: *FieldError (семантика запроса, с полем),
// *AmbiguousError (селектор совпал с несколькими записями) и ErrNotFound
// (цели нет; проверяется errors.Is).
package stateedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// ErrNotFound — цели операции нет (узла, папки, правила, DNS-записи).
var ErrNotFound = errors.New("not found")

// FieldError — запрос разобран, но по смыслу неприменим: Field называет
// поле запроса, к которому относится отказ.
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string { return e.Msg }

func fieldErr(field, format string, args ...interface{}) error {
	return &FieldError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

// AmbiguousError — селектор удаления совпал с несколькими записями; Nums —
// их номера на оси, по которым удаление можно повторить однозначно.
type AmbiguousError struct {
	Msg  string
	Nums []int
}

func (e *AmbiguousError) Error() string { return e.Msg }

// Причины пропуска части ввода, у которых нет текста ошибки разбора.
const (
	// SkipSubscriptionURL — строка — URL подписки: подписка контейнер, а не
	// узел, и через добавление сервера не заводится.
	SkipSubscriptionURL = "subscription_url"
	// SkipDuplicate — такая ссылка уже лежит корневым сервером.
	SkipDuplicate = "duplicate"
	// SkipUnrecognized — строка не ссылка, не подписка, не блок `.conf` и не
	// sing-box JSON.
	SkipUnrecognized = "unrecognized: not a share link, a .conf block or sing-box JSON"
)

// SkippedInput — часть ввода, не ставшая узлом. Сама строка в отчёт не
// попадает (ссылки несут секреты, а отчёт уходит в лог) — только номер строки
// ввода, с которой начинается запись (1 — первая; 0 — запись из JSON-документа),
// и имя узла, если оно известно.
type SkippedInput struct {
	Line   int    `json:"line"`
	Tag    string `json:"tag,omitempty"`
	Reason string `json:"reason"`
}

// AddServersRequest — что добавить и куда.
type AddServersRequest struct {
	// Input — то же, что принимает поле добавления источника Конфигуратора:
	// ссылки по строкам, блоки `.conf` WireGuard/AWG, `vpn://`, sing-box
	// JSON (один outbound, массив или документ с `outbounds`).
	Input string
	// Folder — ULID или имя папки (kind=folder); пусто — корень.
	Folder string
	// Tag — тег узла; допустим, только если из Input получился ровно один узел.
	Tag string
}

// AddedServer — легший узел.
type AddedServer struct {
	Tag      string   `json:"tag"`
	FolderID string   `json:"folder_id,omitempty"`
	Type     string   `json:"type"`
	Warnings []string `json:"warnings"`
}

// AddServersResult — исход добавления: легшие узлы и пропущенные части ввода.
type AddServersResult struct {
	Added   []AddedServer
	Skipped []SkippedInput
}

// parsedServer — узел, разобранный из ввода, с местом во вводе.
type parsedServer struct {
	node state.Node
	line int
}

// AddServers разбирает ввод и кладёт узлы в корень или в папку.
//
// В корне тег уникализируется против всего корневого пространства (как
// Конфигуратор: узлы, Направления, свёртки, системные теги шаблона), а ссылка,
// уже лежащая корневым сервером (Origin.Raw у origin uri), пропускается с
// причиной SkipDuplicate. В папке тег уникализируется против её состава,
// дублей по ссылке там не ищут (как AppendNodesToFolder).
func AddServers(st *state.State, td *template.TemplateData, req AddServersRequest) (*AddServersResult, error) {
	input := strings.TrimSpace(req.Input)
	if input == "" {
		return nil, fieldErr("input", "input is empty")
	}
	folderIdx := -1
	if strings.TrimSpace(req.Folder) != "" {
		idx, err := findFolder(st, req.Folder)
		if err != nil {
			return nil, err
		}
		folderIdx = idx
	}

	fallback := len(st.Sources)
	if folderIdx >= 0 {
		fallback = len(st.Sources[folderIdx].Nodes)
	}
	parsed, skipped, err := parseServerInput(input, fallback)
	if err != nil {
		return nil, err
	}
	if tag := strings.TrimSpace(req.Tag); tag != "" {
		if len(parsed) != 1 {
			return nil, fieldErr("tag", "tag applies only when the input yields exactly one node (got %d)", len(parsed))
		}
		parsed[0].node.Tag = tag
	}

	res := &AddServersResult{Skipped: skipped, Added: []AddedServer{}}
	if folderIdx >= 0 {
		folder := &st.Sources[folderIdx]
		taken := make(map[string]bool, len(folder.Nodes)+len(parsed))
		for i := range folder.Nodes {
			taken[folder.Nodes[i].Tag] = true
		}
		for _, p := range parsed {
			n := p.node
			n.Tag = state.UniqueTag(taken, n.Tag)
			taken[n.Tag] = true
			folder.Nodes = append(folder.Nodes, n)
			res.Added = append(res.Added, addedView(n, folder.ID))
		}
		return res, nil
	}

	taken := KnownRuleTargets(st, td)
	existing := map[string]bool{}
	for i := range st.Sources {
		src := &st.Sources[i]
		if src.Kind == state.SourceKindServer && src.Origin != nil && src.Origin.Kind == state.OriginKindURI && src.Origin.Raw != "" {
			existing[src.Origin.Raw] = true
		}
	}
	for _, p := range parsed {
		n := p.node
		if n.Origin != nil && n.Origin.Kind == state.OriginKindURI {
			if existing[n.Origin.Raw] {
				res.Skipped = append(res.Skipped, SkippedInput{Line: p.line, Tag: n.Tag, Reason: SkipDuplicate})
				continue
			}
			existing[n.Origin.Raw] = true
		}
		n.Tag = state.UniqueTag(taken, n.Tag)
		taken[n.Tag] = true
		st.Sources = append(st.Sources, state.Source{Node: n, ID: state.MakeULID()})
		res.Added = append(res.Added, addedView(n, ""))
	}
	return res, nil
}

func addedView(n state.Node, folderID string) AddedServer {
	warns := make([]string, 0, len(n.Warnings))
	for _, w := range n.Warnings {
		warns = append(warns, w.Code)
	}
	return AddedServer{Tag: n.Tag, FolderID: folderID, Type: bodyType(n.Body), Warnings: warns}
}

// findFolder — индекс папки по ULID, затем по имени. Подписка не папка: её
// состав ведёт провайдер, и узел, положенный руками, снял бы первый же fetch.
func findFolder(st *state.State, ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	idx := -1
	for i := range st.Sources {
		if strings.TrimSpace(st.Sources[i].ID) == ref {
			idx = i
			break
		}
	}
	if idx < 0 {
		for i := range st.Sources {
			k := st.Sources[i].Kind
			if (k == state.SourceKindFolder || k == state.SourceKindSubscription) && strings.TrimSpace(st.Sources[i].Name) == ref {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return -1, fmt.Errorf("folder %q: %w", ref, ErrNotFound)
	}
	switch st.Sources[idx].Kind {
	case state.SourceKindFolder:
		return idx, nil
	case state.SourceKindSubscription:
		return -1, fieldErr("folder", "%q is a subscription: its nodes are managed by the provider", ref)
	}
	return -1, fieldErr("folder", "%q is not a folder (kind=%s)", ref, st.Sources[idx].Kind)
}

// parseServerInput — текст → узлы, ветками parseSourceInput Конфигуратора:
// sing-box JSON выкусывается до построчного классификатора, блоки `.conf`
// — до цикла по строкам, `vpn://` разворачивается во все узлы профиля.
// Ни одна часть ввода не пропадает молча: подписки, нераспознанные строки и
// ошибки разбора уходят в skipped. Безымянный узел получает `server-N` от
// fallbackIndex, как в UI. Записи хранения («Copy JSON» источника) не
// принимаются: это вставка папки, а не сервера.
func parseServerInput(input string, fallbackIndex int) ([]parsedServer, []SkippedInput, error) {
	next := fallbackIndex
	nameOr := func(tag string) string {
		if tag = strings.TrimSpace(tag); tag != "" {
			return tag
		}
		next++
		return fmt.Sprintf("server-%d", next)
	}
	var (
		out     []parsedServer
		skipped = []SkippedInput{}
	)
	add := func(line int, tag string, mat *config.ServerNodeMaterial) {
		out = append(out, parsedServer{line: line, node: state.Node{
			Kind:     state.SourceKindServer,
			Enabled:  true,
			Tag:      tag,
			Body:     mat.Body,
			Origin:   &state.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw},
			Warnings: mat.Warnings,
		}})
	}

	jsonNodes, unsupported, isJSON, err := carveSingboxJSON(input)
	if err != nil {
		return nil, nil, fieldErr("input", "sing-box JSON: %s", err.Error())
	}
	if isJSON {
		for _, jn := range jsonNodes {
			tag := nameOr(jn.tag)
			mat, merr := config.MaterializeServerNode("", jn.body)
			if merr != nil {
				skipped = append(skipped, SkippedInput{Tag: tag, Reason: merr.Error()})
				continue
			}
			add(0, tag, mat)
		}
		for _, t := range unsupported {
			skipped = append(skipped, SkippedInput{Reason: "unsupported outbound type: " + t})
		}
		return out, skipped, nil
	}

	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	// Блоки `.conf` режет ExtractWGConfBlocks; номер строки начала блока
	// находится тем же признаком (строка `[Interface]`), блоки идут подряд до
	// конца ввода, поэтому k-й найденный признак — начало k-го блока.
	_, blocks := subscription.ExtractWGConfBlocks(input)
	var blockStarts []int
	for i, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), "[interface]") {
			blockStarts = append(blockStarts, i+1)
		}
	}
	restEnd := len(lines)
	if len(blockStarts) > 0 {
		restEnd = blockStarts[0] - 1
	}

	for i := 0; i < restEnd; i++ {
		lineNo := i + 1
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			continue
		case subscription.IsSubscriptionURL(line):
			skipped = append(skipped, SkippedInput{Line: lineNo, Reason: SkipSubscriptionURL})
		case subscription.IsAmneziaVPNLink(line):
			mats, verr := config.MaterializeVPNLinkNodes(line)
			if verr != nil {
				skipped = append(skipped, SkippedInput{Line: lineNo, Reason: verr.Error()})
				continue
			}
			for k := range mats {
				add(lineNo, nameOr(mats[k].Tag), &mats[k].ServerNodeMaterial)
			}
		case subscription.IsDirectLink(line):
			tag := uriFragment(line)
			mat, merr := config.MaterializeServerNode(line, nil)
			if merr != nil {
				skipped = append(skipped, SkippedInput{Line: lineNo, Tag: tag, Reason: merr.Error()})
				continue
			}
			add(lineNo, nameOr(tag), mat)
		default:
			skipped = append(skipped, SkippedInput{Line: lineNo, Reason: SkipUnrecognized})
		}
	}

	for k, block := range blocks {
		lineNo := 0
		if k < len(blockStarts) {
			lineNo = blockStarts[k]
		}
		uri, cerr := subscription.ConvertWGConfText(block)
		if cerr != nil {
			skipped = append(skipped, SkippedInput{Line: lineNo, Reason: cerr.Error()})
			continue
		}
		// Тег — из выведенной ссылки (метку считает секция реестра),
		// материал — сам блок: происхождением узла становится текст
		// провайдера со всеми комментариями (SPEC 119).
		tag := uriFragment(uri)
		mat, merr := config.MaterializeServerNode(block, nil)
		if merr != nil {
			skipped = append(skipped, SkippedInput{Line: lineNo, Tag: tag, Reason: merr.Error()})
			continue
		}
		add(lineNo, nameOr(tag), mat)
	}
	return out, skipped, nil
}

// jsonNode — узел, вынутый из вставленного sing-box JSON.
type jsonNode struct {
	tag  string
	body json.RawMessage
}

// carveSingboxJSON — ветка sing-box JSON (как carveSingboxJSON UI): isJSON
// отделяет «не JSON» (уходит построчному классификатору) от «JSON, но
// битый» (ошибка ввода). unsupported — типы outbound'ов, которые разбор тела
// не взял: они называются в отчёте, а не пропадают.
func carveSingboxJSON(input string) (nodes []jsonNode, unsupported []string, isJSON bool, err error) {
	if input == "" || (input[0] != '{' && input[0] != '[') {
		return nil, nil, false, nil
	}
	kind := subscription.ClassifySubscriptionBody(input)
	switch kind {
	case subscription.BodyKindSingboxOutbound:
		// Одиночный outbound — через NodeFromManualConfigJSON: он сохраняет
		// неизвестные типы как есть, ровно как ручная вставка в UI. Compact,
		// а не Unmarshal→Marshal: порядок полей автора сохраняется.
		node, perr := subscription.NodeFromManualConfigJSON([]byte(input))
		if perr != nil {
			return nil, nil, true, perr
		}
		var buf bytes.Buffer
		if cerr := json.Compact(&buf, []byte(input)); cerr != nil {
			return nil, nil, true, cerr
		}
		return []jsonNode{{tag: node.Tag, body: buf.Bytes()}}, nil, true, nil

	case subscription.BodyKindSingboxOutboundArray,
		subscription.BodyKindSingboxConfig,
		subscription.BodyKindSingboxConfigArray:
		res, perr := subscription.ParseSingboxBody(input, kind, nil)
		if perr != nil {
			return nil, nil, true, perr
		}
		for _, n := range res.Nodes {
			if n == nil || len(n.Outbound) == 0 {
				continue
			}
			raw, merr := json.Marshal(n.Outbound)
			if merr != nil {
				return nil, nil, true, merr
			}
			nodes = append(nodes, jsonNode{tag: n.Tag, body: raw})
		}
		if len(nodes) == 0 && len(res.UnsupportedTypes) == 0 {
			return nil, nil, true, fmt.Errorf("no outbounds found")
		}
		return nodes, res.UnsupportedTypes, true, nil
	}
	return nil, nil, false, nil
}

// uriFragment — `vless://…#name` → "name" (percent-decoded); без `#` — "".
func uriFragment(s string) string {
	at := strings.Index(s, "#")
	if at < 0 {
		return ""
	}
	frag := s[at+1:]
	if dec, err := url.QueryUnescape(frag); err == nil {
		return strings.TrimSpace(dec)
	}
	return strings.TrimSpace(frag)
}

// bodyType — поле `type` тела sing-box; "" у тела без него.
func bodyType(body json.RawMessage) string {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Type
}

// ServerView — строка списка серверов: корневой узел или член папки.
type ServerView struct {
	Tag      string `json:"tag"`
	Kind     string `json:"kind"`
	Type     string `json:"type,omitempty"`
	Enabled  bool   `json:"enabled"`
	FolderID string `json:"folder_id,omitempty"`
	Folder   string `json:"folder,omitempty"`
}

// ListServers — корневые серверы и члены папок (kind=folder) в порядке
// состояния: то, что адресует DeleteServer.
func ListServers(st *state.State) []ServerView {
	out := []ServerView{}
	for i := range st.Sources {
		src := &st.Sources[i]
		switch src.Kind {
		case state.SourceKindServer:
			out = append(out, ServerView{Tag: src.Tag, Kind: string(src.Kind), Type: bodyType(src.Body), Enabled: src.Enabled})
		case state.SourceKindFolder:
			for k := range src.Nodes {
				n := &src.Nodes[k]
				out = append(out, ServerView{
					Tag: n.Tag, Kind: string(n.Kind), Type: bodyType(n.Body), Enabled: n.Enabled,
					FolderID: src.ID, Folder: src.Name,
				})
			}
		}
	}
	return out
}

// DeleteServerResult — исход удаления узла.
type DeleteServerResult struct {
	Tag      string `json:"tag"`
	FolderID string `json:"folder_id,omitempty"`
	// LinksRemoved — где сняты NodeLink на узел (detour, хопы цепочек, члены
	// и умолчания групп).
	LinksRemoved []string `json:"links_removed"`
	// DirectionsUpdated — Направления, из `include` которых ушёл тег.
	DirectionsUpdated []string `json:"directions_updated"`
	// Dangling — ссылки на тег по имени, оставленные как есть: цели правил,
	// переменные, detour DNS-серверов, умолчания Направлений. Новую цель
	// выбирает пользователь (как в Конфигураторе: сообщает, не чистит).
	Dangling []string `json:"dangling"`
	// TailscaleStateDir — имя каталога состояния удалённого tailnet-узла;
	// "" — узел не tailnet или каталог задан телом. Сносит его вызывающий:
	// корень каталогов — локальная машина, и у профиля удалённой машины
	// сносить здесь нечего.
	TailscaleStateDir string `json:"-"`
}

// DeleteServer удаляет корневой сервер (folder пуст) или член папки и
// снимает ссылки на него. Тег есть и в корне, и в папке, а folder пуст —
// удаляется корневой.
func DeleteServer(st *state.State, tag, folder string) (*DeleteServerResult, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, fieldErr("tag", "tag is required")
	}
	res := &DeleteServerResult{Tag: tag, LinksRemoved: []string{}, DirectionsUpdated: []string{}, Dangling: []string{}}
	var (
		removed state.Node
		target  state.NodeLink
		policy  *state.TagPolicy
	)
	if strings.TrimSpace(folder) == "" {
		idx := -1
		for i := range st.Sources {
			if st.Sources[i].Kind == state.SourceKindServer && st.Sources[i].Tag == tag {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("server %q in root: %w", tag, ErrNotFound)
		}
		removed = st.Sources[idx].Node
		st.Sources = append(st.Sources[:idx], st.Sources[idx+1:]...)
		target = state.NodeLink{Tag: tag}
	} else {
		fi, err := findFolder(st, folder)
		if err != nil {
			return nil, err
		}
		f := &st.Sources[fi]
		k := -1
		for i := range f.Nodes {
			if f.Nodes[i].Tag == tag {
				k = i
				break
			}
		}
		if k < 0 {
			return nil, fmt.Errorf("node %q in folder %q: %w", tag, folder, ErrNotFound)
		}
		removed = f.Nodes[k]
		f.Nodes = append(f.Nodes[:k], f.Nodes[k+1:]...)
		target = state.NodeLink{FolderID: strings.TrimSpace(f.ID), Tag: tag}
		res.FolderID = target.FolderID
		policy = f.TagPolicy
	}

	res.LinksRemoved = append(res.LinksRemoved, dropNodeLinks(st, target)...)
	if target.FolderID == "" {
		// Корневое имя: строки `include` Направлений гаснут, одиночные ссылки
		// по имени только называются.
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
				res.DirectionsUpdated = append(res.DirectionsUpdated, d.Tag)
			}
		}
		res.Dangling = append(res.Dangling, nameRefs(st, tag)...)
	}
	if isTailscaleNode(&removed) {
		res.TailscaleStateDir = config.TailscaleStateDirName(policy.FinalTag(tag))
	}
	return res, nil
}

// isTailscaleNode — tailnet-узел, чей каталог состояния лаунчер ведёт сам
// (тело без явного `state_directory`).
func isTailscaleNode(n *state.Node) bool {
	if n.Kind != state.SourceKindServer || len(n.Body) == 0 {
		return false
	}
	var body map[string]interface{}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return false
	}
	t, _ := body["type"].(string)
	if strings.ToLower(strings.TrimSpace(t)) != config.SchemeTailscale {
		return false
	}
	_, explicit := body["state_directory"]
	return !explicit
}

// dropNodeLinks снимает все NodeLink, адресующие target: detour узлов и
// папок, хопы цепочек, члены и умолчания групп — в корне и в составе
// контейнеров (реестр ссылок editNodeLinks Конфигуратора). Ссылка без
// folder_id адресует корень у detour и хопа, а у члена группы внутри
// контейнера — сам контейнер. Возвращает места снятых ссылок.
func dropNodeLinks(st *state.State, target state.NodeLink) []string {
	var where []string
	addresses := func(l state.NodeLink, space string) bool {
		fid := strings.TrimSpace(l.FolderID)
		if fid == "" {
			fid = space
		}
		return fid == target.FolderID && l.Tag == target.Tag
	}
	dropList := func(links []state.NodeLink, space string) ([]state.NodeLink, bool) {
		kept := links[:0:0]
		for _, l := range links {
			if !addresses(l, space) {
				kept = append(kept, l)
			}
		}
		return kept, len(kept) != len(links)
	}
	visit := func(n *state.Node, space, holder string) {
		if n.Detour != nil && addresses(*n.Detour, "") {
			n.Detour = nil
			where = append(where, holder+".detour")
		}
		if hops, hit := dropList(n.Hops, ""); hit {
			n.Hops = hops
			where = append(where, holder+".hops")
		}
		if g := n.Group; g != nil {
			if members, hit := dropList(g.Members, space); hit {
				g.Members = members
				where = append(where, holder+".group.members")
			}
			if g.Default != nil && addresses(*g.Default, space) {
				g.Default = nil
				where = append(where, holder+".group.default")
			}
		}
	}
	for i := range st.Sources {
		s := &st.Sources[i]
		name := s.Tag
		if s.Kind == state.SourceKindFolder || s.Kind == state.SourceKindSubscription {
			name = s.Name
		}
		visit(&s.Node, "", fmt.Sprintf("source %q", name))
		space := strings.TrimSpace(s.ID)
		for k := range s.Nodes {
			visit(&s.Nodes[k], space, fmt.Sprintf("source %q node %q", name, s.Nodes[k].Tag))
		}
	}
	return where
}

// nameRefs — ссылки на корневое имя строкой, которые удаление не правит:
// цели правил и переменные пресетов, переменные состояния (route_final и
// прочие), detour и переменные DNS-серверов, умолчание селектора Направлений.
func nameRefs(st *state.State, tag string) []string {
	var out []string
	for i := range st.Rules {
		r := &st.Rules[i]
		num := 0
		if r.Num != nil {
			num = *r.Num
		}
		switch r.Kind {
		case state.RuleKindInline, state.RuleKindSrs:
			if body, err := r.BodyMap(); err == nil {
				if o, _ := body["outbound"].(string); o == tag {
					out = append(out, fmt.Sprintf("rule %q (num %d) outbound", r.Name, num))
				}
			}
		case state.RuleKindPreset:
			for _, k := range sortedKeys(r.Vars) {
				if r.Vars[k] == tag {
					out = append(out, fmt.Sprintf("rule preset %q (num %d) vars.%s", r.Ref, num, k))
				}
			}
		}
	}
	for _, v := range st.Vars {
		if v.Value == tag {
			out = append(out, "vars."+v.Name)
		}
	}
	for i := range st.DNS.Servers {
		srv := &st.DNS.Servers[i]
		name := srv.Tag
		if name == "" {
			name = srv.Ref
		}
		if d, _ := srv.Body["detour"].(string); d == tag {
			out = append(out, fmt.Sprintf("dns server %q detour", name))
		}
		for _, k := range sortedKeys(srv.Vars) {
			if srv.Vars[k] == tag {
				out = append(out, fmt.Sprintf("dns server %q vars.%s", name, k))
			}
		}
	}
	for i := range st.Directions {
		d := &st.Directions[i]
		if def, _ := d.Options["default"].(string); def == tag {
			out = append(out, fmt.Sprintf("direction %q options.default", d.Tag))
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
