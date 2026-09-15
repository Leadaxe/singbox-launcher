// File migration_v7_to_v8.go — миграция состояния v7 → v8 ПО СЫРОМУ ДОКУМЕНТУ
// (SPEC 127 §3).
//
// # Почему по документу, а не по типам
//
// Форму сменили сами записи: `body{name,match,outbound}` разъезжается на
// `name` + `body` = правило sing-box, плоская DNS-запись — на `tag` + `body`,
// `order_num` — на `num`. Прочитать v7-файл в v8-типы нельзя (поля не те), а
// заводить второй комплект типов «v7Rule/v7DNSServer/…» на весь корень значило
// бы дублировать `Source`, `Node`, `Direction` и все их нормализации.
//
// Поэтому верх документа разбирается как `map[string]json.RawMessage`: всё
// незнакомое (источники, направления, vars, любые будущие ключи) проносится
// СЫРЫМИ БАЙТАМИ, а трогаются ровно те ветки, форма которых изменилась.
// Результат тут же читает `parseV8`, поэтому порядок ключей ПРОМЕЖУТОЧНОГО
// документа значения не имеет — кроме байтов внутри `body` правил: там порядок
// ключей матчеров обязан дожить до файла (иначе canonical_roundtrip и golden
// поедут на ровном месте). Ради этого `match` берётся сырым, а цель
// дописывается в конец объекта СКЛЕЙКОЙ, а не через карту.
package state

import (
	"bytes"
	"encoding/json"
	"fmt"

	"singbox-launcher/internal/outboundutil"
)

// ── локальные зеркала v7-форм (живут только здесь) ─────────────────

// v7Rule — запись правила v7: цель и имя внутри тела, номер — `order_num`.
type v7Rule struct {
	Kind     RuleKind        `json:"kind"`
	ID       string          `json:"id"`
	Ref      string          `json:"ref"`
	Enabled  bool            `json:"enabled"`
	OrderNum *int            `json:"order_num"`
	Body     json.RawMessage `json:"body"`
}

// v7InlineBody — тело inline-правила v7. `Match` СЫРОЙ: порядок ключей
// матчеров переживает миграцию побайтно.
type v7InlineBody struct {
	Name     string          `json:"name"`
	Match    json.RawMessage `json:"match"`
	Outbound string          `json:"outbound"`
}

// v7SrsBody — тело srs-правила v7: один URL в `srs_url`, полный список в
// `srs_urls` (писался только при двух и более).
type v7SrsBody struct {
	Name     string   `json:"name"`
	SrsURL   string   `json:"srs_url"`
	SrsURLs  []string `json:"srs_urls"`
	Outbound string   `json:"outbound"`
}

// v7PresetBody — тело preset-правила v7.
type v7PresetBody struct {
	Vars map[string]string `json:"vars"`
}

// ── точка входа ────────────────────────────────────────────────────

// migrateV7DocToV8 переписывает байты v7-документа в v8-документ.
//
// Меняются: `meta.version`/`meta.schema`, `rules[]`, `dns_options` → `dns`,
// `warp_accounts` → `warp`, `sections` у корневых узлов `sources[]` и у
// `nodes[]` контейнеров. Всё остальное проносится как есть.
func migrateV7DocToV8(doc []byte, rep *MigrationReport) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(doc, &root); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8: %w", err)
	}

	if raw, ok := root["meta"]; ok {
		out, err := migrateV8Meta(raw)
		if err != nil {
			return nil, err
		}
		root["meta"] = out
	}

	if raw, ok := root["rules"]; ok {
		out, err := migrateV8Rules(raw, "rules", rep)
		if err != nil {
			return nil, err
		}
		root["rules"] = out
	}

	if raw, ok := root["dns_options"]; ok {
		out, err := migrateV8DNS(raw, "dns_options", rep)
		if err != nil {
			return nil, err
		}
		delete(root, "dns_options")
		root["dns"] = out
	}

	if raw, ok := root["warp_accounts"]; ok {
		delete(root, "warp_accounts")
		root["warp"] = raw
	}

	if raw, ok := root["sources"]; ok {
		out, err := migrateV8Sources(raw, rep)
		if err != nil {
			return nil, err
		}
		root["sources"] = out
	}

	return json.Marshal(root)
}

// migrateV8Meta — версия и имя схемы; остальные ключи шапки как есть.
func migrateV8Meta(raw json.RawMessage) (json.RawMessage, error) {
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8 meta: %w", err)
	}
	meta["version"] = json.RawMessage(fmt.Sprintf("%d", SchemaVersionV8))
	name, err := json.Marshal(SchemaNameV8)
	if err != nil {
		return nil, err
	}
	meta["schema"] = name
	return json.Marshal(meta)
}

// ── правила ────────────────────────────────────────────────────────

// migrateV8Rules — список правил в целевую форму.
func migrateV8Rules(raw json.RawMessage, where string, rep *MigrationReport) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
	}
	out := make([]json.RawMessage, 0, len(list))
	for i, item := range list {
		converted, err := migrateV8Rule(item, fmt.Sprintf("%s[%d]", where, i), rep)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return json.Marshal(out)
}

// migrateV8Rule — одна запись правила.
//
// Неизвестный kind проносится СЫРЫМ: чужая запись (или запись более новой
// стороны) не должна исчезать из файла из-за того, что мы её не поняли.
func migrateV8Rule(raw json.RawMessage, where string, rep *MigrationReport) (json.RawMessage, error) {
	var old v7Rule
	if err := json.Unmarshal(raw, &old); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
	}

	out := Rule{
		Kind:    old.Kind,
		ID:      old.ID,
		Ref:     old.Ref,
		Enabled: old.Enabled,
		Num:     old.OrderNum,
	}

	switch old.Kind {
	case RuleKindPreset:
		var body v7PresetBody
		if len(old.Body) > 0 {
			if err := json.Unmarshal(old.Body, &body); err != nil {
				return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
			}
		}
		if len(body.Vars) > 0 {
			out.Vars = body.Vars
		}

	case RuleKindInline:
		var body v7InlineBody
		if err := json.Unmarshal(old.Body, &body); err != nil {
			return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
		}
		out.Name = body.Name
		out.Body = joinRuleBodyWithTarget(body.Match, body.Outbound)

	case RuleKindSrs:
		var body v7SrsBody
		if err := json.Unmarshal(old.Body, &body); err != nil {
			return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
		}
		out.Name = body.Name
		refs := make([]string, 0, len(body.SrsURLs)+1)
		if body.SrsURL != "" {
			refs = append(refs, body.SrsURL)
		}
		refs = append(refs, body.SrsURLs...)
		out.Refs = dedupNonEmpty(refs)
		out.Body = joinRuleBodyWithTarget(nil, body.Outbound)

	default:
		rep.add("rule of unknown kind %q kept as is (%s)", string(old.Kind), where)
		// Запись проносится сырой — но ключ номера всё-таки переименовывается:
		// `order_num` v8-типы не читают, и первый же Save стёр бы его с диска,
		// а неразмеченная запись уехала бы на оси вперёд соседей.
		return renameOrderNumKey(raw), nil
	}

	return json.Marshal(out)
}

// renameOrderNumKey — `order_num` → `num` в сырой записи; остальные ключи и их
// порядок не трогаются.
//
// Нужен записям неизвестного вида, которые проносятся байтами: тип Rule читает
// только `num`, поэтому оставленный `order_num` молча потерялся бы при первом
// Save, а MarkRuleOrder раздал бы записи новый номер из пользовательского
// диапазона и переставил бы её относительно соседей.
func renameOrderNumKey(raw json.RawMessage) json.RawMessage {
	keys, vals, err := decodeObjectOrdered(raw)
	if err != nil {
		return raw
	}
	orderNum, hasOrder := vals["order_num"]
	if !hasOrder {
		return raw
	}
	if _, hasNum := vals["num"]; hasNum {
		// Обе формы сразу — авторитетна новая, старая просто снимается.
		orderNum = nil
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	writeKV := func(k string, v json.RawMessage) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(v)
	}
	for _, k := range keys {
		if k == "order_num" {
			if orderNum != nil {
				writeKV("num", orderNum)
			}
			continue
		}
		writeKV(k, vals[k])
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.Bytes())
}

// joinRuleBodyWithTarget — тело правила v8: сырые байты матчеров v7 плюс цель
// в форме sing-box, дописанная В КОНЕЦ объекта склейкой.
//
// Ровно склейка, а не пересборка через карту: `encoding/json` сортирует ключи
// карты по алфавиту, и порядок, в котором правило лежало в файле у
// пользователя, поменялся бы у всех сразу.
func joinRuleBodyWithTarget(match json.RawMessage, outbound string) json.RawMessage {
	target := map[string]interface{}{}
	outboundutil.ApplyOutboundToRule(target, outbound)

	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	writeKV := func(k string, raw json.RawMessage) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(raw)
	}

	// Матчеры — В ТОМ ЖЕ ПОРЯДКЕ и теми же байтами, что лежали в файле.
	//
	// Из `match` снимаются ТОЛЬКО те ключи, которые сейчас же перепишет цель:
	// иначе `action` внутри `match` пропал бы молча. В v7 такой `action` был
	// не целью, а самостоятельным эффектом правила (`sniff`, `hijack-dns`,
	// `resolve`) — v7-эмиттер копировал `match` дословно, и ключ доезжал до
	// config.json. Пустой v7-`outbound` цели не даёт (ApplyOutboundToRule на
	// пустой строке не пишет ничего), значит и снимать нечего: `action` и
	// `method` из тела остаются на своих местах.
	if keys, vals, err := decodeObjectOrdered(match); err == nil {
		for _, k := range keys {
			if _, overwritten := target[k]; overwritten {
				continue
			}
			// `outbound` рядом с целью-`action` (и наоборот) — дубль,
			// который сделал бы тело неоднозначным: цель уже дописана ниже.
			if len(target) > 0 && (k == "outbound" || k == "action" || k == "method") {
				continue
			}
			writeKV(k, vals[k])
		}
	}
	// Порядок дописки фиксирован: цель ApplyOutboundToRule выдаёт либо
	// `outbound`, либо `action` (+`method`).
	for _, k := range []string{"outbound", "action", "method"} {
		v, ok := target[k]
		if !ok {
			continue
		}
		vb, _ := json.Marshal(v)
		writeKV(k, vb)
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.Bytes())
}

// ── DNS ────────────────────────────────────────────────────────────

// migrateV8DNS — секция DNS: плоские тела записей уезжают в `body`.
func migrateV8DNS(raw json.RawMessage, where string, rep *MigrationReport) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, nil
	}
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
	}
	if servers, ok := section["servers"]; ok {
		out, err := migrateV8DNSServers(servers, where+".servers", rep)
		if err != nil {
			return nil, err
		}
		section["servers"] = out
	}
	if rules, ok := section["rules"]; ok {
		out, err := migrateV8DNSRules(rules, where+".rules", rep)
		if err != nil {
			return nil, err
		}
		section["rules"] = out
	}
	return json.Marshal(section)
}

// migrateV8DNSServers — список DNS-серверов.
func migrateV8DNSServers(raw json.RawMessage, where string, rep *MigrationReport) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, nil
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
	}
	out := make([]DNSServer, 0, len(list))
	for i, flat := range list {
		kind, _ := flat["kind"].(string)
		srv := DNSServer{Kind: DNSServerKind(kind)}
		srv.Tag, _ = flat["tag"].(string)
		srv.Ref, _ = flat["ref"].(string)
		srv.Enabled, _ = flat["enabled"].(bool)
		if srv.Kind == DNSServerKindUser {
			srv.Body = flatDNSBody(flat, srv.Tag, fmt.Sprintf("%s[%d]", where, i), rep)
		}
		// SPEC 129: значения переменных шаблонного сервера — поле записи.
		// Лаунчер до v8 их в запись не писал, но плоская запись, собранная
		// чужой рукой, могла их нести — молча не теряем.
		if srv.Kind == DNSServerKindTemplate {
			srv.Vars = flatStringMap(flat["vars"])
		}
		out = append(out, srv)
	}
	return json.Marshal(out)
}

// migrateV8DNSRules — список DNS-правил.
func migrateV8DNSRules(raw json.RawMessage, where string, rep *MigrationReport) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, nil
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8 %s: %w", where, err)
	}
	out := make([]DNSRule, 0, len(list))
	for _, flat := range list {
		kind, _ := flat["kind"].(string)
		r := DNSRule{Kind: DNSRuleKind(kind)}
		r.ID, _ = flat["id"].(string)
		r.Ref, _ = flat["ref"].(string)
		r.Name, _ = flat["name"].(string)
		r.Enabled, _ = flat["enabled"].(bool)
		if r.Kind == DNSRuleKindUser {
			// Метаданные записи наружу и ТОЛЬКО наружу: `id` и `name` —
			// поля записи (ONE_NAMESPACE §1), внутри `body` должен остаться
			// объект sing-box и ничего кроме. Списки исключений у сервера и
			// у правила обязаны совпадать по смыслу, иначе один ключ уезжает
			// в конфиг ядра, а второй нет.
			body := make(map[string]interface{}, len(flat))
			for k, v := range flat {
				switch k {
				case "kind", "ref", "enabled", "id", "name":
					continue
				}
				body[k] = v
			}
			if len(body) > 0 {
				r.Body = body
			}
		}
		out = append(out, r)
	}
	return json.Marshal(out)
}

// flatDNSBody — тело user-сервера из плоской v7-записи: метаданные наружу,
// `tag` из тела выброшен (он метаданные; расхождение — в отчёт).
func flatDNSBody(flat map[string]interface{}, tag, where string, rep *MigrationReport) map[string]interface{} {
	body := make(map[string]interface{}, len(flat))
	for k, v := range flat {
		switch k {
		case "kind", "ref", "enabled":
			continue
		case "tag":
			if inner, ok := v.(string); ok && inner != "" && inner != tag {
				rep.add("DNS server %s carried tag %q inside its body; the record tag %q wins", where, inner, tag)
			}
			continue
		}
		body[k] = v
	}
	if len(body) == 0 {
		return nil
	}
	return body
}

// ── источники: секции узлов ────────────────────────────────────────

// migrateV8Sources — секции корневых узлов и узлов контейнеров; всё остальное
// в источнике проносится сырыми байтами.
func migrateV8Sources(raw json.RawMessage, rep *MigrationReport) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, nil
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("state: migrate v7→v8 sources: %w", err)
	}
	for i := range list {
		if err := migrateV8NodeSections(list[i], fmt.Sprintf("sources[%d]", i), rep); err != nil {
			return nil, err
		}
		if err := migrateV8SourceIdentity(list[i], fmt.Sprintf("sources[%d]", i)); err != nil {
			return nil, err
		}
		nodesRaw, ok := list[i]["nodes"]
		if !ok {
			continue
		}
		if len(bytes.TrimSpace(nodesRaw)) == 0 || bytes.Equal(bytes.TrimSpace(nodesRaw), []byte("null")) {
			continue
		}
		var nodes []map[string]json.RawMessage
		if err := json.Unmarshal(nodesRaw, &nodes); err != nil {
			return nil, fmt.Errorf("state: migrate v7→v8 sources[%d].nodes: %w", i, err)
		}
		for j := range nodes {
			if err := migrateV8NodeSections(nodes[j], fmt.Sprintf("sources[%d].nodes[%d]", i, j), rep); err != nil {
				return nil, err
			}
		}
		out, err := json.Marshal(nodes)
		if err != nil {
			return nil, err
		}
		list[i]["nodes"] = out
	}
	return json.Marshal(list)
}

// migrateV8SourceIdentity — четыре плоских ключа подписки в один объект
// `identity` (SPEC 127 §6.0).
//
// Ключи снимаются сырыми байтами и кладутся под теми же именами внутрь
// объекта: у поля состояния и у поля контракта имена совпадают, поэтому
// переводить значения не нужно — меняется только уровень вложенности.
// Отсутствие всех четырёх ключей оставляет запись без `identity` вовсе:
// пустой объект в каждой подписке отличал бы два одинаковых состояния.
func migrateV8SourceIdentity(src map[string]json.RawMessage, where string) error {
	flat := []string{"user_agent", "send_hwid", "hwid", "hash_device_model"}
	identity := map[string]json.RawMessage{}
	for _, k := range flat {
		raw, ok := src[k]
		if !ok {
			continue
		}
		delete(src, k)
		if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		// Пустая строка UA/HWID в v7 значила «как в системе» — ровно то же,
		// что отсутствие ключа; переносить её значило бы завести объект
		// identity там, где пользователь ничего не переопределял.
		if bytes.Equal(bytes.TrimSpace(raw), []byte(`""`)) {
			continue
		}
		identity[k] = raw
	}
	if len(identity) == 0 {
		return nil
	}
	out, err := json.Marshal(identity)
	if err != nil {
		return fmt.Errorf("state: migrate v7→v8 %s.identity: %w", where, err)
	}
	src["identity"] = out
	return nil
}

// migrateV8NodeSections — `sections` одного узла теми же функциями, что корень:
// `sections.rules[]` как `rules[]`, `sections.dns.*` как `dns.*` (SPEC 127 §3).
func migrateV8NodeSections(node map[string]json.RawMessage, where string, rep *MigrationReport) error {
	raw, ok := node["sections"]
	if !ok {
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return fmt.Errorf("state: migrate v7→v8 %s.sections: %w", where, err)
	}
	if rules, ok := sections["rules"]; ok {
		out, err := migrateV8Rules(rules, where+".sections.rules", rep)
		if err != nil {
			return err
		}
		sections["rules"] = out
	}
	if dns, ok := sections["dns"]; ok {
		out, err := migrateV8DNS(dns, where+".sections.dns", rep)
		if err != nil {
			return err
		}
		sections["dns"] = out
	}
	out, err := json.Marshal(sections)
	if err != nil {
		return err
	}
	node["sections"] = out
	return nil
}

// ── вход для цепочки v5/v6 ─────────────────────────────────────────
//
// Правила и DNS у v6 (и у v5 после migrateV5ToV6) лежат в ТОЙ ЖЕ старой
// форме, что у v7: тело правила разобрано на части, DNS-запись плоская.
// Поэтому легаси-парсеры читают их теми же функциями, а не прямым unmarshal
// в типы v8 — иначе `name`/`refs`/`order_num` молча терялись бы.

// rulesFromLegacyShape — список правил старой формы в записи v8.
func rulesFromLegacyShape(raw json.RawMessage, where string, rep *MigrationReport) ([]Rule, error) {
	converted, err := migrateV8Rules(raw, where, rep)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(converted)) == 0 || bytes.Equal(bytes.TrimSpace(converted), []byte("null")) {
		return nil, nil
	}
	var out []Rule
	if err := json.Unmarshal(converted, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// dnsFromLegacyShape — DNS-секция старой (плоской) формы в записи v8.
func dnsFromLegacyShape(raw json.RawMessage, where string, rep *MigrationReport) (DNSOptions, error) {
	var out DNSOptions
	converted, err := migrateV8DNS(raw, where, rep)
	if err != nil {
		return out, err
	}
	if len(bytes.TrimSpace(converted)) == 0 || bytes.Equal(bytes.TrimSpace(converted), []byte("null")) {
		return out, nil
	}
	if err := json.Unmarshal(converted, &out); err != nil {
		return out, err
	}
	return out, nil
}

// flatStringMap — объект string→string плоской записи; не-строковые значения
// отбрасываются, пустая карта — nil (SPEC 129 Н1: пустой объект = нет ключа).
func flatStringMap(v interface{}) map[string]string {
	m, ok := v.(map[string]interface{})
	if !ok || len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if str, ok := val.(string); ok {
			out[k] = str
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
