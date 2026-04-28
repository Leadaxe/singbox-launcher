package build

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/services"
)

// Action-имена для sing-box `route.rules[].action`.
// Зеркалят `ui/wizard/models/wizard_model.go::RejectActionName/Method`;
// дублируются здесь, чтобы core/build не импортировал ui/.
const (
	routeActionReject     = "reject"
	routeActionMethodDrop = "drop"
)

// RouteRule — одно custom-rule из state, в форме, готовой для merge'а.
//
// Это clean-input для MergeRouteSection: каллер (wizard / Configurator)
// извлекает данные из своей модели (RuleState и т.п.) в эту структуру.
//
// `Outbound` — уже-разрешённый outbound (было `GetEffectiveOutbound(ruleState)`
// у вызывающего): либо имя outbound, либо routeActionReject ("reject"), либо
// "drop" (особый псевдо-outbound: action=reject, method=drop).
type RouteRule struct {
	Enabled  bool
	Outbound string

	// Каждое custom-rule может предоставить либо одно правило (PrimaryRule),
	// либо набор правил (Rules). Если оба заданы — приоритет у Rules
	// (соответствует legacy: `if len(Rule.Rules) > 0 ... else if Rule.Rule != nil`).
	PrimaryRule map[string]interface{}
	Rules       []map[string]interface{}

	// RuleSets — SRS rule_set-объекты, добавляемые в route.rule_set; формат
	// как в `wizard_template.json`: {tag, type, format, url} либо local-форма.
	RuleSets []json.RawMessage
}

// RouteConfig — clean-input для MergeRouteSection.
type RouteConfig struct {
	// Rules — пользовательские правила; disabled пропускаются на этапе merge'а.
	Rules []RouteRule
	// FinalOutbound — итоговый default outbound для route.final.
	FinalOutbound string
	// ExecDir — директория для разрешения SRS local-path
	// (services.RuleSRSPath / services.SRSFileExists).
	ExecDir string
	// DefaultDomainResolver — переопределяет ключ route.default_domain_resolver.
	// Игнорируется, если OmitDefaultDomainResolver=true.
	DefaultDomainResolver string
	// OmitDefaultDomainResolver — true → ключ default_domain_resolver удаляется
	// из секции (даже если был в шаблоне).
	OmitDefaultDomainResolver bool
}

// MergeRouteSection накладывает custom-rules + SRS rule_sets поверх
// шаблонной секции route, сохраняя все остальные ключи шаблона
// (например `final` если RouteConfig.FinalOutbound пустой).
//
// Совместимо с поведением `ui/wizard/business/create_config.go::MergeRouteSection`
// для байт-в-байт паритета (test'ы на той стороне в generator_test.go
// продолжают проходить через шим).
//
// Шаги:
//  1. Парсит raw → map.
//  2. Берёт template-rules (например hijack-dns) и template-rule_set как базу;
//     далее **только append** (а не replace).
//  3. Для каждого enabled-RouteRule: добавляет SRS-records (с local-конверсией
//     при наличии файла) и собственно rules-записи (PrimaryRule или Rules)
//     с применением outbound через applyRouteOutbound.
//  4. Финализирует ключи route.final / .default_domain_resolver.
//
// Pure (модулём): без I/O кроме filesystem-проверки наличия SRS-файлов
// (`services.SRSFileExists`) — но это идемпотентно и safe.
func MergeRouteSection(raw json.RawMessage, cfg RouteConfig) (json.RawMessage, error) {
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		return nil, err
	}

	var rules []interface{}
	if existing, ok := route["rules"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			rules = arr
		}
	}

	var ruleSets []interface{}
	if existing, ok := route["rule_set"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			ruleSets = arr
		}
	}

	for _, r := range cfg.Rules {
		if !r.Enabled {
			continue
		}
		// SRS rule_sets от этого правила.
		for _, rs := range r.RuleSets {
			rsObj := convertRuleSetToLocalIfNeeded(rs, cfg.ExecDir)
			if rsObj != nil {
				ruleSets = append(ruleSets, rsObj)
			}
		}
		// Маршрутные правила: либо набор, либо одиночка.
		if len(r.Rules) > 0 {
			for _, sub := range r.Rules {
				cloned := shallowCopyStringMap(sub)
				applyRouteOutbound(cloned, r.Outbound)
				rules = append(rules, cloned)
			}
		} else if r.PrimaryRule != nil {
			cloned := shallowCopyStringMap(r.PrimaryRule)
			applyRouteOutbound(cloned, r.Outbound)
			rules = append(rules, cloned)
		}
	}

	if len(rules) > 0 {
		route["rules"] = rules
	}
	if len(ruleSets) > 0 {
		route["rule_set"] = ruleSets
	}
	if cfg.FinalOutbound != "" {
		route["final"] = cfg.FinalOutbound
	}

	if cfg.OmitDefaultDomainResolver {
		delete(route, "default_domain_resolver")
	} else if s := strings.TrimSpace(cfg.DefaultDomainResolver); s != "" {
		route["default_domain_resolver"] = s
	}

	return json.Marshal(route)
}

// applyRouteOutbound — устанавливает outbound/action/method у клонированной
// rule-map'ы. Семантика идентична legacy:
//   - "reject" → action=reject, нет outbound, нет method
//   - "drop"   → action=reject, method=drop, нет outbound (action=reject method=drop = sing-box "drop");
//   - иначе непустой outbound → outbound=X, action/method удаляются.
func applyRouteOutbound(cloned map[string]interface{}, outbound string) {
	switch outbound {
	case routeActionReject:
		delete(cloned, "outbound")
		cloned["action"] = routeActionReject
		delete(cloned, "method")
	case "drop":
		delete(cloned, "outbound")
		cloned["action"] = routeActionReject
		cloned["method"] = routeActionMethodDrop
	default:
		if outbound != "" {
			cloned["outbound"] = outbound
			delete(cloned, "action")
			delete(cloned, "method")
		}
	}
}

// convertRuleSetToLocalIfNeeded для remote-SRS подставляет type:local + path,
// если локальный файл `bin/rule-sets/<tag>.srs` существует. Если файла нет —
// rule-set остаётся remote, sing-box скачает его при старте.
//
// Это поведение нужно для двух кейсов:
//  1. Шаблонные SRS, которые скачаны при первом Save → быстрая загрузка из disk.
//  2. Пользовательские SRS, добавленные через UI → тоже local.
//
// Идемпотентно: повторный вызов с тем же rule-set даёт тот же результат.
func convertRuleSetToLocalIfNeeded(rs json.RawMessage, execDir string) interface{} {
	var m map[string]interface{}
	if err := json.Unmarshal(rs, &m); err != nil {
		return nil
	}
	typ, _ := m["type"].(string)
	tag, _ := m["tag"].(string)
	if typ != "remote" || tag == "" || execDir == "" {
		return m
	}

	if services.SRSFileExists(execDir, tag) {
		return map[string]interface{}{
			"tag":    tag,
			"type":   "local",
			"format": "binary",
			"path":   services.RuleSRSPath(execDir, tag),
		}
	}
	return m
}

// shallowCopyStringMap — поверхностная копия map[string]interface{}.
// Достаточно, потому что MergeRouteSection меняет только верхне-уровневые
// ключи (outbound/action/method).
func shallowCopyStringMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
