// File record_var_decls.go — мост шаблон → state.RecordVarDecls (SPEC 129).
//
// Нормализация значений переменных записи живёт в core/state, который о
// шаблоне не знает (тот же приём, что у оси: state.RuleOrderSpec заполняет
// RuleOrderSpecs). Здесь объявления шаблона превращаются в форму без типов
// шаблона: имя, тип и умолчание, уже разрешённое для цели состояния.
package template

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/state"
)

// RecordVarDeclsFor — объявления шаблона для нормализации записей состояния
// цели target. globalValues — значения переменных шаблона этого состояния:
// умолчание переменной сервера может зависеть от них через `#if` в
// default_value (nil — умолчания шаблона).
//
// nil td — nil: шаблона нет, нормализовать нечем.
func RecordVarDeclsFor(td *TemplateData, globalValues map[string]string, target TargetSpec) *state.RecordVarDecls {
	if td == nil {
		return nil
	}
	target = target.Normalized()
	out := &state.RecordVarDecls{
		DNSServers:       map[string][]state.RecordVarDecl{},
		DNSServerEnabled: map[string]bool{},
		Presets:          map[string][]state.RecordVarDecl{},
	}

	var section struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if len(td.DNSOptionsRaw) > 0 {
		_ = json.Unmarshal(td.DNSOptionsRaw, &section)
	}
	for _, srv := range section.Servers {
		tag, _ := srv["tag"].(string)
		if tag == "" {
			continue
		}
		decls := td.DNSServerVars[tag]
		list := make([]state.RecordVarDecl, 0, len(decls))
		if len(decls) > 0 {
			_, resolved := ResolveDNSServerVars(decls, nil, td.Vars, globalValues, target)
			for _, d := range decls {
				r := resolved[d.Name]
				def := r.Scalar
				if d.Type == "text_list" {
					def = strings.Join(r.List, "\n")
				}
				list = append(list, state.RecordVarDecl{Name: d.Name, Type: d.Type, Default: def})
			}
		}
		out.DNSServers[tag] = list
		out.DNSServerEnabled[tag] = dnsServerDefaultEnabled(srv)
	}

	for i := range td.Presets {
		p := &td.Presets[i]
		if p.ID == "" {
			continue
		}
		out.Presets[p.ID] = PresetRecordVarDecls(p)
	}
	return out
}

// PresetRecordVarDecls — объявления переменных пресета для норм записи
// (SPEC 129): умолчание пресета — простая строка, от платформы не зависит.
func PresetRecordVarDecls(p *Preset) []state.RecordVarDecl {
	if p == nil {
		return nil
	}
	list := make([]state.RecordVarDecl, 0, len(p.Vars))
	for _, v := range p.Vars {
		if v.Name == "" {
			continue
		}
		list = append(list, state.RecordVarDecl{
			Name:    v.Name,
			Type:    v.Type,
			Default: strings.TrimSpace(v.Default),
			Ref:     v.Ref != "",
		})
	}
	return list
}

// dnsServerDefaultEnabled — включённость сервера шаблона по умолчанию: то же
// правило, что у сборки (build.bodyEnabled; `required` включён всегда).
// Нужна Н8: запись, созданная переносом корневого имени, не должна менять
// включённость, которую сборка и так брала из шаблона.
func dnsServerDefaultEnabled(srv map[string]interface{}) bool {
	if req, ok := srv["required"].(bool); ok && req {
		return true
	}
	if v, ok := srv["default_enabled"].(bool); ok {
		return v
	}
	if v, ok := srv["enabled"].(bool); ok {
		return v
	}
	return true
}
