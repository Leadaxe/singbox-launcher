package linkmap

// План маппера: развёрнутая, готовая к исполнению форма секции реестра.
//
// Разворачивание `include`, подстановка `$base`, раскладка записей на проход A
// (селекторы) и проход B, компиляция регулярок делаются ОДИН РАЗ при загрузке
// (MAPPER_ENGINE.md, «Рекомендации» §1): реестр вшит в бинарь и в рантайме не
// меняется.
//
// Порядок записей в плане НОРМАТИВЕН (MAPPER_ENGINE.md §7): Go-карта отдаёт
// ключи в случайном порядке, поэтому порядок объявления восстанавливается из
// ИСХОДНОГО текста JSON — иначе два запуска дали бы разные тела на конфликте
// двух записей в один путь.
//
// Код не знает ни одной схемы по имени: всё, чем он оперирует, приезжает с
// диска.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/contract"
	"singbox-launcher/core/config/registry"
)

// Entry — одна запись плана: параметр таблицы вместе со своим именем и
// позицией объявления.
type Entry struct {
	// Name — имя записи: канон для aliases и для кода uri_param_unknown.
	Name string
	// Param — сама запись.
	Param *registry.Param
	// Decl — позиция объявления (порядок из исходного JSON). Меньше = раньше.
	Decl int
	// From — откуда приехала запись: "" для записей протокола, иначе имя
	// include-блока ("tls#uri"). Нужна диагностике и трассе.
	From string
}

// Plan — секция-маппер, развёрнутая для исполнения.
type Plan struct {
	// Mapper — исходная секция.
	Mapper *registry.Mapper
	// Selectors — записи прохода A, в порядке объявления.
	Selectors []Entry
	// Rest — записи прохода B, в порядке объявления.
	Rest []Entry
	// Declared — множество ОБЪЯВЛЕННЫХ имён параметров (fold-case), включая
	// aliases и включая блоки, которые не применились по when
	// (MAPPER_ENGINE.md §8): иначе tcp_keep_alive* давали бы по три info на
	// каждый узел.
	Declared map[string]bool
	// BodyOrder — порядок ключей тела схемы; нужен канону сериализации.
	BodyOrder []string
}

// PlanSet — планы всех секций реестра.
type PlanSet struct {
	byKey map[string]*Plan
	set   *registry.MapperSet
}

// Mappers — исходный набор секций.
func (p *PlanSet) Mappers() *registry.MapperSet {
	if p == nil {
		return nil
	}
	return p.set
}

// Plan возвращает план секции схемы по виду источника.
func (p *PlanSet) Plan(scheme, kind string) (*Plan, bool) {
	if p == nil {
		return nil, false
	}
	pl, ok := p.byKey[scheme+"#"+kind]
	return pl, ok
}

// Schemes — схемы, у которых есть планы, по алфавиту.
func (p *PlanSet) Schemes() []string {
	if p == nil {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for key := range p.byKey {
		scheme := key
		if i := strings.Index(key, "#"); i >= 0 {
			scheme = key[:i]
		}
		if !seen[scheme] {
			seen[scheme] = true
			out = append(out, scheme)
		}
	}
	sort.Strings(out)
	return out
}

// blockFiles — файлы общих блоков. Список явный по той же причине, что и
// protocolFiles в реестре: embed.FS не обходится по шаблону дёшево, а список
// должен быть виден глазами.
var blockFiles = []string{"tls", "transports", "dialer", "multiplex"}

// BuildPlans разворачивает все секции-мапперы реестра в исполняемые планы.
//
// Отсутствие секции `mappers` у схемы — НЕ ошибка: реестр переезжает волнами,
// и до перевода схемы движок просто не находит плана (схема идёт старым
// парсером).
func BuildPlans(set *registry.MapperSet, bodyOrder func(scheme string) []string) (*PlanSet, error) {
	blocks, err := loadBlocks()
	if err != nil {
		return nil, err
	}
	out := &PlanSet{byKey: map[string]*Plan{}, set: set}
	for _, scheme := range set.Schemes() {
		for _, kind := range set.Kinds(scheme) {
			m, ok := set.Mapper(scheme, kind)
			if !ok {
				continue
			}
			pl, err := buildPlan(scheme, kind, m, blocks)
			if err != nil {
				return nil, err
			}
			if bodyOrder != nil {
				pl.BodyOrder = bodyOrder(scheme)
			}
			out.byKey[scheme+"#"+kind] = pl
		}
	}
	return out, nil
}

// blockTable — один общий блок в одном диалекте: имя записи → запись, плюс
// порядок объявления.
type blockTable struct {
	names  []string
	params map[string]*registry.Param
}

// blockSet — все общие блоки: "tls" → "uri" → таблица; у транспортов есть
// вложенный уровень ("transports" → "uri" → "ws" → таблица), который
// разворачивается в плоское имя "transports.ws".
type blockSet map[string]map[string]*blockTable

// loadBlocks читает `blocks` из файлов общих блоков.
//
// Вложенность у транспортов на один уровень глубже, чем у tls: там сперва тип
// транспорта ("ws", "grpc", "$selector"), и только под ним записи. Различить
// их можно НЕ по имени файла (кода, знающего имена, здесь быть не должно), а
// по форме значения: если значение ключа — объект, все значения которого тоже
// объекты с полем `source`, это таблица; иначе это ещё один уровень группы.
func loadBlocks() (blockSet, error) {
	out := blockSet{}
	for _, name := range blockFiles {
		data, err := contract.ReadRegistry(name + ".json")
		if err != nil {
			// Файла может не быть (multiplex/dialer заводятся волнами).
			continue
		}
		var f struct {
			Blocks map[string]json.RawMessage `json:"blocks"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("linkmap: %s.json: blocks: %w", name, err)
		}
		for dialect, raw := range f.Blocks {
			if dialect == "note" {
				continue
			}
			// Именованные таблицы value_map ("fp_dialect") — не диалект: у них
			// нет записей с source. Их читает $ref, а не include.
			group, table, err := classifyBlock(raw)
			if err != nil {
				return nil, fmt.Errorf("linkmap: %s.json: blocks.%s: %w", name, dialect, err)
			}
			switch {
			case table != nil:
				putBlock(out, name, dialect, table)
			case group != nil:
				for sub, t := range group {
					putBlock(out, name+"."+sub, dialect, t)
				}
			}
		}
	}
	return out, nil
}

func putBlock(bs blockSet, block, dialect string, t *blockTable) {
	if bs[block] == nil {
		bs[block] = map[string]*blockTable{}
	}
	bs[block][dialect] = t
}

// classifyBlock различает таблицу записей и группу таблиц ПО ФОРМЕ значения,
// а не по имени: имён схем и блоков код не знает.
//
// Возвращает ровно одно из двух (или обоих nil, если это не блок вовсе —
// например именованная таблица value_map с ключами prefix/strip).
func classifyBlock(raw json.RawMessage) (map[string]*blockTable, *blockTable, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, nil, nil
	}
	names, err := objectKeyOrder(raw)
	if err != nil {
		return nil, nil, err
	}

	// Проба «это таблица записей»: хотя бы одно значение — объект с `source`.
	isTable := false
	for _, v := range top {
		if hasSourceKey(v) {
			isTable = true
			break
		}
	}
	if isTable {
		t, err := parseBlockTable(raw, names)
		return nil, t, err
	}

	// Иначе — группа: каждое значение может быть таблицей.
	group := map[string]*blockTable{}
	for name, v := range top {
		if name == "note" {
			continue
		}
		subNames, err := objectKeyOrder(v)
		if err != nil {
			continue
		}
		inner := false
		var im map[string]json.RawMessage
		if err := json.Unmarshal(v, &im); err != nil {
			continue
		}
		for _, iv := range im {
			if hasSourceKey(iv) {
				inner = true
				break
			}
		}
		if !inner {
			continue
		}
		t, err := parseBlockTable(v, subNames)
		if err != nil {
			return nil, nil, err
		}
		group[name] = t
	}
	if len(group) == 0 {
		return nil, nil, nil
	}
	return group, nil, nil
}

// hasSourceKey — значение является записью таблицы (объект с ключом `source`).
func hasSourceKey(raw json.RawMessage) bool {
	var probe struct {
		Source json.RawMessage `json:"source"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return len(probe.Source) > 0
}

// parseBlockTable разбирает таблицу записей блока.
//
// Прозаические ключи (`note`, `impl`) живут рядом с записями и записями не
// являются: они пропускаются по ФОРМЕ значения (строка), а не по имени —
// список имён в коде разъехался бы с реестром при первом новом ключе.
func parseBlockTable(raw json.RawMessage, names []string) (*blockTable, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	params := map[string]*registry.Param{}
	kept := make([]string, 0, len(names))
	for _, n := range names {
		v, ok := top[n]
		if !ok || !hasSourceKey(v) {
			continue
		}
		var p registry.Param
		if err := json.Unmarshal(v, &p); err != nil {
			return nil, fmt.Errorf("запись %q: %w", n, err)
		}
		params[n] = &p
		kept = append(kept, n)
	}
	return &blockTable{names: kept, params: params}, nil
}

// buildPlan разворачивает одну секцию.
func buildPlan(scheme, kind string, m *registry.Mapper, blocks blockSet) (*Plan, error) {
	pl := &Plan{Mapper: m, Declared: map[string]bool{}}

	var all []Entry
	decl := 0

	// include ДО записей протокола: так блок tls#uri со своим security
	// объявляется раньше, а запись протокола с тем же именем перекрывает его
	// (буквально своей позицией, без молчаливого слияния).
	for _, inc := range m.Include {
		name, dialect := splitInclude(inc, kind)
		table := lookupBlock(blocks, name, dialect)
		if table == nil {
			return nil, fmt.Errorf("linkmap: %s#%s: include %q не найден", scheme, kind, inc)
		}
		for _, n := range table.names {
			all = append(all, Entry{Name: n, Param: table.params[n], Decl: decl, From: inc})
			decl++
		}
	}

	// Записи самой секции — в порядке объявления в файле.
	protoNames, err := mapperParamOrder(scheme, kind)
	if err != nil {
		return nil, err
	}
	for _, n := range protoNames {
		p := m.Params[n]
		if p == nil {
			continue
		}
		all = append(all, Entry{Name: n, Param: p, Decl: decl})
		decl++
	}

	// Проходы A/B: селекторы отдельно, оба — в порядке объявления.
	for _, e := range all {
		if e.Param.Selector {
			pl.Selectors = append(pl.Selectors, e)
		} else {
			pl.Rest = append(pl.Rest, e)
		}
		declare(pl.Declared, e.Name)
		for _, a := range e.Param.Aliases {
			declare(pl.Declared, a)
		}
		// Имя параметра ссылки может отличаться от имени записи: запись
		// читает `query.<name>`, и объявленным считается именно он.
		for _, src := range e.Param.Source.All() {
			if strings.HasPrefix(src, "query.") {
				declare(pl.Declared, strings.TrimPrefix(src, "query."))
			}
		}
	}

	// Стабильная сортировка по позиции объявления: порядок в плане НОРМАТИВЕН,
	// и он не должен зависеть от того, как записи легли в срез.
	sortEntries(pl.Selectors)
	sortEntries(pl.Rest)
	return pl, nil
}

func declare(set map[string]bool, name string) {
	if name == "" || strings.HasPrefix(name, "$") {
		return
	}
	set[strings.ToLower(name)] = true
}

func sortEntries(list []Entry) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].Decl < list[j].Decl })
}

// splitInclude разбирает "tls#uri" / "transports.ws#uri" / "tls"
// (без диалекта — берётся вид источника секции).
func splitInclude(inc, kind string) (string, string) {
	if i := strings.Index(inc, "#"); i >= 0 {
		return inc[:i], inc[i+1:]
	}
	return inc, kind
}

// lookupBlock достаёт таблицу блока; у «транспортов целиком» ("transports")
// склеивает все подблоки в объявленном порядке.
func lookupBlock(blocks blockSet, name, dialect string) *blockTable {
	if byDialect, ok := blocks[name]; ok {
		if t, ok := byDialect[dialect]; ok {
			return t
		}
	}
	// "transports#uri" без указания подблока = все подблоки: селектор первым
	// (он объявлен первым в файле), затем типы транспортов.
	prefix := name + "."
	var subs []string
	for key := range blocks {
		if strings.HasPrefix(key, prefix) {
			if _, ok := blocks[key][dialect]; ok {
				subs = append(subs, key)
			}
		}
	}
	if len(subs) == 0 {
		return nil
	}
	sort.Strings(subs)
	merged := &blockTable{params: map[string]*registry.Param{}}
	// $selector-подблок идёт первым: его записи строят transport.type, по
	// которому дальше проверяется when остальных.
	ordered := make([]string, 0, len(subs))
	for _, s := range subs {
		if strings.HasSuffix(s, ".$selector") {
			ordered = append(ordered, s)
		}
	}
	for _, s := range subs {
		if !strings.HasSuffix(s, ".$selector") {
			ordered = append(ordered, s)
		}
	}
	for _, s := range ordered {
		t := blocks[s][dialect]
		sub := strings.TrimPrefix(s, prefix)
		for _, n := range t.names {
			// Имя записи в склейке уточняется подблоком: у ws, http и
			// httpupgrade есть свой `path` и свой `host`, и в одном плоском
			// пространстве имён они обязаны различаться. Для объявленности
			// (uri_param_unknown) берётся source, а не это имя.
			key := sub + "." + n
			merged.names = append(merged.names, key)
			merged.params[key] = t.params[n]
		}
	}
	return merged
}

// mapperParamOrder возвращает имена params секции В ПОРЯДКЕ ОБЪЯВЛЕНИЯ.
//
// Go-карта порядка не хранит, а он нормативен (MAPPER_ENGINE.md §7), поэтому
// он восстанавливается из исходного текста файла — единственного места, где
// порядок объявления вообще есть.
func mapperParamOrder(scheme, kind string) ([]string, error) {
	data, err := contract.ReadRegistry("protocols/" + protocolFileFor(scheme) + ".json")
	if err != nil {
		return nil, err
	}
	var root struct {
		Mappers map[string]json.RawMessage `json:"mappers"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	raw, ok := root.Mappers[kind]
	if !ok {
		return nil, nil
	}
	var sec struct {
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil, err
	}
	if len(sec.Params) == 0 {
		return nil, nil
	}
	return objectKeyOrder(sec.Params)
}

// protocolFileFor — имя файла протокола по схеме.
//
// Совпадение имени файла и схемы — общее правило реестра; исключения
// (ss→shadowsocks) читаются из самого файла загрузчиком реестра, а здесь
// достаточно пробы: если файла со схемой нет, ищем по полю scheme.
func protocolFileFor(scheme string) string {
	if _, err := contract.ReadRegistry("protocols/" + scheme + ".json"); err == nil {
		return scheme
	}
	for _, name := range registry.ProtocolFileNames() {
		data, err := contract.ReadRegistry("protocols/" + name + ".json")
		if err != nil {
			continue
		}
		var probe struct {
			Scheme string `json:"scheme"`
		}
		if err := json.Unmarshal(data, &probe); err == nil && probe.Scheme == scheme {
			return name
		}
	}
	return scheme
}

// namedValueMaps — кэш именованных таблиц value_map ("tls.fp_dialect").
//
// Таблица — это ДАННЫЕ блока, не диалект: у неё нет записей с `source`, и
// include её не подключает. Её адресует `{"$ref": "<файл>.<имя>"}` у записи.
var namedValueMaps = map[string]map[string]interface{}{}
var namedValueMapsLoaded bool

// lookupNamedValueMap разрешает `$ref` записи.
//
// Имя строится как "<файл>.<ключ в blocks>" — оба конца приезжают с диска,
// поэтому ни одного имени схемы или блока в коде нет.
func lookupNamedValueMap(ref string) map[string]interface{} {
	if !namedValueMapsLoaded {
		loadNamedValueMaps()
		namedValueMapsLoaded = true
	}
	return namedValueMaps[ref]
}

func loadNamedValueMaps() {
	for _, name := range blockFiles {
		data, err := contract.ReadRegistry(name + ".json")
		if err != nil {
			continue
		}
		var f struct {
			Blocks map[string]json.RawMessage `json:"blocks"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		for key, raw := range f.Blocks {
			if key == "note" {
				continue
			}
			// Именованная таблица — та, у которой нет записей с `source`:
			// признак структурный, а не по имени.
			group, table, err := classifyBlock(raw)
			if err != nil || table != nil || group != nil {
				continue
			}
			var vm map[string]interface{}
			if err := json.Unmarshal(raw, &vm); err != nil {
				continue
			}
			namedValueMaps[name+"."+key] = vm
		}
	}
}

// objectKeyOrder возвращает ключи JSON-объекта в порядке их появления в
// ТЕКСТЕ. Стандартный Unmarshal в карту этот порядок теряет безвозвратно.
func objectKeyOrder(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("linkmap: ожидался объект")
	}
	var out []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("linkmap: ожидался ключ объекта")
		}
		out = append(out, key)
		// Пропускаем значение целиком.
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return out, nil
}
