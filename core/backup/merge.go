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
}

// mergeSubscriptions сливает секцию subscriptions[] в состояние по URL.
//
// Ключ — `url` БАЙТ В БАЙТ, без всякой нормализации: строка не трогается ни
// TrimSpace, ни приведением регистра хоста, ни выравниванием слэша и порядка
// query. Любая нормализация обязана совпасть у двух реализаций посимвольно,
// иначе одна сторона сольёт записи, а вторая заведёт вторую подписку на тот
// же адрес; договориться о «как есть» дешевле и надёжнее. Ввод и экспорт с
// обеих сторон и так хранят адрес уже подрезанным.
//
// Совпала → локальная запись ОСТАЁТСЯ (её id, nodes[] с историей, UpdateStatus),
// а настройки берутся из файла. Не совпала → добавляется в конец.
func mergeSubscriptions(s *state.State, subs []Subscription, warns *[]Warning, cnt *mergeCounters) {
	// Индекс локальных подписок по URL. Первая победившая: два локальных
	// источника на один URL — состояние, которое лаунчер сам не создаёт, но
	// в чужом файле состояния встретиться может; сливать в обе значило бы
	// размножить настройки файла.
	byURL := map[string]int{}
	for i := range s.Sources {
		if s.Sources[i].Kind != state.SourceKindSubscription {
			continue
		}
		url := s.Sources[i].URL
		if url == "" {
			continue
		}
		if _, dup := byURL[url]; !dup {
			byURL[url] = i
		}
	}

	takenIDs := takenSourceIDs(s.Sources)

	for i, sub := range subs {
		incoming, w := importSubscription(sub, i)
		*warns = append(*warns, w...)

		url := sub.URL
		at, hit := byURL[url]
		if url == "" || !hit {
			// Новая подписка: id из файла, а при коллизии — свежий ULID.
			// Совпавший id у РАЗНЫХ подписок означал бы два владельца одной
			// адресации (NodeLink.folderId, каталоги профилей, отчёты).
			incoming.ID = freshIDIfTaken(incoming.ID, takenIDs)
			takenIDs[incoming.ID] = true
			s.Sources = append(s.Sources, incoming)
			cnt.AddedSubscriptions++
			continue
		}

		applySubscriptionSettings(&s.Sources[at], incoming)
		cnt.UpdatedSubscriptions++
	}
}

// applySubscriptionSettings переносит настройки приехавшей подписки на
// локальную, оставляя ей идентичность и историю.
//
// Что НЕ трогается: ID (на него ссылаются NodeLink и каталоги профилей),
// Nodes (состав с отметками включения — он живёт от fetch к fetch, а в файл
// не едет вовсе), UpdateStatus и Meta (диагностика ЭТОЙ машины: чужая история
// обновлений здесь врала бы), RelaysInDirections (настройка вида лаунчера, в
// контракте её нет — приехавшая запись о ней ничего не знает).
//
// Что применяется — ровно то, что сторона умеет и что файл несёт: имя,
// включённость, политика тегов, интервал обновления, кап, фильтры отсева,
// свёртка, detour и слепок identity.
func applySubscriptionSettings(dst *state.Source, src state.Source) {
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

	// Identity — СЛЕПОК целиком, а не поле за полем: отсутствие объекта
	// identity в файле значит «как в системе», и оставить здесь прежний UA
	// значило бы сохранить настройку, которую пользователь на той машине
	// снял. Слепок ставится тем, что уже разобрал importSourceIdentity: при
	// отсутствии объекта поля пусты — то самое «как в системе».
	dst.UserAgent = src.UserAgent
	dst.HWID = src.HWID
	dst.SendHWID = src.SendHWID
	dst.HashDeviceModel = src.HashDeviceModel

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

// mergeServers сливает секцию servers[] — корневые записи и папки.
//
// Дедуп по ТЕЛУ, а не по тегу: тег у одиночного узла — локальное имя, и один
// и тот же сервер, переименованный на другой машине, обязан узнаться, иначе
// каждый импорт плодил бы его копию. Совпавшее тело — пропуск БЕЗ warning:
// это не потеря, а «у тебя уже есть».
//
// Тег новой записи уникализируется против занятых имён своего пространства
// (корень или папка) суффиксом `-2` — той же формой, что у ручного добавления.
//
// Папка не имеет отдельной секции в файле и собирается ПО ИМЕНИ — так же, как
// на другой стороне: одно имя = одна папка, второй встреченный
// `folder: "Proton"` дополняет уже существующую, а не заводит тёзку. Пустое
// имя — это корень, а не папка без имени: у безымянной папки не было бы
// способа адресовать её членов. Существующие члены остаются на местах, новые
// дописываются в конец в порядке файла.
func mergeServers(s *state.State, list []Server, warns *[]Warning, cnt *mergeCounters) {
	rootBodies := map[string]bool{}
	rootTags := takenRootTags(s)
	takenIDs := takenSourceIDs(s.Sources)
	for i := range s.Sources {
		switch s.Sources[i].Kind {
		case state.SourceKindServer:
			if key := nodeBodyKey(&s.Sources[i].Node); key != "" {
				rootBodies[key] = true
			}
		}
	}

	// folderAt — папка по ИМЕНИ; ищется среди уже существующих, а новая
	// заводится один раз на имя (то же «одно имя = одна папка», что у чистого
	// импорта). Индекс, а не указатель: срез растёт и переезжает в памяти.
	folderAt := map[string]int{}
	for i := range s.Sources {
		if s.Sources[i].Kind != state.SourceKindFolder {
			continue
		}
		name := s.Sources[i].Name
		if _, dup := folderAt[name]; !dup {
			folderAt[name] = i
		}
	}

	for _, srv := range list {
		incoming, w := importServer(srv)
		*warns = append(*warns, w...)

		// Имя папки сравнивается КАК ЕСТЬ — без подрезки и без учёта
		// регистра: «DE» и «de » — две разные папки. Нормализация имени
		// обязана совпасть у двух реализаций посимвольно, а цена ошибки
		// здесь — молча слитый состав двух разных папок.
		name := srv.Folder
		if name == "" {
			key := nodeBodyKey(&incoming.Node)
			if key != "" && rootBodies[key] {
				cnt.SkippedServers++
				continue
			}
			incoming.Tag = uniqueTag(rootTags, incoming.Tag)
			incoming.ID = freshIDIfTaken(incoming.ID, takenIDs)
			takenIDs[incoming.ID] = true
			if key != "" {
				rootBodies[key] = true
			}
			if incoming.Tag != "" {
				rootTags[incoming.Tag] = true
			}
			s.Sources = append(s.Sources, incoming)
			cnt.AddedServers++
			continue
		}

		at, ok := folderAt[name]
		if !ok {
			at = len(s.Sources)
			folderAt[name] = at
			s.Sources = append(s.Sources, state.Source{
				Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
				ID:   state.MakeULID(),
				Name: name,
			})
			cnt.AddedFolders++
		}
		folder := &s.Sources[at]
		if folderHasBody(folder, &incoming.Node) {
			cnt.SkippedServers++
			continue
		}
		taken := map[string]bool{}
		for i := range folder.Nodes {
			taken[folder.Nodes[i].Tag] = true
		}
		incoming.Node.Tag = uniqueTag(taken, incoming.Node.Tag)
		folder.Nodes = append(folder.Nodes, incoming.Node)
		cnt.AddedServers++
	}
}

// folderHasBody — тело уже лежит в этой папке.
//
// Дедуп в пределах ОДНОЙ папки, а не по всему состоянию: один и тот же сервер
// в двух разных папках — законная раскладка (одна «рабочая», другая
// «запасная»), и схлопывать её импорт не вправе.
func folderHasBody(folder *state.Source, node *state.Node) bool {
	key := nodeBodyKey(node)
	if key == "" {
		return false
	}
	for i := range folder.Nodes {
		if nodeBodyKey(&folder.Nodes[i]) == key {
			return true
		}
	}
	return false
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
