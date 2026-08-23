package template

import (
	"encoding/json"
	"testing"
)

// Вложенная запись LxBox разворачивается в плоскую, а её vars становятся
// переменными шаблона с префиксом тега. Тело взято из настоящего
// app/assets/wizard_template.json LxBox, а не придумано: разрыв N8 живёт
// именно в рассинхроне двух реальных форм.
func TestNormalizeDNSOptionsNestedEntry(t *testing.T) {
	raw := json.RawMessage(`{"servers":[
	  {"description":"Google DoT","enabled":true,
	   "vars":[
	     {"name":"outbound","type":"outbound","default_value":"vpn-1","title":"Outbound"},
	     {"name":"dns_ip","type":"enum","default_value":"8.8.8.8",
	      "options":[{"title":"8.8.8.8 · Primary v4","value":"8.8.8.8"},
	                 {"title":"8.8.4.4 · Secondary v4","value":"8.8.4.4"}]}],
	   "server":{"type":"tls","tag":"google_dot","server_port":853,
	             "server":"@dns_ip","detour":"@outbound"}}]}`)

	normalized, vars := normalizeDNSOptions(raw)

	var got struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(normalized, &got); err != nil {
		t.Fatalf("разбор результата: %v", err)
	}
	if len(got.Servers) != 1 {
		t.Fatalf("серверов %d, ожидался 1", len(got.Servers))
	}
	s := got.Servers[0]

	// Тег обязан оказаться на ВЕРХНЕМ уровне: все шесть читателей ниже
	// берут его оттуда.
	if s["tag"] != "google_dot" {
		t.Errorf("tag = %v, ожидался google_dot на верхнем уровне", s["tag"])
	}
	if s["type"] != "tls" {
		t.Errorf("type = %v, ожидался tls", s["type"])
	}
	if s["enabled"] != true {
		t.Errorf("enabled = %v, ожидалось true (поле обёртки)", s["enabled"])
	}
	if s["description"] != "Google DoT" {
		t.Errorf("description = %v, потеряно поле обёртки", s["description"])
	}
	if _, leftover := s["server_obj"]; leftover {
		t.Error("вложенный объект остался в теле")
	}
	if _, leftover := s["vars"]; leftover {
		t.Error("vars остались в теле сервера — ядро отвергнет чужой ключ")
	}

	// Плейсхолдеры переименованы с префиксом тега: без него `outbound` от
	// Google DoT затирал бы `outbound` от Cloudflare DoT.
	if s["server"] != "@dns_google_dot_dns_ip" {
		t.Errorf("server = %v, ожидался @dns_google_dot_dns_ip", s["server"])
	}
	if s["detour"] != "@dns_google_dot_outbound" {
		t.Errorf("detour = %v, ожидался @dns_google_dot_outbound", s["detour"])
	}

	if len(vars) != 2 {
		t.Fatalf("переменных %d, ожидалось 2: %+v", len(vars), vars)
	}
	byName := map[string]TemplateVar{}
	for _, v := range vars {
		byName[v.Name] = v
	}
	ob, ok := byName["dns_google_dot_outbound"]
	if !ok {
		t.Fatalf("нет переменной канала: %+v", byName)
	}
	if ob.Type != "outbound" {
		t.Errorf("тип переменной канала = %q, ожидался outbound", ob.Type)
	}
	if ob.DefaultValue.Scalar != "vpn-1" {
		t.Errorf("дефолт канала = %q, ожидался vpn-1", ob.DefaultValue.Scalar)
	}
	ip, ok := byName["dns_google_dot_dns_ip"]
	if !ok {
		t.Fatal("нет переменной адреса")
	}
	if len(ip.Options) != 2 || ip.Options[0] != "8.8.8.8" {
		t.Errorf("варианты адреса = %v, ожидались два значения", ip.Options)
	}
	if len(ip.OptionTitles) != 2 || ip.OptionTitles[0] != "8.8.8.8 · Primary v4" {
		t.Errorf("подписи вариантов = %v, потеряны", ip.OptionTitles)
	}
}

// Шаблон в нашей плоской форме обязан грузиться байт-в-байт как раньше.
func TestNormalizeDNSOptionsFlatUnchanged(t *testing.T) {
	raw := json.RawMessage(`{"servers":[{"type":"udp","tag":"x","server":"1.1.1.1","enabled":true}]}`)
	normalized, vars := normalizeDNSOptions(raw)
	if len(vars) != 0 {
		t.Errorf("плоская запись объявила переменные: %+v", vars)
	}
	if string(normalized) != string(raw) {
		t.Errorf("плоская секция переписана:\n%s\n vs\n%s", normalized, raw)
	}
}

// Пустая и битая секция не роняют загрузку: шаблон без dns_options валиден.
func TestNormalizeDNSOptionsTolerant(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(``), json.RawMessage(`{`), json.RawMessage(`{"servers":"nope"}`)} {
		out, vars := normalizeDNSOptions(raw)
		if len(vars) != 0 {
			t.Errorf("%q → переменные %+v, ожидалось пусто", raw, vars)
		}
		if string(out) != string(raw) {
			t.Errorf("%q переписано в %q", raw, out)
		}
	}
}
