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
var registryNormalizeModes = map[string]bool{
	"trim": true, "lower": true, "trim_lower": true, "hex_only": true,
}

// registryOnInvalidActions — допустимые действия on_invalid.
var registryOnInvalidActions = map[string]bool{
	"drop": true, "coerce": true, "drop_node": true,
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
	Skip           string                `json:"skip"`
	DescEn         string                `json:"desc_en"`
	DescRu         string                `json:"desc_ru"`
}

// bodyDefaultWhen — дефолт, который реестр велит МАТЕРИАЛИЗОВАТЬ явно
// (SPEC 131 §3.2): обычные `default` в тело не пишутся.
type bodyDefaultWhen struct {
	Absent bool        `json:"absent"`
	Value  interface{} `json:"value"`
	Code   string      `json:"code"`
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
	Core     string                `json:"core"`
	Order    []string              `json:"order"`
	Fields   map[string]*bodyField `json:"fields"`
	Skipped  map[string]string     `json:"skipped"`
	Variants map[string]*struct {
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
