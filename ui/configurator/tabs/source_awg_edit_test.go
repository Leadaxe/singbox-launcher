package tabs

import (
	"encoding/json"
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// wgNodeWithBody — узел-сервер с телом WireGuard-endpoint'а.
func wgNodeWithBody(t *testing.T, body map[string]interface{}) *wizardmodels.Node {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return &wizardmodels.Node{
		Kind: wizardmodels.SourceKindServer,
		Tag:  "wg",
		Body: raw,
	}
}

// wgBody — годное по реестру тело WireGuard-endpoint'а (wg-quick от Proton:
// address / private_key / peers) плюс extra. Форма пишет только выход
// конвейера, поэтому тело без обязательных полей она не сохранит.
func wgBody(extra map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"type":        "wireguard",
		"address":     []interface{}{"10.2.0.2/32"},
		"private_key": "2BfWIBWcDEZnpn/iMT272pAFH0lmX+ix1xHP4QDM+28=",
		"peers": []interface{}{map[string]interface{}{
			"address": "79.127.186.193", "port": float64(51820),
			"public_key":  "tw0rM/g0IiZKM+Hz0T7jJTSDk8aiOgGEUuG6NkR2mik=",
			"allowed_ips": []interface{}{"0.0.0.0/0", "::/0"},
		}},
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// bodyOf — тело узла как map, чтобы проверять состав ключей.
func bodyOf(t *testing.T, node *wizardmodels.Node) map[string]interface{} {
	t.Helper()
	var ob map[string]interface{}
	if err := json.Unmarshal(node.Body, &ob); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return ob
}

// Запись обфускации в готовый узел: набор полей исчерпывающий, чужие поля
// тела переживают правку, а несовместимые с сахаром i1–i5 снимаются.
func TestApplyAWGSettings_WritesExactFieldSet(t *testing.T) {
	node := wgNodeWithBody(t, wgBody(map[string]interface{}{
		"mtu": 1280,
		// Поле, которого форма не знает: пересборка тела молча потеряла бы его.
		"listen_port": 51000,
		// Явный junk-тег из чужой ссылки — ядро отвергает его рядом с id/ip.
		"i1": "<b 0x00>",
	}))

	err := applyAWGSettings(node, awgSettings{
		Enabled: true,
		Domain:  "example.com",
		Browser: "chrome",
		JC:      "4", JMin: "40", JMax: "70",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	ob := bodyOf(t, node)
	for key, want := range map[string]interface{}{
		"id": "example.com", "ip": awgFixedIP, "ib": "chrome",
	} {
		if got, _ := ob[key].(string); got != want {
			t.Errorf("%s: got %v, want %v", key, ob[key], want)
		}
	}
	for key, want := range map[string]float64{"jc": 4, "jmin": 40, "jmax": 70} {
		if got, _ := ob[key].(float64); got != want {
			t.Errorf("%s: got %v, want %v", key, ob[key], want)
		}
	}
	// Чужие поля на месте, узел не пересобран из нуля.
	if got, _ := ob["listen_port"].(float64); got != 51000 {
		t.Errorf("listen_port lost: body was rebuilt instead of patched: %v", ob["listen_port"])
	}
	if _, ok := ob["peers"]; !ok {
		t.Error("peers lost")
	}
	// Ничего сверх набора: i1–i5 несовместимы с сахаром, а s/h ломают
	// рукопожатие любым значением, кроме дефолтного.
	for _, k := range []string{"i1", "i2", "i3", "i4", "i5", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4"} {
		if _, present := ob[k]; present {
			t.Errorf("%s must not be present: got %v", k, ob[k])
		}
	}
}

// Форма показывает то, что в узле сейчас: настроенный узел не должен
// открываться с пустыми полями.
func TestReadAWGSettings_RoundTrip(t *testing.T) {
	plain := wgNodeWithBody(t, map[string]interface{}{
		"type": "wireguard", "server": "vpn.example",
	})
	if got := readAWGSettings(plain); got.Enabled {
		t.Errorf("plain WireGuard must read as disabled: %+v", got)
	}

	node := wgNodeWithBody(t, wgBody(nil))
	want := awgSettings{
		Enabled: true, Domain: "example.com", Browser: "firefox",
		JC: "8", JMin: "10", JMax: "90",
	}
	if err := applyAWGSettings(node, want); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := readAWGSettings(node)
	if got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}

// Снятая галочка возвращает обычный WireGuard: половина полей осталась бы
// набором, который ломает рукопожатие с не-AWG сервером.
func TestClearAWGSettings_LeavesPlainWireGuard(t *testing.T) {
	node := wgNodeWithBody(t, wgBody(map[string]interface{}{
		"mtu": 1280,
		"jc":  4, "jmin": 40, "jmax": 70, "ip": "quic", "id": "example.com", "ib": "chrome",
		"s1": 20, "h1": 5, "i1": "<b 0x00>",
	}))
	if err := clearAWGSettings(node); err != nil {
		t.Fatalf("clear: %v", err)
	}
	ob := bodyOf(t, node)
	for _, k := range []string{
		"jc", "jmin", "jmax", "ip", "id", "ib",
		"s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1",
	} {
		if _, present := ob[k]; present {
			t.Errorf("%s survived clear: %v", k, ob[k])
		}
	}
	if _, ok := ob["private_key"]; !ok {
		t.Error("private_key lost")
	}
	if got, _ := ob["mtu"].(float64); got != 1280 {
		t.Errorf("mtu lost: %v", ob["mtu"])
	}
	if got := readAWGSettings(node); got.Enabled {
		t.Errorf("node must read as plain after clear: %+v", got)
	}
}

// Негодный ввод отвергается ДО записи: узел остаётся прежним, а человек
// видит причину. Молча записанный мусор ушёл бы в конфиг и уронил узел.
func TestApplyAWGSettings_RejectsBadInput(t *testing.T) {
	valid := awgSettings{
		Enabled: true, Domain: "example.com", Browser: "chrome",
		JC: "4", JMin: "40", JMax: "70",
	}
	for name, mutate := range map[string]func(*awgSettings){
		"empty domain":    func(s *awgSettings) { s.Domain = "" },
		"domain hyphen":   func(s *awgSettings) { s.Domain = "-bad.example" },
		"domain bad rune": func(s *awgSettings) { s.Domain = "пример.рф" },
		"empty jc":        func(s *awgSettings) { s.JC = "" },
		"jc not a number": func(s *awgSettings) { s.JC = "many" },
		"negative jc":     func(s *awgSettings) { s.JC = "-1" },
		"jmin over jmax":  func(s *awgSettings) { s.JMin, s.JMax = "70", "40" },
	} {
		node := wgNodeWithBody(t, map[string]interface{}{
			"type": "wireguard", "server": "vpn.example",
		})
		before := string(node.Body)

		in := valid
		mutate(&in)
		if err := applyAWGSettings(node, in); err == nil {
			t.Errorf("%s must fail", name)
		}
		if string(node.Body) != before {
			t.Errorf("%s: node body changed on a rejected input", name)
		}
	}

	// Тело, которое конвейер собрать не может (нет private_key — вердикт
	// уровня узла), форма не пишет: откат, как у редактора JSON, а не запись
	// сырой карты мимо санитайзера (SPEC 142 A11).
	broken := wgBody(nil)
	delete(broken, "private_key")
	node := wgNodeWithBody(t, broken)
	before := string(node.Body)
	if err := applyAWGSettings(node, valid); err == nil {
		t.Error("body without private_key must not be written")
	}
	if string(node.Body) != before {
		t.Error("node body changed on a pipeline drop")
	}
}

// Блок показывается только у WireGuard-узла: у прочих типов обфусцировать
// нечего, и форма молчит про них.
func TestAWGEditableNode(t *testing.T) {
	wg := wgNodeWithBody(t, map[string]interface{}{"type": "wireguard"})
	if !awgEditableNode(wg) {
		t.Error("wireguard node must be editable")
	}
	vless := wgNodeWithBody(t, map[string]interface{}{"type": "vless"})
	if awgEditableNode(vless) {
		t.Error("non-wireguard node must not show the block")
	}
	if awgEditableNode(nil) {
		t.Error("nil node must not show the block")
	}
	if awgEditableNode(&wizardmodels.Node{Kind: wizardmodels.SourceKindServer}) {
		t.Error("node without a body must not show the block")
	}
}

// Запись тела через конвейер обязана сохранить "type".
//
// `nodeflow.Emit` отдаёт тело без managed-ключа "type". На реальном теле
// (wg-quick от Proton: address / private_key / peers) конвейер ничего не
// теряет, и форма писала результат Emit как есть — узел оставался без типа:
// выпадал из сборки («body has no "type"») и терял саму форму обфускации
// (awgEditableNode смотрит type).
func TestWriteAWGBody_KeepsTypeOnCleanPipeline(t *testing.T) {
	body := wgBody(nil)
	node := wgNodeWithBody(t, body)
	if err := writeAWGBody(node, body, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, _ := bodyOf(t, node)["type"].(string); got != "wireguard" {
		t.Fatalf("type lost after write: body=%s", string(node.Body))
	}
	if !awgEditableNode(node) {
		t.Fatalf("node must stay editable by the AWG form")
	}
	if err := clearAWGSettings(node); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ := bodyOf(t, node)["type"].(string); got != "wireguard" {
		t.Fatalf("type lost after clear: body=%s", string(node.Body))
	}
}
