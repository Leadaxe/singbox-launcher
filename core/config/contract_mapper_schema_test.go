package config

// Линтер формы ИСПОЛНЯЕМЫХ секций-мапперов реестра (SPEC 133).
//
// Зачем отдельный тест, а не проверка загрузчиком: загрузчик
// (core/config/registry/mapper.go) разбирает секции в типизированную модель и
// на неизвестный ключ МОЛЧИТ — encoding/json его просто выбрасывает. Ровно от
// этого молчания и затеяна кампания, поэтому рубеж нужен на данных: секция,
// написанная с опечаткой в имени атрибута или с атрибутом не из грамматики,
// обязана падать здесь, а не терять поле в рантайме.
//
// Почему свой обход, а не библиотека JSON Schema: в модуле её нет, и тащить
// зависимость ради одного линтера дороже, чем сорок строк обхода. Проверяется
// то, ради чего схема и писалась, — ЗАКРЫТОСТЬ множества имён
// (additionalProperties:false) на каждом уровне; значения атрибутов судит
// загрузчик и корпус.
//
// Схема и реестр читаются с диска (не из embed): линтер ходит по ИСХОДНИКУ
// контракта, включая файлы, которые в бинарь не вшиты.
//
// go1.20-совместимо: без slices/maps/min/max (легаси-джоба win7).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mapperSchemaPath / mapperRegistryDir — адреса относительно core/config.
const (
	mapperSchemaPath  = "../../contract/schema/registry_mapper.schema.json"
	mapperRegistryDir = "../../contract/registry"
)

// jsonSchemaNode — тот минимум схемы draft-07, которым описаны секции-мапперы:
// объект с закрытым набором свойств, ссылки внутрь documents, композиция oneOf
// и однородные additionalProperties (карта «имя → запись»).
type jsonSchemaNode struct {
	Ref                  string                     `json:"$ref"`
	Type                 any                        `json:"type"`
	Properties           map[string]*jsonSchemaNode `json:"properties"`
	AdditionalProperties json.RawMessage            `json:"additionalProperties"`
	Items                *jsonSchemaNode            `json:"items"`
	OneOf                []*jsonSchemaNode          `json:"oneOf"`
	Enum                 []any                      `json:"enum"`
}

type jsonSchemaDoc struct {
	Definitions map[string]*jsonSchemaNode `json:"definitions"`
	Properties  map[string]*jsonSchemaNode `json:"properties"`
}

// additional разбирает additionalProperties в две различимые формы: false
// (набор имён закрыт) и схема-значение (карта однородных записей).
func (n *jsonSchemaNode) additional() (closed bool, value *jsonSchemaNode) {
	if len(n.AdditionalProperties) == 0 {
		return false, nil
	}
	var b bool
	if err := json.Unmarshal(n.AdditionalProperties, &b); err == nil {
		return !b, nil
	}
	var sub jsonSchemaNode
	if err := json.Unmarshal(n.AdditionalProperties, &sub); err == nil {
		return false, &sub
	}
	return false, nil
}

// TestContractMapperSectionsMatchSchema — все секции `mappers` протоколов и все
// диалектные `blocks` общих файлов написаны атрибутами из грамматики.
func TestContractMapperSectionsMatchSchema(t *testing.T) {
	raw, err := os.ReadFile(mapperSchemaPath)
	if err != nil {
		t.Fatalf("схема мапперов не читается: %v", err)
	}
	var doc jsonSchemaDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("схема мапперов не разбирается: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(mapperRegistryDir, "protocols", "*.json"))
	if err != nil {
		t.Fatalf("список протоколов: %v", err)
	}
	common, err := filepath.Glob(filepath.Join(mapperRegistryDir, "*.json"))
	if err != nil {
		t.Fatalf("список общих файлов: %v", err)
	}
	files = append(files, common...)
	sort.Strings(files)

	checked := 0
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal(body, &top); err != nil {
			t.Fatalf("%s: не разбирается: %v", file, err)
		}
		name := filepath.Base(file)

		// mappers.<kind> — секция на вид источника.
		if rawMappers, ok := top["mappers"]; ok {
			var mappers map[string]any
			if err := json.Unmarshal(rawMappers, &mappers); err != nil {
				t.Errorf("%s: mappers не разбирается: %v", name, err)
				continue
			}
			kinds := sortedKeys(mappers)
			for _, kind := range kinds {
				checked++
				errs := validateAgainst(&doc, doc.Definitions["mapper"], mappers[kind],
					fmt.Sprintf("%s: mappers.%s", name, kind))
				reportSchemaErrors(t, errs)
			}
		}

		// source_kinds.kinds[] — виды источника (source_kinds.json).
		// Судятся тем же линтером и тем же определением схемы: уровень
		// документа не должен получить собственный, более слабый рубеж —
		// опечатка в имени атрибута теряется здесь так же молча, как в
		// секции-маппере.
		if rawKinds, ok := top["source_kinds"]; ok {
			var spec struct {
				Kinds []any `json:"kinds"`
			}
			if err := json.Unmarshal(rawKinds, &spec); err != nil {
				t.Errorf("%s: source_kinds не разбирается: %v", name, err)
				continue
			}
			errs := validateAgainst(&doc, doc.Properties["source_kinds"], rawJSONAny(t, rawKinds),
				fmt.Sprintf("%s: source_kinds", name))
			reportSchemaErrors(t, errs)
			checked += len(spec.Kinds)
		}

		// blocks.<блок>.<диалект> — записи общих блоков (tls.json, transports.json):
		// по форме это те же params, поэтому судятся определением param.
		if rawBlocks, ok := top["blocks"]; ok {
			var blocks map[string]any
			if err := json.Unmarshal(rawBlocks, &blocks); err != nil {
				t.Errorf("%s: blocks не разбирается: %v", name, err)
				continue
			}
			// Глубина вложенности у блоков РАЗНАЯ, и это не небрежность:
			// tls.json несёт диалект сразу (blocks.uri.<запись>), а
			// transports.json — ещё уровень транспорта
			// (blocks.uri.ws.<запись>). Раскладывать их одной формулой нельзя,
			// поэтому уровень записи узнаётся по самому значению: объект,
			// у которого есть `source` либо `$ref`, — это запись param;
			// объект без них — ещё один уровень группировки.
			for _, blockName := range sortedKeys(blocks) {
				sub, ok := blocks[blockName].(map[string]any)
				if !ok {
					continue // note — строка, не блок
				}
				checked += lintBlockLevel(t, &doc, sub, fmt.Sprintf("%s: blocks.%s", name, blockName))
			}
		}
	}

	if checked == 0 {
		t.Fatal("линтер не нашёл ни одной секции-маппера — проверь адреса реестра")
	}
	t.Logf("проверено записей грамматики: %d", checked)
}

// lintBlockLevel — рекурсивный спуск по уровням группировки блока до записей.
// Запись узнаётся по наличию `source` или `$ref`: у групп (диалект, транспорт)
// таких ключей нет. Возвращает число проверенных записей.
func lintBlockLevel(t *testing.T, doc *jsonSchemaDoc, level map[string]any, path string) int {
	t.Helper()
	checked := 0
	for _, key := range sortedKeys(level) {
		value, ok := level[key].(map[string]any)
		if !ok {
			continue // note и прочая проза
		}
		_, isEntry := value["source"]
		if _, ok := value["$ref"]; ok {
			isEntry = true
		}
		if isEntry {
			checked++
			errs := validateAgainst(doc, doc.Definitions["param"], value, path+"."+key)
			reportSchemaErrors(t, errs)
			continue
		}
		checked += lintBlockLevel(t, doc, value, path+"."+key)
	}
	return checked
}

func reportSchemaErrors(t *testing.T, errs []string) {
	t.Helper()
	for _, e := range errs {
		t.Error(e)
	}
}

// rawJSONAny перекладывает сырой фрагмент в any: обходчик схемы работает по
// динамическому значению (map/[]any), а не по типизированной модели.
func rawJSONAny(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("фрагмент не разбирается: %v", err)
	}
	return v
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// validateAgainst — обход значения по узлу схемы. Собирает ВСЕ нарушения, а не
// падает на первом: список расхождений целиком полезнее одной строки.
func validateAgainst(doc *jsonSchemaDoc, node *jsonSchemaNode, value any, path string) []string {
	if node == nil {
		return nil
	}
	if node.Ref != "" {
		return validateAgainst(doc, resolveRef(doc, node.Ref), value, path)
	}
	if len(node.OneOf) > 0 {
		// Достаточно одной подходящей ветки; сообщаем, только если не подошла ни одна.
		for _, branch := range node.OneOf {
			if len(validateAgainst(doc, branch, value, path)) == 0 {
				return nil
			}
		}
		// Ветки могут быть чистыми типами без properties — тогда судить нечего.
		if !hasNamedShape(doc, node.OneOf) {
			return nil
		}
		return []string{fmt.Sprintf("%s: не подходит ни под одну ветку oneOf", path)}
	}

	switch typed := value.(type) {
	case map[string]any:
		closed, valueSchema := node.additional()
		// Узел без properties и без additionalProperties — СВОБОДНАЯ форма
		// (sets, implies, when, value_map: ключ там — путь тела или значение
		// чужого диалекта, а не имя атрибута). Обход обязан остановиться:
		// иначе имена из свободной карты судились бы набором родителя, и
		// "sets.tls" объявлялось бы «атрибутом не из грамматики».
		if len(node.Properties) == 0 && valueSchema == nil {
			return nil
		}
		var errs []string
		for _, key := range sortedKeys(typed) {
			if sub, ok := node.Properties[key]; ok {
				errs = append(errs, validateAgainst(doc, sub, typed[key], path+"."+key)...)
				continue
			}
			if valueSchema != nil {
				errs = append(errs, validateAgainst(doc, valueSchema, typed[key], path+"."+key)...)
				continue
			}
			if closed {
				errs = append(errs, fmt.Sprintf("%s.%s: атрибут не из грамматики (схема закрыта; известны: %s)",
					path, key, strings.Join(namesOf(node.Properties), ", ")))
			}
		}
		return errs
	case []any:
		if node.Items == nil {
			return nil
		}
		var errs []string
		for i, item := range typed {
			errs = append(errs, validateAgainst(doc, node.Items, item, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return errs
	case string:
		if len(node.Enum) > 0 && !enumHas(node.Enum, typed) {
			return []string{fmt.Sprintf("%s: значение %q вне набора схемы", path, typed)}
		}
		return nil
	}
	return nil
}

// hasNamedShape — есть ли среди веток oneOf хоть одна с именованными
// свойствами: только такие ветки и можно судить этим обходом.
func hasNamedShape(doc *jsonSchemaDoc, branches []*jsonSchemaNode) bool {
	for _, b := range branches {
		if b == nil {
			continue
		}
		node := b
		if node.Ref != "" {
			node = resolveRef(doc, node.Ref)
		}
		if node != nil && (len(node.Properties) > 0 || len(node.Enum) > 0) {
			return true
		}
	}
	return false
}

func enumHas(enum []any, value string) bool {
	for _, e := range enum {
		if s, ok := e.(string); ok && s == value {
			return true
		}
	}
	return false
}

func namesOf(props map[string]*jsonSchemaNode) []string {
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func resolveRef(doc *jsonSchemaDoc, ref string) *jsonSchemaNode {
	const prefix = "#/definitions/"
	if !strings.HasPrefix(ref, prefix) {
		return nil
	}
	return doc.Definitions[strings.TrimPrefix(ref, prefix)]
}

// TestContractUserInfoDeclaresSingleInto — у каждой секции с `userinfo`
// написано, куда едет ОДИНОЧНЫЙ (беспарный) userinfo.
//
// Умолчания «первое имя `into`» быть не должно — договорённость с LxBox
// (`TASKS_LXBOX` §24.29 п. 3). Причина не в аккуратности: позиция и смысл
// расходятся ровно там, где ошибиться дороже всего. У naive одиночный
// userinfo это ПАРОЛЬ, хотя `into[0]` зовётся `username`; у ssh и socks —
// наоборот имя, потому что пароля в такой ссылке нет по протоколу; у
// shadowsocks `into[0]` вообще ШИФР, а не учётные данные. Молчащий атрибут
// читается как «здесь думать не надо» — и каждый раз это неправда.
//
// Линтер требует ЯВНОГО написания даже там, где значение совпадает с
// `into[0]`: разница между «совпало» и «никто не смотрел» видна только в
// тексте секции.
func TestContractUserInfoDeclaresSingleInto(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(mapperRegistryDir, "protocols", "*.json"))
	if err != nil {
		t.Fatalf("обход протоколов: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("файлов протоколов не найдено")
	}

	checked := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var doc struct {
			Mappers map[string]struct {
				UserInfo *struct {
					Into       []string `json:"into"`
					SingleInto string   `json:"single_into"`
				} `json:"userinfo"`
			} `json:"mappers"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		for kind, m := range doc.Mappers {
			if m.UserInfo == nil {
				continue
			}
			checked++
			where := fmt.Sprintf("%s mappers.%s.userinfo", name, kind)
			if m.UserInfo.SingleInto == "" {
				t.Errorf("%s: не объявлен single_into при into=%v — "+
					"допиши, куда едет БЕСПАРНЫЙ userinfo; умолчания «первое имя into» нет",
					where, m.UserInfo.Into)
				continue
			}
			// Цель обязана быть одной из объявленных: имя вне `into`
			// пространство userinfo не перенаправляет, и запись, читающая
			// `userinfo.user` напрямую, получила бы тот же текст вторым
			// путём (Q133-43).
			found := false
			for _, n := range m.UserInfo.Into {
				if n == m.UserInfo.SingleInto {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: single_into=%q нет среди into=%v — "+
					"цель вне into не перенаправляет само пространство",
					where, m.UserInfo.SingleInto, m.UserInfo.Into)
			}
		}
	}
	if checked == 0 {
		t.Fatal("ни одной секции с userinfo не проверено — линтер смотрит не туда")
	}
	t.Logf("проверено секций с userinfo: %d", checked)
}
