package build

// rule_set_policy.go — настройки наборов правил (SPEC 185): кто качает .srs
// (лаунчер или ядро), как часто и через какой выход. Значение берётся у
// правила, иначе из переменной шаблона, иначе встроенное умолчание.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/srstag"
)

// RuleSetPolicy — итоговые настройки наборов одного правила.
type RuleSetPolicy struct {
	// Remote — набор качает ядро (type:remote); иначе лаунчер (type:local).
	Remote bool
	// IntervalHours — период обновления, часы; 0 — не обновлять.
	IntervalHours int
	// DownloadDetour — тег выхода для загрузки ядром (только Remote).
	DownloadDetour string
}

// ResolveRuleSetPolicy сводит настройки правила с общими значениями.
// globals — значения переменных шаблона (могут не содержать нужных имён:
// шаблон старше приложения — тогда встроенные умолчания).
func ResolveRuleSetPolicy(o state.RuleSetOptions, globals map[string]string) RuleSetPolicy {
	p := RuleSetPolicy{IntervalHours: state.DefaultSrsUpdateIntervalHours}

	mode := o.Mode
	if mode == "" {
		mode = strings.TrimSpace(globals[state.VarSrsMode])
	}
	p.Remote = mode == state.SrsModeRemote

	if o.IntervalHours != nil {
		p.IntervalHours = *o.IntervalHours
	} else if raw := strings.TrimSpace(globals[state.VarSrsUpdateIntervalHours]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			p.IntervalHours = n
		}
	}

	p.DownloadDetour = o.DownloadDetour
	if p.DownloadDetour == "" {
		p.DownloadDetour = strings.TrimSpace(globals[state.VarSrsDownloadDetour])
	}
	return p
}

// RulePolicy — настройки наборов правила с учётом его ссылок (SPEC 187):
// правило kind=srs, у которого хотя бы один набор — локальный файл
// (file://), целиком идёт в режиме local. Ядро по file:// не качает, а
// правило — одна единица: делить его наборы между режимами незачем.
func RulePolicy(rule state.Rule, globals map[string]string) RuleSetPolicy {
	p := ResolveRuleSetPolicy(rule.RuleSetOptions(), globals)
	if p.Remote && rule.Kind == state.RuleKindSrs && HasFileRef(rule.Refs) {
		p.Remote = false
	}
	return p
}

// HasFileRef — среди ссылок на наборы есть локальный файл (file://).
func HasFileRef(refs []string) bool {
	for _, r := range refs {
		if srstag.IsFileRef(r) {
			return true
		}
	}
	return false
}

// remoteRuleSetBody — запись rule_set для ядра в режиме remote. ok=false —
// цель download_detour недоступна в итоговом конфиге: набор не эмитится
// (ядро отвергло бы конфиг целиком). finalTags == nil — проверки нет (превью).
func remoteRuleSetBody(tag, format, rawURL string, p RuleSetPolicy, finalTags map[string]bool) (map[string]interface{}, bool) {
	if p.DownloadDetour != "" && len(finalTags) > 0 && !finalTags[p.DownloadDetour] {
		return nil, false
	}
	if format == "" {
		format = srstag.FormatFromRef(rawURL)
	}
	rs := map[string]interface{}{
		"tag":    tag,
		"type":   "remote",
		"format": format,
		"url":    services.NormalizeSRSURL(rawURL),
	}
	if p.IntervalHours > 0 {
		rs["update_interval"] = fmt.Sprintf("%dh", p.IntervalHours)
	}
	if p.DownloadDetour != "" {
		rs["download_detour"] = p.DownloadDetour
	}
	return rs, true
}

// RuleSetFetch — набор, который качает ЛАУНЧЕР (режим local) для включённого
// правила: откуда, под каким именем файла и как часто.
type RuleSetFetch struct {
	// URL — адрес загрузки (GitHub blob-ссылка уже приведена к raw).
	URL string
	// FileTag — имя файла без расширения (от URL, srstag.TagFromURL).
	FileTag string
	// IntervalHours — период обновления; 0 — только докачка отсутствующего.
	IntervalHours int
}

// RuleSetFetches — local-наборы включённых правил состояния (SPEC 185 §2.1).
// Один файл у нескольких правил — наименьший ненулевой период.
//
// Пресеты раскрываются тем же ExpandPresetForNodes, что и сборка: URL с
// переменными и наборы под гейтом обязаны совпасть с тем, что уедет в конфиг.
func (c PresetMergeContext) RuleSetFetches() []RuleSetFetch {
	globals := c.globalVarValues()
	presetByID := make(map[string]*template.Preset, len(c.Presets))
	for i := range c.Presets {
		presetByID[c.Presets[i].ID] = &c.Presets[i]
	}

	var out []RuleSetFetch
	index := make(map[string]int)
	add := func(rawURL string, interval int) {
		tag := srsTagFromURLLocal(rawURL)
		if rawURL == "" || tag == "" {
			return
		}
		if i, ok := index[tag]; ok {
			if interval > 0 && (out[i].IntervalHours == 0 || interval < out[i].IntervalHours) {
				out[i].IntervalHours = interval
			}
			return
		}
		index[tag] = len(out)
		out = append(out, RuleSetFetch{URL: services.NormalizeSRSURL(rawURL), FileTag: tag, IntervalHours: interval})
	}

	rules := state.NormalizeRuleOrder(c.Rules, template.RuleOrderSpecs(c.Presets))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		policy := RulePolicy(rule, globals)
		if policy.Remote {
			continue
		}
		switch rule.Kind {
		case state.RuleKindSrs:
			for _, u := range rule.Refs {
				add(u, policy.IntervalHours)
			}
		case state.RuleKindPreset:
			p, ok := presetByID[rule.Ref]
			if !ok {
				continue
			}
			frags, _, ok := ExpandPresetForNodes(p, rule.Vars, globals, c.TemplateVars, c.Target, c.PresetNodes)
			if !ok {
				continue
			}
			for _, rs := range frags.RuleSets {
				if typ, _ := rs["type"].(string); typ != "remote" {
					continue
				}
				u, _ := rs["url"].(string)
				add(u, policy.IntervalHours)
			}
		}
	}
	return out
}

// RuleSetFileDir — каталог .srs машины цели на стороне лаунчера.
func (c PresetMergeContext) RuleSetFileDir() string {
	if c.Target.SrsLocalDir != "" {
		return c.Target.SrsLocalDir
	}
	return platform.GetRuleSetsDir(c.DataDir)
}

// DropUncachedSrsPaths убирает из карты CollectSrsCachedPaths правила, у
// которых на диске нет хотя бы одного файла: правило kind=srs тогда выпадает
// из конфига целиком (как правило пресета), а не уезжает туда с путём к
// несуществующему файлу — ядро на таком конфиге не стартует (SPEC 185 §2.2).
// dir — каталог .srs на стороне лаунчера; имена файлов берутся из путей.
func DropUncachedSrsPaths(cached map[string][]string, dir string) map[string][]string {
	for id, paths := range cached {
		for _, p := range paths {
			name := p[strings.LastIndexAny(p, `/\`)+1:]
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				delete(cached, id)
				break
			}
		}
	}
	return cached
}
