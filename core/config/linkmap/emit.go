package linkmap

// ОБРАТНЫЙ ХОД: тело узла → share-ссылка, по ТЕМ ЖЕ секциям `mappers.uri`
// реестра, что читает прямой ход (MAPPER_ENGINE.md §9).
//
// Одна таблица в обе стороны, а не два плана. Обратные операции:
//
//	maps_to⁻¹   путь тела → значение параметра
//	value_map⁻¹ обратная таблица значений (только для инъективной)
//	sets⁻¹      сопоставление присваиваний: чей набор лежит в теле — то и имя
//	compose     обращение extract: один параметр из нескольких путей тела
//	into⁻¹      userinfo собирается из путей, объявленных в userinfo.into
//	form_from   написание схемы выбирается по телу либо по роду узла
//
// Как и весь пакет, код НЕ ЗНАЕТ НИ ОДНОЙ СХЕМЫ по имени (страж
// TestNoSchemeNamesInEngine): всё, чем он оперирует, приезжает с диска.
//
// Норма вида ссылки (решение владельца 19.09.2026): Copy link существует для
// обмена с ЧУЖИМИ клиентами, поэтому норма вида — де-факто формат схемы, а не
// внутренняя симметрия таблицы. Отсюда `emit_when: always`, `emit.names`,
// `json_always` — они объявляются РЕЕСТРОМ, движок их только исполняет.
//
// go1.20-совместимо: без slices/maps/min/max.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// ErrEmitNoSection — у схемы нет секции обратного хода (`emit: null`).
// Отдельная ошибка, потому что «обратного хода нет» — объявленное решение
// реестра, а не сбой.
var ErrEmitNoSection = fmt.Errorf("linkmap: обратного хода у секции нет")

// EmitInput — вход обратного хода.
type EmitInput struct {
	// Body — тело узла в каноне ядра (то же, что отдаёт прямой ход после
	// санитайзера).
	Body map[string]interface{}
	// Label — метка узла: едет во фрагмент либо в ключ JSON-формы по
	// `label.source`.
	Label string
	// Kind — РОД узла (MAPPER_ENGINE.md «Контекст санитайзера»). Обратный
	// ход читает его через `emit.form_from`: узел, у которого в теле не
	// осталось ни одного признака рода, обязан выбрать написание схемы по
	// роду, иначе род теряется на круге.
	Kind string
	// BodyType — тип тела (`$type` в условиях `when`).
	BodyType string
}

// emitState — состояние одной сборки ссылки.
type emitState struct {
	plan  *Plan
	in    EmitInput
	spec  *registry.EmitSpec
	names map[string]string

	// pairs — собранные параметры в порядке добавления; порядок окончательно
	// решает param_order.
	pairs []emitPair
	// seen — имя параметра уже занято: первая запись выигрывает, как и на
	// разборе.
	seen map[string]bool

	// consumed — пути тела, уже отданные какой-то записи НАПРЯМУЮ
	// (`maps_to`). Правило №3 обратного хода: ветка `sets` не пишется, когда
	// все её пути принадлежат другим записям напрямую — на чтении побеждает
	// хозяин пути, и обратный ход обязан повторить тот же выбор.
	consumed map[string]bool

	// userinfo — собранный userinfo (уже в выходной форме).
	userinfo string
	// scheme — написание схемы (form_from либо emit.form).
	scheme string
	// jsonObj — тело JSON-формы (`emit.json_map`), когда она объявлена.
	jsonObj map[string]interface{}
	// jsonOrder — порядок ключей JSON-формы.
	jsonOrder []string
}

// emitPair — один параметр выхода.
type emitPair struct {
	name string
	val  string
}

// Emit собирает share-ссылку из тела узла по секции плана.
func Emit(plan *Plan, in EmitInput) (string, error) {
	if plan == nil || plan.Mapper == nil {
		return "", fmt.Errorf("linkmap: план не задан")
	}
	spec := plan.Mapper.Emit
	if spec == nil {
		return "", ErrEmitNoSection
	}
	st := &emitState{
		plan:     plan,
		in:       in,
		spec:     spec,
		names:    spec.Names,
		seen:     map[string]bool{},
		consumed: map[string]bool{},
	}
	if st.in.Body == nil {
		st.in.Body = map[string]interface{}{}
	}
	if err := st.checkRefuse(); err != nil {
		return "", err
	}
	st.scheme = st.resolveScheme()

	// Формa-контейнер объявляется `json_map`: она собирает не query-ссылку, а
	// base64(JSON). Ключи её и порядок объявляет реестр.
	if len(spec.JSONMap) > 0 {
		return st.emitContainer()
	}
	return st.emitURL()
}

// checkRefuse — объявленные секцией условия «ссылкой не выражается».
//
// Отказ, а не молчаливая выдача части: узел с двумя пирами, отданный ссылкой
// про одного, выглядит рабочим и ведёт половиной маршрута.
func (st *emitState) checkRefuse() error {
	for _, r := range st.spec.RefuseWhen {
		if r.Path == "" {
			continue
		}
		v, ok := getPath(st.in.Body, r.Path)
		if !ok {
			continue
		}
		if n, counted := lengthOf(v); counted && n > r.LenGt {
			return fmt.Errorf("linkmap: %s", r.Why)
		}
	}
	return nil
}

// lengthOf — длина массива; (0, false) у всего прочего.
func lengthOf(v interface{}) (int, bool) {
	switch t := v.(type) {
	case []interface{}:
		return len(t), true
	case []map[string]interface{}:
		return len(t), true
	case []string:
		return len(t), true
	}
	return 0, false
}

// resolveScheme выбирает НАПИСАНИЕ схемы.
//
// `form_from` — карта «ключ → {значение → написание}». Ключ читается сначала
// как путь ТЕЛА, потом как `$kind` (род узла): род объявляет ВХОД, и у узла,
// с которого все признаки рода снялись, судить по телу нечем по построению
// (MAPPER_ENGINE.md, «Почему род нельзя вывести из тела»).
//
// `any_set` — обращение `kind_when`: написание выбирается по тому, есть ли в
// теле хоть один из перечисленных путей.
func (st *emitState) resolveScheme() string {
	// Умолчание — имя схемы реестра. `emit.form` называет ФОРМУ (оболочку
	// выхода), а не написание: `sip002` — это форма userinfo, а схема
	// по-прежнему своя.
	fallback := st.plan.Mapper.Scheme()
	if len(st.spec.FormFrom) == 0 {
		return fallback
	}
	keys := make([]string, 0, len(st.spec.FormFrom))
	for k := range st.spec.FormFrom {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		branch, ok := st.spec.FormFrom[key].(map[string]interface{})
		if !ok {
			continue
		}
		got, present := st.formFromValue(key)
		if !present {
			continue
		}
		if name, ok := branchLookup(branch, got); ok {
			return name
		}
	}
	// "*" — ветка «всё остальное»; она объявлена у любого ключа и читается
	// после того, как ни один точный признак не сошёлся.
	for _, key := range keys {
		branch, ok := st.spec.FormFrom[key].(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := branch["*"].(string); ok {
			return name
		}
	}
	return fallback
}

// formFromValue читает значение ключа form_from: путь тела, `$kind`, либо
// предикат `any_set`.
func (st *emitState) formFromValue(key string) (string, bool) {
	if key == emitKeyAnySet {
		return "", false
	}
	if key == emitKeyKind {
		if st.in.Kind == "" {
			return "", false
		}
		return st.in.Kind, true
	}
	v, ok := getPath(st.in.Body, key)
	if !ok || v == nil {
		return "", false
	}
	return toString(v), true
}

// branchLookup — ветка карты по значению; `any_set` в ветке означает «любой
// из этих путей есть в теле».
func branchLookup(branch map[string]interface{}, got string) (string, bool) {
	for k, v := range branch {
		if k == "*" {
			continue
		}
		name, ok := v.(string)
		if !ok {
			continue
		}
		if strings.EqualFold(k, got) {
			return name, true
		}
	}
	return "", false
}

const (
	formURL       = "url"
	formContainer = "container"
	emitKeyKind   = "$kind"
	emitKeyAnySet = "any_set"
)

// emitURL собирает обычную ссылку `<схема>://<userinfo>@<хост>:<порт>?<query>#<метка>`.
func (st *emitState) emitURL() (string, error) {
	st.collect()
	st.buildUserInfo()

	host, port, err := st.hostPort()
	if err != nil {
		return "", err
	}

	// Ссылка печатается ПО ЧАСТЯМ, а не через url.URL: `url.User` экранирует
	// userinfo по своим правилам (`:` уезжает как %3A), а выходная форма
	// userinfo объявлена реестром (`emit.userinfo`), и переписывать её кодом
	// нельзя — пара «пользователь:пароль» у половины схем и есть разделитель,
	// который чужой клиент ищет буквально.
	var b strings.Builder
	b.WriteString(st.scheme)
	b.WriteString("://")
	// Пустой userinfo со значимым разделителем: `hy2://@host` — способ схемы
	// сказать «пароля нет», и объявляет это реестр (`empty_separator`).
	if st.userinfo != "" || st.emptySeparator() {
		b.WriteString(st.userinfo)
		b.WriteByte('@')
	}
	b.WriteString(hostJoin(host, port))
	if q := st.encodeQuery(); q != "" {
		b.WriteByte('?')
		b.WriteString(q)
	}
	if st.in.Label != "" {
		b.WriteByte('#')
		b.WriteString(fragmentEscape(st.in.Label))
	}
	return b.String(), nil
}

// fragmentEscape — экранирование метки во фрагменте.
//
// Правила фрагмента мягче query: `/` и `?` в нём законны, и экранировать их
// значило бы менять текст метки, который человек видит в чужом клиенте.
func fragmentEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscapeFragment(c) {
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0xf])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func shouldEscapeFragment(c byte) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return false
	}
	switch c {
	case '-', '.', '_', '~', '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=', ':', '@', '/', '?':
		return false
	}
	return true
}

// emptySeparator — писать ли «@» у узла без userinfo.
func (st *emitState) emptySeparator() bool {
	if st.spec.UserInfo == nil {
		return false
	}
	return st.spec.UserInfo.EmptySeparator
}

// keepEmptyTail — писать ли разделитель у пустого хвостового компонента.
func (st *emitState) keepEmptyTail() bool {
	if st.spec.UserInfo == nil {
		return false
	}
	return st.spec.UserInfo.KeepEmptyTail
}

// hostPort достаёт адрес и порт из тела по путям, объявленным разбором.
//
// Пути НЕ зашиты: их называют записи, чей источник — `host` и `port`. Так
// адрес узла-endpoint (`peers[].address`) находится тем же кодом, что и
// адрес обычного outbound'а.
func (st *emitState) hostPort() (string, int, error) {
	hostPath := st.pathForSource(srcHost)
	portPath := st.pathForSource(srcPort)
	host := ""
	if hostPath != "" {
		if v, ok := getPath(st.in.Body, hostPath); ok {
			host = toString(v)
		}
	}
	port := 0
	if portPath != "" {
		if v, ok := getPath(st.in.Body, portPath); ok {
			port = intOf(v)
		}
	}
	if host == "" {
		return "", 0, fmt.Errorf("linkmap: у тела нет адреса (%s)", hostPath)
	}
	if st.spec.OmitPort && port == st.declaredDefaultPort(portPath) {
		port = 0
	}
	return host, port, nil
}

const (
	srcHost = "host"
	srcPort = "port"
)

// declaredDefaultPort — порт, объявленный `defaults` секции по тому же пути.
func (st *emitState) declaredDefaultPort(path string) int {
	if path == "" {
		return 0
	}
	if v, ok := st.plan.Mapper.Defaults[path]; ok {
		return intOf(v)
	}
	return 0
}

// pathForSource находит путь тела у записи, чей источник — названное имя
// пространства (`host`, `port`).
func (st *emitState) pathForSource(name string) string {
	for _, list := range [][]Entry{st.plan.Selectors, st.plan.Rest} {
		for i := range list {
			e := &list[i]
			if e.Param == nil {
				continue
			}
			for _, s := range e.Param.Source.All() {
				if s == name {
					if p := st.emitPath(e.Param); p != "" {
						return p
					}
				}
			}
		}
	}
	return ""
}

// emitPath — путь тела записи с учётом типа тела.
func (st *emitState) emitPath(p *registry.Param) string {
	if p.MapsTo == nil {
		return ""
	}
	if p.MapsTo.Path != "" {
		return p.MapsTo.Path
	}
	if len(p.MapsTo.ByType) > 0 && st.in.BodyType != "" {
		return p.MapsTo.ByType[st.in.BodyType]
	}
	return ""
}

// collect проходит записи плана в том же порядке, что и разбор, и собирает
// параметры выхода.
//
// Два прохода нужны и здесь, но по другой причине: сначала помечаются ПУТИ,
// принадлежащие записям напрямую (`consumed`), и только потом исполняются
// ветки `sets` — иначе селектор с `sets` написал бы имя, принадлежащее
// соседней записи (правило №3 обратного хода: `packetEncoding=xudp` уезжал
// как `flow=…-udp443`).
func (st *emitState) collect() {
	entries := st.orderedEntries()
	for _, e := range entries {
		if e.Param == nil {
			continue
		}
		if p := st.emitPath(e.Param); p != "" {
			st.consumed[p] = true
		}
	}
	for _, e := range entries {
		st.emitEntry(e)
	}
}

// orderedEntries — записи плана в порядке исполнения разбора.
func (st *emitState) orderedEntries() []*Entry {
	out := make([]*Entry, 0, len(st.plan.Selectors)+len(st.plan.Rest))
	for i := range st.plan.Selectors {
		out = append(out, &st.plan.Selectors[i])
	}
	for i := range st.plan.Rest {
		out = append(out, &st.plan.Rest[i])
	}
	return out
}

// emitEntry — обратный ход ОДНОЙ записи.
func (st *emitState) emitEntry(e *Entry) {
	p := e.Param
	if p == nil {
		return
	}
	// Объявленный отказ от обратного хода — данные, а не исключение кода.
	if p.RoundTrip != nil && !*p.RoundTrip {
		return
	}
	// Запись, действующая только на разборе, на выходе молчит.
	if p.RoundTripOnly == roundTripParseOnly {
		return
	}
	// implicit — значение подставлено конвенцией, а не источником: в ссылку
	// не пишется, хотя в теле присутствует.
	if p.Implicit {
		return
	}
	// userinfo собирается отдельно: его источник — не query.
	name, ok := st.queryName(p)
	if !ok {
		// Запись без query-имени всё равно может нести ветку `sets`: именно
		// так селектор с `maps_to: null` остаётся селектором (правило №2).
		st.emitSets(e, "")
		return
	}
	if st.seen[name] {
		return
	}

	// emit_when: "always" сильнее ветки умолчания. «Разбор восстановит сам»
	// верно только для СВОЕГО разбора, а ссылка едет чужому клиенту
	// (правило №1 обратного хода).
	always := st.alwaysFor(e.Name, p)

	// compose — обращение extract: параметр собирается из нескольких путей.
	if p.Compose != nil {
		if v, ok := st.composeValue(p); ok {
			st.putEncoded(name, v, encodePasses(p))
		}
		return
	}

	// split_into⁻¹ — список ссылки собирается обратно из НЕСКОЛЬКИХ путей
	// тела. Ядро держит раздельно то, что ссылка пишет вместе (локальные
	// адреса туннеля: `ip` и `ipv6` против одного `address=`), и записи у
	// такой пары нет ни одной — истину несёт сам `split_into`.
	if len(p.SplitInto) > 0 {
		if v, ok := st.splitIntoValue(p); ok {
			st.put(name, v)
		}
		return
	}

	path := st.emitPath(p)
	if path == "" {
		st.emitSets(e, name)
		return
	}
	v, present := getPath(st.in.Body, path)
	if !present || v == nil || isEmptyValue(v) {
		if always {
			if def := st.alwaysValue(e.Name, p); def != "" {
				st.put(name, def)
			}
		}
		return
	}
	out, ok := st.serialize(p, v)
	if !ok {
		return
	}
	if !always && st.omitted(e.Name, out) {
		return
	}
	st.putEncoded(name, out, encodePasses(p))
}

// alwaysFor — объявлено ли у записи `emit_when: "always"`.
//
// Читается из двух мест: у самой записи (`emit_when`) и у секции
// (`emit.emit_when` по имени записи) — у схемы бывает своя норма поверх
// общего блока.
func (st *emitState) alwaysFor(entry string, p *registry.Param) bool {
	_, ok := st.alwaysSpec(entry, p)
	return ok
}

// alwaysSpec возвращает объявление `emit_when` записи: либо "always" (писать
// всегда, значение выводится), либо КОНКРЕТНОЕ написание, которое схема
// требует писать всегда.
//
// Второе нужно там, где обратная таблица неоднозначна ПО ПОСТРОЕНИЮ: у
// anytls все ветки `security` ведут в один и тот же `tls.enabled: true`
// (схема живёт только поверх TLS), и выбрать из них написание нечем — его
// называет сама схема.
func (st *emitState) alwaysSpec(entry string, p *registry.Param) (string, bool) {
	if s, ok := alwaysString(p.EmitWhen); ok {
		return s, true
	}
	if st.spec.EmitWhen != nil {
		if v, ok := st.spec.EmitWhen[entry]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

const emitAlways = "always"

func alwaysString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s, true
	}
	return "", false
}

// alwaysValue — что писать, когда `emit_when` требует параметр, а вывести
// значение из тела нечем.
//
// Порядок: объявленное схемой написание; затем ключ `value_map`,
// отображаемый в null, — он и есть написание «этого слоя нет» (у vless
// `encryption=none`).
func (st *emitState) alwaysValue(entry string, p *registry.Param) string {
	if s, ok := st.alwaysSpec(entry, p); ok && s != emitAlways {
		return s
	}
	keys := make([]string, 0, len(p.ValueMap))
	for k := range p.ValueMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if p.ValueMap[k] == nil && k != "" {
			return k
		}
	}
	return ""
}

// omitted — значение совпало с тем, что секция объявила «не писать».
//
// `emit.omit_default` — список ИМЁН записей; имя с двоеточием
// (`fp:random`) сужает правило до ОДНОГО значения. Сужение нужно потому, что
// «не писать параметр вовсе» и «не писать его при дефолтном значении» — разные
// вещи: `fp=random` у vless не пишется, а `fp=chrome` обязан доехать.
func (st *emitState) omitted(entry, val string) bool {
	for _, n := range st.spec.OmitDefault {
		if n == entry {
			return true
		}
		if i := strings.Index(n, ":"); i > 0 && n[:i] == entry && n[i+1:] == val {
			return true
		}
	}
	return false
}

// queryName — КАНОНИЧЕСКОЕ имя параметра в ссылке.
//
// Норма: канон = ПЕРВЫЙ источник записи (`query.<имя>`), а `aliases` —
// альтернативные написания ЧТЕНИЯ, и писать их на выходе нельзя (у
// `tcp_keep_alive` алиас `tcpKeepAlive`, у `headers` — `extra-headers`;
// оба принадлежат чужим диалектам, а не канону).
// Поверх неё действует `emit.names` — переименование НА ВЫХОДЕ, объявленное
// секцией: имя флага «не проверять сертификат» есть свойство СХЕМЫ, а не
// общего блока, и чужие клиенты читают у каждой схемы своё написание.
//
// Имя берётся только у записи, чей источник — query: остальные (host, port,
// userinfo, fragment) собираются своими местами.
func (st *emitState) queryName(p *registry.Param) (string, bool) {
	base := ""
	prefixes := []string{st.sourcePrefix()}
	if alt := st.altSourcePrefix(); alt != "" {
		prefixes = append(prefixes, alt)
	}
	for _, prefix := range prefixes {
		for _, s := range p.Source.ForForm(st.spec.Form) {
			if n, ok := paramTail(s, prefix); ok {
				base = n
				break
			}
		}
		if base == "" {
			for _, s := range p.Source.All() {
				if n, ok := paramTail(s, prefix); ok {
					base = n
					break
				}
			}
		}
		if base != "" {
			break
		}
	}
	if base == "" {
		return "", false
	}
	if st.names != nil {
		if renamed, ok := st.names[base]; ok {
			return renamed, true
		}
	}
	return base, true
}

// sourcePrefix — из какого пространства форма выхода берёт имена.
//
// У query-ссылки это `query.`, у формы-контейнера — `json.`: имена ключей
// контейнера объявлены ТЕМИ ЖЕ записями, что читают его на разборе, и второй
// таблицы под них не нужно.
func (st *emitState) sourcePrefix() string {
	if len(st.spec.JSONMap) > 0 {
		return prefixJSON
	}
	return prefixQuery
}

// altSourcePrefix — второе пространство имён формы-контейнера.
//
// Контейнер собирается из записей ДВУХ родов: свои читают `json.<ключ>`,
// подключённые блоки (транспорт, TLS) — `query.<имя>`, потому что блок общий
// и второго написания под контейнер у него нет. Обе группы называют одно и то
// же поле узла, и раскладка по ключам контейнера обязана видеть обе.
func (st *emitState) altSourcePrefix() string {
	if len(st.spec.JSONMap) > 0 {
		return prefixQuery
	}
	return ""
}

const (
	prefixQuery = "query."
	prefixJSON  = "json."
)

// paramTail — имя параметра из источника вида `<пространство>.<имя>`.
// Вложенный путь (`json.a.b`) именем параметра не является.
func paramTail(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	tail := s[len(prefix):]
	if tail == "" || strings.Contains(tail, ".") {
		return "", false
	}
	return tail, true
}

// splitIntoValue собирает список обратно из путей, названных `split_into`.
//
// Порядок элементов — порядок ОБЪЯВЛЕНИЯ ключей (отсортированный по имени
// пути), потому что карта Go отдаёт ключи случайно, а ссылка входит в
// identity узла. Значение по каждому пути может быть и скаляром, и списком:
// `take: "first"` на чтении брал один элемент, но правило обратного хода на
// этом не завязано.
func (st *emitState) splitIntoValue(p *registry.Param) (string, bool) {
	paths := make([]string, 0, len(p.SplitInto))
	for path := range p.SplitInto {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var parts []string
	for _, path := range paths {
		v, ok := getPath(st.in.Body, path)
		if !ok || v == nil || isEmptyValue(v) {
			continue
		}
		s := joinList(v, listSep(p))
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, listSep(p)), true
}

// composeValue собирает значение по шаблону `compose` из путей тела.
//
// `omit_when_empty` перечисляет ПУТИ, чьё отсутствие вырезает из шаблона весь
// хвост, начинающийся с предшествующего им литерала. Без этого `?ed=` уезжал
// бы в ссылку пустым у каждого ws-узла без early data: хвост пути принадлежит
// величине, которой нет, и чужой клиент прочёл бы его как часть пути.
func (st *emitState) composeValue(p *registry.Param) (string, bool) {
	tpl := p.Compose.Template
	if tpl == "" {
		return "", false
	}
	empty := map[string]bool{}
	vals := map[string]string{}
	any := false
	for _, path := range p.Compose.From {
		v, ok := getPath(st.in.Body, path)
		s := ""
		if ok && v != nil && !isEmptyValue(v) {
			s = toString(v)
		}
		if s == "" {
			empty[path] = true
		} else {
			any = true
		}
		vals[path] = s
	}
	if !any {
		return "", false
	}
	for _, path := range p.Compose.OmitWhenEmpty {
		if empty[path] {
			tpl = cutTail(tpl, "{"+path+"}")
		}
	}
	out := tpl
	for path, s := range vals {
		out = strings.ReplaceAll(out, "{"+path+"}", s)
	}
	if out == "" {
		return "", false
	}
	return out, true
}

// cutTail отрезает шаблон по месту подстановки вместе с литералом перед ней.
//
// Литералом считается всё от конца ПРЕДЫДУЩЕЙ подстановки (или от начала
// шаблона) до этой: он принадлежит вырезаемой величине и без неё смысла не
// имеет (`?ed=` без числа — мусор в пути).
func cutTail(tpl, placeholder string) string {
	at := strings.Index(tpl, placeholder)
	if at < 0 {
		return tpl
	}
	prev := strings.LastIndex(tpl[:at], "}")
	if prev < 0 {
		return ""
	}
	return tpl[:prev+1]
}

// emitSets — обратный ход ветки `sets`.
//
// Сопоставление: ветка выигрывает, когда ВСЕ её присваивания лежат в теле
// ровно такими, как объявлены. Ветка не пишется, когда все её пути
// принадлежат другим записям напрямую (правило №3) — на чтении побеждает
// хозяин пути, и обратный ход обязан повторить тот же выбор.
//
// Ветка со значением `null` в присваивании означает СНЯТИЕ пути: на обратном
// ходе ей соответствует отсутствие пути в теле.
func (st *emitState) emitSets(e *Entry, name string) {
	p := e.Param
	if len(p.Sets) == 0 {
		return
	}
	if name == "" {
		var ok bool
		name, ok = st.queryName(p)
		if !ok {
			return
		}
	}
	if st.seen[name] {
		return
	}
	forced, always := st.alwaysSpec(e.Name, p)
	// Схема назвала написание прямо: обратная таблица неоднозначна по
	// построению, и выбирать движку нечего. `omit_default` при этом остаётся
	// сильнее: «писать всегда» и «кроме вот этого написания» — не спор, а
	// два правила об одном параметре, и второе уточняет первое.
	if always && forced != emitAlways {
		if !st.omitted(e.Name, forced) {
			st.put(name, forced)
		}
		return
	}

	keys := make([]string, 0, len(p.Sets))
	for k := range p.Sets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	best := ""
	bestScore := -1
	for _, k := range keys {
		assigns := p.Sets[k]
		if len(assigns) == 0 {
			continue
		}
		// ПУСТОЙ ключ означает «параметра во входе не было» — это не
		// написание, а его отсутствие, и предлагать его чужому клиенту
		// нечего. Ветку под пустым ключом обратный ход не выбирает никогда:
		// она существует ради `sets` при молчащем источнике.
		if k == "" {
			continue
		}
		owned, score := st.setsMatch(assigns)
		if score < 0 {
			continue
		}
		// Все пути ветки принадлежат другим записям напрямую — писать нечего.
		if owned {
			continue
		}
		if score > bestScore {
			best, bestScore = k, score
		}
	}
	if bestScore < 0 {
		return
	}
	if best == "" {
		return
	}
	// `omit_default` сильнее «писать всегда»: это не спор, а уточнение.
	// Так у vless не пишется `security=reality` — REALITY-ветку в ссылке
	// объявляет наличие `pbk`, и второе имя того же факта только путает.
	if st.omitted(e.Name, best) {
		return
	}
	st.put(name, best)
}

// setsMatch проверяет, что присваивания ветки лежат в теле.
//
// Возвращает (все ли пути ветки заняты другими записями напрямую, число
// СОВПАВШИХ присваиваний либо -1, если ветка не подошла).
func (st *emitState) setsMatch(assigns map[string]interface{}) (bool, int) {
	paths := make([]string, 0, len(assigns))
	for k := range assigns {
		if isReservedAssignKey(k) {
			continue
		}
		paths = append(paths, k)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return false, -1
	}
	owned := true
	score := 0
	for _, path := range paths {
		want := assigns[path]
		got, present := getPath(st.in.Body, path)
		if want == nil {
			// Присваивание снимает путь: на обратном ходе ему соответствует
			// ОТСУТСТВИЕ пути в теле.
			if present && !isEmptyValue(got) {
				return false, -1
			}
			score++
			// Снятие пути — не владение им: путь никто не занял.
			owned = false
			continue
		}
		if !present {
			return false, -1
		}
		if !sameJSON(want, normalizeNumber(got)) {
			return false, -1
		}
		score++
		if !st.consumed[path] {
			owned = false
		}
	}
	return owned, score
}

// serialize приводит значение тела к тексту параметра ссылки.
//
// `emit_as` объявляет сериализацию там, где тело хранит НЕ строку: угадывать
// по типу нельзя — булев `true` у одной схемы пишется как `1`, у другой
// словом, и это свойство ДИАЛЕКТА, а не типа.
func (st *emitState) serialize(p *registry.Param, v interface{}) (string, bool) {
	// `emit_as: raw` — писать значение тела КАК ЕСТЬ, без обращения таблицы.
	// Нужно там, где два написания входа читаются в одно значение, а на
	// выходе канон один: `h2` и `http` у контейнера дают один транспорт, и
	// обращение вернуло бы устаревшее написание.
	if p.EmitAs == emitAsRaw {
		return toString(v), true
	}
	// value_map⁻¹ — только для ИНЪЕКТИВНОЙ таблицы; ветка со значением null
	// из обращения выпадает (она означает «ключа нет»).
	if s, ok := st.reverseValueMap(p, v); ok {
		// Составное имя (ключ, у которого есть ещё и ветка `sets`) пишется
		// только когда ВСЁ, что оно объявляет, принадлежит ему. Если пути его
		// ветки заняты другими записями напрямую, истину этих полей несут
		// они, и составное имя сказало бы то же самое вторым голосом —
		// правило №3 обратного хода (`packetEncoding=xudp` уезжал как
		// `flow=…-udp443`).
		if assigns, has := p.Sets[s]; has {
			if owned, score := st.setsMatch(assigns); score < 0 || owned {
				return toString(v), true
			}
		}
		return s, s != ""
	}
	// Карта тела в строку пар: обращение `extract` с группами `$key`/`$value`.
	// Разделитель пар — `list.sep`, разделитель внутри пары объявляется
	// `emit_pair_sep`: у заголовков это «: », и вывести его из регулярки
	// нельзя (она описывает ЧТЕНИЕ, где после двоеточия допустим любой
	// пробельный хвост).
	if p.EmitAs == emitAsPairs {
		return joinPairs(v, listSep(p), p.EmitPairSep), true
	}
	if p.EmitNormalize != "" {
		if p.EmitAs == emitAsJoin || p.List != nil {
			return joinNormalized(v, listSep(p), p.EmitNormalize), true
		}
		return emitNormalizeValue(p.EmitNormalize, toString(v)), true
	}
	switch p.EmitAs {
	case emitAsJoin:
		return joinList(v, listSep(p)), true
	case emitAsBool01:
		if boolOf(v) {
			return "1", true
		}
		return "", false
	case emitAsJSON:
		raw, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return string(raw), true
	case emitAsRaw:
		return toString(v), true
	}
	switch t := v.(type) {
	case bool:
		if !t {
			return "", false
		}
		return "true", true
	case []interface{}, []string:
		return joinList(v, listSep(p)), true
	}
	return toString(v), true
}

const (
	emitAsPairs  = "pairs"
	emitAsJoin   = "join"
	emitAsBool01 = "bool01"
	emitAsJSON   = "json"
	emitAsRaw    = "raw"
)

func listSep(p *registry.Param) string {
	if p.List != nil && p.List.Sep != "" {
		return p.List.Sep
	}
	return ","
}

// emitNormalizeValue — обращение normalize, объявленное записью.
//
// Правила перечислены здесь, но выбирает их РЕЕСТР: имя правила приезжает с
// диска, и ни одно из них не названо по схеме.
func emitNormalizeValue(kind, v string) string {
	switch kind {
	case emitNormPortRangeURI:
		// Форма ядра `low:high` обратно в написание ссылки `low-high`;
		// вырожденная пара `N:N` — это «ровно этот порт», и ссылка пишет
		// его одним числом.
		t := strings.TrimSpace(v)
		if t == "" {
			return v
		}
		i := strings.Index(t, ":")
		if i < 0 {
			return t
		}
		// Вырожденная пара `N:N` пишется `N-N`, а не одним числом: чужой
		// клиент читает дефис как признак ДИАПАЗОНА, и одиночное число
		// сказало бы ему «порт-хоппинга нет», хотя узел его несёт.
		return t[:i] + "-" + t[i+1:]
	}
	return v
}

const emitNormPortRangeURI = "port_range_spec_uri"

// joinNormalized склеивает список, применяя обращение normalize к каждому
// элементу.
func joinNormalized(v interface{}, sep, kind string) string {
	switch t := v.(type) {
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, one := range t {
			s := emitNormalizeValue(kind, toString(one))
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, sep)
	case []string:
		parts := make([]string, 0, len(t))
		for _, one := range t {
			parts = append(parts, emitNormalizeValue(kind, one))
		}
		return strings.Join(parts, sep)
	}
	return emitNormalizeValue(kind, toString(v))
}

// joinPairs печатает карту тела строкой пар в детерминированном порядке.
//
// Порядок — лексикографический по ключу: карта Go отдаёт ключи случайно, а
// ссылка входит в identity узла и обязана быть одинаковой между запусками.
func joinPairs(v interface{}, sep, pairSep string) string {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return toString(v)
	}
	if pairSep == "" {
		pairSep = ": "
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		val := toString(obj[k])
		// Пара, несущая сам разделитель списка, порвала бы строку на чтении:
		// такая запись в ссылку не едет (прежний путь делал то же молча).
		if strings.Contains(val, sep) || strings.Contains(val, "\x00") {
			continue
		}
		parts = append(parts, k+pairSep+val)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, sep)
}

func joinList(v interface{}, sep string) string {
	switch t := v.(type) {
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, one := range t {
			s := toString(one)
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, sep)
	case []string:
		return strings.Join(t, sep)
	}
	return toString(v)
}

// reverseValueMap — обратная таблица значений.
//
// Применяется ТОЛЬКО к инъективной таблице: если два ключа ведут в одно
// значение, обращение неоднозначно и его не делают вовсе. Ветка со значением
// null из обращения выпадает по построению.
func (st *emitState) reverseValueMap(p *registry.Param, v interface{}) (string, bool) {
	// Явная обратная таблица сильнее выведенной: она объявлена схемой именно
	// потому, что вывести нечего.
	if len(p.EmitValueMap) > 0 {
		if s, ok := p.EmitValueMap[toString(v)]; ok {
			return s, true
		}
	}
	if len(p.ValueMap) == 0 {
		return "", false
	}
	rev := map[string]string{}
	dup := map[string]bool{}
	keys := make([]string, 0, len(p.ValueMap))
	for k := range p.ValueMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := p.ValueMap[k]
		if val == nil {
			continue
		}
		s := toString(val)
		if _, exists := rev[s]; exists {
			dup[s] = true
			continue
		}
		rev[s] = k
	}
	s := toString(v)
	if dup[s] {
		return "", false
	}
	if k, ok := rev[s]; ok {
		return k, true
	}
	return "", false
}

// buildUserInfo собирает userinfo из путей, объявленных `userinfo.into`.
//
// Обращение `into⁻¹`: позиции into называют пути тела, `split.sep` — чем их
// склеить, `emit.userinfo` — выходную форму (raw / base64, паддинг объявлен
// явно, потому что стороны пишут его по-разному).
func (st *emitState) buildUserInfo() {
	ui := st.plan.Mapper.UserInfo
	if ui == nil {
		return
	}
	// Объявленный отказ писать userinfo: канон схемы кладёт то же значение в
	// query, и запись в оба места отдала бы секрет дважды.
	if ui.Emit != nil && !*ui.Emit {
		return
	}
	targets := ui.Into
	if len(targets) == 0 && ui.SingleInto != "" {
		targets = []string{ui.SingleInto}
	}
	parts := make([]string, 0, len(targets))
	any := false
	for _, target := range targets {
		s := ""
		if target != "" {
			if v, ok := getPath(st.in.Body, target); ok && v != nil {
				s = toString(v)
			}
		}
		if s != "" {
			any = true
		}
		parts = append(parts, s)
	}
	if !any {
		return
	}
	sep := ":"
	if ui.Split != nil && ui.Split.Sep != "" {
		sep = ui.Split.Sep
	}
	// Одиночный userinfo: секция объявила, КАКОЕ поле он несёт
	// (`single_into`). Если заполнено только оно, ссылка пишет его БЕЗ
	// разделителя — ровно так её и прочтут обратно, а `:onlypass` чужой
	// клиент разберёт как пустое имя плюс пароль, то есть иначе.
	// `keep_empty_tail` — явное объявление схемы «разделитель писать всегда»,
	// и оно сильнее общей конвенции одиночного userinfo: вторая существует
	// ради схем, ничего не объявивших.
	if ui.SingleInto != "" && !st.keepEmptyTail() {
		lone := true
		for i, target := range targets {
			if target == ui.SingleInto {
				continue
			}
			if i < len(parts) && parts[i] != "" {
				lone = false
				break
			}
		}
		if lone {
			for i, target := range targets {
				if target == ui.SingleInto && i < len(parts) && parts[i] != "" {
					parts = []string{parts[i]}
					break
				}
			}
		}
	}
	// Хвостовые пустые компоненты не пишутся: `user:` и `user` для чужого
	// клиента одно и то же, а лишний разделитель ломает разбор у части
	// панелей. Схема вправе объявить обратное (`keep_empty_tail`): у socks4
	// пароля нет по протоколу, но разделитель клиенты пишут всегда.
	if !st.keepEmptyTail() {
		for len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
	}
	// Разделитель ЗНАЧИМ только там, где секция объявила резку: у схемы с
	// одним компонентом userinfo двоеточие — законная часть значения
	// (`p@ss:word` у anytls), и оставить его сырым значит сказать чужому
	// клиенту «здесь пара», которой нет. Поэтому решение об экранировании
	// принимает объявление секции, а не код.
	sepLiteral := len(parts) > 1
	raw := strings.Join(parts, sep)
	st.userinfo = st.encodeUserInfoWith(raw, sepLiteral, sep)
}

// encodeUserInfoWith применяет объявленную выходную форму userinfo.
//
// `sepLiteral` — писать ли разделитель сырым: он значим ровно тогда, когда
// компонентов больше одного.
func (st *emitState) encodeUserInfoWith(raw string, sepLiteral bool, sep string) string {
	spec := st.spec.UserInfo
	if spec == nil || spec.Form == "" || spec.Form == userInfoRaw {
		return percentEscapeUserInfo(raw, sepLiteral, sep)
	}
	enc := base64.StdEncoding
	if spec.Padding != nil && !*spec.Padding {
		enc = base64.RawStdEncoding
	}
	return enc.EncodeToString([]byte(raw))
}

const userInfoRaw = "raw"

// percentEscapeUserInfo экранирует userinfo по правилам ссылки.
//
// Разделитель компонентов остаётся сырым только когда компонентов несколько;
// иначе он — обычный символ значения и экранируется вместе с прочими.
func percentEscapeUserInfo(s string, sepLiteral bool, sep string) string {
	var b strings.Builder
	left := 0
	if sepLiteral {
		left = 1
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		// Разделитель остаётся сырым ровно столько раз, сколько границ между
		// компонентами: двоеточие ВНУТРИ последнего компонента — часть
		// значения (`p:ss` у http), и оставить его сырым значило бы сказать
		// чужому клиенту о третьем компоненте, которого нет.
		if left > 0 && len(sep) == 1 && c == sep[0] {
			left--
			b.WriteByte(c)
			continue
		}
		if shouldEscapeUserInfo(c) {
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0xf])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

const upperHex = "0123456789ABCDEF"

func shouldEscapeUserInfo(c byte) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return false
	}
	switch c {
	case '-', '.', '_', '~', '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=':
		return false
	}
	return true
}

// encodePasses — сколько раз значение обязано быть закодировано на выходе.
//
// `decode_extra.passes` объявляет, сколько проходов декода применяет ЧТЕНИЕ
// поверх декодера формы. Обратный ход обязан столько же раз закодировать:
// путь, объявивший два прохода, вернётся собой только будучи закодированным
// дважды (`/%2F` → `%2F%252F`), иначе круг съедает один уровень и сервер
// получает другой путь.
//
// `until_stable` числом проходов не выражается — такое значение кодируется
// один раз: «до стабилизации» означает «сколько бы ни было», и добавить
// уровней на выходе значит выдумать их.
//
// Проходы СУММИРУЮТСЯ с декодом самой формы: печать query — это первый
// уровень, `decode_extra` объявляет то, что читается ПОВЕРХ него.
func encodePasses(p *registry.Param) int {
	if p.DecodeExtra == nil {
		return 1
	}
	n, untilStable := p.DecodeExtra.PassCount()
	if untilStable || n < 1 {
		return 1
	}
	return n + 1
}

// put добавляет параметр, если имя ещё не занято.
func (st *emitState) put(name, val string) {
	if name == "" || val == "" {
		return
	}
	if st.seen[name] {
		return
	}
	st.seen[name] = true
	st.pairs = append(st.pairs, emitPair{name: name, val: val})
}

// putEncoded добавляет параметр, предварительно закодировав его столько раз,
// сколько проходов декода объявило чтение (сверх одного, который сделает
// печать query).
func (st *emitState) putEncoded(name, val string, passes int) {
	for i := 1; i < passes; i++ {
		// Лишний проход добавляется ТОЛЬКО значению, которое его переживёт
		// иначе: `%` в значении — единственный символ, который второй проход
		// декода съест. Кодировать всё подряд значило бы менять вид каждой
		// ссылки ради одного кейса из трёхсот, а чужой клиент, делающий один
		// проход, получил бы путь с лишними `%25`.
		if !strings.Contains(val, "%") {
			break
		}
		val = queryEscape(val)
	}
	st.put(name, val)
}

// encodeQuery печатает query в объявленном порядке.
//
// `param_order`: правило "alphabetical" либо ПЕРЕЧЕНЬ имён. Имена, не
// попавшие в перечень, идут следом по алфавиту.
//
// Пробел на выходе — `%20` (норма PRIMITIVES §0.6): `url.Values.Encode()`
// пишет `+`, и чужие клиенты читают его буквально в пути.
func (st *emitState) encodeQuery() string {
	order := st.paramOrder()
	pairs := st.orderPairs(order)
	var b strings.Builder
	for _, p := range pairs {
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(queryEscape(p.name))
		b.WriteByte('=')
		b.WriteString(queryEscape(p.val))
	}
	return b.String()
}

// paramOrder — перечень имён либо nil при правиле «алфавит».
func (st *emitState) paramOrder() []string {
	if len(st.spec.ParamOrder) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(st.spec.ParamOrder, &list); err == nil {
		return list
	}
	return nil
}

// orderPairs раскладывает собранные параметры по объявленному порядку.
func (st *emitState) orderPairs(order []string) []emitPair {
	if len(order) == 0 {
		out := append([]emitPair{}, st.pairs...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].name < out[j].name })
		return out
	}
	index := map[string]int{}
	for i, n := range order {
		index[n] = i
	}
	var named, rest []emitPair
	for _, p := range st.pairs {
		if _, ok := index[p.name]; ok {
			named = append(named, p)
		} else {
			rest = append(rest, p)
		}
	}
	sort.SliceStable(named, func(i, j int) bool { return index[named[i].name] < index[named[j].name] })
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].name < rest[j].name })
	return append(named, rest...)
}

// queryEscape — percent-кодирование значения query с `%20` вместо `+`.
func queryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// emitContainer собирает форму-контейнер: base64(JSON) вместо query-ссылки.
//
// Карта `emit.json_map` называет «ключ контейнера → путь тела либо литерал»,
// `emit.json_always` — ключи, которые клиенты ждут ДАЖЕ ПУСТЫМИ (опускание
// такого ключа ломает чтение у панелей, а не экономит байты).
// `emit.param_order` задаёт порядок ключей и здесь.
func (st *emitState) emitContainer() (string, error) {
	st.jsonObj = map[string]interface{}{}
	always := map[string]interface{}{}
	for k, v := range st.spec.JSONAlways {
		always[k] = v
	}

	// Значения полей, которые контейнер берёт НЕ из тела напрямую, а через
	// те же записи таблицы: сначала собирается обычный набор параметров,
	// потом он раскладывается по ключам контейнера.
	st.collect()
	byName := map[string]string{}
	for _, p := range st.pairs {
		byName[p.name] = p.val
	}

	keys := make([]string, 0, len(st.spec.JSONMap))
	for k := range st.spec.JSONMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ref := st.spec.JSONMap[key]
		val, ok := st.containerValue(ref, byName)
		if !ok {
			if def, has := always[key]; has {
				st.jsonObj[key] = def
			}
			continue
		}
		st.jsonObj[key] = val
	}
	for k, v := range always {
		if _, has := st.jsonObj[k]; !has {
			st.jsonObj[k] = v
		}
	}
	raw, err := st.marshalContainer()
	if err != nil {
		return "", err
	}
	return st.scheme + "://" + base64.StdEncoding.EncodeToString(raw), nil
}

// containerValue читает значение одного ключа контейнера.
//
// Ссылка — путь ТЕЛА (`tls.server_name`), имя собранного параметра
// (`$param.<имя>`), метка (`$label`), либо литерал (`=…`).
func (st *emitState) containerValue(ref string, byName map[string]string) (interface{}, bool) {
	switch {
	case ref == "":
		return nil, false
	case ref == containerLabel:
		if st.in.Label == "" {
			return nil, false
		}
		return st.in.Label, true
	case strings.HasPrefix(ref, containerLiteral):
		return ref[len(containerLiteral):], true
	case strings.HasPrefix(ref, containerParam):
		v, ok := byName[ref[len(containerParam):]]
		if !ok || v == "" {
			return nil, false
		}
		return v, true
	}
	v, ok := getPath(st.in.Body, ref)
	if !ok || v == nil || isEmptyValue(v) {
		return nil, false
	}
	return normalizeNumber(v), true
}

const (
	containerLabel   = "$label"
	containerLiteral = "="
	containerParam   = "$param."
)

// marshalContainer печатает контейнер в объявленном порядке ключей.
func (st *emitState) marshalContainer() ([]byte, error) {
	order := st.paramOrder()
	names := make([]string, 0, len(st.jsonObj))
	if len(order) > 0 {
		for _, n := range order {
			if _, ok := st.jsonObj[n]; ok {
				names = append(names, n)
			}
		}
		var rest []string
		for n := range st.jsonObj {
			if _, ok := indexOf(order, n); !ok {
				rest = append(rest, n)
			}
		}
		sort.Strings(rest)
		names = append(names, rest...)
	} else {
		for n := range st.jsonObj {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, n := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(n)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		v, err := json.Marshal(st.jsonObj[n])
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func indexOf(list []string, want string) (int, bool) {
	for i, v := range list {
		if v == want {
			return i, true
		}
	}
	return 0, false
}

// hostJoin — адрес с портом; IPv6 в скобках (норма §3 и на выходе).
func hostJoin(host string, port int) string {
	if port <= 0 {
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			return "[" + host + "]"
		}
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func intOf(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

func boolOf(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "1" || strings.EqualFold(t, "true") || strings.EqualFold(t, "yes")
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return false
}

// KindFromBody восстанавливает РОД узла по ТЕЛУ.
//
// `kind_when` объявляет род условием на ВХОД (`query.jc` и родня), и при
// разборе иначе нельзя: ссылка `awg://` с негодными значениями оставляет тело,
// неотличимое от обычного WireGuard. Но узел, сохранённый ТЕЛОМ, входа больше
// не имеет, а род ему всё равно нужен — его читает `emit.form_from`.
//
// Соответствие «источник → путь тела» берётся из САМИХ записей секции: имя
// `query.jc` принадлежит записи, у которой есть `maps_to`. Второй список
// awg-имён в коде разъехался бы с первым при первом же новом наборе, и род
// стал бы зависеть от того, какой из двух забыли дописать.
//
// Это ПРИБЛИЖЕНИЕ, и лучшего из тела не получить: условие по входу, чьё
// значение санитайзер снял, здесь не восстановится. Ровно поэтому норма
// требует хранить род рядом с телом (`Origin.Kind`), а этот вывод остаётся
// последним звеном цепочки восстановления.
func KindFromBody(spec map[string]map[string]interface{}, body map[string]interface{}) string {
	if len(spec) == 0 || len(body) == 0 {
		return ""
	}
	names := make([]string, 0, len(spec))
	for name := range spec {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if kindCondHoldsInBody(spec[name], body) {
			return name
		}
	}
	return ""
}

// kindCondHoldsInBody — то же условие, прочитанное по телу.
//
// Поддерживается только `any_set`: остальные формы условия спрашивают про
// НАПИСАНИЕ входа (`{matches: …}` по схеме), которого у тела нет по
// построению, и выдумывать ответ хуже, чем не отвечать.
func kindCondHoldsInBody(cond map[string]interface{}, body map[string]interface{}) bool {
	for key, want := range cond {
		if strings.HasPrefix(key, "$") {
			continue
		}
		if key != emitKeyAnySet {
			return false
		}
		list, _ := want.([]interface{})
		for _, item := range list {
			name, _ := item.(string)
			tail, ok := paramTail(name, prefixQuery)
			if !ok {
				continue
			}
			// Имя параметра и имя поля тела совпадают не всегда; здесь
			// проверяется НАЛИЧИЕ пути, а не значение — `jc: 0` есть
			// законное «мусор выключен» у настоящего AmneziaWG-узла.
			if _, exists := getPath(body, tail); exists {
				return true
			}
		}
		return false
	}
	return false
}
