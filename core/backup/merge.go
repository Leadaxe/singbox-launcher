package backup

// Слияние источников при импорте (контракт 0.12.5, D-095).
//
// Импорт бэкапа — НЕ замена состояния, а слияние по идентичности: локальное,
// чего в файле нет, остаётся жить; совпавшее по идентичности получает
// настройки файла, не теряя истории; несовпавшее дописывается. Это логика
// LxBox, и лаунчер её повторяет — у пользователя с двумя приложениями один
// файл обязан давать один и тот же итог (D-095 перекрывает D-089).
//
// Идентичность у каждого вида своя, и это не косметика:
//
//   - подписка — URL: он и есть договор с провайдером, а тег/имя у неё
//     локальные и правятся руками;
//   - одиночный сервер — ТЕЛО (uri либо config_json): тег у него локальное
//     имя, и сравнивать по тегу значило бы плодить копии одного сервера,
//     переименованного на другой машине;
//   - цепочка и Направление — тег: они ссылочные сущности, на их имя метят
//     правила, и второго владельца у имени быть не может (§4).
//
// Полной замене подлежит РОВНО ОДНА секция — rules[] (§9 п. 7): ось порядка
// правил у сторон своя, и «долить» чужие номера в неё нечем. DNS сливается
// «своё сильнее» (importDNS), warp[] добавляется, vars и route.final живут по
// своим правилам — подробности в §9 нормы.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/state"
)

// mergeCounters — что сделало слияние, по видам записей.
//
// Считаем отдельно «добавлено» и «обновлено»: пользователю после импорта надо
// понимать, дописал он себе чужое или переписал своё, а одним числом
// «применено» эти два события неразличимы.
type mergeCounters struct {
	AddedSubscriptions   int
	UpdatedSubscriptions int
	AddedServers         int
	SkippedServers       int
	AddedFolders         int
	UpdatedFolders       int
	AddedChains          int
}

// applyFolderSettings переносит собственные настройки приехавшей папки на
// совпавшую локальную, оставляя ей идентичность и состав.
//
// Что НЕ трогается: ID (на него ссылаются NodeLink членов, detour и hops
// цепочек — переписать его значило бы осиротить ссылки ЭТОЙ машины), Name
// (он и есть ключ совпадения) и Nodes (состав сливается по телу отдельно).
//
// Что применяется — ровно то, что схема 1.0 у папки объявляет: включённость,
// политика тегов, свёртка и общий detour. Замещение целиком, а не по
// непустым: снятая на другой машине политика тегов обязана доехать снятой,
// иначе «слить» превратилось бы в «только добавить», и настройку нельзя было
// бы отменить переносом.
func applyFolderSettings(dst *state.Source, src state.Source) {
	dst.Enabled = src.Enabled
	dst.TagPolicy = src.TagPolicy
	dst.Replace = src.Replace
	dst.Detour = src.Detour
}

// applySubscriptionSettings переносит настройки приехавшей подписки на
// локальную, оставляя ей идентичность и историю.
//
// Что НЕ трогается: ID (на него ссылаются NodeLink и каталоги профилей),
// Nodes (состав с отметками включения — он живёт от fetch к fetch, а в файл
// не едет вовсе), UpdateStatus и Meta (диагностика ЭТОЙ машины: чужая история
// обновлений здесь врала бы).
//
// Что применяется — ровно то, что сторона умеет и что файл несёт: имя,
// включённость, политика тегов, интервал обновления, кап, фильтры отсева,
// свёртка, detour и слепок identity.
//
// fullSettings — несёт ли формат настройки, которых в схеме 0.12 нет
// (сегодня это одна `relays_in_directions`). У 1.0 дом им есть (§6.0), и у
// совпавшей подписки значение файла замещает локальное; у 0.12 поля в файле
// нет вовсе, и применить его «ноль» значило бы снять галку пользователя
// импортом старого файла — там оно не трогается, как и раньше.
func applySubscriptionSettings(dst *state.Source, src state.Source, fullSettings bool) {
	// Имя — только НЕПУСТОЕ: `label` в схеме необязателен, и запись без него
	// значит «эта сторона имени не носит», а не «сотри своё». Пустая строка
	// затёрла бы подпись, которой пользователь называет источник в списке.
	if src.Name != "" {
		dst.Name = src.Name
	}
	dst.Enabled = src.Enabled
	dst.TagPolicy = src.TagPolicy
	dst.Update = src.Update
	dst.MaxNodes = src.MaxNodes
	dst.Skip = src.Skip
	dst.Replace = src.Replace
	dst.Detour = src.Detour
	if fullSettings {
		dst.RelaysInDirections = src.RelaysInDirections
	}

	// Identity — СЛЕПОК целиком, а не поле за полем: отсутствие объекта
	// identity в файле значит «как в системе», и оставить здесь прежний UA
	// значило бы сохранить настройку, которую пользователь на той машине
	// снял. Слепок ставится тем, что уже разобрал importSourceIdentity: при
	// отсутствии объекта поля пусты — то самое «как в системе».
	dst.Identity = src.Identity.Clone()

	mergeDisabledMarks(dst, src.PendingDisabled)
}

// mergeDisabledMarks ОБЪЕДИНЯЕТ отметки выключения: свои не перетираются,
// приехавшие доливаются.
//
// Объединение, а не замена, потому что отметка «этот узел мне не нужен» —
// решение пользователя, принятое на КОНКРЕТНОЙ машине, и файл с другой машины
// не знает, что здесь выключали. Включить обратно узел, снятый вручную,
// импорт настроек не должен.
//
// Тег, который есть среди уже загруженных узлов, применяется сразу
// (Enabled=false); остальные ждут первого достоверного fetch в
// PendingDisabled — тот же вердикт O2, что у чистого импорта.
func mergeDisabledMarks(dst *state.Source, incoming []string) {
	if len(incoming) == 0 {
		return
	}
	have := map[string]bool{}
	for _, tag := range dst.PendingDisabled {
		have[tag] = true
	}
	byTag := map[string]int{}
	for i := range dst.Nodes {
		byTag[dst.Nodes[i].Tag] = i
	}
	for _, tag := range incoming {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if at, ok := byTag[tag]; ok {
			dst.Nodes[at].Enabled = false
			continue
		}
		if !have[tag] {
			have[tag] = true
			dst.PendingDisabled = append(dst.PendingDisabled, tag)
		}
	}
	sort.Strings(dst.PendingDisabled)
}

// applyImportedSections накладывает секции файла на УЖЕ ЛЕЖАЩИЙ узел.
//
// Поля в файле нет → локальные секции остаются: молчание файла не значит
// «сотри». Поле есть → замещает целиком, включая пустой набор (пользователь
// снял секции на другой машине, и «слить» тут нечего — фрагменты не имеют
// ключа, по которому их можно было бы сопоставить поштучно).
func applyImportedSections(node *state.Node, sec *state.NodeSections, present bool) {
	if node == nil || !present {
		return
	}
	node.Sections = sec
	node.NormalizeNodeSections()
}

// folderNodeWithBody — индекс УЖЕ ЛЕЖАЩЕГО в этой папке того же узла; -1,
// если его нет. Индекс, а не bool: при совпадении на найденный узел
// накладываются секции файла (SPEC 121).
//
// Дедуп в пределах ОДНОЙ папки, а не по всему состоянию: один и тот же сервер
// в двух разных папках — законная раскладка (одна «рабочая», другая
// «запасная»), и схлопывать её импорт не вправе.
func folderNodeWithBody(folder *state.Source, node *state.Node) int {
	key := folderMemberKey(node)
	if key == "" {
		return -1
	}
	for i := range folder.Nodes {
		if folderMemberKey(&folder.Nodes[i]) == key {
			return i
		}
	}
	return -1
}

// folderMemberKey — ключ идентичности члена папки, по ВИДУ узла.
//
// У сервера идентичность — ТЕЛО (§9 п. 2): тег одиночного узла локальное имя,
// и переименованный на другой машине сервер обязан узнаться. У цепочки и
// провайдерской группы тела нет вовсе — их состав живёт в hops/group, а
// адресуются они ТЕГОМ, и в корне слияние их так и ключует (mergeChainItem по
// тегу). Внутри папки ключ обязан быть тем же: пока его не было, ключ выходил
// пустым, «сравнивать нечем» означало «не дубль», и каждый повторный импорт
// одного файла дописывал ещё одну копию (auto-eu, auto-eu-2, auto-eu-3…) —
// лишняя urltest-группа в конфиге и безграничный рост состояния.
func folderMemberKey(n *state.Node) string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case state.SourceKindChain, state.SourceKindAuto:
		tag := strings.TrimSpace(n.Tag)
		if tag == "" {
			// Безымянная ссылочная запись: адресоваться ей нечем, и отличить
			// её от второй такой же тоже. Пустой ключ = «не дубль».
			return ""
		}
		return string(n.Kind) + "\x00" + tag
	default:
		return nodeBodyKey(n)
	}
}

// nodeBodyKey — ключ дедупа одиночного узла: его ТЕЛО, без имени.
//
// Ключ строится от ORIGIN, а Body берётся ТОЛЬКО когда origin'а нет. Порядок
// здесь не вкусовой, а единственно работающий: origin — это исходник записи в
// той же форме, в какой она едет в файле (`uri` либо `config_json`), и он есть
// у обеих сторон всегда. Body же появляется у локального узла ПОСЛЕ
// материализации (сборка, fetch, правка JSON), а у только что приехавшего из
// файла uri-узла его ещё нет вовсе.
//
// Раньше первым проверялся Body — и дедуп разваливался ровно на живом
// состоянии: материализованный локальный сервер давал ключ по телу
// ("json\x00…"), приехавший с тем же `uri` — ключ по исходнику ("raw\x00…"),
// один и тот же сервер не совпадал, и каждый повторный импорт удваивал его.
// В корпусе это не ловилось, потому что там обе стороны не материализованы.
//
// Формы ключа:
//
//   - origin.kind=uri: `TrimSpace` и БЕЗ фрагмента `#имя` — у share-URI это
//     имя узла, а не часть тела;
//   - origin.kind=json и wg_ini: тело = сам текст исходника; json идёт в
//     нормальную форму (см. canonicalJSONKey), wg-quick сравнивается текстом —
//     разбирать INI ради ключа нечем, а `#` там начинает комментарий строки и
//     отрезать по нему нельзя;
//   - origin нет → Body в нормальной форме: узел, созданный руками с нуля,
//     исходника не имеет, и тело — единственное, что у него есть.
//
// Пустой ключ означает «сравнивать нечем»: такая запись никогда не считается
// дублем — молча проглотить её значило бы потерять запись, о которой
// пользователь ничего не узнает.
func nodeBodyKey(n *state.Node) string {
	if n == nil {
		return ""
	}
	if n.Origin != nil {
		raw := strings.TrimSpace(n.Origin.Raw)
		if raw == "" {
			return ""
		}
		switch n.Origin.Kind {
		case state.OriginKindURI:
			if at := strings.Index(raw, "#"); at >= 0 {
				raw = strings.TrimSpace(raw[:at])
			}
			if raw == "" {
				return ""
			}
			return "raw\x00" + raw
		case state.OriginKindJSON:
			if key := canonicalJSONKey(json.RawMessage(raw)); key != "" {
				return "json\x00" + key
			}
			// Исходник объявлен JSON'ом, но не разбирается: сравниваем
			// текстом — это лучше, чем не сравнивать вовсе.
			return "raw\x00" + raw
		default:
			// wg_ini и всякий будущий вид: тело = текст исходника как есть.
			return "raw\x00" + raw
		}
	}
	if len(n.Body) > 0 {
		if key := canonicalJSONKey(n.Body); key != "" {
			return "json\x00" + key
		}
	}
	return ""
}

// canonicalJSONKey — тело в НОРМАЛЬНОЙ ФОРМЕ: ключи объектов отсортированы
// рекурсивно, пробелов нет, текст — сырой UTF-8 без HTML- и юникод-экранирования.
//
// Форма ровно та же, что у identity-хэша (D-007, marshalCanonicalJSON в
// core/config): два приложения обязаны получить один ключ из одного тела, а
// «то же тело с переставленными ключами» — это одно тело, а не второй сервер.
// SetEscapeHTML(false) здесь не косметика: с ним `&` в пути или пароле даёт
// `&`, и то же тело, прошедшее через эмиттер с другими настройками
// экранирования, перестало бы узнаваться.
//
// Пустая строка = разобрать не удалось (тогда сравнивают сырой текст).
func canonicalJSONKey(raw json.RawMessage) string {
	var v interface{}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	// Верхнеуровневые `tag` и `detour` снимаются — ровно та же форма, что у
	// identity-хэша (D-007), и симметрично отрезанному у uri фрагменту:
	// `tag` это ИМЯ узла, а `detour` — путь дозвона к нему, и ни то, ни
	// другое телом сервера не является. Тот же сервер с другим "tag" в
	// JSON — та же запись, иначе дедуп по телу ловил бы переименование у
	// uri-узлов и пропускал у json-узлов.
	if obj, ok := v.(map[string]interface{}); ok {
		delete(obj, "tag")
		delete(obj, "detour")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(canonicalJSONValue(v)) != nil {
		return ""
	}
	// Encode дописывает "\n" — он в ключе лишний.
	return strings.TrimRight(buf.String(), "\n")
}

// canonicalJSONValue рекурсивно сортирует ключи объектов; порядок элементов
// массивов сохраняется — в alpn и allowed_ips он значим.
func canonicalJSONValue(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(x))
		for _, k := range keys {
			out[k] = canonicalJSONValue(x[k])
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i := range x {
			out[i] = canonicalJSONValue(x[i])
		}
		return out
	default:
		return v
	}
}

// takenRootTags — занятые имена КОРНЕВОГО пространства финальных тегов.
//
// Не только теги верхних узлов: в том же пространстве живут теги Направлений
// и теги замен свёрнутых папок/подписок. Узел, вставший в корень именем
// свёртки, дал бы двух владельцев одного имени, и в сборке они спорили бы за
// него — ровно та причина, по которой список у UI-стороны тоже полный
// (rootTagSet, node_move.go).
func takenRootTags(s *state.State) map[string]bool {
	taken := map[string]bool{}
	for i := range s.Sources {
		src := &s.Sources[i]
		switch src.Kind {
		case state.SourceKindServer, state.SourceKindChain, state.SourceKindAuto:
			if t := src.NodeTagOrLabel(); t != "" {
				taken[t] = true
			}
		}
		if src.Replace != nil && src.Replace.Tag != "" {
			taken[src.Replace.Tag] = true
			// Двойник режима both: `<tag>-auto` занят тем же владельцем.
			taken[src.Replace.Tag+"-auto"] = true
		}
	}
	for _, d := range s.Directions {
		if d.Tag != "" {
			taken[d.Tag] = true
		}
	}
	return taken
}

// takenSourceIDs — занятые ULID источников.
func takenSourceIDs(sources []state.Source) map[string]bool {
	out := map[string]bool{}
	for i := range sources {
		if sources[i].ID != "" {
			out[sources[i].ID] = true
		}
	}
	return out
}

// freshIDIfTaken — id из файла, а при коллизии свежий ULID (§9 п.7).
func freshIDIfTaken(id string, taken map[string]bool) string {
	if id == "" || taken[id] {
		return state.MakeULID()
	}
	return id
}

// uniqueTag подбирает свободное имя вида `X`, `X-2`, `X-3` — та же форма
// суффикса, что у ручного добавления узла (uniqueTagIn, node_move.go) и у
// уникализации на эмиссии, чтобы имена из разных путей выглядели одинаково.
func uniqueTag(taken map[string]bool, tag string) string {
	if tag == "" || !taken[tag] {
		return tag
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", tag, n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// nodeAddr — адрес узла в состоянии: индекс источника и индекс внутри его
// состава (-1 = сам источник, то есть корневой узел).
//
// Адрес, а не указатель: слияние ДОПИСЫВАЕТ в s.Sources, и срез при росте
// переезжает в памяти — указатель, взятый до append'а, указывал бы в
// освобождённый массив. Ошибка тихая: правки уходили бы в никуда, и ось
// правил узла «иногда» оставалась бы неперенумерованной.
type nodeAddr struct {
	src  int
	node int
}

func (a nodeAddr) resolve(s *state.State) *state.Node {
	if a.src < 0 || a.src >= len(s.Sources) {
		return nil
	}
	if a.node < 0 {
		return &s.Sources[a.src].Node
	}
	if a.node >= len(s.Sources[a.src].Nodes) {
		return nil
	}
	return &s.Sources[a.src].Nodes[a.node]
}

// mergedInfo — что слияние обязано рассказать вызывающему.
//
// Два факта, которых по самому состоянию уже не восстановить: какие узлы
// ПРИЕХАЛИ этим файлом (их секции участвуют в перенумерации оси) и как
// переехали id папок (по ним переписываются ссылки detour/hops формата 1.0).
type mergedInfo struct {
	// nodes — узлы состояния, которые этот импорт принёс или обновил.
	nodes []nodeAddr
	// folderIDs — карта «id папки в файле → id папки здесь». Совпавшая по
	// имени папка держит СВОЙ id, поэтому ссылки файла обязаны переехать.
	folderIDs map[string]string
	// linked — записи, чьи ссылки надо переписать по карте (detour, hops).
	linked []nodeAddr
}

func (m *mergedInfo) merge(other mergedInfo) {
	m.nodes = append(m.nodes, other.nodes...)
	m.linked = append(m.linked, other.linked...)
	if m.folderIDs == nil {
		m.folderIDs = other.folderIDs
		return
	}
	for k, v := range other.folderIDs {
		m.folderIDs[k] = v
	}
}

// sectionRules — правила, которые приехавшие узлы носят с собой.
//
// Указатели, потому что перенумерация ставит номера в тех самых записях,
// которые уже лежат в состоянии: копия здесь означала бы, что ось пересчитана
// и выброшена. Берутся они в самом конце, когда s.Sources больше не растёт.
func (m *mergedInfo) sectionRules(s *state.State) []*state.Rule {
	var out []*state.Rule
	for _, addr := range m.nodes {
		n := addr.resolve(s)
		if n == nil || n.Sections == nil {
			continue
		}
		for i := range n.Sections.Rules {
			out = append(out, &n.Sections.Rules[i])
		}
	}
	return out
}

// rewriteFolderLinks переписывает ссылки на папки по карте «id файла → id
// здесь» (формат 1.0).
//
// Ссылка, чьей папки в файле не было, остаётся КАК ЕСТЬ — ровно как у 0.12:
// импорт не выдумывает адрес, а сборка скажет о недостижимой цели сама
// (fail-closed). Молча снять ссылку было бы хуже: узел тихо пошёл бы напрямую.
func (m *mergedInfo) rewriteFolderLinks(s *state.State) {
	if len(m.folderIDs) == 0 {
		return
	}
	fix := func(link *state.NodeLink) {
		if link == nil || link.FolderID == "" {
			return
		}
		if local, ok := m.folderIDs[link.FolderID]; ok {
			link.FolderID = local
		}
	}
	for _, addr := range m.linked {
		n := addr.resolve(s)
		if n == nil {
			continue
		}
		fix(n.Detour)
		for i := range n.Hops {
			fix(&n.Hops[i])
		}
	}
}

// folderIndex — локальные папки, разложенные по обоим ключам сопоставления:
// по `id` (ULID) и по имени.
//
// Имя — норма §9 п. 3 («одно имя = одна папка»), и для файла 0.12 она
// единственно возможная: у папки там нет собственной записи, а значит и id.
// В форме 1.0 у папки id есть, и он строже имени: UI допускает ДВЕ папки с
// одним именем (это законная раскладка), а карта «имя → папка» видит из них
// только первую. Импорт собственного экспорта 1.0 в то же состояние из-за
// этого матчил вторую папку файла в ПЕРВУЮ локальную и дописывал её состав
// туда (дедуп по телу не спасал — тела разные): состояние росло на каждом
// импорте, идемпотентность ломалась.
//
// Поэтому порядок: сперва id, потом имя. Совпадение по id — это «та же самая
// папка, файл с этой машины»; совпадение по имени — «папка с тем же именем на
// ЧУЖОЙ машине», и это по-прежнему норма §9 (id совпавшей остаётся локальным,
// у новой берётся из файла). Первая победившая в обеих картах: второй ключ на
// ту же папку размножил бы настройки файла.
type folderIndex struct {
	byID   map[string]int
	byName map[string]int
	// freshByName — имена, под которыми в индексе стоит папка, ЗАВЕДЁННАЯ
	// этим же импортом из записи 1.0 с собственным id. По имени такие не
	// находятся: см. lookup.
	freshByName map[string]bool
}

func newFolderIndex() *folderIndex {
	return &folderIndex{byID: map[string]int{}, byName: map[string]int{}, freshByName: map[string]bool{}}
}

// addExisting — папка, которая была в состоянии ДО импорта. Находится обоими
// ключами.
func (f *folderIndex) addExisting(id, name string, at int) {
	f.put(id, name, at, false)
}

// addCreated — папка, заведённая ЭТИМ импортом из записи `sources[]` формата
// 1.0. Находится по своему id (повторная запись с тем же id в одном файле —
// это одна папка) и НЕ находится по имени.
func (f *folderIndex) addCreated(id, name string, at int) {
	f.put(id, name, at, true)
}

// addCreated0x — папка, заведённая этим импортом из плоского `servers[]`
// формата 0.x (`ensureFolderAt`). Находится ПО ИМЕНИ — иначе каждая
// следующая запись с тем же `folder:` заводила бы ещё одну папку: имя там
// единственный ключ, которым записи файла связаны между собой.
func (f *folderIndex) addCreated0x(id, name string, at int) {
	f.put(id, name, at, false)
}

func (f *folderIndex) put(id, name string, at int, fresh bool) {
	if id != "" {
		if _, dup := f.byID[id]; !dup {
			f.byID[id] = at
		}
	}
	if _, dup := f.byName[name]; !dup {
		f.byName[name] = at
		f.freshByName[name] = fresh
	}
}

// lookup — папка файла среди локальных: сначала по id (если он у входа
// есть), затем по имени. Пустой fileID = вход без id (0.x) — там остаётся
// ровно сегодняшнее сопоставление по имени.
//
// По имени находятся только папки, которые существовали в состоянии ДО этого
// импорта. Папка, заведённая этим же импортом из ДРУГОГО id, по имени не
// матчится — заводится ещё одна. Иначе импорт файла-близнеца в ПУСТОЕ
// состояние терял структуру самого файла: первая «Folder 1» (id A) заводилась,
// вторая (id B) по id не находилась, по имени попадала в только что
// заведённую A, и 18 источников превращались в 17 с составом обеих папок в
// первой. «Одно имя = одна папка» (§9 п. 3) — правило про СОСТОЯНИЕ
// приёмника, а не про содержимое файла: в файле папок две, и схлопывать их
// импорт не вправе.
func (f *folderIndex) lookup(fileID, name string) (int, bool) {
	if fileID != "" {
		if at, ok := f.byID[fileID]; ok {
			return at, true
		}
	}
	if f.freshByName[name] {
		return 0, false
	}
	at, ok := f.byName[name]
	return at, ok
}

// mergeSources — ОДИН проход слияния источников по §9, в порядке файла.
//
// Порядок нормативен («новые встают в конец, в порядке файла», §9 п. 8), и
// проход поэтому один на все виды записей: разложи его на «сперва подписки,
// потом серверы, потом цепочки» — и состав s.Sources после импорта зависел бы
// от вида записи, а не от файла; обратный экспорт переставлял бы записи
// местами, и круг «экспорт → импорт → экспорт» перестал бы быть тождеством.
//
// rootTags — занятые имена корневого пространства, снятые ВЫЗЫВАЮЩИМ до
// импорта Направлений (см. applyDecoded): считать их здесь значило бы
// уникализировать приехавший узел против Направления из того же файла.
func mergeSources(s *state.State, items []decodedSource, rootTags map[string]bool, warns *[]Warning, cnt *mergeCounters) mergedInfo {
	info := mergedInfo{folderIDs: map[string]string{}}

	// Индексы идентичности строятся ОДИН раз на проход и поддерживаются по
	// ходу: пересобирать их на каждой записи значило бы не увидеть только что
	// добавленную (второй сервер с тем же телом удвоился бы).
	byURL := map[string]int{}
	for i := range s.Sources {
		if s.Sources[i].Kind != state.SourceKindSubscription {
			continue
		}
		// Первая победившая: два локальных источника на один URL — состояние,
		// которое лаунчер сам не создаёт, но в чужом файле встретиться может;
		// сливать в обе значило бы размножить настройки файла.
		if url := s.Sources[i].URL; url != "" {
			if _, dup := byURL[url]; !dup {
				byURL[url] = i
			}
		}
	}
	rootBodies := map[string]int{}
	folderAt := newFolderIndex()
	for i := range s.Sources {
		switch s.Sources[i].Kind {
		case state.SourceKindServer:
			if key := nodeBodyKey(&s.Sources[i].Node); key != "" {
				if _, dup := rootBodies[key]; !dup {
					rootBodies[key] = i
				}
			}
		case state.SourceKindFolder:
			folderAt.addExisting(s.Sources[i].ID, s.Sources[i].Name, i)
		}
	}
	existingChains := map[string]bool{}
	for i := range s.Sources {
		if s.Sources[i].Kind == state.SourceKindChain {
			existingChains[s.Sources[i].NodeTagOrLabel()] = true
		}
	}
	takenIDs := takenSourceIDs(s.Sources)

	for _, item := range items {
		switch item.Kind {
		case decodedSubscription:
			mergeSubscriptionItem(s, item, byURL, takenIDs, cnt, &info)
		case decodedServer:
			mergeServerItem(s, item, rootBodies, folderAt, rootTags, takenIDs, cnt, &info)
		case decodedFolder:
			mergeFolderItem(s, item, folderAt, takenIDs, cnt, &info)
		case decodedChain:
			mergeChainItem(s, item, existingChains, takenIDs, warns, cnt, &info)
		}
	}
	return info
}

// mergeSubscriptionItem — подписка по URL (§9 п. 1).
//
// Совпала → локальная запись ОСТАЁТСЯ (её id, nodes[] с историей,
// UpdateStatus), а настройки берутся из файла. Не совпала → добавляется в
// конец. Ключ — `url` БАЙТ В БАЙТ, без всякой нормализации: любая нормализация
// обязана совпасть у двух реализаций посимвольно, иначе одна сторона сольёт
// записи, а вторая заведёт вторую подписку на тот же адрес.
func mergeSubscriptionItem(s *state.State, item decodedSource, byURL map[string]int, takenIDs map[string]bool, cnt *mergeCounters, info *mergedInfo) {
	incoming := item.Src
	url := incoming.URL
	at, hit := byURL[url]
	if url == "" || !hit {
		// Новая подписка: id из файла, а при коллизии — свежий ULID.
		// Совпавший id у РАЗНЫХ подписок означал бы два владельца одной
		// адресации (NodeLink.folderId, каталоги профилей, отчёты).
		incoming.ID = freshIDIfTaken(incoming.ID, takenIDs)
		takenIDs[incoming.ID] = true
		s.Sources = append(s.Sources, incoming)
		at = len(s.Sources) - 1
		if url != "" {
			byURL[url] = at
		}
		// Общий detour подписки — такая же ссылка на папку, как у узла, и
		// переписки по карте id она требует ровно так же.
		info.linked = append(info.linked, nodeAddr{src: at, node: -1})
		cnt.AddedSubscriptions++
		return
	}
	applySubscriptionSettings(&s.Sources[at], incoming, item.FullSettings)
	// Настройки файла заместили detour локальной записи — значит ссылка
	// теперь ИЗ ФАЙЛА, и её id тоже надо переписать.
	info.linked = append(info.linked, nodeAddr{src: at, node: -1})
	cnt.UpdatedSubscriptions++
}

// mergeServerItem — одиночный узел по ТЕЛУ (§9 пп. 2–3).
//
// Тег у одиночного узла — локальное имя, и один и тот же сервер,
// переименованный на другой машине, обязан узнаться, иначе каждый импорт
// плодил бы его копию. Совпавшее тело — пропуск БЕЗ warning: это не потеря, а
// «у тебя уже есть». Секции файла при этом сильнее локальных (SPEC 121).
//
// Имя папки (item.Folder) сравнивается КАК ЕСТЬ — без подрезки и с учётом
// регистра: «DE» и «de » — две разные папки.
func mergeServerItem(s *state.State, item decodedSource, rootBodies map[string]int, folderAt *folderIndex, rootTags map[string]bool, takenIDs map[string]bool, cnt *mergeCounters, info *mergedInfo) {
	incoming := item.Src
	if item.Folder == "" {
		key := nodeBodyKey(&incoming.Node)
		if at, dup := rootBodies[key]; key != "" && dup {
			// Узел уже есть. Тело у него то же, но секции файла сильнее
			// локальных — иначе связка, ради которой бэкап и делали,
			// пропала бы «пропуском без warning».
			applyImportedSections(&s.Sources[at].Node, incoming.Node.Sections, item.Sections)
			info.nodes = append(info.nodes, nodeAddr{src: at, node: -1})
			cnt.SkippedServers++
			return
		}
		incoming.Tag = uniqueTag(rootTags, incoming.Tag)
		incoming.ID = freshIDIfTaken(incoming.ID, takenIDs)
		takenIDs[incoming.ID] = true
		if incoming.Tag != "" {
			rootTags[incoming.Tag] = true
		}
		s.Sources = append(s.Sources, incoming)
		at := len(s.Sources) - 1
		if key != "" {
			rootBodies[key] = at
		}
		info.nodes = append(info.nodes, nodeAddr{src: at, node: -1})
		info.linked = append(info.linked, nodeAddr{src: at, node: -1})
		cnt.AddedServers++
		return
	}

	addFolderMember(s, ensureFolderAt(s, item.Folder, folderAt, cnt), incoming.Node, item.Sections, cnt, info)
}

// mergeFolderItem — папка формата 1.0: сама папка по ID, затем по ИМЕНИ, её
// состав по телу.
//
// Настройки самой папки (политика тегов, свёртка, общий detour) едут только в
// 1.0 — в 0.12 у папки дома не было вовсе. Совпавшая по имени папка держит
// СВОЙ id: на него ссылаются NodeLink и каталоги профилей, и переписать его
// значило бы осиротить ссылки этой машины. Поэтому id файла запоминается в
// карте — по ней переедут ссылки приехавших записей.
//
// Настройки СОВПАВШЕЙ папки берутся из файла ровно так же, как у совпавшей
// подписки (§9 п. 1): у записи из файла своя идентичность — имя, и держать
// локальные настройки значило бы применить полписи. Раньше настройки
// применялись только к новой папке, и главный сценарий переноса (на приёмнике
// папка с тем же именем уже есть) терял их молча — при том что §6.0 завёл им
// дом в 1.0 именно ради переноса.
//
// Сопоставление — folderIndex.lookup: id файла сильнее имени. Нормы §9 это не
// меняет (совпавшая держит локальный id, новая берёт из файла) — это
// уточнение для формата, у которого id папки вообще есть.
func mergeFolderItem(s *state.State, item decodedSource, folderAt *folderIndex, takenIDs map[string]bool, cnt *mergeCounters, info *mergedInfo) {
	at, existed := folderAt.lookup(item.FileFolderID, item.Src.Name)
	if !existed {
		incoming := item.Src
		incoming.Nodes = nil
		incoming.ID = freshIDIfTaken(incoming.ID, takenIDs)
		takenIDs[incoming.ID] = true
		at = len(s.Sources)
		folderAt.addCreated(incoming.ID, incoming.Name, at)
		s.Sources = append(s.Sources, incoming)
		info.linked = append(info.linked, nodeAddr{src: at, node: -1})
		cnt.AddedFolders++
	} else if item.FullSettings {
		// Состав не трогаем: он сливается ниже по телу (§9 п. 3). Заменяются
		// ровно собственные настройки контейнера — те, что 1.0 везёт, а 0.12
		// не выражала вовсе (поэтому под флагом: у 0.x папки отдельной записи
		// нет, она собирается из членов, и «пустых настроек» там не бывает).
		applyFolderSettings(&s.Sources[at], item.Src)
		info.linked = append(info.linked, nodeAddr{src: at, node: -1})
		cnt.UpdatedFolders++
	}
	if item.FileFolderID != "" {
		info.folderIDs[item.FileFolderID] = s.Sources[at].ID
	}
	for i, n := range item.Src.Nodes {
		present := i < len(item.MemberSections) && item.MemberSections[i]
		addFolderMember(s, at, n, present, cnt, info)
	}
}

// ensureFolder — папка по имени: найденная или заведённая (0.x-вход, где
// отдельной записи у папки нет и она собирается из членов).
//
// Пустое имя сюда не доходит: оно означает корень, а не папку без имени — у
// безымянной папки не было бы способа адресовать её членов.
//
// Ключ здесь ТОЛЬКО имя: у 0.x папка — это поле `folder` записи servers[], id
// у неё нет вовсе, и сопоставлять по нему нечем.
func ensureFolderAt(s *state.State, name string, folderAt *folderIndex, cnt *mergeCounters) int {
	at, ok := folderAt.lookup("", name)
	if !ok {
		at = len(s.Sources)
		id := state.MakeULID()
		folderAt.addCreated0x(id, name, at)
		s.Sources = append(s.Sources, state.Source{
			Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			ID:   id,
			Name: name,
		})
		cnt.AddedFolders++
	}
	return at
}

// addFolderMember — член папки: дедуп по телу В ПРЕДЕЛАХ ЭТОЙ папки.
//
// Один и тот же сервер в двух разных папках — законная раскладка (одна
// «рабочая», другая «запасная»), и схлопывать её импорт не вправе.
// Существующие члены остаются на местах, новые дописываются в конец в порядке
// файла, тег уникализируется внутри папки.
func addFolderMember(s *state.State, folderAt int, node state.Node, sectionsPresent bool, cnt *mergeCounters, info *mergedInfo) {
	folder := &s.Sources[folderAt]
	if hit := folderNodeWithBody(folder, &node); hit >= 0 {
		applyImportedSections(&folder.Nodes[hit], node.Sections, sectionsPresent)
		info.nodes = append(info.nodes, nodeAddr{src: folderAt, node: hit})
		cnt.SkippedServers++
		return
	}
	taken := map[string]bool{}
	for i := range folder.Nodes {
		taken[folder.Nodes[i].Tag] = true
	}
	node.Tag = uniqueTag(taken, node.Tag)
	folder.Nodes = append(folder.Nodes, node)
	at := len(folder.Nodes) - 1
	info.nodes = append(info.nodes, nodeAddr{src: folderAt, node: at})
	info.linked = append(info.linked, nodeAddr{src: folderAt, node: at})
	cnt.AddedServers++
}

// mergeChainItem — цепочка по ТЕГУ (§9 п. 4).
//
// Занятый тег → приехавшая запись НЕ применяется: цепочка — ссылочная
// сущность, на её имя метят правила, и второго владельца у имени быть не
// может. Warning ставится ВСЕГДА, даже когда «своя победила», — молчание
// скрыло бы случайных тёзок.
//
// Достижимость hops здесь не проверяется: хоп — чаще всего узел подписки,
// которого до её обновления не существует; рубеж у обеих сторон один — сборка
// (chain_hop_missing).
func mergeChainItem(s *state.State, item decodedSource, existingChains map[string]bool, takenIDs map[string]bool, warns *[]Warning, cnt *mergeCounters, info *mergedInfo) {
	tag := item.Src.NodeTagOrLabel()
	if tag == "" {
		// Безымянная цепочка — запись, которой нечем адресоваться: на её имя
		// метят правила, а имени нет. Применить её означало бы завести в
		// состоянии сущность, на которую нельзя сослаться и которую нельзя
		// отличить от второй такой же.
		return
	}
	if existingChains[tag] {
		*warns = append(*warns, Warning{Code: WarnBackupChainExists, Detail: tag})
		return
	}
	incoming := item.Src
	// id из файла держится, пока он свободен: при коллизии с уже живущим
	// источником — свежий ULID (§9 п. 8). Два источника с одним id дали бы
	// двух владельцев одной адресации.
	incoming.ID = freshIDIfTaken(incoming.ID, takenIDs)
	takenIDs[incoming.ID] = true
	s.Sources = append(s.Sources, incoming)
	info.linked = append(info.linked, nodeAddr{src: len(s.Sources) - 1, node: -1})
	existingChains[tag] = true
	cnt.AddedChains++
}
