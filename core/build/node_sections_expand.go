// File node_sections_expand.go — разворачивание секций узла (SPEC 121 §4 п. 2).
//
// # Что это
//
// Узел может нести с собой фрагменты конфига, без которых он бесполезен:
// DNS-сервер, привязанный к нему, DNS-правило на его домены, правило маршрута
// на его подсети. Хранятся они сырым JSON (порядок ключей значим), и превратить
// их в куски `dns` / `route` можно только здесь: до эмиссии финальный тег узла
// неизвестен.
//
// # Почему не через template.Preset
//
// Структура пресета типизирует DNS-сервер (`PresetDNSServer`), и незнакомое
// поле — `endpoint` tailscale-сервера, например — потерялось бы молча. Поэтому
// секции проходят мимо структуры пресета, а из его конвейера переиспользуются
// только примитивы: `TagSeparator` и строгая подстановка
// `template.SubstituteVarsInJSONStrict`.
//
// # Правила развёртывания
//
//  1. `@self` → финальный тег узла, в ЛЮБОМ строковом значении любой секции
//     (`outbound`, `detour`, `endpoint`, что угодно). Подстановка строгая:
//     любая другая `@var` роняет ФРАГМЕНТ (не всю секцию) с warning —
//     Dropped-каскад пофрагментный;
//  2. тег DNS-сервера префиксуется `<финальный тег>:<локальный тег>` — та же
//     схема, что у пресетов (D-012). Поле `server` DNS-правила получает тот же
//     префикс, ТОЛЬКО если равно локальному тегу сервера из этой же секции;
//     любое другое значение — ссылка на шаблонный/пресетный сервер и остаётся
//     как есть;
//  3. правило маршрута без `outbound` и без `action` получает
//     `outbound: <финальный тег>` — редактор ставит `@self` при сохранении, а
//     здесь то же делается защитно: state мог приехать из бэкапа;
//  4. правило маршрута с ключом `rule_set` выпадает с warning: в v1 секции
//     наборов правил не объявляют и на них не ссылаются, а висячая ссылка
//     роняет конфиг целиком.
package build

import (
	"bytes"
	"encoding/json"
	"fmt"

	"singbox-launcher/core/template"
)

// nodeSelfVar — единственное имя, доступное телу секции.
const nodeSelfVar = "self"

// NodeSectionFragments — развёрнутые секции одного узла, готовые к слиянию.
//
// Форма зеркалит PresetFragments в той части, которая нужна: карты
// `map[string]interface{}`, а не сырой JSON, потому что дальше их читают
// dedup по тегу, прун состава групп и починка висячих ссылок.
type NodeSectionFragments struct {
	// DNSServers — записи для dns.servers[] с префиксованными тегами.
	DNSServers []map[string]interface{}
	// DNSRules — записи для dns.rules[].
	DNSRules []map[string]interface{}
	// RoutingRules — записи для route.rules[] в порядке исходного списка.
	RoutingRules []map[string]interface{}
}

// IsEmpty — развернулось пусто (всё выпало или секций не было).
func (f NodeSectionFragments) IsEmpty() bool {
	return len(f.DNSServers) == 0 && len(f.DNSRules) == 0 && len(f.RoutingRules) == 0
}

// ExpandNodeSections разворачивает секции ОДНОГО узла.
//
// Возвращает фрагменты и список предупреждений (человеческие строки для
// WarnLog). Ошибки не возвращаются: каждая беда локальна — фрагмент выпадает,
// остальные едут дальше.
func ExpandNodeSections(set NodeSectionSet) (NodeSectionFragments, []string) {
	var out NodeSectionFragments
	var warns []string

	name := set.FinalTag
	if name == "" {
		name = set.Link.Tag
	}

	// Локальные теги серверов собираются ДО префиксации: по ним DNS-правило
	// узнаёт «свой» сервер (шаг 2). Собираются из СЫРЫХ тел — подстановка
	// `@self` тега не меняет, а фрагмент, который выпадет ниже, своим тегом
	// всё равно ссылаться некому.
	localServerTags := make(map[string]bool, len(set.DNSServers))

	for i, raw := range set.DNSServers {
		obj, ok := expandNodeFragment(raw, set.FinalTag, name, "dns_servers", i, &warns)
		if !ok {
			continue
		}
		localTag, _ := obj["tag"].(string)
		if localTag != "" {
			localServerTags[localTag] = true
			obj["tag"] = set.FinalTag + TagSeparator + localTag
		}
		out.DNSServers = append(out.DNSServers, obj)
	}

	for i, raw := range set.DNSRules {
		obj, ok := expandNodeFragment(raw, set.FinalTag, name, "dns_rules", i, &warns)
		if !ok {
			continue
		}
		// Префикс получает ТОЛЬКО ссылка на сервер из этой же секции: чужое
		// имя (шаблонный `local_dns_resolver`, пресетный `russian:yandex`)
		// префиксовать нельзя — ссылка уехала бы в никуда.
		if srv, _ := obj["server"].(string); srv != "" && localServerTags[srv] {
			obj["server"] = set.FinalTag + TagSeparator + srv
		}
		out.DNSRules = append(out.DNSRules, obj)
	}

	for i, raw := range set.Rules {
		obj, ok := expandNodeFragment(raw, set.FinalTag, name, "rules", i, &warns)
		if !ok {
			continue
		}
		if _, hasRuleSet := obj["rule_set"]; hasRuleSet {
			warns = append(warns, fmt.Sprintf(
				"node %q: rules[%d] carries %q — node sections neither declare nor reference rule sets; fragment dropped",
				name, i, "rule_set"))
			continue
		}
		// Правило без цели — про этот узел: иначе оно ничего не значит.
		_, hasOutbound := obj["outbound"]
		_, hasAction := obj["action"]
		if !hasOutbound && !hasAction && set.FinalTag != "" {
			obj["outbound"] = set.FinalTag
		}
		out.RoutingRules = append(out.RoutingRules, obj)
	}

	return out, warns
}

// expandNodeFragment — общий путь одного фрагмента: строгая подстановка
// `@self` и разбор в карту с сохранением числовой точности.
//
// Возвращает ok=false, когда фрагмент выпал; причина уже дописана в warns.
func expandNodeFragment(raw json.RawMessage, finalTag, name, section string, idx int, warns *[]string) (map[string]interface{}, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false
	}
	vars := []template.TemplateVar{{Name: nodeSelfVar, Type: "text"}}
	resolved := map[string]template.ResolvedVar{nodeSelfVar: {Scalar: finalTag}}
	substituted, _, err := template.SubstituteVarsInJSONStrict(raw, vars, resolved, targetOfNodeSections())
	if err != nil {
		*warns = append(*warns, fmt.Sprintf(
			"node %q: unresolved @var in %s[%d] — fragment dropped (%v)", name, section, idx, err))
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(substituted))
	dec.UseNumber()
	var obj map[string]interface{}
	if err := dec.Decode(&obj); err != nil {
		*warns = append(*warns, fmt.Sprintf(
			"node %q: %s[%d] is not a JSON object — fragment dropped (%v)", name, section, idx, err))
		return nil, false
	}
	if obj == nil {
		*warns = append(*warns, fmt.Sprintf("node %q: %s[%d] is empty — fragment dropped", name, section, idx))
		return nil, false
	}
	return obj, true
}

// targetOfNodeSections — TargetSpec для подстановки в секциях узла.
//
// Пустой намеренно: `#if`-предикатов и `@runtime.*` в секциях нет (SPEC 121
// «Чего в секциях нет»), а разрешить их значило бы завести у узла второй
// язык шаблона мимо пресетов.
func targetOfNodeSections() template.TargetSpec { return template.TargetSpec{} }
