// File dns_server_form.go — нормализация формы записей `dns_options.servers`
// (SPEC 109, разрыв N8) и переменные шаблонного DNS-сервера (SPEC 129).
//
// Две формы одной сущности:
//
//	плоская (наша)     {"type":"udp","tag":"x","server":"1.1.1.1","enabled":true}
//	вложенная (LxBox)  {"description":"...","enabled":true,
//	                    "vars":[{"name":"dns_ip","type":"enum",...}],
//	                    "server":{"type":"udp","tag":"x","server":"@dns_ip"}}
//
// Вложенная богаче: `vars` объявляют настраиваемые точки записи — канал
// (`type: outbound`), адрес провайдера (`enum` с подписями), резолвер имени
// (`dns_server`). Именно они дают выбор «Primary v4 / Secondary v6» и
// профиль Safe DNS вместо одного зашитого адреса.
//
// Читатели ниже (build.ParseTemplateDNSDefaults, ExtractTemplateDNSTags и
// ещё четыре точки) берут `tag` с ВЕРХНЕГО уровня записи. Учить каждую из
// них двум формам значило бы шесть раз повторить одно решение — вместо
// этого вложенная форма разворачивается в плоскую здесь, один раз при
// загрузке шаблона, и весь код ниже видит то же, что видел всегда.
//
// Переменные записи остаются ПРИ СЕРВЕРЕ (SPEC 129): объявления — в
// TemplateData.DNSServerVars по тегу, тело держит локальные `@outbound`,
// `@dns_ip`, значения — в записи состояния `dns.servers[kind=template].vars`.
// До SPEC 129 они склеивались в переменные шаблона `dns_<tag>_<var>`, и
// переносимость настройки решалась по склеенному имени, а не по записи.
package template

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// NormalizeDNSOptions разворачивает вложенные записи серверов в плоские и
// возвращает объявления их переменных по тегу сервера.
//
// Экспортирована: этот шов — часть контракта с LxBox (разрыв N8), и
// конформанс-раннер корпуса обязан идти ровно через него, а не через свою
// копию логики.
//
// Плоские записи проходят насквозь без изменений: шаблон, написанный в
// нашей форме, обязан грузиться байт-в-байт как раньше. Объявления у
// плоской записи нет.
func NormalizeDNSOptions(raw json.RawMessage) (json.RawMessage, map[string][]TemplateVar) {
	if len(raw) == 0 {
		return raw, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw, nil
	}
	rawServers, ok := root["servers"]
	if !ok {
		return raw, nil
	}
	var servers []map[string]interface{}
	if err := json.Unmarshal(rawServers, &servers); err != nil {
		return raw, nil
	}

	changed := false
	out := make([]map[string]interface{}, 0, len(servers))
	var decls map[string][]TemplateVar

	for _, entry := range servers {
		nested, isNested := entry["server"].(map[string]interface{})
		if !isNested {
			out = append(out, entry) // уже плоская
			continue
		}
		changed = true

		flat := make(map[string]interface{}, len(nested)+3)
		for k, v := range nested {
			flat[k] = v
		}
		// Поля обёртки, осмысленные для нашей модели. `description` у нас
		// тоже есть, а всё прочее (например, порядок в мобильном UI) не
		// переносим: чужие ключи в теле сервера ядро отвергает.
		for _, k := range []string{"enabled", "required", "description"} {
			if v, has := entry[k]; has {
				flat[k] = v
			}
		}

		tag, _ := flat["tag"].(string)
		if declared := dnsEntryVars(entry, tag); len(declared) > 0 && tag != "" {
			if decls == nil {
				decls = make(map[string][]TemplateVar)
			}
			decls[tag] = declared
		}
		out = append(out, flat)
	}

	if !changed {
		return raw, nil
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		debuglog.WarnLog("template: dns_options: could not re-encode the servers: %v", err)
		return raw, nil
	}
	root["servers"] = encoded
	normalized, err := json.Marshal(root)
	if err != nil {
		debuglog.WarnLog("template: dns_options: could not assemble the section: %v", err)
		return raw, nil
	}
	debuglog.DebugLog("template: dns_options: nested entries expanded, %d servers declare variables", len(decls))
	return normalized, decls
}

// dnsEntryVars читает `vars` вложенной записи — объявления с ЛОКАЛЬНЫМИ
// именами, как их назвал автор шаблона.
func dnsEntryVars(entry map[string]interface{}, tag string) []TemplateVar {
	rawVars, ok := entry["vars"].([]interface{})
	if !ok || len(rawVars) == 0 {
		return nil
	}
	encoded, err := json.Marshal(rawVars)
	if err != nil {
		return nil
	}
	var parsed []TemplateVar
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		debuglog.WarnLog("template: dns_options: entry %q: vars unreadable: %v", tag, err)
		return nil
	}
	out := make([]TemplateVar, 0, len(parsed))
	for _, v := range parsed {
		// Разделитель — оформление вкладки Settings, у записи сервера его
		// показывать негде; безымянную запись подставить некуда.
		if v.Separator || strings.TrimSpace(v.Name) == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

// DNSServerVarScope — переменные, видимые телу шаблонного DNS-сервера
// (SPEC 129 §4.1): переменные шаблона верхнего уровня (расширение лаунчера)
// и поверх них объявления самого сервера. Локальное имя затеняет
// одноимённое глобальное — глобальное из списка убирается, иначе резолв
// увидел бы два объявления одного имени.
//
// Порядок — глобальные, затем локальные: `#if` в default_value видит только
// объявленные выше (ForTargetIn), и это тот же порядок, в котором до SPEC 129
// склеенные переменные серверов дописывались в конец списка шаблона.
func DNSServerVarScope(decls, globals []TemplateVar) []TemplateVar {
	local := make(map[string]bool, len(decls))
	for _, d := range decls {
		if !d.Separator && d.Name != "" {
			local[d.Name] = true
		}
	}
	out := make([]TemplateVar, 0, len(globals)+len(decls))
	for _, g := range globals {
		if g.Separator || local[g.Name] {
			continue
		}
		out = append(out, g)
	}
	for _, d := range decls {
		if d.Separator || d.Name == "" {
			continue
		}
		out = append(out, d)
	}
	return out
}

// ResolveDNSServerVars — значения переменных для тела шаблонного DNS-сервера.
//
// Имя, объявленное сервером: значение из `vars` записи (непустое после
// подрезки, Н3) → умолчание объявления для цели → «не задано». Значение
// записи под именем, которого сервер не объявил, не видно никогда (Н2).
// Имя, не объявленное сервером, — переменная шаблона со значением из
// globalValues (расширение лаунчера); одноимённое глобальное значение для
// локального имени не просачивается.
//
// Возвращает область видимости (DNSServerVarScope) и разрешённые значения —
// ровно то, что нужно движку подстановки.
func ResolveDNSServerVars(decls []TemplateVar, record map[string]string, globals []TemplateVar, globalValues map[string]string, target TargetSpec) ([]TemplateVar, map[string]ResolvedVar) {
	scope := DNSServerVarScope(decls, globals)
	local := make(map[string]bool, len(decls))
	for _, d := range decls {
		if !d.Separator && d.Name != "" {
			local[d.Name] = true
		}
	}
	values := make(map[string]string, len(globalValues)+len(decls))
	for k, v := range globalValues {
		if !local[k] {
			values[k] = v
		}
	}
	for name := range local {
		if v := strings.TrimSpace(record[name]); v != "" {
			values[name] = v
		}
	}
	return scope, ResolveTemplateVarsFor(scope, values, nil, target)
}

// DNSServerVarValues — значения переменных сервера строками: то, что уедет в
// его тело. Для подписи строки списка и формы окна — те же правила, что у
// сборки (ResolveDNSServerVars), иначе строка показывала бы не то, что
// уезжает в конфиг.
func DNSServerVarValues(decls []TemplateVar, record map[string]string, globals []TemplateVar, globalValues map[string]string, target TargetSpec) map[string]string {
	scope, resolved := ResolveDNSServerVars(decls, record, globals, globalValues, target)
	out := make(map[string]string, len(scope))
	for _, v := range scope {
		r, ok := resolved[v.Name]
		if !ok {
			continue
		}
		value := r.Scalar
		if v.Type == "text_list" {
			value = strings.Join(r.List, "\n")
		}
		if value != "" {
			out[v.Name] = value
		}
	}
	return out
}

// DNSServerVarDefault — умолчание переменной сервера для цели: то, что
// подставит сборка, когда в записи значения нет. Пусто — умолчания нет.
func DNSServerVarDefault(decls []TemplateVar, name string, globals []TemplateVar, globalValues map[string]string, target TargetSpec) string {
	_, resolved := ResolveDNSServerVars(decls, nil, globals, globalValues, target)
	r, ok := resolved[name]
	if !ok {
		return ""
	}
	for _, d := range decls {
		if d.Name == name && d.Type == "text_list" {
			return strings.Join(r.List, "\n")
		}
	}
	return r.Scalar
}

// validateDNSServerVars проверяет объявления переменных шаблонных
// DNS-серверов и плейсхолдеры их тел (SPEC 129, Н11).
//
// Объявления — теми же проверками, что переменные шаблона (лексика и дубли
// имён, зарезервированные имена, if/if_or, #if в default_value), но в своей
// области: локальные имена поверх глобальных. До SPEC 129 их проверял общий
// вызов ValidateWizardTemplate по склеенным именам; после разделения без
// этого прохода объявления сервера не проверял бы никто.
//
// Плейсхолдер тела обязан быть объявлен сервером или (расширение лаунчера)
// шаблоном. Необъявленное имя — ошибка ШАБЛОНА, а не данных пользователя:
// шаблон с ней не грузится, и скачанный такой шаблон не заменит рабочий.
func validateDNSServerVars(dnsOptions json.RawMessage, decls map[string][]TemplateVar, globals []TemplateVar) error {
	if len(dnsOptions) == 0 {
		return nil
	}
	var section struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(dnsOptions, &section); err != nil {
		return nil // форму секции проверяют её читатели; здесь проверять нечего
	}
	globalByName := make(map[string]TemplateVar, len(globals))
	for _, g := range globals {
		if !g.Separator && strings.TrimSpace(g.Name) != "" {
			globalByName[strings.TrimSpace(g.Name)] = g
		}
	}
	for i, srv := range section.Servers {
		tag, _ := srv["tag"].(string)
		ctx := fmt.Sprintf("dns_options.servers[%d] (%s)", i, tag)
		local := decls[tag]

		varByName := make(map[string]TemplateVar, len(globalByName)+len(local))
		for k, v := range globalByName {
			varByName[k] = v
		}
		names := make(map[string]bool, len(local))
		for j, v := range local {
			vctx := fmt.Sprintf("%s.vars[%d]", ctx, j)
			nm := strings.TrimSpace(v.Name)
			if !validWizardVarNameRE.MatchString(nm) {
				return fmt.Errorf("%s: invalid name %q (expected [A-Za-z_][A-Za-z0-9_]*)", vctx, nm)
			}
			if names[nm] {
				return fmt.Errorf("%s: duplicate name %q", ctx, nm)
			}
			if _, reserved := reservedVarNames[nm]; reserved {
				return fmt.Errorf("%s: name %q is reserved (runtime global namespace); rename", vctx, nm)
			}
			names[nm] = true
			varByName[nm] = v
		}
		earlier := make(map[string]TemplateVar, len(globalByName)+len(local))
		for k, v := range globalByName {
			earlier[k] = v
		}
		for j, v := range local {
			vctx := fmt.Sprintf("%s.vars[%d]", ctx, j)
			if err := validateOuterIfRefs(vctx, v.If, v.IfOr, varByName); err != nil {
				return err
			}
			if err := validateDefaultValueIf(v.DefaultValue, earlier, vctx); err != nil {
				return err
			}
			earlier[strings.TrimSpace(v.Name)] = v
		}

		body, err := json.Marshal(srv)
		if err != nil {
			continue
		}
		refs, err := collectPlaceholderNamesFromJSON(body)
		if err != nil {
			return fmt.Errorf("%s: %w", ctx, err)
		}
		for _, ref := range refs {
			if names[ref] || isRuntimeGlobalRef(ref) {
				continue
			}
			if _, ok := globalByName[ref]; ok {
				continue
			}
			return fmt.Errorf("%s: @%s is not declared by the server or the template", ctx, ref)
		}
	}
	return nil
}
