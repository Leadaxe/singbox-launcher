package build

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DNSConfig — clean-input для MergeDNSSection без зависимостей от UI-слоя.
//
// Соответствует полям WizardModel.DNS*; вызывающий слой (wizard / Configurator)
// извлекает их в эту структуру перед передачей в core/build. Это держит
// core/build pure: без знаний об UI-моделях.
type DNSConfig struct {
	// Servers — каждый элемент — sing-box dns server-объект с опциональными
	// wizard-only ключами "description" (string) и "enabled" (bool, default true).
	// Wizard-only поля стрипаются перед merge'ом.
	Servers []json.RawMessage

	// RulesText — текст из редактора DNS rules. Принимает форматы:
	//   - полный JSON-объект {"rules":[...]}
	//   - голый JSON-массив [...]
	//   - legacy-многострочный (один объект на строку; # и пустые — комментарии)
	RulesText string

	// Final — переопределение dns.final. Пусто → используется тег первого enabled-сервера.
	Final string

	// Strategy — необязательное переопределение dns.strategy.
	Strategy string

	// SPEC: IndependentCache УДАЛЕНО — deprecated в sing-box 1.14.0
	// (кэш всегда per-transport). Emit поля прекращён, поле сняти из конфига.
}

// MergeDNSSection накладывает DNSConfig поверх шаблонного `dns` JSON-объекта,
// сохраняя любые неизвестные ключи шаблона. Совместимо с поведением
// `ui/wizard/business/wizard_dns.go::MergeDNSSection` для байт-в-байт паритета.
//
// Поведение:
//  1. Парсит templateDNS как map (если пуст — пустой map);
//  2. Заменяет ключ "servers" на отфильтрованный + stripped список из cfg.Servers
//     (wizard-only поля description/enabled убираются; disabled-серверы пропускаются);
//  3. Парсит cfg.RulesText → массив объектов; кладёт под ключ "rules";
//  4. cfg.Final (или fallback — тег первого enabled-сервера) → "final";
//     пусто и нет fallback'а — ключ удаляется из map;
//  5. cfg.Strategy непустой → "strategy"; иначе ключ из шаблона остаётся;
// (SPEC: independent_cache emit удалён — sing-box 1.14 deprecation.)
//
// Pure: без I/O, без shared state.
func MergeDNSSection(templateDNS json.RawMessage, cfg DNSConfig) (json.RawMessage, error) {
	var dnsObj map[string]interface{}
	if len(templateDNS) > 0 {
		if err := json.Unmarshal(templateDNS, &dnsObj); err != nil {
			return nil, fmt.Errorf("template dns: %w", err)
		}
	}
	if dnsObj == nil {
		dnsObj = make(map[string]interface{})
	}

	servers := make([]interface{}, 0, len(cfg.Servers))
	for _, raw := range cfg.Servers {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("dns server: %w", err)
		}
		if !dnsServerEnabled(m) {
			continue
		}
		// SPEC 053 v6 path: legacyDNSOptionsFromV6 эмитит template_servers
		// override'ы как `{tag, enabled}` без type/server — это маркеры для UI
		// (показать template-сервер с tag X как enabled/disabled), а не полные
		// server-config'и. Они НЕ должны попасть в финальный sing-box config
		// (без type sing-box 1.12+ считает это legacy DNS format и валит).
		// Реальные template servers с полной body уже в template dns.servers
		// (dnsObj['servers'] до этого merge'а), их фильтрация по enabled
		// override'у — задача MergePresetsIntoDNS.
		if !dnsServerHasBody(m) {
			continue
		}
		servers = append(servers, stripDNSWizardOnlyFields(m))
	}
	dnsObj["servers"] = servers

	rules, err := ParseDNSRulesText(cfg.RulesText)
	if err != nil {
		return nil, err
	}
	dnsObj["rules"] = rules

	final := strings.TrimSpace(cfg.Final)
	if final == "" {
		final = firstEnabledDNSServerTag(cfg.Servers)
	}
	if final != "" {
		dnsObj["final"] = final
	} else {
		delete(dnsObj, "final")
	}

	if s := strings.TrimSpace(cfg.Strategy); s != "" {
		dnsObj["strategy"] = s
	}
	// SPEC: independent_cache emit удалён (sing-box 1.14 deprecation).
	// Если в шаблоне случайно остался ключ — defensive cleanup, чтобы emit
	// не содержал deprecated поле.
	delete(dnsObj, "independent_cache")
	return json.Marshal(dnsObj)
}

// dnsServerHasBody — true, если у DNS-server entry есть хотя бы одно поле
// определяющее реальный сервер (type / server / address). Иначе это
// wizard-only override marker (`{tag, enabled}`), не для emit в config.
func dnsServerHasBody(m map[string]interface{}) bool {
	for _, k := range []string{"type", "server", "address"} {
		if v, ok := m[k]; ok && v != nil {
			return true
		}
	}
	return false
}

// dnsServerEnabled — true, если у объекта DNS-сервера wizard-only поле
// "enabled" отсутствует, нечитается как bool, или равно true.
// Соответствует: missing/invalid → enabled (как у sing-box: нет такого поля).
func dnsServerEnabled(m map[string]interface{}) bool {
	v, ok := m["enabled"]
	if !ok || v == nil {
		return true
	}
	b, ok := v.(bool)
	if !ok {
		return true
	}
	return b
}

// stripDNSWizardOnlyFields убирает wizard-only ключи перед merge'ом в
// финальный config.json: "description"/"enabled"/"required" (UI поля),
// "title" (preset UI label), "if"/"if_or" (preset условия — уже резолвлены
// к моменту эмита), "default_enabled" (legacy template default — keep for
// backward compat), и любые ключи с префиксом "_" (sing-box их отвергает
// strict-decoder'ом начиная с 1.12+).
//
// **Always returns a NEW map** — caller'у безопасно передавать оригинал из
// state/template без боязни мутировать его (SPEC 056 pattern: immutable
// inputs, defensive copies перед any emit).
//
// Используется во всех путях DNS server emit:
//   - MergeDNSSection (legacy v5 cfg.Servers)
//   - MergePresetsIntoDNS (preset bundled + state.DNS.ExtraServers)
//
// Single source of truth — если sing-box добавит ещё одно «unknown field»
// замечание в будущем, патчим здесь, не в нескольких местах.
func stripDNSWizardOnlyFields(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch k {
		case "description", "enabled", "required", "title", "if", "if_or", "default_enabled":
			continue
		}
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		out[k] = v
	}
	return out
}

// firstEnabledDNSServerTag возвращает тег первого enabled-сервера в списке.
// Используется как fallback для dns.final, если cfg.Final пуст.
// Возвращает пустую строку, если нет ни одного enabled-сервера с тегом.
func firstEnabledDNSServerTag(servers []json.RawMessage) string {
	for _, raw := range servers {
		var o map[string]interface{}
		if json.Unmarshal(raw, &o) != nil {
			continue
		}
		if !dnsServerEnabled(o) {
			continue
		}
		if t, ok := o["tag"].(string); ok {
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		}
	}
	return ""
}

// ParseDNSRulesText парсит текст редактора DNS-rules в массив правил.
//
// Принимает (в порядке предпочтения):
//   - JSON-объект {"rules":[...]} — канонический формат
//   - JSON-массив [...]
//   - один JSON-объект (трактуется как одно правило)
//   - legacy-многострочный: один объект на строку; "#" и пустые строки — комментарии
//
// Пустой/whitespace-only вход → (nil, nil) — вызывающий обнулит `rules` в DNS-объекте.
func ParseDNSRulesText(text string) ([]interface{}, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	var root interface{}
	if err := json.Unmarshal([]byte(text), &root); err == nil {
		switch v := root.(type) {
		case map[string]interface{}:
			if rulesVal, ok := v["rules"]; ok {
				arr, ok := rulesVal.([]interface{})
				if !ok {
					return nil, fmt.Errorf(`field "rules": expected JSON array`)
				}
				return parseDNSRulesArray(arr)
			}
			// Одиночный объект = одно правило (для удобства).
			return []interface{}{v}, nil
		case []interface{}:
			return parseDNSRulesArray(v)
		default:
			return nil, fmt.Errorf("expected JSON object or array")
		}
	}

	// Legacy fallback: один JSON-объект на строку, # — комментарии.
	lines := strings.Split(text, "\n")
	var rules []interface{}
	for lineNum, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		var obj interface{}
		if err := json.Unmarshal([]byte(s), &obj); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
		}
		m, isObj := obj.(map[string]interface{})
		if !isObj {
			return nil, fmt.Errorf("line %d: expected JSON object", lineNum+1)
		}
		rules = append(rules, m)
	}
	return rules, nil
}

// DNSRulesToText форматирует dns.rules как канонический объект
// {"rules":[...]} с отступами. Зеркало `ParseDNSRulesText`.
//
// Пустой/nil rules → "" (вызывающий редактор обнулит свой буфер).
//
// Живёт в core/build чтобы дебаг-API (`GET /state/dns/rules`) мог отдавать
// тот же текст, что показывает визард — без зависимости от UI-слоя.
func DNSRulesToText(rules []interface{}) string {
	if len(rules) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(map[string]interface{}{"rules": rules}, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func parseDNSRulesArray(arr []interface{}) ([]interface{}, error) {
	rules := make([]interface{}, 0, len(arr))
	for i, item := range arr {
		if _, ok := item.(map[string]interface{}); !ok {
			return nil, fmt.Errorf("rules[%d]: expected JSON object", i+1)
		}
		rules = append(rules, item)
	}
	return rules, nil
}
