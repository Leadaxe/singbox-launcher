package build

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/outboundutil"
)

// RouteRule — одно custom-rule из state, в форме, готовой для merge'а.
//
// Это clean-input для MergeRouteSection: каллер (wizard / Configurator)
// извлекает данные из своей модели (RuleState и т.п.) в эту структуру.
//
// `Outbound` — уже-разрешённый outbound (было `GetEffectiveOutbound(ruleState)`
// у вызывающего): либо имя outbound, либо "reject", либо "drop" (особый
// псевдо-outbound: action=reject, method=drop). Применяется через
// outboundutil.ApplyOutboundToRule.
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
//     с применением outbound через outboundutil.ApplyOutboundToRule.
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
		// SRS rule_sets от этого правила. Build pipeline эмитит ТОЛЬКО type:local
		// (см. convertRuleSetToLocalRequired) — sing-box получает гарантированно
		// локальные пути, никаких runtime-фетчей через VPN. UI gate
		// (rules_tab.go createRuleEnableCheckbox) обеспечивает что файл
		// уже скачан до того как rule был enabled. Если файл всё-таки missing
		// (manual delete, multi-stage переключение, race) — Rebuild возвращает
		// error, конфиг не пересобирается; пользователь видит сообщение и
		// перекачивает SRS вручную через Wizard.
		for _, rs := range r.RuleSets {
			rsObj, err := convertRuleSetToLocalRequired(rs, cfg.ExecDir)
			if err != nil {
				return nil, err
			}
			if rsObj != nil {
				ruleSets = append(ruleSets, rsObj)
			}
		}
		// Маршрутные правила: либо набор, либо одиночка.
		if len(r.Rules) > 0 {
			for _, sub := range r.Rules {
				cloned := shallowCopyStringMap(sub)
				outboundutil.ApplyOutboundToRule(cloned, r.Outbound)
				rules = append(rules, cloned)
			}
		} else if r.PrimaryRule != nil {
			cloned := shallowCopyStringMap(r.PrimaryRule)
			outboundutil.ApplyOutboundToRule(cloned, r.Outbound)
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

// convertRuleSetToLocalRequired эмитит rule-set строго как type:local (или
// inline). Логика по типу:
//
//   - inline → пропускаем как есть
//   - local  → проверяем что файл по path существует; если нет — error
//   - remote → проверяем что bin/rule-sets/<tag>.srs существует; если есть —
//     переписываем на type:local + path и эмитим, если нет — error
//
// Это поведение восстановления старого процесса из v0.8.x: sing-box при старте
// никогда не должен пытаться скачивать rule-set через VPN-прокси (триггерит
// 404-storm на cold-start, sing-box падает с FATAL). Кэш в bin/rule-sets/
// гарантирует оффлайн Rebuild и оффлайн запуск sing-box.
//
// UI gate в Configurator (rules_tab.go createRuleEnableCheckbox) обеспечивает
// download до enable rule. Build error здесь — safety net на 0.1% кейсов:
//   - пользователь руками удалил файл из bin/rule-sets/
//   - multi-stage: переключение на стейдж со старыми ссылками после GC соседнего
//   - state.json пришёл с другой машины с уже-local entry, но файла нет
//
// Идемпотентно: повторный вызов с тем же rule-set даёт тот же результат.
func convertRuleSetToLocalRequired(rs json.RawMessage, execDir string) (interface{}, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(rs, &m); err != nil {
		return nil, fmt.Errorf("rule-set: invalid JSON: %w", err)
	}
	typ, _ := m["type"].(string)
	tag, _ := m["tag"].(string)

	switch typ {
	case "inline":
		return m, nil

	case "local":
		// Проверяем существование указанного path. Сам entry эмитим без
		// модификаций (path уже в нём).
		path, _ := m["path"].(string)
		if path == "" {
			if tag == "" {
				return nil, fmt.Errorf("rule-set: local entry missing both tag and path")
			}
			return nil, fmt.Errorf("rule-set %q: local entry missing path", tag)
		}
		if _, err := os.Stat(path); err != nil {
			label := tag
			if label == "" {
				label = path
			}
			return nil, fmt.Errorf("rule-set %q: local file missing at %s — open Configurator → Rules and re-download",
				label, path)
		}
		return m, nil

	case "remote":
		if tag == "" {
			return nil, fmt.Errorf("rule-set: remote entry missing tag")
		}
		if execDir == "" {
			return nil, fmt.Errorf("rule-set %q: cannot resolve local path (empty execDir)", tag)
		}
		if !services.SRSFileExists(execDir, tag) {
			return nil, fmt.Errorf("rule-set %q: local file missing at %s — open Configurator → Rules and re-download",
				tag, services.RuleSRSPath(execDir, tag))
		}
		return map[string]interface{}{
			"tag":    tag,
			"type":   "local",
			"format": "binary",
			"path":   services.RuleSRSPath(execDir, tag),
		}, nil

	default:
		// Неизвестный type → пропускаем без изменений (sing-box разберётся
		// или упадёт на validate). Не наша зона ответственности.
		return m, nil
	}
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
