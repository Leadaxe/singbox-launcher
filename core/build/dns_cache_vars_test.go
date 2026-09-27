package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/template"
)

// dnsCacheFields собирает конфиг боевого шаблона с заданными переменными и
// возвращает три поля кэша DNS (SPEC 147, LxBox §580).
func dnsCacheFields(t *testing.T, vars map[string]string) (capacity, optimistic, storeDNS interface{}, res Result) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "bin", "wizard_template.json"))
	if err != nil {
		t.Fatalf("read wizard_template.json: %v", err)
	}
	target := template.TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: "local"}.Normalized()
	res = buildWizardTemplateCase(t, raw, wizardTemplateCase{name: "dns_cache", vars: vars}, target)
	var cfg struct {
		DNS          map[string]interface{} `json:"dns"`
		Experimental struct {
			CacheFile map[string]interface{} `json:"cache_file"`
		} `json:"experimental"`
	}
	if err := json.Unmarshal(jsonc.ToJSON(res.ConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v\n%s", err, res.ConfigJSON)
	}
	return cfg.DNS["cache_capacity"], cfg.DNS["optimistic"], cfg.Experimental.CacheFile["store_dns"], res
}

// Состояние без трёх переменных (пользователь до SPEC 147): действуют
// значения по умолчанию шаблона.
func TestDNSCacheSettings_DefaultsInConfig(t *testing.T) {
	c, o, s, _ := dnsCacheFields(t, map[string]string{"tun": "true"})
	if c != float64(4000) || o != true || s != true {
		t.Fatalf("defaults: cache_capacity=%v optimistic=%v store_dns=%v", c, o, s)
	}
}

func TestDNSCacheSettings_ChangedValuesInConfig(t *testing.T) {
	c, o, s, _ := dnsCacheFields(t, map[string]string{
		VarDNSCacheCapacity: "4096", VarDNSOptimistic: "false", VarDNSStoreCache: "false",
	})
	if c != float64(4096) || o != false || s != false {
		t.Fatalf("changed: cache_capacity=%v optimistic=%v store_dns=%v", c, o, s)
	}
}

func TestDNSCacheSettings_CapacityOutOfBoundsNotInConfig(t *testing.T) {
	for _, bad := range []string{"100", "1023", "65536", "65537", "1000000", "abc", ""} {
		vars := map[string]string{VarDNSCacheCapacity: bad}
		c, _, _, res := dnsCacheFields(t, vars)
		if c != float64(4000) {
			t.Fatalf("%q: cache_capacity=%v, want template default 4000", bad, c)
		}
		if len(res.Validation.Warnings) == 0 {
			t.Fatalf("%q: no validation warning", bad)
		}
		if vars[VarDNSCacheCapacity] != bad {
			t.Fatalf("%q: caller vars mutated", bad)
		}
	}
	for _, ok := range []string{"1024", "65535"} {
		if !ValidDNSCacheCapacity(ok) {
			t.Fatalf("%q must be valid", ok)
		}
	}
}
