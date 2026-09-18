package linkmap

// Исполнитель таблицы маппера (SPEC 133, W1).
//
// Один движок на все схемы и все виды источника. В пакете НЕТ ни одного имени
// схемы, ни одного `if scheme == …`: всё, чем он оперирует, приезжает из
// реестра. Греп-страж — engine_no_scheme_names_test.go.
//
// Нормативный порядок исполнения — MAPPER_ENGINE.md §7: проход A (селекторы),
// проход B (остальные), внутри прохода — порядок объявления.
//
// go1.20-совместимо (Win7-джоба собирает весь модуль тулчейном go1.20): без
// slices/maps/min/max/clear.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// Result — итог исполнения секции над одним элементом входа.
type Result struct {
	// Body — тело узла (карта sing-box), без служебных ключей tag/type.
	Body map[string]interface{}
	// Label — метка узла до фолбэка; пустая, если во входе метки не было.
	Label string
	// LabelFallback — шаблон фолбэка метки, если метки не нашлось.
	LabelFallback string
	// BodySource — как объявила секция (uri/xray/singbox/wgconf/amnezia).
	BodySource string
	// Notes — коды, поставленные движком (uri_param_unknown и
	// объявленные записями on_*). Порядок = порядок появления.
	Notes []Note
	// Trace — трасса, если она была включена.
	Trace *Trace
}

// Note — код с параметрами; в узел их перекладывает вызывающий, потому что
// формат warning'а принадлежит подписке, а не движку.
type Note struct {
	Code   string
	Params map[string]string
}

// execState — рабочее состояние одного исполнения.
type execState struct {
	plan  *Plan
	space *Space
	form  registry.Form
	res   *Result
	trace *Trace

	// writtenBy — какая запись заняла путь тела: нужно разрешению конфликта
	// (priority, затем порядок объявления) и трассе.
	writtenBy map[string]*writeMark
	// bodyType — тип тела (для maps_to по типу и when.$type).
	bodyType string
	// mapperName — "<схема>.<вид>[.<форма>]" для трассы.
	mapperName string
}

type writeMark struct {
	entry    string
	priority int
	decl     int
}

// Exec исполняет план над распакованным пространством источников.
//
// bodyType — тип тела узла (singbox_type схемы): движок сам его не выводит,
// потому что «как называется тип» — свойство реестра, а не таблицы.
func Exec(plan *Plan, space *Space, form registry.Form, bodyType string, trace *Trace) (*Result, error) {
	if plan == nil || plan.Mapper == nil {
		return nil, fmt.Errorf("linkmap: план не задан")
	}
	st := &execState{
		plan:      plan,
		space:     space,
		form:      form,
		trace:     trace,
		writtenBy: map[string]*writeMark{},
		bodyType:  bodyType,
		res: &Result{
			Body:       map[string]interface{}{},
			BodySource: plan.Mapper.BodySource,
			Trace:      trace,
		},
	}
	st.mapperName = plan.Mapper.Scheme() + "." + plan.Mapper.Kind()
	if form.ID != "" {
		st.mapperName += "." + form.ID
	}

	// scheme_sets — написание схемы задаёт присваивания (socks4/socks4a).
	st.applySchemeSets()

	// userinfo — до таблицы: её записи могут ссылаться на userinfo.* как на
	// источник, а метка из userinfo не берётся вовсе.
	st.applyUserInfo()

	// Проход A: селекторы строят тело, по которому дальше проверяется when.
	for i := range plan.Selectors {
		st.applyEntry(&plan.Selectors[i])
	}
	// Проход B: остальные записи.
	for i := range plan.Rest {
		st.applyEntry(&plan.Rest[i])
	}

	// defaults секции — то, чего не написал никто.
	st.applyDefaults()

	st.applyLabel()
	st.noteUnknownParams()

	if trace != nil {
		trace.ResultEvent(st.mapperName, st.res.Body, st.res.Label, st.res.BodySource)
	}
	return st.res, nil
}

// applySchemeSets — присваивания по написанию схемы.
func (st *execState) applySchemeSets() {
	sets := st.plan.Mapper.SchemeSets
	if len(sets) == 0 {
		return
	}
	assigns, ok := sets[st.space.Scheme]
	if !ok {
		return
	}
	st.applyAssigns("$scheme_sets", assigns, 0, 0, "", "")
}

// applyUserInfo раскладывает userinfo по объявленным полям.
//
// Резка по разделителю — только когда секция её объявила: у trojan пароль это
// ВЕСЬ userinfo (D133-8), и резка по ':' молча теряла пароли с двоеточием.
func (st *execState) applyUserInfo() {
	ui := st.plan.Mapper.UserInfo
	if ui == nil {
		return
	}
	raw := st.space.UserInfo
	if raw == "" {
		return
	}
	parts := []string{raw}
	if ui.Split != nil && ui.Split.Sep != "" {
		limit := ui.Split.Limit
		if limit <= 0 {
			limit = -1
		}
		parts = strings.SplitN(raw, ui.Split.Sep, limit)
	}
	targets := ui.Into
	if len(targets) == 0 && ui.SingleInto != "" {
		targets = []string{ui.SingleInto}
	}
	// Одиночный userinfo без разделителя при объявленном single_into едет
	// туда, а не в первый into (конвенция naive/hysteria2).
	if ui.SingleInto != "" && len(parts) == 1 {
		targets = []string{ui.SingleInto}
	}
	for i, target := range targets {
		if i >= len(parts) || target == "" {
			continue
		}
		v := parts[i]
		if v == "" {
			continue
		}
		src := "userinfo"
		if len(targets) > 1 {
			src = "userinfo." + strconv.Itoa(i)
		}
		st.write("$userinfo", src, v, v, target, 0, 0, "")
	}
}

// applyDefaults пишет объявленные секцией значения по умолчанию туда, где
// путь остался пустым.
func (st *execState) applyDefaults() {
	if len(st.plan.Mapper.Defaults) == 0 {
		return
	}
	keys := make([]string, 0, len(st.plan.Mapper.Defaults))
	for k := range st.plan.Mapper.Defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, path := range keys {
		if _, taken := st.writtenBy[path]; taken {
			continue
		}
		if _, exists := getPath(st.res.Body, path); exists {
			continue
		}
		v := st.plan.Mapper.Defaults[path]
		setPath(st.res.Body, path, v)
		st.writtenBy[path] = &writeMark{entry: "$defaults", priority: 1 << 30, decl: 1 << 30}
		st.trace.Add(Event{
			Stage: StageDefault, Mapper: st.mapperName, Entry: "$defaults",
			Src: "-", Raw: nil, Val: v, Path: path, Act: ActWrite, Why: WhyDefault,
		})
	}
}

// applyEntry исполняет одну запись таблицы.
func (st *execState) applyEntry(e *Entry) {
	p := e.Param
	if p == nil {
		return
	}

	// when по телу / по источнику / по $type / по $form.
	if !st.whenHolds(p.When) {
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
			Src: "-", Raw: nil, Val: nil, Path: nil, Act: ActSkip, Why: WhyWhenFalse,
		})
		return
	}

	rawVal, src, found := st.lookupSource(p)
	if !found {
		st.applyMissing(e)
		return
	}

	// Декодирование поверх декодера формы, потом форм-семантика `+`.
	val := st.decodeValue(p, rawVal)

	// normalize / type / value_map — перевод диалекта, не суждение.
	typed, drop := st.convert(p, val)
	if drop {
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
			Src: src, Raw: rawVal, Val: nil, Path: nil, Act: ActSkip, Why: WhyEmpty,
		})
		// value_map → null означает «ключа нет»; но sets/implies по этому
		// значению всё равно могли быть объявлены.
		st.applySets(e, val)
		return
	}

	// extract раскладывает ОДНО значение по нескольким путям.
	if p.Extract != nil {
		st.applyExtract(e, src, rawVal, toString(typed))
		st.applySets(e, val)
		st.applyImplies(e)
		return
	}

	// split_into / list / coerce / flatten / lift / sort_keys.
	if p.Flatten != nil || (p.Coerce != nil && p.Coerce.ObjectToScalar != "") {
		st.applyFlatten(e, src)
	}

	path := st.pathOf(p)
	if path == "" {
		// Запись без maps_to (служебная или осознанно никуда): sets/implies
		// остаются её единственной работой.
		st.applySets(e, val)
		st.applyImplies(e)
		if p.MapsTo != nil && p.MapsTo.ExplicitNull {
			st.trace.Add(Event{
				Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
				Src: src, Raw: rawVal, Val: typed, Path: nil, Act: ActSkip, Why: WhyNone,
			})
		}
		return
	}

	st.writeMerge(e.Name, src, rawVal, typed, path, p.Priority, e.Decl, "", p.Merge)
	st.applySets(e, val)
	st.applyImplies(e)
}

// applyMissing — источник промолчал: materialize_default, default_from, sets по
// пустому значению.
func (st *execState) applyMissing(e *Entry) {
	p := e.Param

	// default_from — источник значения по умолчанию (эвристика SNI у trojan
	// выражается именно им, а не веткой кода).
	if len(p.DefaultFrom) > 0 {
		if name := rawString(p.DefaultFrom); name != "" {
			if v, ok := st.space.Lookup(name); ok && v != "" {
				if path := st.pathOf(p); path != "" {
					st.write(e.Name, name, v, v, path, p.Priority, e.Decl, WhyDefault)
					st.applyImplies(e)
					return
				}
			}
		}
	}

	if p.MaterializeDefault {
		// Значение дефолта берётся из sets[""] секции либо из value_map[""].
		if assigns, ok := p.Sets[""]; ok {
			st.applyAssigns(e.Name, assigns, p.Priority, e.Decl, WhyMaterializeDefault, p.Merge)
			return
		}
	}

	// sets[""] — присваивания «источник молчал» (так security="" даёт
	// tls.enabled у trojan).
	if assigns, ok := p.Sets[""]; ok {
		st.applyAssigns(e.Name, assigns, p.Priority, e.Decl, "", p.Merge)
		return
	}

	st.trace.Add(Event{
		Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
		Src: "-", Raw: nil, Val: nil, Path: nil, Act: ActSkip, Why: WhyEmpty,
	})
}

// applySets — присваивания по ЗНАЧЕНИЮ.
func (st *execState) applySets(e *Entry, val string) {
	if len(e.Param.Sets) == 0 {
		return
	}
	assigns, ok := e.Param.Sets[val]
	if !ok {
		// Значение читается fold-case: `security=NONE` в живых списках.
		low := strings.ToLower(val)
		for k, v := range e.Param.Sets {
			if strings.ToLower(k) == low {
				assigns, ok = v, true
				break
			}
		}
	}
	if !ok {
		return
	}
	st.applyAssigns(e.Name, assigns, e.Param.Priority, e.Decl, "", e.Param.Merge)
}

// applyImplies — присваивания по НАЛИЧИЮ.
func (st *execState) applyImplies(e *Entry) {
	if len(e.Param.Implies) == 0 {
		return
	}
	st.applyAssigns(e.Name, e.Param.Implies, e.Param.Priority, e.Decl, "", e.Param.Merge)
}

// applyAssigns кладёт набор присваиваний. null СНИМАЕТ путь — это отличается
// от «не писать» (MAPPER_ENGINE.md §7, G2).
func (st *execState) applyAssigns(entry string, assigns map[string]interface{}, priority, decl int, why, merge string) {
	paths := make([]string, 0, len(assigns))
	for k := range assigns {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	for _, path := range paths {
		v := assigns[path]
		if v == nil {
			delPath(st.res.Body, path)
			delete(st.writtenBy, path)
			st.trace.Add(Event{
				Stage: StageSets, Mapper: st.mapperName, Entry: entry,
				Src: "-", Raw: nil, Val: nil, Path: path, Act: ActRemove, Why: why,
			})
			continue
		}
		// Вложенное объявление {value, implicit} — значение подставлено
		// конвенцией; на эмите оно не пишется.
		if obj, ok := v.(map[string]interface{}); ok {
			if inner, has := obj["value"]; has {
				v = inner
			}
		}
		st.writeAssign(entry, v, path, priority, decl, why, merge)
	}
}

// applyExtract раскладывает значение регуляркой с именованными группами.
func (st *execState) applyExtract(e *Entry, src, raw, val string) {
	re, err := compileShared(e.Param.Extract.Re)
	if err != nil {
		return
	}
	m := re.FindStringSubmatch(val)
	if m == nil {
		if code := codeOf(e.Param.OnNoMatch); code != "" {
			st.note(code, nil)
		}
		return
	}
	names := re.SubexpNames()
	// Порядок групп = порядок в регулярке: он объявлен автором записи и
	// потому нормативен.
	for gi, name := range names {
		if gi == 0 || name == "" {
			continue
		}
		spec, ok := e.Param.Extract.Into[name]
		if !ok {
			continue
		}
		g := m[gi]
		if g == "" {
			continue
		}
		st.applyExtractGroup(e, src, raw, name, g, spec)
	}
}

// applyExtractGroup кладёт одну группу: либо строкой-путём, либо объектом
// {path, type, implies}.
func (st *execState) applyExtractGroup(e *Entry, src, raw, group, val string, spec interface{}) {
	switch t := spec.(type) {
	case string:
		st.write(e.Name+"."+group, src, raw, val, t, e.Param.Priority, e.Decl, "")
	case map[string]interface{}:
		path, _ := t["path"].(string)
		if path == "" {
			return
		}
		var out interface{} = val
		if typ, _ := t["type"].(string); typ != "" {
			conv, drop := convertType(typ, val)
			if drop {
				if code := codeOf(mapOf(t["on_invalid"])); code != "" {
					st.note(code, nil)
				}
				return
			}
			out = conv
		}
		st.write(e.Name+"."+group, src, raw, out, path, e.Param.Priority, e.Decl, "")
		if impl, ok := t["implies"].(map[string]interface{}); ok {
			st.applyAssigns(e.Name+"."+group, impl, e.Param.Priority, e.Decl, "", e.Param.Merge)
		}
	}
}

// applyFlatten разворачивает члены вложенного объекта в плоский слой
// пространства: дальше их читают обычные записи по своим source.
func (st *execState) applyFlatten(e *Entry, src string) {
	rawVal, ok := st.space.LookupRaw(src)
	if !ok {
		return
	}
	obj, ok := rawVal.(map[string]interface{})
	if !ok {
		return
	}
	for _, key := range e.Param.Flatten {
		inner, ok := obj[key].(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range inner {
			if _, exists := obj[k]; exists {
				continue
			}
			obj[k] = v
		}
	}
}

// lookupSource — значение по цепочке источников записи.
//
// Цепочка «первый НЕПУСТОЙ»: так читает и сегодняшний queryParam, и корпус на
// этом стоит (sni → peer → host). Возвращает (сырое значение, имя
// сработавшего источника, найдено ли).
func (st *execState) lookupSource(p *registry.Param) (string, string, bool) {
	names := p.Source.List
	if len(p.Source.ByForm) > 0 {
		if byForm, ok := p.Source.ByForm[st.form.ID]; ok {
			names = byForm
		}
	}
	for _, name := range names {
		name = st.substituteBase(name)
		v, ok := st.space.Lookup(name)
		if !ok {
			continue
		}
		// «Пусто = отсутствует», опт-аут `empty: "significant"`.
		if strings.TrimSpace(v) == "" && p.Empty != "significant" {
			continue
		}
		return v, name, true
	}
	// Алиасы записи — те же источники под другими именами параметра.
	if len(p.Aliases) > 0 {
		for _, alias := range p.Aliases {
			for _, name := range names {
				if !strings.HasPrefix(name, "query.") {
					continue
				}
				v, ok := st.space.Lookup("query." + alias)
				if !ok {
					continue
				}
				if strings.TrimSpace(v) == "" && p.Empty != "significant" {
					continue
				}
				return v, "query." + alias, true
			}
		}
	}
	return "", "", false
}

// substituteBase подставляет якорь формы вместо $base.
func (st *execState) substituteBase(name string) string {
	if st.form.Base == "" || !strings.Contains(name, "$base") {
		return name
	}
	return strings.Replace(name, "$base", st.form.Base, 1)
}

// decodeValue применяет decode_extra и форм-семантику `+`.
func (st *execState) decodeValue(p *registry.Param, raw string) string {
	val := raw
	if p.DecodeExtra != nil {
		n, untilStable := p.DecodeExtra.PassCount()
		val = decodePasses(val, p.DecodeExtra.Mode, n, untilStable, p.DecodeExtra.Max)
	}
	if !st.plusLiteral(p) {
		val = PlusToSpace(val)
	}
	return val
}

// plusLiteral — читается ли `+` буквально.
//
// По умолчанию выводится из формата поля (base64*), явное указание
// перекрывает. Одно правило вместо заплат у каждого ключа (D133-7).
func (st *execState) plusLiteral(p *registry.Param) bool {
	if p.DecodeExtra != nil && p.DecodeExtra.PlusLiteral != nil {
		return *p.DecodeExtra.PlusLiteral
	}
	if p.DecodeExtra != nil && p.DecodeExtra.Mode == "path" {
		// В пути `+` литерален: `/ws+v2` не должен стать `/ws v2` (D133-14).
		return true
	}
	return strings.HasPrefix(p.Type, "base64")
}

// convert применяет normalize, type и value_map.
//
// Второе возвращаемое — «значения нет» (value_map в null, либо тип не сошёлся
// и запись объявила on_invalid с drop).
func (st *execState) convert(p *registry.Param, val string) (interface{}, bool) {
	v := val
	if p.Normalize != "" {
		v = normalizeValue(p.Normalize, v)
	}

	if len(p.ValueMap) > 0 {
		mapped, hit, isNull := applyValueMap(p.ValueMap, v)
		if isNull {
			return nil, true
		}
		if hit {
			v = mapped
		}
	}

	if p.List != nil {
		return st.buildList(p, v), false
	}

	if p.Type != "" {
		conv, drop := convertType(p.Type, v)
		if drop {
			return nil, true
		}
		return conv, false
	}
	return v, false
}

// buildList режет значение по разделителю, обрезая края элементов.
func (st *execState) buildList(p *registry.Param, v string) []interface{} {
	sep := p.List.Sep
	if sep == "" {
		sep = ","
	}
	parts := strings.Split(v, sep)
	out := make([]interface{}, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if p.List.Item != "" {
			conv, drop := convertType(p.List.Item, item)
			if drop {
				continue
			}
			out = append(out, conv)
			continue
		}
		out = append(out, item)
	}
	return out
}

// pathOf — целевой путь записи с учётом карты по типу тела.
func (st *execState) pathOf(p *registry.Param) string {
	if p.MapsTo == nil {
		return ""
	}
	if p.MapsTo.Path != "" {
		return p.MapsTo.Path
	}
	if len(p.MapsTo.ByType) > 0 {
		return p.MapsTo.ByType[st.bodyType]
	}
	return ""
}

// write кладёт значение в тело, разрешая конфликт двух записей в один путь.
func (st *execState) write(entry, src string, raw, val interface{}, path string, priority, decl int, why string) {
	st.writeMerge(entry, src, raw, val, path, priority, decl, why, "")
}

func (st *execState) writeMerge(entry, src string, raw, val interface{}, path string, priority, decl int, why, merge string) {
	if !st.claim(entry, path, priority, decl, merge) {
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: entry,
			Src: src, Raw: raw, Val: val, Path: path, Act: ActSkip,
			Why: WhyLowerPriority(st.writtenBy[path].entry),
		})
		return
	}
	act := ActWrite
	if _, exists := getPath(st.res.Body, path); exists {
		act = ActOverride
	}
	setPath(st.res.Body, path, val)
	st.trace.Add(Event{
		Stage: StageField, Mapper: st.mapperName, Entry: entry,
		Src: src, Raw: raw, Val: val, Path: path, Act: act, Why: orDash(why),
	})
}

// writeAssign — то же для присваивания из sets/implies (стадия трассы иная).
func (st *execState) writeAssign(entry string, val interface{}, path string, priority, decl int, why, merge string) {
	if !st.claim(entry, path, priority, decl, merge) {
		st.trace.Add(Event{
			Stage: StageSets, Mapper: st.mapperName, Entry: entry,
			Src: "-", Raw: nil, Val: val, Path: path, Act: ActSkip,
			Why: WhyLowerPriority(st.writtenBy[path].entry),
		})
		return
	}
	act := ActWrite
	if _, exists := getPath(st.res.Body, path); exists {
		act = ActOverride
	}
	setPath(st.res.Body, path, val)
	st.trace.Add(Event{
		Stage: StageSets, Mapper: st.mapperName, Entry: entry,
		Src: "-", Raw: nil, Val: val, Path: path, Act: act, Why: orDash(why),
	})
}

// claim решает, кто владеет путём.
//
// Норма MAPPER_ENGINE.md §7: `priority` (МЕНЬШЕ = раньше), при равном
// приоритете — порядок объявления, а что делать с ЗАНЯТЫМ путём, решает
// `merge` (`keep_first` по умолчанию). То есть первый писавший держит путь,
// пока пришедший позже явно не объявил `overwrite`.
//
// Правило «sets побеждает» неверно: побеждает не источник значения, а
// объявленный порядок.
func (st *execState) claim(entry, path string, priority, decl int, merge string) bool {
	prev, taken := st.writtenBy[path]
	if !taken {
		st.writtenBy[path] = &writeMark{entry: entry, priority: priority, decl: decl}
		return true
	}
	// Запись, которая по порядку ДОЛЖНА была идти раньше занявшей путь,
	// всё равно её перебивает: порядок исполнения и порядок объявления в
	// пределах одного прохода совпадают, поэтому сюда попадает только
	// селектор против записи прохода B — и он объявлен раньше.
	earlier := priority < prev.priority || (priority == prev.priority && decl < prev.decl)
	if earlier {
		st.writtenBy[path] = &writeMark{entry: entry, priority: priority, decl: decl}
		return true
	}
	switch merge {
	case "overwrite", "prepend", "append":
		st.writtenBy[path] = &writeMark{entry: entry, priority: priority, decl: decl}
		return true
	}
	return false
}

// whenHolds проверяет условие записи.
//
// Ключ-путь тела читается по УЖЕ ПОСТРОЕННОМУ телу, ключ-источник — по входу;
// `$type` — тип тела, `$form` — id сработавшей формы. Условие по источнику
// необходимо там, где род узла объявляет ВХОД, а не тело.
func (st *execState) whenHolds(when map[string]interface{}) bool {
	if len(when) == 0 {
		return true
	}
	keys := make([]string, 0, len(when))
	for k := range when {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !st.oneWhen(key, when[key]) {
			return false
		}
	}
	return true
}

func (st *execState) oneWhen(key string, want interface{}) bool {
	var got interface{}
	var present bool
	switch {
	case key == "$type":
		got, present = st.bodyType, st.bodyType != ""
	case key == "$form":
		got, present = st.form.ID, st.form.ID != ""
	case isSourceName(key):
		key = st.substituteBase(key)
		v, ok := st.space.Lookup(key)
		got, present = v, ok
	default:
		got, present = getPath(st.res.Body, key)
	}

	switch w := want.(type) {
	case nil:
		return !present
	case string:
		if !present {
			return w == ""
		}
		return strings.EqualFold(toString(got), w)
	case bool:
		if !present {
			return !w
		}
		b, ok := got.(bool)
		if !ok {
			b = flagTrue(toString(got))
		}
		return b == w
	case float64:
		if !present {
			return false
		}
		return toString(got) == formatNumber(w)
	case map[string]interface{}:
		return st.whenOperator(got, present, w)
	}
	return false
}

// whenOperator — операторы условия: in / not_in / present / absent.
func (st *execState) whenOperator(got interface{}, present bool, spec map[string]interface{}) bool {
	if list, ok := spec["in"].([]interface{}); ok {
		if !present {
			return false
		}
		return valueInList(got, list)
	}
	if list, ok := spec["not_in"].([]interface{}); ok {
		if !present {
			return true
		}
		return !valueInList(got, list)
	}
	if v, ok := spec["present"].(bool); ok {
		return present == v
	}
	if v, ok := spec["absent"].(bool); ok {
		return present != v
	}
	return false
}

func valueInList(got interface{}, list []interface{}) bool {
	s := toString(got)
	for _, item := range list {
		switch t := item.(type) {
		case string:
			if strings.EqualFold(s, t) {
				return true
			}
		case float64:
			if s == formatNumber(t) {
				return true
			}
		case bool:
			if s == strconv.FormatBool(t) {
				return true
			}
		}
	}
	return false
}

// isSourceName — ключ адресует ВХОД, а не тело.
func isSourceName(key string) bool {
	for _, p := range []string{"query.", "json.", "ini.", "userinfo"} {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	switch key {
	case "scheme", "authority", "host", "port", "port_raw", "path", "fragment":
		return true
	}
	return false
}

// applyLabel собирает метку по цепочке источников.
func (st *execState) applyLabel() {
	spec := st.plan.Mapper.Label
	if spec == nil {
		return
	}
	for _, name := range spec.Source.All() {
		v, ok := st.space.Lookup(st.substituteBase(name))
		if !ok || v == "" {
			continue
		}
		st.res.Label = v
		st.trace.Add(Event{
			Stage: StageLabel, Mapper: st.mapperName, Entry: "$label",
			Src: name, Raw: v, Val: v, Path: nil, Act: ActWrite, Why: WhyNone,
		})
		break
	}
	for _, n := range spec.Normalize {
		st.res.Label = normalizeValue(n, st.res.Label)
	}
	if len(spec.ValueMap) > 0 {
		for from, to := range spec.ValueMap {
			if s, ok := to.(string); ok {
				st.res.Label = strings.ReplaceAll(st.res.Label, from, s)
			}
		}
	}
	if spec.Fallback != nil {
		st.res.LabelFallback = spec.Fallback.Template
	}
}

// noteUnknownParams ставит код на каждый НЕобъявленный параметр ссылки.
//
// Объявленным считается имя из params секции, из любого include-блока
// (включая не применившиеся по when) и из aliases любого из них
// (MAPPER_ENGINE.md §8).
func (st *execState) noteUnknownParams() {
	uk := st.plan.Mapper.UnknownKey
	if uk == nil || uk.Code == "" {
		return
	}
	for _, name := range st.space.QueryNames() {
		if st.plan.Declared[strings.ToLower(name)] {
			continue
		}
		st.note(uk.Code, map[string]string{"query_name": name})
		st.trace.Add(Event{
			Stage: StageUnknown, Mapper: st.mapperName, Entry: "$unknown",
			Src: "query." + name, Raw: nil, Val: nil, Path: nil,
			Act: ActKeep, Why: WhyNotDeclared,
		})
	}
}

func (st *execState) note(code string, params map[string]string) {
	st.res.Notes = append(st.res.Notes, Note{Code: code, Params: params})
}

// --- общие преобразования значений ---

// applyValueMap переводит значение. Возвращает (новое значение, было ли
// попадание, означает ли попадание «ключа нет»).
//
// Поддерживает и точную карту, и форму {prefix, strip} (диалект uTLS).
func applyValueMap(vm map[string]interface{}, v string) (string, bool, bool) {
	// $ref — именованная таблица общих блоков, разрешённая при сборке плана.
	if ref, ok := vm["$ref"].(string); ok {
		resolved := lookupNamedValueMap(ref)
		if resolved == nil {
			return v, false, false
		}
		vm = resolved
	}
	if raw, ok := vm["prefix"].(map[string]interface{}); ok {
		key := strings.ToLower(v)
		if strip, ok := vm["strip"].([]interface{}); ok {
			for _, s := range strip {
				if ss, ok := s.(string); ok {
					key = strings.ReplaceAll(key, ss, "")
				}
			}
		}
		prefixes := make([]string, 0, len(raw))
		for k := range raw {
			prefixes = append(prefixes, k)
		}
		// Длинный префикс раньше короткого: "hellorandomized" не должен
		// проиграть "hellorandom".
		sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
		for _, p := range prefixes {
			if strings.HasPrefix(key, p) {
				s, _ := raw[p].(string)
				return s, true, false
			}
		}
		return v, false, false
	}

	if mapped, ok := vm[v]; ok {
		if mapped == nil {
			return "", true, true
		}
		return toString(mapped), true, false
	}
	// Регистронезависимое попадание.
	low := strings.ToLower(v)
	for k, mapped := range vm {
		if strings.ToLower(k) == low {
			if mapped == nil {
				return "", true, true
			}
			return toString(mapped), true, false
		}
	}
	return v, false, false
}

// convertType приводит значение к объявленному типу. Второе возвращаемое —
// «значения нет» (негодная форма).
func convertType(typ, v string) (interface{}, bool) {
	switch typ {
	case "", "string":
		return v, false
	case "int":
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, true
		}
		return n, false
	case "bool", "bool_spelled":
		if !flagTrue(v) {
			return nil, true
		}
		return true, false
	default:
		return v, false
	}
}

// flagTrue — одно правило истинности на все булевы параметры всех схем.
func flagTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// normalizeValue — объявленные нормализации формы значения.
func normalizeValue(kind, v string) string {
	switch kind {
	case "trim":
		return strings.TrimSpace(v)
	case "trim_lower":
		return strings.ToLower(strings.TrimSpace(v))
	case "strip_control":
		return stripControl(v)
	}
	return v
}

// stripControl снимает C0-управляющие и DEL, оставляя tab/CR/LF.
func stripControl(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			b.WriteRune(r)
			continue
		}
		if r <= 0x1F || r == 0x7F {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// decodePasses выполняет дополнительные проходы percent-декода.
func decodePasses(v, mode string, n int, untilStable bool, max int) string {
	if untilStable {
		if max <= 0 {
			max = 16
		}
		for i := 0; i < max; i++ {
			next := decodeOnce(v, mode)
			if next == v {
				return v
			}
			v = next
		}
		return v
	}
	for i := 0; i < n; i++ {
		v = decodeOnce(v, mode)
	}
	return v
}

// decodeOnce — один проход percent-декода в объявленном режиме.
//
// Режимы различаются НАМЕРЕННО: в пути `+` литерален, и query-семантика
// превратила бы `/ws+v2%2Fdata` в `/ws v2/data` — сервер отвечает 404.
func decodeOnce(v, mode string) string {
	dec, err := percentUnescape(v)
	if err != nil {
		return v
	}
	if mode == "query" {
		return dec
	}
	return dec
}

// percentUnescape — percent-декод, НЕ трогающий `+` (путь и query решают о нём
// сами). Битая последовательность оставляется как есть: она законна в живых
// подписках, и ронять из-за неё узел нельзя.
func percentUnescape(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		hi, ok1 := unhex(s[i+1])
		lo, ok2 := unhex(s[i+2])
		if !ok1 || !ok2 {
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String(), nil
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// --- пути тела ---

// setPath кладёт значение по точечному пути, создавая недостающие уровни.
func setPath(root map[string]interface{}, path string, v interface{}) {
	parts := strings.Split(path, ".")
	cur := root
	for i := 0; i < len(parts)-1; i++ {
		next, ok := cur[parts[i]].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			cur[parts[i]] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = v
}

// getPath читает значение по точечному пути.
func getPath(root map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var cur interface{} = root
	for _, p := range parts {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// delPath снимает путь; опустевшие родительские уровни убираются вместе с ним,
// иначе в теле остались бы пустые объекты, которых никто не писал.
func delPath(root map[string]interface{}, path string) {
	parts := strings.Split(path, ".")
	maps := []map[string]interface{}{root}
	cur := root
	for i := 0; i < len(parts)-1; i++ {
		next, ok := cur[parts[i]].(map[string]interface{})
		if !ok {
			return
		}
		maps = append(maps, next)
		cur = next
	}
	delete(cur, parts[len(parts)-1])
	for i := len(maps) - 1; i > 0; i-- {
		if len(maps[i]) == 0 {
			delete(maps[i-1], parts[i-1])
		}
	}
}

// --- мелочи ---

func toString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case float64:
		return formatNumber(t)
	}
	return ""
}

func orDash(why string) string {
	if why == "" {
		return WhyNone
	}
	return why
}

func codeOf(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	s, _ := m["code"].(string)
	return s
}

func mapOf(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// rawString читает json.RawMessage как строку (default_from объявлен строкой).
func rawString(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		return s[1 : len(s)-1]
	}
	return ""
}

// сохраняем ссылку на regexp, чтобы импорт не выпал при правках выше.
var _ = regexp.MustCompile
