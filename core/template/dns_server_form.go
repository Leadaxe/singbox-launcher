// File dns_server_form.go — нормализация формы записей `dns_options.servers`
// (SPEC 109, разрыв N8).
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
// Переменные записи становятся переменными шаблона с именем
// `dns_<tag>_<var>`: они живут в одном пространстве имён с остальными
// `@placeholder`, и без префикса `outbound` от Google DoT затирал бы
// `outbound` от Cloudflare DoT.
package template

import (
	"encoding/json"

	"singbox-launcher/internal/debuglog"
)

// dnsServerVarPrefix — префикс имени переменной, порождённой записью DNS-сервера.
const dnsServerVarPrefix = "dns_"

// NormalizeDNSOptions разворачивает вложенные записи серверов в плоские и
// возвращает переменные, которые эти записи объявили.
//
// Экспортирована: этот шов — часть контракта с LxBox (разрыв N8), и
// конформанс-раннер корпуса обязан идти ровно через него, а не через свою
// копию логики.
//
// Плоские записи проходят насквозь без изменений: шаблон, написанный в
// нашей форме, обязан грузиться байт-в-байт как раньше.
func NormalizeDNSOptions(raw json.RawMessage) (json.RawMessage, []TemplateVar) {
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
	var vars []TemplateVar

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
		if declared := dnsEntryVars(entry, tag); len(declared) > 0 {
			vars = append(vars, dnsEntryTemplateVars(declared)...)
			renameDNSPlaceholders(flat, declared)
		}
		out = append(out, flat)
	}

	if !changed {
		return raw, nil
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		debuglog.WarnLog("template: dns_options: не удалось перекодировать серверы: %v", err)
		return raw, nil
	}
	root["servers"] = encoded
	normalized, err := json.Marshal(root)
	if err != nil {
		debuglog.WarnLog("template: dns_options: не удалось собрать секцию: %v", err)
		return raw, nil
	}
	debuglog.DebugLog("template: dns_options: развёрнуто вложенных записей, объявлено %d переменных", len(vars))
	return normalized, vars
}

// dnsEntryVar — переменная записи и её итоговое имя в шаблоне.
type dnsEntryVar struct {
	local  string // как названа внутри записи: "outbound"
	global string // как названа в шаблоне: "dns_google_dot_outbound"
	tmpl   TemplateVar
}

// dnsEntryVars читает `vars` записи и переименовывает их с префиксом тега.
func dnsEntryVars(entry map[string]interface{}, tag string) []dnsEntryVar {
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
		debuglog.WarnLog("template: dns_options: запись %q: не читаются vars: %v", tag, err)
		return nil
	}

	out := make([]dnsEntryVar, 0, len(parsed))
	for _, v := range parsed {
		if v.Name == "" {
			continue
		}
		local := v.Name
		v.Name = dnsServerVarPrefix + tag + "_" + local
		// Переменные DNS-сервера не показываются на вкладке Settings: их
		// место — форма самого сервера, где видно, к какой записи они
		// относятся. Иначе список настроек распух бы на два десятка строк
		// вида «Outbound», неотличимых друг от друга.
		v.WizardUI = "hidden"
		out = append(out, dnsEntryVar{local: local, global: v.Name, tmpl: v})
	}
	return out
}

// renameDNSPlaceholders переписывает `@local` → `@global` в теле сервера.
//
// Только точные совпадения целой строки: `"server": "@dns_ip"` — да,
// `"path": "/dns-query?x=@dns_ip"` — нет. Подстановка внутри строк здесь
// не встречается, а частичная замена молча испортила бы чужое значение.
func renameDNSPlaceholders(body map[string]interface{}, vars []dnsEntryVar) {
	byLocal := make(map[string]string, len(vars))
	for _, v := range vars {
		byLocal["@"+v.local] = "@" + v.global
	}
	var walk func(v interface{}) interface{}
	walk = func(v interface{}) interface{} {
		switch t := v.(type) {
		case string:
			if repl, ok := byLocal[t]; ok {
				return repl
			}
			return t
		case map[string]interface{}:
			for k, inner := range t {
				t[k] = walk(inner)
			}
			return t
		case []interface{}:
			for i, inner := range t {
				t[i] = walk(inner)
			}
			return t
		default:
			return v
		}
	}
	for k, v := range body {
		body[k] = walk(v)
	}
}

// dnsEntryTemplateVars отдаёт объявленные переменные в виде TemplateVar.
func dnsEntryTemplateVars(vars []dnsEntryVar) []TemplateVar {
	out := make([]TemplateVar, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.tmpl)
	}
	return out
}
