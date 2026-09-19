package config

// Линтер секций body реестра контракта (SPEC 131, волна W1).
//
// Секция body описывает ТЕЛО узла sing-box: порядок полей структур ядра,
// типы, enum'ы, конфликты и коды деградации. Из неё в волне W2 работают
// санитайзер и эмиттер обеих сторон, поэтому реестр обязан быть
// самосогласованным: неописанное поле молча уедет в ядро (и уронит весь
// config.json), а код без записи в warnings.json UI нарисовать не сможет.
//
// Что проверяется:
//   - order покрывает ровно fields (и наоборот) на всех уровнях вложенности;
//   - каждый ref указывает на существующую суб-схему;
//   - каждый code объявлен в registry/warnings.json (в том числе код
//     деградации on_invalid — иначе снятие поля осталось бы молчаливым);
//   - on_invalid.action из словаря §3.2, value только у coerce и только из
//     values enum'а: подстановка вне набора уронила бы весь конфиг;
//   - normalize из словаря §4;
//   - type из словаря SPEC §4, у enum есть values, у ref есть ref;
//   - каждое поле из SPECS/131-.../core_schema.draft.json отражено в реестре
//     либо помечено skip с причиной.
//
// Go 1.20-совместимо: Win7-джоба собирает весь модуль тулчейном go1.20 —
// ни slices, ни maps, ни min/max (память win7-build-go120).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	registryBodyDir = "../../contract/registry"
	coreSchemaDraft = "../../SPECS/131-F-O-UNIFIED_NODE_PIPELINE/core_schema.draft.json"
)

// registryFieldTypes — словарь типов поля из SPEC 131 §4.
var registryFieldTypes = map[string]bool{
	"string":          true,
	"int":             true,
	"uint16":          true,
	"bool":            true,
	"duration":        true,
	"listable_string": true,
	"string_array":    true,
	"object":          true,
	"enum":            true,
	"ref":             true,
	"array":           true,
	// SPEC 131 W2c: два типа сверх словаря §4, оба — форма ЯДРА, а не
	// выдумка кода.
	//
	// awg_range — AWGRange форка (option/wireguard_awg.go): число ИЛИ строка
	// "min-max", ядро принимает обе формы (проверено `sing-box check` на
	// 1.14.1-lx.4). Пока эти поля стояли как "string", санитайзер снимал у
	// них числовую форму — то есть ровно ту, которой их пишут парсеры, — и
	// AWG-узел терял всю обфускацию h1..h4 молча.
	//
	// int_array — []uint8 из трёх байт (peers[].reserved, Cloudflare WARP);
	// string_array ронял поле целиком.
	"awg_range": true,
	"int_array": true,
}

// registryNormalizeModes — допустимые значения normalize.
//
// Флаг ставится ТОЛЬКО там, где ядро регистрозависимо И обе стороны
// нормализуют (SPEC 131 §4): без него регистр не трогается. Пример цены
// ошибки — xhttp x_padding_placement принимает лишь camelCase
// "queryInHeader", и нормализация к lowercase ломает рабочий узел
// (DRIFT §9.2).
// range_order — своп перевёрнутой пары границ у типа awg_range («40-10» →
// «10-40»), МОЛЧА: порядок границ смысла не несёт (ядро выбирает значение ИЗ
// диапазона), а замена без смены смысла кода не даёт — то же правило, что у
// trim. У таймингов AWG 3.x флаг не ставится: там перевёрнутая пара —
// опечатка человека, и он обязан её увидеть (контракт 1.1.11).
var registryNormalizeModes = map[string]bool{
	"trim": true, "lower": true, "trim_lower": true, "hex_only": true,
	"range_order": true,
	// Перенесены из маппера (SPEC 133 §6.0): правят ЗНАЧЕНИЕ, а значит
	// принадлежат телу и обязаны действовать на всех входах, не только на
	// ссылке.
	"cidr_prefix": true, "base64_std": true, "duration_bare_seconds": true,
}

// registryMinWhenActions — допустимые действия условного минимума.
var registryMinWhenActions = map[string]bool{"drop": true, "drop_node": true}

// registryRelationKinds — виды связей МЕЖДУ НЕСКОЛЬКИМИ полями (body.relations).
//
// Вид, которого нет в словаре, санитайзер пропускает МОЛЧА (реестр вправе
// уехать вперёд кода), то есть опечатка в `kind` тихо отключила бы правило —
// ловим её здесь, как и опечатку в `pattern`.
var registryRelationKinds = map[string]bool{"ranges_disjoint": true, "cooccurrence": true}

// registryRelationWhenOps — служебные операторы условия связи (`$…`).
//
// Оператора, которого нет в словаре, санитайзер считает НЕвыполненным, и
// правило молча не срабатывает — опечатку ловим здесь.
var registryRelationWhenOps = map[string]bool{"$range_width": true}

// registryRelationActions — что связь делает с узлом.
var registryRelationActions = map[string]bool{"warn": true, "drop_node": true}

// registryOnInvalidActions — допустимые действия on_invalid.
var registryOnInvalidActions = map[string]bool{
	"drop": true, "coerce": true, "drop_node": true,
}

// registryNodeSources — словарь ВХОДОВ узла (секция `sources` схем реестра).
//
// Его читают правила значений, различающие, КТО сочинил значение: тело в
// форме ядра (`singbox`) человек или подписка написали сами, остальное собрал
// маппер из ссылки, .conf или профиля.
var registryNodeSources = map[string]bool{
	"uri": true, "singbox": true, "xray": true, "wgconf": true, "amnezia": true,
}

// checkBodyCondition — линтер условия применимости правила значения.
//
// Условие без путей молча «выполнено всегда», то есть правило перестаёт быть
// условным и тихо расползается на узлы, которым не адресовано. Пустой список
// здесь опаснее отсутствия условия: отсутствие видно, пустота — нет.
func checkBodyCondition(t *testing.T, where string, c *bodyCondition) {
	t.Helper()
	if c == nil {
		return
	}
	if len(c.AnySet) == 0 {
		t.Errorf("%s: when без any_set — условие выполнено всегда, правило перестаёт быть условным", where)
		return
	}
	seen := make(map[string]bool, len(c.AnySet))
	for _, p := range c.AnySet {
		if strings.TrimSpace(p) == "" {
			t.Errorf("%s: when.any_set содержит пустой путь", where)
			continue
		}
		if seen[p] {
			t.Errorf("%s: when.any_set повторяет путь %q", where, p)
		}
		seen[p] = true
	}
}

// registryPendingCodes — коды, которых в warnings.json ещё нет: они
// объявлены в SPECS/131-F-O-UNIFIED_NODE_PIPELINE/new_codes.md и доедут
// вместе с правкой реестра кодов (её ведёт отдельный агент).
//
// Список обязан пустеть: код, задержавшийся здесь, UI нарисовать не сможет.
var registryPendingCodes = map[string]bool{
	// SPEC 131 W1: ждёт warnings.json
}

// registryFieldFormats — допустимые значения format.
var registryFieldFormats = map[string]bool{
	"uuid": true, "hex": true, "base64": true, "host": true,
	"port": true, "ipv4": true, "cidr": true,
	// SPEC 131 W2d: ключ Curve25519/X25519 — 32 байта ПОСЛЕ декода. Одного
	// «декодируется» мало: `enabled` и `true` — валидный base64, и на них
	// ядро отвечает «invalid public_key» отказом всего конфига.
	"base64_32": true,
	// SPEC 131 W2c: путь, который ядро разбирает через url.Parse
	// (ws/httpupgrade/http). Битое percent-кодирование там = «invalid URL
	// escape» и отказ ВСЕГО конфига, а не одного узла.
	"url_path": true,
}

// bodyField — поле схемы тела. Разбирается лениво: вложенность описывается
// теми же структурами, а неизвестные атрибуты ловит проверка по схеме.
type bodyField struct {
	Type           string                `json:"type"`
	Ref            string                `json:"ref"`
	Inline         bool                  `json:"inline"`
	Items          *bodyField            `json:"items"`
	Order          []string              `json:"order"`
	Fields         map[string]*bodyField `json:"fields"`
	Values         []interface{}         `json:"values"`
	Format         string                `json:"format"`
	Pattern        string                `json:"pattern"`
	AbsentValues   []interface{}         `json:"absent_values"`
	Code           string                `json:"code"`
	ForbiddenFor   []string              `json:"forbidden_for"`
	ForbiddenCodes map[string]string     `json:"forbidden_codes"`
	AllowedFor     []string              `json:"allowed_for"`
	Conflicts      []bodyRelation        `json:"conflicts"`
	Requires       []bodyRelation        `json:"requires"`
	OnInvalid      *bodyOnInvalid        `json:"on_invalid"`
	Advisory       []bodyAdvisory        `json:"advisory"`
	Normalize      string                `json:"normalize"`
	NormalizeCode  string                `json:"normalize_code"`
	DefaultWhen    *bodyDefaultWhen      `json:"default_when"`
	MaxWhen        *bodyMaxWhen          `json:"max_when"`
	MinWhen        *bodyMinWhen          `json:"min_when"`
	Skip           string                `json:"skip"`
	DescEn         string                `json:"desc_en"`
	DescRu         string                `json:"desc_ru"`
}

// bodyDefaultWhen — дефолт, который реестр велит МАТЕРИАЛИЗОВАТЬ явно
// (SPEC 131 §3.2): обычные `default` в тело не пишутся.
type bodyDefaultWhen struct {
	Absent bool           `json:"absent"`
	Value  interface{}    `json:"value"`
	Code   string         `json:"code"`
	When   *bodyCondition `json:"when"`
}

// bodyMaxWhen — условный потолок значения (контракт 1.1.5).
type bodyMaxWhen struct {
	Max           *float64       `json:"max"`
	Code          string         `json:"code"`
	When          *bodyCondition `json:"when"`
	ExceptSources []string       `json:"except_sources"`
	NoteCode      string         `json:"note_code"`
}

// bodyMinWhen — условный минимум значения (контракт 1.1.11).
type bodyMinWhen struct {
	Min          *float64       `json:"min"`
	Code         string         `json:"code"`
	Action       string         `json:"action"`
	AbsentIsZero bool           `json:"absent_is_zero"`
	When         *bodyCondition `json:"when"`
}

// bodyRelation2 — связь между НЕСКОЛЬКИМИ полями тела (body.relations).
type bodyRelation2 struct {
	Kind     string    `json:"kind"`
	Paths    []string  `json:"paths"`
	Defaults []float64 `json:"defaults"`
	Action   string    `json:"action"`
	Code     string    `json:"code"`
	DescEn   string    `json:"desc_en"`
	DescRu   string    `json:"desc_ru"`
	// When — условие срабатывания связи; у `cooccurrence` обязательно.
	When map[string]interface{} `json:"when"`
}

// bodyCondition — условие применимости правила значения.
type bodyCondition struct {
	AnySet []string `json:"any_set"`
}

// bodyOnInvalid — правило SPEC 131 §3.2: что делать со значением, не
// прошедшим ограничение поля.
type bodyOnInvalid struct {
	Action string      `json:"action"`
	Value  interface{} `json:"value"`
	Code   string      `json:"code"`
}

// bodyAdvisory — значения enum, которые ядро принимает, но узел получает
// информационный код (ss legacy-шифры → ss_method_legacy, D-122;
// отпечаток без гибридного шара под REALITY → reality_fp_not_chrome, D-119).
type bodyAdvisory struct {
	Values []interface{} `json:"values"`
	Except []interface{} `json:"except"`
	When   *bodyRelation `json:"when"`
	Code   string        `json:"code"`
}

type bodyRelation struct {
	With string `json:"with"`
	Path string `json:"path"`
	Code string `json:"code"`
}

// bodySection — секция body (или common) одного файла реестра.
type bodySection struct {
	Core      string                `json:"core"`
	Order     []string              `json:"order"`
	Fields    map[string]*bodyField `json:"fields"`
	Skipped   map[string]string     `json:"skipped"`
	Relations []bodyRelation2       `json:"relations"`
	Variants  map[string]*struct {
		Order  []string              `json:"order"`
		Fields map[string]*bodyField `json:"fields"`
	} `json:"variants"`
	Discriminator string `json:"discriminator"`
}

type registryBodyFile struct {
	Body   *bodySection `json:"body"`
	Common *bodySection `json:"common"`
}

func readRegistryJSON(t *testing.T, rel string, dst interface{}) bool {
	t.Helper()
	path := filepath.Join(registryBodyDir, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("реестр не найден (%s) — контракт не синхронизирован", path)
		return false
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("разбор %s: %v", path, err)
	}
	return true
}

// loadWarningCodes возвращает множество кодов, объявленных в warnings.json.
func loadWarningCodes(t *testing.T) map[string]bool {
	t.Helper()
	var f struct {
		Warnings map[string]json.RawMessage `json:"warnings"`
	}
	readRegistryJSON(t, "warnings.json", &f)
	codes := make(map[string]bool, len(f.Warnings))
	for code := range f.Warnings {
		codes[code] = true
	}
	return codes
}

// registryProtocolSchemes — схемы протоколов реестра. Список явный (тот же,
// что у загрузчика в core/config/registry): появление новой схемы должно быть
// осознанным, а не подхватываться обходом каталога молча.
var registryProtocolSchemes = []string{
	"anytls", "chain", "http", "hysteria", "hysteria2", "masque",
	"naive", "shadowsocks", "socks", "ssh", "tailscale", "trojan", "tuic",
	"vless", "vmess", "wireguard",
}

// registryBodyFiles — файлы реестра с секцией body и имена суб-схем, на
// которые из них можно ссылаться.
func registryBodyFiles(t *testing.T) map[string]*registryBodyFile {
	t.Helper()
	names := []string{"tls.json", "transports.json", "multiplex.json", "dialer.json"}
	for _, scheme := range registryProtocolSchemes {
		names = append(names, "protocols/"+scheme+".json")
	}
	out := make(map[string]*registryBodyFile, len(names))
	for _, name := range names {
		f := &registryBodyFile{}
		if !readRegistryJSON(t, name, f) {
			return nil
		}
		if f.Body == nil {
			t.Errorf("%s: нет секции body", name)
			continue
		}
		out[name] = f
	}
	return out
}

// knownRefs — имена суб-схем, допустимые в атрибуте ref.
var knownRefs = map[string]bool{
	"tls":           true,
	"transports":    true,
	"multiplex":     true,
	"dialer":        true,
	"dialer.common": true,
}

// walkFields обходит поля секции вглубь, вызывая fn на каждом с его путём.
func walkFields(prefix string, order []string, fields map[string]*bodyField, fn func(path string, f *bodyField)) {
	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		fn(path, f)
		if len(f.Fields) > 0 {
			walkFields(path, f.Order, f.Fields, fn)
		}
		if f.Items != nil {
			fn(path+"[]", f.Items)
			if len(f.Items.Fields) > 0 {
				walkFields(path+"[]", f.Items.Order, f.Items.Fields, fn)
			}
		}
	}
}

// checkOrderCoversFields сверяет order и fields в обе стороны.
func checkOrderCoversFields(t *testing.T, where string, order []string, fields map[string]*bodyField) {
	t.Helper()
	seen := make(map[string]bool, len(order))
	for _, name := range order {
		if seen[name] {
			t.Errorf("%s: order повторяет поле %q", where, name)
			continue
		}
		seen[name] = true
		if _, ok := fields[name]; !ok {
			t.Errorf("%s: order упоминает %q, которого нет в fields", where, name)
		}
	}
	missing := make([]string, 0, len(fields))
	for name := range fields {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("%s: поле %q есть в fields, но не упомянуто в order", where, name)
	}
	// Вложенные объекты — той же проверкой.
	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		if len(f.Fields) > 0 {
			checkOrderCoversFields(t, where+"."+name, f.Order, f.Fields)
		}
		if f.Items != nil && len(f.Items.Fields) > 0 {
			checkOrderCoversFields(t, where+"."+name+"[]", f.Items.Order, f.Items.Fields)
		}
	}
}

// TestRegistryBodyStructure — order/fields, типы, enum, ref, коды.
func TestRegistryBodyStructure(t *testing.T) {
	files := registryBodyFiles(t)
	if files == nil {
		return
	}
	codes := loadWarningCodes(t)

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		f := files[name]
		sections := map[string]*bodySection{"body": f.Body}
		if f.Common != nil {
			sections["common"] = f.Common
		}
		secNames := make([]string, 0, len(sections))
		for k := range sections {
			secNames = append(secNames, k)
		}
		sort.Strings(secNames)

		for _, secName := range secNames {
			sec := sections[secName]
			where := name + " " + secName
			if sec.Core == "" {
				t.Errorf("%s: пустой core — секция обязана называть тег ядра, по которому сверена", where)
			}
			// Вариантная форма (транспорты) и обычная.
			if len(sec.Variants) > 0 {
				if sec.Discriminator == "" {
					t.Errorf("%s: есть variants, но нет discriminator", where)
				}
				varNames := make([]string, 0, len(sec.Variants))
				for k := range sec.Variants {
					varNames = append(varNames, k)
				}
				sort.Strings(varNames)
				for _, v := range varNames {
					variant := sec.Variants[v]
					vWhere := where + " variant=" + v
					checkOrderCoversFields(t, vWhere, variant.Order, variant.Fields)
					walkFields("", variant.Order, variant.Fields, func(path string, fl *bodyField) {
						checkField(t, vWhere, path, fl, codes)
					})
				}
				continue
			}
			checkOrderCoversFields(t, where, sec.Order, sec.Fields)
			walkFields("", sec.Order, sec.Fields, func(path string, fl *bodyField) {
				checkField(t, where, path, fl, codes)
			})
			checkBodyRelations(t, where, sec.Relations, sec.Order, sec.Fields, codes)
		}
	}
}

// checkBodyRelations — линтер связей между несколькими полями (body.relations).
//
// Санитайзер неизвестный `kind` пропускает МОЛЧА (реестр вправе уехать вперёд
// кода), поэтому опечатка в виде связи тихо отключила бы правило — её ловим
// здесь, как и опечатку в `pattern`. Пути обязаны существовать: связь на
// несуществующее поле не сработает никогда и выглядит как рабочая.
func checkBodyRelations(t *testing.T, where string, rels []bodyRelation2, order []string, fields map[string]*bodyField, codes map[string]bool) {
	t.Helper()
	known := collectPaths(order, fields)
	for i, rel := range rels {
		full := fmt.Sprintf("%s relations[%d]", where, i)
		if !registryRelationKinds[rel.Kind] {
			t.Errorf("%s: kind %q вне словаря связей — санитайзер такую связь пропустит молча", full, rel.Kind)
		}
		if !registryRelationActions[rel.Action] {
			t.Errorf("%s: action %q вне словаря (warn|drop_node)", full, rel.Action)
		}
		if rel.Code == "" {
			t.Errorf("%s: без code — срабатывание связи было бы молчаливым", full)
		} else if !codes[rel.Code] && !registryPendingCodes[rel.Code] {
			t.Errorf("%s: код %q не объявлен в warnings.json", full, rel.Code)
		}
		if len(rel.Paths) < 2 {
			t.Errorf("%s: связь МЕЖДУ полями объявляет %d путь(ей) — паре хватило бы conflicts у поля", full, len(rel.Paths))
		}
		for _, p := range rel.Paths {
			if !known[p] {
				t.Errorf("%s: путь %q не описан в fields — связь не сработает никогда", full, p)
			}
		}
		// defaults перечисляет значение участника при отсутствии ключа, и
		// длина обязана совпадать: короткий список молча оставил бы часть
		// полей без дефолта ядра, то есть вне проверки.
		if len(rel.Defaults) > 0 && len(rel.Defaults) != len(rel.Paths) {
			t.Errorf("%s: defaults (%d) короче/длиннее paths (%d) — часть участников осталась бы без дефолта ядра",
				full, len(rel.Defaults), len(rel.Paths))
		}
		if rel.DescEn == "" || rel.DescRu == "" {
			t.Errorf("%s: нет desc_en/desc_ru — связь не попадёт в документацию", full)
		}
		// `cooccurrence` без условия санитайзер пропускает молча: связь,
		// срабатывающая на одном лишь наличии полей, ставила бы код каждому
		// узлу, у которого они есть, — заведомо не то, что имел в виду
		// реестр. Ловим здесь, иначе правило тихо не работает.
		if rel.Kind == "cooccurrence" && len(rel.When) == 0 {
			t.Errorf("%s: cooccurrence без when — санитайзер такую связь пропустит молча", full)
		}
		// Ключи `when` — либо путь из paths, либо служебный оператор с
		// ведущим `$`. Опечатка в пути дала бы условие, которое не выполнится
		// никогда, то есть снова молчащее правило.
		for k := range rel.When {
			if strings.HasPrefix(k, "$") {
				if !registryRelationWhenOps[k] {
					t.Errorf("%s: when: оператор %q вне словаря — санитайзер считает его невыполненным", full, k)
				}
				continue
			}
			if !known[k] {
				t.Errorf("%s: when: путь %q не описан в fields — условие не выполнится никогда", full, k)
			}
		}
	}
}

func checkField(t *testing.T, where, path string, f *bodyField, codes map[string]bool) {
	t.Helper()
	full := where + " " + path
	if !registryFieldTypes[f.Type] {
		t.Errorf("%s: тип %q вне словаря SPEC 131 §4", full, f.Type)
	}
	if f.Type == "enum" && len(f.Values) == 0 {
		t.Errorf("%s: type=enum без values", full)
	}
	if f.Type == "ref" && f.Ref == "" {
		t.Errorf("%s: type=ref без ref", full)
	}
	if f.Ref != "" && !knownRefs[f.Ref] && !knownRefs[refParent(f.Ref)] {
		t.Errorf("%s: ref %q не указывает ни на одну суб-схему реестра", full, f.Ref)
	}
	if f.Type == "array" && f.Items == nil {
		t.Errorf("%s: type=array без items", full)
	}
	if f.Format != "" && !registryFieldFormats[f.Format] {
		t.Errorf("%s: format %q вне словаря SPEC 131 §4", full, f.Format)
	}
	// pattern обязан компилироваться: невалидное выражение санитайзер
	// ПРОПУСКАЕТ (реестр вправе уехать вперёд кода), то есть опечатка в
	// контракте молча отключила бы проверку. Ловим её здесь.
	//
	// Диалект — общее подмножество Go RE2 и ECMAScript/Dart: конструкции, на
	// которых RE2 не спотыкается, но вторая сторона поведёт себя иначе,
	// перечислены явно. Совпадение по всей строке задают якоря в самом
	// выражении, поэтому их отсутствие — тоже ошибка: без них правило
	// проверяло бы ПОДстроку и пропускало мусор по краям.
	// absent_values — только непустые строки: атрибут повторяет ЛИТЕРАЛ ядра,
	// а пустую строку и так снимает omitAsUnset.
	for _, a := range f.AbsentValues {
		str, ok := a.(string)
		if !ok {
			t.Errorf("%s: absent_values содержит не строку (%T) — литерал ядра всегда строка", full, a)
			continue
		}
		if str == "" {
			t.Errorf("%s: absent_values содержит пустую строку — её снимает omitAsUnset, запись лишняя", full)
		}
	}
	if f.Pattern != "" {
		if _, err := regexp.Compile(f.Pattern); err != nil {
			t.Errorf("%s: pattern %q не компилируется: %v", full, f.Pattern, err)
		}
		if !strings.HasPrefix(f.Pattern, "^") || !strings.HasSuffix(f.Pattern, "$") {
			t.Errorf("%s: pattern %q без якорей ^…$ — правило проверяло бы подстроку", full, f.Pattern)
		}
		for _, bad := range []string{"(?=", "(?!", "(?<", "(?i)", "(?m)", "(?s)", "\\1", "\\2"} {
			if strings.Contains(f.Pattern, bad) {
				t.Errorf("%s: pattern %q содержит %q — вне общего подмножества RE2 и ECMAScript/Dart", full, f.Pattern, bad)
			}
		}
	}
	if len(f.Fields) > 0 && f.Type != "object" {
		t.Errorf("%s: fields заданы при type=%q (ожидался object)", full, f.Type)
	}
	// Коды: forbidden_for/allowed_for и связи.
	if len(f.ForbiddenFor) > 0 && f.Code == "" {
		t.Errorf("%s: forbidden_for без code", full)
	}
	if len(f.AllowedFor) > 0 && f.Code == "" {
		t.Errorf("%s: allowed_for без code", full)
	}
	if f.Code != "" && !codes[f.Code] && !registryPendingCodes[f.Code] {
		t.Errorf("%s: код %q не объявлен в warnings.json", full, f.Code)
	}
	// forbidden_codes переопределяет код запрета для отдельной схемы: исход у
	// схем разный (naive теряет настройку, QUIC — бессмыслицу). Схема, которой
	// поле НЕ запрещено, в словаре бессмысленна и означает опечатку.
	for scheme, code := range f.ForbiddenCodes {
		if !codes[code] && !registryPendingCodes[code] {
			t.Errorf("%s: forbidden_codes[%q] = %q не объявлен в warnings.json", full, scheme, code)
		}
		found := false
		for _, sc := range f.ForbiddenFor {
			if sc == scheme {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: forbidden_codes называет схему %q, которой поле не запрещено", full, scheme)
		}
	}
	if f.Normalize != "" && !registryNormalizeModes[f.Normalize] {
		t.Errorf("%s: normalize %q вне словаря SPEC 131 §4", full, f.Normalize)
	}
	if f.Normalize == "range_order" {
		// Своп границ осмыслен только у диапазона: у строки или числа
		// переставлять нечего, и флаг там означает опечатку.
		if f.Type != "awg_range" {
			t.Errorf("%s: normalize=range_order на поле типа %q — свопать границы можно только у awg_range", full, f.Type)
		}
		// Своп ТИХИЙ по определению (смысл диапазона не меняется), и код
		// рядом с ним противоречил бы самому правилу.
		if f.NormalizeCode != "" {
			t.Errorf("%s: normalize_code при range_order — своп границ смысла не меняет и кода не даёт", full)
		}
	}
	if f.NormalizeCode != "" {
		if f.Normalize == "" {
			t.Errorf("%s: normalize_code без normalize — сообщать не о чем", full)
		}
		if !codes[f.NormalizeCode] && !registryPendingCodes[f.NormalizeCode] {
			t.Errorf("%s: normalize_code %q не объявлен в warnings.json", full, f.NormalizeCode)
		}
	}
	if dw := f.DefaultWhen; dw != nil {
		if !dw.Absent {
			t.Errorf("%s: default_when без absent — иных условий санитайзер не знает", full)
		}
		if dw.Value == nil {
			t.Errorf("%s: default_when без value — подставлять нечего", full)
		}
		if dw.Code != "" && !codes[dw.Code] && !registryPendingCodes[dw.Code] {
			t.Errorf("%s: default_when.code %q не объявлен в warnings.json", full, dw.Code)
		}
		checkBodyCondition(t, full+" default_when", dw.When)
	}
	if mw := f.MaxWhen; mw != nil {
		if mw.Max == nil {
			t.Errorf("%s: max_when без max — потолка нет", full)
		}
		if mw.Code == "" {
			t.Errorf("%s: max_when без code — замена значения была бы молчаливой", full)
		} else if !codes[mw.Code] && !registryPendingCodes[mw.Code] {
			t.Errorf("%s: max_when.code %q не объявлен в warnings.json", full, mw.Code)
		}
		// Потолок ВСЕГДА условный: безусловный выражается обычным `max`, и
		// правило без `when` сняло бы поле у всех, кому оно не адресовано
		// (у mtu — у каждого обычного WireGuard-узла).
		if mw.When == nil {
			t.Errorf("%s: max_when без when — безусловный потолок пишется обычным max", full)
		}
		checkBodyCondition(t, full+" max_when", mw.When)
		// Исключение по входу и код-уведомление ходят парой: без кода
		// сохранение завышенного значения стало бы молчаливым, а код без
		// исключения некому поставить.
		if len(mw.ExceptSources) > 0 && mw.NoteCode == "" {
			t.Errorf("%s: max_when.except_sources без note_code — исключение было бы молчаливым", full)
		}
		if mw.NoteCode != "" {
			if len(mw.ExceptSources) == 0 {
				t.Errorf("%s: max_when.note_code без except_sources — код ставить некому", full)
			}
			if !codes[mw.NoteCode] && !registryPendingCodes[mw.NoteCode] {
				t.Errorf("%s: max_when.note_code %q не объявлен в warnings.json", full, mw.NoteCode)
			}
		}
		for _, src := range mw.ExceptSources {
			if !registryNodeSources[src] {
				t.Errorf("%s: max_when.except_sources называет вход %q вне словаря sources", full, src)
			}
		}
	}
	if mw := f.MinWhen; mw != nil {
		if mw.Min == nil {
			t.Errorf("%s: min_when без min — порога нет", full)
		}
		if mw.Code == "" {
			t.Errorf("%s: min_when без code — снятие поля было бы молчаливым", full)
		} else if !codes[mw.Code] && !registryPendingCodes[mw.Code] {
			t.Errorf("%s: min_when.code %q не объявлен в warnings.json", full, mw.Code)
		}
		if !registryMinWhenActions[mw.Action] {
			t.Errorf("%s: min_when.action %q вне словаря (drop|drop_node)", full, mw.Action)
		}
		// Минимум ВСЕГДА условный — ровно как потолок: безусловный
		// записывается обычным `min`, а правило без `when` сняло бы поле у
		// всех, кому оно не адресовано (у s1..s4 — у каждого AmneziaWG-узла
		// без защиты заголовков, то есть почти у всех).
		if mw.When == nil {
			t.Errorf("%s: min_when без when — безусловный порог пишется обычным min", full)
		}
		checkBodyCondition(t, full+" min_when", mw.When)
		// absent_is_zero осмыслен только у числового поля: у строки
		// «отсутствует» не значит «ноль», и порог там читался бы наугад.
		if mw.AbsentIsZero && f.Type != "int" && f.Type != "uint16" && f.Type != "awg_range" {
			t.Errorf("%s: min_when.absent_is_zero на поле типа %q — «нет ключа = 0» осмысленно только у числа", full, f.Type)
		}
	}
	for i, adv := range f.Advisory {
		if adv.Code == "" {
			t.Errorf("%s: advisory[%d] без code", full, i)
		} else if !codes[adv.Code] && !registryPendingCodes[adv.Code] {
			t.Errorf("%s: advisory-код %q не объявлен в warnings.json", full, adv.Code)
		}
		if len(adv.Values) == 0 && len(adv.Except) == 0 {
			t.Errorf("%s: advisory[%d] без values и без except", full, i)
		}
		if len(adv.Values) > 0 && len(adv.Except) > 0 {
			t.Errorf("%s: advisory[%d] объявляет и values, и except — правило читается двояко", full, i)
		}
		// advisory-значение обязано быть в enum: иначе оно не «принимается с
		// кодом», а снимается. То же и для except: исключение из правила
		// «все, кроме этих» обязано быть значением, которое вообще бывает.
		//
		// Проверка идёт ТОЛЬКО по полям, которые перечисляют значения сами.
		// У bool домен закрыт типом, а не списком: `values` там пуст (и
		// заполнять его парой true/false незачем — ограничение проверяет не
		// список, а coerce). Требовать вхождения в пустой список значило бы
		// запретить advisory на bool вовсе — ровно так отвергался tls.insecure,
		// единственная форма которого «код на значении true» (контракт 1.1.6).
		if len(f.Values) > 0 {
			for _, av := range append(append([]interface{}{}, adv.Values...), adv.Except...) {
				found := false
				for _, v := range f.Values {
					if fmt.Sprintf("%v", v) == fmt.Sprintf("%v", av) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: advisory-значение %v отсутствует в values", full, av)
				}
			}
		} else if f.Type != "bool" {
			t.Errorf("%s: advisory[%d] на поле без values и не bool — домен значений неизвестен", full, i)
		}
	}
	if oi := f.OnInvalid; oi != nil {
		if !registryOnInvalidActions[oi.Action] {
			t.Errorf("%s: on_invalid.action %q вне словаря SPEC 131 §3.2", full, oi.Action)
		}
		if oi.Action == "coerce" && oi.Value == nil {
			t.Errorf("%s: on_invalid.action=coerce без value — санитайзеру нечем заменить", full)
		}
		if oi.Action != "coerce" && oi.Value != nil {
			t.Errorf("%s: on_invalid.value задан при action=%q (value осмыслен только у coerce)", full, oi.Action)
		}
		if oi.Code == "" {
			t.Errorf("%s: on_invalid без code — деградация была бы молчаливой", full)
		} else if !codes[oi.Code] && !registryPendingCodes[oi.Code] {
			t.Errorf("%s: код деградации %q не объявлен в warnings.json", full, oi.Code)
		}
		// coerce обязан подставлять значение, которое ядро примет.
		if oi.Action == "coerce" && f.Type == "enum" && len(f.Values) > 0 {
			ok := false
			for _, v := range f.Values {
				if fmt.Sprintf("%v", v) == fmt.Sprintf("%v", oi.Value) {
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("%s: on_invalid подставляет %v — значения нет в values, ядро отвергнет весь конфиг",
					full, oi.Value)
			}
		}
	}
	for _, rel := range f.Conflicts {
		if rel.With == "" {
			t.Errorf("%s: conflicts без with", full)
		}
		if !codes[rel.Code] {
			t.Errorf("%s: код конфликта %q не объявлен в warnings.json", full, rel.Code)
		}
	}
	for _, rel := range f.Requires {
		if rel.Path == "" {
			t.Errorf("%s: requires без path", full)
		}
		if !codes[rel.Code] {
			t.Errorf("%s: код требования %q не объявлен в warnings.json", full, rel.Code)
		}
	}
	// Описания для документации: генератор W4 берёт их отсюда.
	if f.Skip == "" && f.Type != "ref" {
		if f.DescEn == "" || f.DescRu == "" {
			t.Errorf("%s: нет desc_en/desc_ru — поле не попадёт в документацию", full)
		}
	}
}

// ---------- сверка с core_schema.draft.json ----------

type draftField struct {
	Ref    string `json:"ref"`
	Client *bool  `json:"client"`
}

type draftSchema struct {
	Common map[string]struct {
		Fields map[string]draftField `json:"fields"`
	} `json:"common"`
	TLS      struct{ Fields map[string]draftField } `json:"tls"`
	UTLS     struct{ Fields map[string]draftField } `json:"utls"`
	Reality  struct{ Fields map[string]draftField } `json:"reality"`
	ECH      struct{ Fields map[string]draftField } `json:"ech"`
	Multiplx struct{ Fields map[string]draftField } `json:"multiplex"`
	Brutal   struct{ Fields map[string]draftField } `json:"brutal"`
	QUIC     struct{ Fields map[string]draftField } `json:"quic_options"`
	AWG      struct{ Fields map[string]draftField } `json:"amneziawg"`
	Peer     struct{ Fields map[string]draftField } `json:"wireguard_peer"`
	Trans    map[string]json.RawMessage             `json:"transports"`
	Types    map[string]struct {
		Fields map[string]draftField `json:"fields"`
	} `json:"types"`
}

func loadDraft(t *testing.T) *draftSchema {
	t.Helper()
	data, err := os.ReadFile(coreSchemaDraft)
	if err != nil {
		t.Skipf("core_schema.draft.json не найден (%s)", coreSchemaDraft)
		return nil
	}
	d := &draftSchema{}
	if err := json.Unmarshal(data, d); err != nil {
		t.Fatalf("разбор core_schema.draft.json: %v", err)
	}
	return d
}

// collectPaths собирает плоское множество имён полей секции (верхний уровень
// и вложенные объекты — отдельными записями по короткому имени).
func collectPaths(order []string, fields map[string]*bodyField) map[string]bool {
	out := map[string]bool{}
	walkFields("", order, fields, func(path string, f *bodyField) {
		parts := strings.Split(path, ".")
		out[parts[len(parts)-1]] = true
		out[path] = true
	})
	return out
}

// TestRegistryBodyCoversCoreSchema — каждое поле ядра описано либо помечено skip.
func TestRegistryBodyCoversCoreSchema(t *testing.T) {
	draft := loadDraft(t)
	if draft == nil {
		return
	}
	files := registryBodyFiles(t)
	if files == nil {
		return
	}

	// Суб-схемы: где искать поля ядра.
	tlsFile := files["tls.json"]
	tlsPaths := collectPaths(tlsFile.Body.Order, tlsFile.Body.Fields)
	checkDraftCoverage(t, "tls", draft.TLS.Fields, tlsPaths, nil)
	checkDraftCoverage(t, "tls.utls", draft.UTLS.Fields, tlsPaths, nil)
	checkDraftCoverage(t, "tls.reality", draft.Reality.Fields, tlsPaths, nil)
	checkDraftCoverage(t, "tls.ech", draft.ECH.Fields, tlsPaths, nil)

	muxFile := files["multiplex.json"]
	muxPaths := collectPaths(muxFile.Body.Order, muxFile.Body.Fields)
	checkDraftCoverage(t, "multiplex", draft.Multiplx.Fields, muxPaths, nil)
	checkDraftCoverage(t, "multiplex.brutal", draft.Brutal.Fields, muxPaths, nil)

	dialerFile := files["dialer.json"]
	dialerPaths := collectPaths(dialerFile.Body.Order, dialerFile.Body.Fields)
	if dialerFile.Common != nil {
		for k := range collectPaths(dialerFile.Common.Order, dialerFile.Common.Fields) {
			dialerPaths[k] = true
		}
	}
	checkDraftCoverage(t, "common.server", draft.Common["server"].Fields, dialerPaths, nil)
	checkDraftCoverage(t, "common.dialer", draft.Common["dialer"].Fields, dialerPaths, dialerFile.Body.Skipped)

	// Транспорты: по варианту на тип.
	transFile := files["transports.json"]
	for _, name := range []string{"http", "ws", "quic", "grpc", "httpupgrade", "xhttp", "xmux"} {
		raw, ok := draft.Trans[name]
		if !ok {
			continue
		}
		var v struct {
			Fields map[string]draftField `json:"fields"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("разбор transports.%s из драфта: %v", name, err)
		}
		variantName := name
		if name == "xmux" {
			variantName = "xhttp" // xmux — вложенный объект внутри xhttp
		}
		variant := transFile.Body.Variants[variantName]
		if variant == nil {
			t.Errorf("transports.json: нет варианта %q", variantName)
			continue
		}
		checkDraftCoverage(t, "transport."+name, v.Fields,
			collectPaths(variant.Order, variant.Fields), nil)
	}

	// Протоколы.
	schemeFile := map[string]string{
		"vless": "vless", "vmess": "vmess", "trojan": "trojan",
		"shadowsocks": "shadowsocks", "hysteria": "hysteria", "hysteria2": "hysteria2",
		"tuic": "tuic", "anytls": "anytls", "naive": "naive", "socks": "socks",
		"http": "http", "ssh": "ssh", "masque": "masque", "chain": "chain",
		"wireguard": "wireguard", "tailscale": "tailscale",
	}
	schemes := make([]string, 0, len(schemeFile))
	for s := range schemeFile {
		schemes = append(schemes, s)
	}
	sort.Strings(schemes)

	for _, scheme := range schemes {
		typ, ok := draft.Types[scheme]
		if !ok {
			t.Errorf("core_schema.draft.json: нет типа %q", scheme)
			continue
		}
		file := files["protocols/"+schemeFile[scheme]+".json"]
		if file == nil {
			continue
		}
		paths := collectPaths(file.Body.Order, file.Body.Fields)
		// Плоско встроенные группы: _quic и _awg разворачиваются в корень.
		fields := map[string]draftField{}
		for k, v := range typ.Fields {
			if k == "_quic" {
				for qk, qv := range draft.QUIC.Fields {
					fields[qk] = qv
				}
				continue
			}
			if k == "_awg" {
				for ak, av := range draft.AWG.Fields {
					fields[ak] = av
				}
				continue
			}
			fields[k] = v
		}
		checkDraftCoverage(t, scheme, fields, paths, file.Body.Skipped)
		// Пиры wireguard — вложенный массив.
		if scheme == "wireguard" {
			checkDraftCoverage(t, "wireguard.peers[]", draft.Peer.Fields, paths, nil)
		}
	}
}

// checkDraftCoverage требует, чтобы каждое поле драфта было в реестре или в skipped.
func checkDraftCoverage(t *testing.T, where string, draftFields map[string]draftField, registryPaths map[string]bool, skipped map[string]string) {
	t.Helper()
	names := make([]string, 0, len(draftFields))
	for name := range draftFields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if registryPaths[name] {
			continue
		}
		if skipped != nil {
			if reason, ok := skipped[name]; ok {
				if strings.TrimSpace(reason) == "" {
					t.Errorf("%s: поле %q помечено skip без причины", where, name)
				}
				continue
			}
		}
		t.Errorf("%s: поле ядра %q не описано в реестре и не помечено skip (%s)",
			where, name, fmt.Sprintf("источник — %s", coreSchemaDraft))
	}
}

// refParent — для ссылки на поле внутри суб-схемы (`dialer.common.network`)
// возвращает саму суб-схему (`dialer.common`); иные ссылки — как есть.
func refParent(ref string) string {
	i := strings.LastIndex(ref, ".")
	if i <= 0 {
		return ref
	}
	return ref[:i]
}

// registryMapperKinds — словарь вида перевода секции mapper.
//
// Секция описательная: в рантайме её не исполняет никто, её читает только
// генератор документации. Именно поэтому линтер здесь строже обычного — у
// неверной записи нет ни одного шанса упасть на тесте конвейера, она просто
// молча соврёт в документации.
var registryMapperKinds = map[string]bool{
	// structure — решение о НАЛИЧИИ или форме блока: отсутствие ключа
	// санитайзеру неотличимо от «не задано» (security=none → нет tls).
	"structure": true,
	// spelling — то же значение, записанное в чужом диалекте
	// (HelloChrome_120 → chrome).
	"spelling": true,
	// split — один вход разворачивается в несколько полей тела
	// (?ed=N → max_early_data + early_data_header_name).
	"split": true,
	// default — материализация дефолта-КОНВЕНЦИИ, а не дефолта ядра
	// (пустой fp → random). От default_when отличается основанием: там ядро
	// без поля не работает, здесь работает, но обе стороны договорились.
	"default": true,
	// drop — вход осознанно не доезжает до тела (packetEncoding=none).
	"drop": true,
}

// mapperRule — запись секции mapper.
type mapperRule struct {
	ID        string   `json:"id"`
	AppliesTo []string `json:"applies_to"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Kind      string   `json:"kind"`
	Code      string   `json:"code"`
	DescEn    string   `json:"desc_en"`
	DescRu    string   `json:"desc_ru"`
	Impl      string   `json:"impl"`
}

// TestRegistryMapperSection — линтер секции mapper: id уникален по всему
// реестру, kind из словаря, code объявлен в warnings.json, desc_en непуст,
// applies_to называет существующие схемы.
func TestRegistryMapperSection(t *testing.T) {
	codes := loadWarningCodes(t)

	// `group` секции body не имеет (это не узел, а состав из чужих тегов), но
	// структурные переводы у неё есть — поэтому mapper читается шире, чем
	// список схем с телом.
	mapperSchemes := append(append([]string{}, registryProtocolSchemes...), "group")
	sort.Strings(mapperSchemes)

	names := []string{"tls.json", "transports.json"}
	for _, scheme := range mapperSchemes {
		names = append(names, "protocols/"+scheme+".json")
	}

	schemes := make(map[string]bool, len(mapperSchemes))
	for _, s := range mapperSchemes {
		schemes[s] = true
	}

	seen := map[string]string{} // id → файл, где он уже встретился
	total := 0
	for _, name := range names {
		var f struct {
			Mapper []mapperRule `json:"mapper"`
		}
		if !readRegistryJSON(t, name, &f) {
			return
		}
		shared := name == "tls.json" || name == "transports.json"
		for _, r := range f.Mapper {
			total++
			where := fmt.Sprintf("%s mapper[%s]", name, r.ID)
			if strings.TrimSpace(r.ID) == "" {
				t.Errorf("%s: пустой id", name)
				continue
			}
			if prev, ok := seen[r.ID]; ok {
				t.Errorf("%s: id %q уже объявлен в %s", name, r.ID, prev)
			}
			seen[r.ID] = name

			if !registryMapperKinds[r.Kind] {
				t.Errorf("%s: kind %q вне словаря", where, r.Kind)
			}
			if strings.TrimSpace(r.From) == "" || strings.TrimSpace(r.To) == "" {
				t.Errorf("%s: from/to обязаны быть непустыми", where)
			}
			// desc_en уходит в англоязычную документацию: без него страница
			// покажет правило без объяснения.
			if strings.TrimSpace(r.DescEn) == "" {
				t.Errorf("%s: пустой desc_en", where)
			}
			if r.Code != "" && !codes[r.Code] && !registryPendingCodes[r.Code] {
				t.Errorf("%s: код %q не объявлен в warnings.json", where, r.Code)
			}
			// Общее правило обязано называть схемы, к которым относится:
			// иначе документация не знает, на чью страницу его вывести.
			if shared && len(r.AppliesTo) == 0 {
				t.Errorf("%s: общее правило без applies_to", where)
			}
			if !shared && len(r.AppliesTo) > 0 {
				t.Errorf("%s: applies_to у схемного правила (оно и так относится к своей схеме)", where)
			}
			for _, s := range r.AppliesTo {
				if !schemes[s] {
					t.Errorf("%s: applies_to называет неизвестную схему %q", where, s)
				}
			}
		}
	}
	if total == 0 {
		t.Error("секция mapper пуста во всём реестре — структурные переводы описаны только в коде")
	}
}

// TestRegistryMapperCodesDeclared — каждый код СЕКЦИЙ-МАППЕРОВ объявлен
// в warnings.json.
//
// Почему отдельным проходом, а не внутри линтера тела: предмет другой.
// Линтер тела судит схему ТЕЛА узла и ходит по типизированным структурам
// (`body.fields.*.on_invalid.code` и родня). Коды мапперов живут в секции
// `mappers.*` — `unknown_key.code`, `params.*.on_present|on_invalid|
// on_no_match.code`, `ini_dialect.*.on_extra.code` — и до этого прохода их
// не собирал никто (Q133-65). Два живых кода доехали до узла, не имея в
// warnings.json ни заголовка, ни текста, ни подсказки «что делать»: UI
// такую деградацию нарисовать не может, то есть человек получал потерю
// без слов — ровно то, против чего заведён сам линтер.
//
// Обход рекурсивный и по СЫРОМУ JSON намеренно. Типизировать секцию
// мапперов значило бы завести второй её читатель рядом с движком и
// разойтись с ним на первой же новой записи; страж же обязан ловить код
// В ЛЮБОМ месте секции, включая те, которых сегодня ещё нет. Ключом
// считается любое поле с именем `code` и строковым значением — ровно так
// код и объявляется везде в реестре.
func TestRegistryMapperCodesDeclared(t *testing.T) {
	codes := loadWarningCodes(t)

	names := []string{"tls.json", "transports.json", "multiplex.json", "dialer.json"}
	for _, scheme := range registryProtocolSchemes {
		names = append(names, "protocols/"+scheme+".json")
	}

	seen := 0
	for _, name := range names {
		var f struct {
			Mappers json.RawMessage `json:"mappers"`
		}
		if !readRegistryJSON(t, name, &f) {
			return
		}
		if len(f.Mappers) == 0 {
			continue
		}
		var tree interface{}
		if err := json.Unmarshal(f.Mappers, &tree); err != nil {
			t.Fatalf("%s: разбор mappers: %v", name, err)
		}
		walkRegistryCodes(tree, "mappers", func(where, code string) {
			seen++
			if !codes[code] && !registryPendingCodes[code] {
				t.Errorf("%s %s: код %q не объявлен в warnings.json", name, where, code)
			}
		})
	}
	if seen == 0 {
		t.Error("в секциях mappers не найдено ни одного кода — страж смотрит не туда")
	}
}

// walkRegistryCodes зовёт fn на каждом поле `code` со строковым значением.
//
// Go 1.20-совместимо: ни generics поверх карт, ни maps/slices (win7-джоба
// собирает модуль тулчейном go1.20).
func walkRegistryCodes(v interface{}, path string, fn func(where, code string)) {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := path + "." + k
			if k == "code" {
				if s, ok := t[k].(string); ok && strings.TrimSpace(s) != "" {
					fn(child, s)
					continue
				}
			}
			walkRegistryCodes(t[k], child, fn)
		}
	case []interface{}:
		for i := range t {
			walkRegistryCodes(t[i], fmt.Sprintf("%s[%d]", path, i), fn)
		}
	}
}

// TestRegistryWarningsHaveCauseAndFix — у каждого кода есть причина и хотя бы
// одно действие.
//
// Код без этой пары оставляет человека наедине с фактом «что-то сняли»: текст
// говорит, ЧТО случилось, но не говорит, откуда такое значение берётся и что с
// ним делать. Для info-кодов честный ответ «ничего делать не нужно» — тоже
// действие, и он обязан быть написан явно.
func TestRegistryWarningsHaveCauseAndFix(t *testing.T) {
	var f struct {
		Warnings map[string]struct {
			CauseEn string   `json:"cause_en"`
			CauseRu string   `json:"cause_ru"`
			FixEn   []string `json:"fix_en"`
			FixRu   []string `json:"fix_ru"`
		} `json:"warnings"`
	}
	if !readRegistryJSON(t, "warnings.json", &f) {
		return
	}
	codes := make([]string, 0, len(f.Warnings))
	for c := range f.Warnings {
		codes = append(codes, c)
	}
	sort.Strings(codes)

	for _, c := range codes {
		w := f.Warnings[c]
		if strings.TrimSpace(w.CauseEn) == "" {
			t.Errorf("%s: пустой cause_en — непонятно, откуда такое значение берётся", c)
		}
		if strings.TrimSpace(w.CauseRu) == "" {
			t.Errorf("%s: пустой cause_ru", c)
		}
		if len(w.FixEn) == 0 {
			t.Errorf("%s: пустой fix_en — человеку не сказано, что делать", c)
		}
		for i, fix := range w.FixEn {
			if strings.TrimSpace(fix) == "" {
				t.Errorf("%s: fix_en[%d] пуст", c, i)
			}
		}
		// Списки идут парой: UI берёт их по индексу языка, и разъехавшаяся
		// длина означала бы, что на одном языке совет пропал.
		if len(w.FixRu) != len(w.FixEn) {
			t.Errorf("%s: fix_ru (%d) и fix_en (%d) разной длины",
				c, len(w.FixRu), len(w.FixEn))
		}
	}
}
