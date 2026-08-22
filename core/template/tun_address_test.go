package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// loadShippedTemplate читает реальный bin/wizard_template.json из корня репозитория.
// Тесты ниже намеренно идут по нему, а не по синтетическому шаблону: баг из issue #97
// живёт именно в рассинхроне между объявлением вара (type) и формой его подстановки
// в inbounds ("address": ["@tun_address"]) — синтетика такой рассинхрон не поймает.
func loadShippedTemplate(t *testing.T) json.RawMessage {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		path := filepath.Join(dir, "bin", "wizard_template.json")
		if _, err := os.Stat(path); err == nil {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("bin/wizard_template.json not found upwards from %s", dir)
		}
		dir = parent
	}
}

// tunAddressFromTemplate резолвит вары шаблона с заданным state и возвращает
// значение inbounds[0].address из секции config.inbounds (TUN-инбаунд).
func tunAddressFromTemplate(t *testing.T, raw json.RawMessage, state map[string]string) interface{} {
	t.Helper()
	var root struct {
		Vars   []TemplateVar   `json:"vars"`
		Params []TemplateParam `json:"params"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	// Секций inbounds в шаблоне несколько (TUN и mixed proxy-in) — нужна та,
	// что гейтится по @tun.
	var inbounds json.RawMessage
	for _, p := range root.Params {
		if p.Name != "inbounds" {
			continue
		}
		// SPEC 107: гейт живёт в #enable (легаси if/if_or читаются как алиасы),
		// поэтому ищем секцию по ЗАВИСИМОСТЯМ нормализованного гейта, а не по
		// конкретному полю — тест не должен ломаться от формы записи.
		deps := p.Gate().Deps()
		if len(deps) == 1 && deps[0] == "tun" {
			inbounds = p.Value
		}
	}
	if len(inbounds) == 0 {
		t.Fatal("template params: TUN inbounds section not found")
	}
	target := TargetSpec{GOOS: "linux", GOARCH: "amd64"}.Normalized()
	resolved := ResolveTemplateVarsFor(root.Vars, state, raw, target)
	out, err := SubstituteVarsInJSON(inbounds, root.Vars, resolved, target)
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(out, &list); err != nil {
		t.Fatalf("inbounds is not an array of objects: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("inbounds is empty")
	}
	return list[0]["address"]
}

func assertAddressList(t *testing.T, got interface{}, want []string) {
	t.Helper()
	arr, ok := got.([]interface{})
	if !ok {
		t.Fatalf("address: %T %v — want JSON array", got, got)
	}
	if len(arr) != len(want) {
		t.Fatalf("address: %v — want %v", arr, want)
	}
	for i, w := range want {
		if arr[i] != w {
			t.Fatalf("address[%d] = %v, want %q", i, arr[i], w)
		}
	}
}

// Один адрес в state (в том числе state, записанный старой версией лаунчера,
// где tun_address был type=text) остаётся одноэлементным массивом.
func TestTunAddress_SingleStaysOneElement(t *testing.T) {
	raw := loadShippedTemplate(t)
	got := tunAddressFromTemplate(t, raw, map[string]string{
		"tun": "true", "tun_address": "172.16.0.1/30",
	})
	assertAddressList(t, got, []string{"172.16.0.1/30"})
}

// IPv6-адрес рядом с IPv4 — вторым элементом address.
//
// Прежде (issue #97) он задавался ВТОРОЙ СТРОКОЙ в самом tun_address:
// поле было text_list, и в него клали оба семейства сразу. Это был второй
// способ настроить IPv6 рядом с галкой ipv6_enabled, и он же расходился с
// мобильным (разрыв N7). Теперь у каждого семейства своё однострочное
// поле, а многострочные значения из старого state разводятся при загрузке
// (models.MigrateTunAddressSplit) — сюда они уже не доходят.
func TestTunAddress_IPv4AndIPv6(t *testing.T) {
	raw := loadShippedTemplate(t)
	got := tunAddressFromTemplate(t, raw, map[string]string{
		"tun":          "true",
		"tun_address":  "172.16.0.1/30",
		"ipv6_enabled": "true",
		"tun_address6": "fdfe:dcba:9876::1/126",
	})
	assertAddressList(t, got, []string{"172.16.0.1/30", "fdfe:dcba:9876::1/126"})
}

// Выключенная галка IPv6 не пускает адрес в config, даже если он задан:
// иначе туннель получал бы IPv6 вопреки настройке.
func TestTunAddress_IPv6DisabledOmitsAddress(t *testing.T) {
	raw := loadShippedTemplate(t)
	got := tunAddressFromTemplate(t, raw, map[string]string{
		"tun":          "true",
		"tun_address":  "172.16.0.1/30",
		"ipv6_enabled": "false",
		"tun_address6": "fdfe:dcba:9876::1/126",
	})
	assertAddressList(t, got, []string{"172.16.0.1/30"})
}

// Дефолт шаблона (state пуст) тоже должен давать валидный одноэлементный массив.
func TestTunAddress_DefaultIsArray(t *testing.T) {
	raw := loadShippedTemplate(t)
	got := tunAddressFromTemplate(t, raw, map[string]string{"tun": "true"})
	assertAddressList(t, got, []string{"172.16.0.1/30"})
}
