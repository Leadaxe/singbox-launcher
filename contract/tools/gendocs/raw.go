package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"singbox-launcher/contract"
	"singbox-launcher/core/config/registry"
)

// rawRegistry — те части реестра, которые загрузчику лаунчера не нужны и им не
// разбираются: метаданные схемы и секция `uri.*` (параметры ссылки).
// Санитайзер и эмиттер работают только по `body`, а документации нужен вход —
// то, что человек видит в ссылке. Значения здесь не интерпретируются: они
// переносятся на страницу как есть.
type rawRegistry struct {
	protocols map[string]*rawProtocol
	// tlsParams/realityParams/transportParams — общие параметры ссылки,
	// вынесенные из схем в tls.json и transports.json.
	tlsParams      map[string]*uriParam
	tlsOrder       []string
	realityParams  map[string]*uriParam
	realityOrder   []string
	transports     map[string]*rawTransport
	transportOrder []string
	version        string

	// reg — разрешённый реестр. Странице схемы он нужен, чтобы по адресу
	// `maps_to` параметра ссылки найти правило ПОЛЯ ТЕЛА: только оно знает,
	// что случится с негодным значением, а сам параметр ссылки решений о
	// значениях не принимает.
	reg *registry.Registry

	// mapper — описательные переводы маппера: общие (tls/transports) и
	// схемные. Правилами `body` они не выражаются, потому что санитайзер
	// видит уже готовое тело.
	sharedMapper []mapperRule
	schemeMapper map[string][]mapperRule
}

// mapperRule — запись секции `mapper` реестра (SPEC 131 W2d §2).
// `impl` объявлен, чтобы заметка разработчика не попала в документацию
// случайно вместе с прочим.
type mapperRule struct {
	ID        string   `json:"id"`
	AppliesTo []string `json:"applies_to"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Kind      string   `json:"kind"`
	Code      string   `json:"code"`
	DescEn    string   `json:"desc_en"`
	DescRu    string   `json:"desc_ru"`
	Impl      string   `json:"impl"` // в документацию НЕ попадает
}

// appliesTo говорит, относится ли общее правило к схеме.
func (m mapperRule) appliesTo(scheme string) bool {
	for _, s := range m.AppliesTo {
		if s == scheme {
			return true
		}
	}
	return false
}

// bodyField — поле тела схемы по его пути. Возвращает nil, если схема или
// поле неизвестны: у части схем тела в реестре нет вовсе (group).
func (r *rawRegistry) bodyField(scheme, path string) *registry.Field {
	if r.reg == nil {
		return nil
	}
	f, ok := r.reg.Field(scheme, path)
	if !ok {
		return nil
	}
	return f
}

// mapperFor — все переводы, относящиеся к схеме: сперва её собственные,
// затем общие. Порядок фиксирован файлами реестра, вывод детерминирован.
func (r *rawRegistry) mapperFor(scheme string) []mapperRule {
	out := make([]mapperRule, 0, len(r.schemeMapper[scheme])+len(r.sharedMapper))
	out = append(out, r.schemeMapper[scheme]...)
	for _, m := range r.sharedMapper {
		if m.appliesTo(scheme) {
			out = append(out, m)
		}
	}
	return out
}

type rawProtocol struct {
	Scheme      string          `json:"scheme"`
	Kind        string          `json:"kind"`
	SingboxType string          `json:"singbox_type"`
	Aliases     []string        `json:"aliases"`
	Sources     []string        `json:"sources"`
	Extension   *string         `json:"extension"`
	Note        string          `json:"note"`
	URI         *rawURI         `json:"uri"`
	Mapper      []mapperRule    `json:"mapper"`
	Body        json.RawMessage `json:"body"`
}

type rawURI struct {
	Userinfo *uriParam            `json:"userinfo"`
	Query    map[string]*uriParam `json:"query"`
	Fragment string               `json:"fragment"`

	queryOrder []string
}

// uriParam — параметр ссылки. Набор атрибутов — словарь секции uri.* реестра;
// `impl` объявлен, чтобы не попасть в документацию случайно вместе с прочим.
type uriParam struct {
	Type       string      `json:"type"`
	Values     []string    `json:"values"`
	Allowlist  string      `json:"allowlist"`
	Default    interface{} `json:"default"`
	Aliases    []string    `json:"aliases"`
	MapsTo     interface{} `json:"maps_to"`
	Meaning    string      `json:"meaning"`
	Ext        string      `json:"ext"`
	Validation string      `json:"validation"`
	DropAlways bool        `json:"drop_always"`
	Support    string      `json:"support"`
	DescEn     string      `json:"desc_en"`
	DescRu     string      `json:"desc_ru"`
	Impl       string      `json:"impl"` // в документацию НЕ попадает
}

type rawTransport struct {
	SingboxType string `json:"singbox_type"`
	MapsTo      string `json:"maps_to"`

	// Params заполняется поэлементно (paramMap), а не тегом: рядом с
	// параметрами в секции лежат служебные строки.
	Params     map[string]*uriParam `json:"-"`
	name       string
	paramOrder []string
}

// mapsToPaths нормализует maps_to: реестр пишет либо строку, либо список путей
// (ws.ed → max_early_data + early_data_header_name).
func (p *uriParam) mapsToPaths() []string {
	switch v := p.MapsTo.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func loadRaw(reg *registry.Registry) (*rawRegistry, error) {
	r := &rawRegistry{
		protocols:    map[string]*rawProtocol{},
		schemeMapper: map[string][]mapperRule{},
		reg:          reg,
	}

	entries, err := fs.ReadDir(contract.Registry, "registry/protocols")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		data, err := contract.ReadRegistry("protocols/" + e.Name())
		if err != nil {
			return nil, err
		}
		p := &rawProtocol{}
		if err := json.Unmarshal(data, p); err != nil {
			return nil, fmt.Errorf("protocols/%s: %w", e.Name(), err)
		}
		if p.Scheme == "" {
			p.Scheme = name
		}
		if p.URI != nil {
			p.URI.queryOrder = orderedKeys(data, "uri", "query")
		}
		if len(p.Mapper) > 0 {
			r.schemeMapper[name] = p.Mapper
		}
		r.protocols[name] = p
	}

	tlsData, err := contract.ReadRegistry("tls.json")
	if err != nil {
		return nil, err
	}
	// Общие переводы маппера лежат рядом с общими параметрами ссылки: TLS в
	// tls.json, транспорты в transports.json.
	if shared, err := readMapper(tlsData); err != nil {
		return nil, fmt.Errorf("tls.json: %w", err)
	} else {
		r.sharedMapper = append(r.sharedMapper, shared...)
	}
	// Секции tls.params и tls.reality несут не только параметры: рядом лежат
	// служебные заметки строкой (tls.reality.impl). Разбор поэлементный, всё,
	// что не объект, пропускается — это не параметр ссылки.
	r.tlsParams, r.tlsOrder, err = paramMap(tlsData, "tls", "params")
	if err != nil {
		return nil, fmt.Errorf("tls.json: %w", err)
	}
	r.realityParams, r.realityOrder, err = paramMap(tlsData, "tls", "reality")
	if err != nil {
		return nil, fmt.Errorf("tls.json: %w", err)
	}

	trData, err := contract.ReadRegistry("transports.json")
	if err != nil {
		return nil, err
	}
	if shared, err := readMapper(trData); err != nil {
		return nil, fmt.Errorf("transports.json: %w", err)
	} else {
		r.sharedMapper = append(r.sharedMapper, shared...)
	}
	var trFile struct {
		Transports map[string]*rawTransport `json:"transports"`
	}
	if err := json.Unmarshal(trData, &trFile); err != nil {
		return nil, fmt.Errorf("transports.json: %w", err)
	}
	r.transports = trFile.Transports
	r.transportOrder = orderedKeys(trData, "transports")
	for name, tr := range r.transports {
		tr.name = name
		tr.Params, tr.paramOrder, err = paramMap(trData, "transports", name, "params")
		if err != nil {
			return nil, fmt.Errorf("transports.json: %s: %w", name, err)
		}
	}

	return r, nil
}

// readMapper читает секцию `mapper` файла реестра.
func readMapper(data []byte) ([]mapperRule, error) {
	var f struct {
		Mapper []mapperRule `json:"mapper"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Mapper, nil
}

// schemes — схемы реестра в алфавитном порядке (порядок файлов на диске
// нормой не закреплён, поэтому он задаётся здесь).
func (r *rawRegistry) schemes() []string {
	out := make([]string, 0, len(r.protocols))
	for name := range r.protocols {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// orderedKeys возвращает ключи объекта JSON в том порядке, в каком они лежат в
// файле. Порядок параметров ссылки в реестре осмыслен (userinfo → значимые
// параметры → редкие), и терять его ради сортировки незачем; при этом порядок
// фиксирован файлом, то есть вывод остаётся детерминированным.
func orderedKeys(data []byte, path ...string) []string {
	var node interface{}
	if err := json.Unmarshal(data, &node); err != nil {
		return nil
	}
	cur := node
	for _, p := range path {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		cur, ok = m[p]
		if !ok {
			return nil
		}
	}
	m, ok := cur.(map[string]interface{})
	if !ok {
		return nil
	}
	// json.Unmarshal в map теряет порядок — восстанавливаем его сканированием
	// исходного текста по именам ключей этого объекта.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	raw := rawObject(data, path...)
	if raw == nil {
		return keys
	}
	ordered := scanKeyOrder(raw, m)
	if len(ordered) != len(keys) {
		return keys
	}
	return ordered
}

func rawObject(data []byte, path ...string) []byte {
	cur := json.RawMessage(data)
	for _, p := range path {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(cur, &m); err != nil {
			return nil
		}
		next, ok := m[p]
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// scanKeyOrder читает ключи объекта потоковым декодером — единственный способ
// узнать порядок, который map в Go не хранит.
func scanKeyOrder(raw []byte, want map[string]interface{}) []string {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	out := make([]string, 0, len(want))
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
		if _, ok := want[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

// paramMap разбирает объект реестра как карту параметров ссылки, пропуская
// всё, что параметром не является (служебные заметки строкой рядом с
// параметрами: tls.reality.impl и подобные). Второе значение — порядок ключей
// в файле.
func paramMap(data []byte, path ...string) (map[string]*uriParam, []string, error) {
	raw := rawObject(data, path...)
	if raw == nil {
		return map[string]*uriParam{}, nil, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, nil, err
	}
	out := make(map[string]*uriParam, len(entries))
	for k, v := range entries {
		p := &uriParam{}
		if err := json.Unmarshal(v, p); err != nil {
			continue // не объект — не параметр
		}
		out[k] = p
	}
	order := orderedKeys(data, path...)
	kept := make([]string, 0, len(order))
	for _, k := range order {
		if _, ok := out[k]; ok {
			kept = append(kept, k)
		}
	}
	return out, kept, nil
}

// schemeFileName — имя страницы схемы.
func schemeFileName(scheme string) string {
	return path.Clean(scheme) + ".md"
}
