package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// SPEC 094, критерий приёмки 8/25 — импортированный конфиг обязан проходить
// настоящий `sing-box check`, а не только сравнение структур в памяти.
//
// Тест ищет ядро в bin/sing-box относительно корня репозитория и пропускается,
// если его нет (CI без ядра, чужая машина). Так проверка остаётся честной там,
// где ядро есть, и не делает тесты хрупкими там, где его нет.

func locateSingboxBinary(t *testing.T) string {
	t.Helper()

	// Ядро в репозитории не хранится (.gitignore: «downloaded artifact»), а
	// bin/ проекта — рабочий каталог, куда его кладёт установка. Ищем сначала
	// установленное приложение: там лежит ядро с нужными build-тегами
	// (with_xhttp и прочие расширения форка sing-box-lx). Апстримный sing-box
	// из PATH отвергнет конфиг с xhttp-транспортом, что выглядело бы как
	// дефект генератора, хотя это разница сборок ядра.
	//
	// SINGBOX_TEST_CORE позволяет указать бинарь явно — для CI и чужих машин.
	if custom := strings.TrimSpace(os.Getenv("SINGBOX_TEST_CORE")); custom != "" {
		if info, err := os.Stat(custom); err == nil && !info.IsDir() {
			return custom
		}
	}

	candidates := []string{
		// Установленное приложение (macOS).
		"/Applications/singbox-launcher.app/Contents/MacOS/bin/sing-box",
		// Рабочий каталог проекта, если ядро туда положили вручную.
		filepath.Join("..", "..", "bin", "sing-box"),
		filepath.Join("..", "..", "bin", "sing-box.exe"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

// buildCheckableConfig собирает минимальный, но полноценный config.json вокруг
// переданных outbound'ов.
func buildCheckableConfig(t *testing.T, outbounds []map[string]interface{}) []byte {
	t.Helper()

	cfg := map[string]interface{}{
		"log": map[string]interface{}{"level": "error"},
		"inbounds": []interface{}{
			map[string]interface{}{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      "127.0.0.1",
				"listen_port": 2080,
			},
		},
		"outbounds": outbounds,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return data
}

func runSingboxCheck(t *testing.T, binary string, config []byte) (string, error) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, config, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(binary, "check", "-c", path)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Узлы и цепочка, полученные из импорта sing-box конфига, дают конфиг,
// который ядро принимает.
func TestImportedNodesPassSingboxCheck(t *testing.T) {
	binary := locateSingboxBinary(t)
	if binary == "" {
		t.Skip("bin/sing-box not found — skipping real-core validation")
	}

	outbounds := []map[string]interface{}{
		{
			"type":        "vless",
			"tag":         "reality-node",
			"server":      "reality.example.com",
			"server_port": 443,
			"uuid":        "b831381d-6324-4d53-ad4f-8cda48b30811",
			"flow":        "xtls-rprx-vision",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "www.microsoft.com",
				"utls":        map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0",
					"short_id":   "0123abcd",
				},
			},
		},
		{
			"type":        "trojan",
			"tag":         "chained-node",
			"server":      "trojan.example.com",
			"server_port": 443,
			"password":    "trojan-password",
			"tls":         map[string]interface{}{"enabled": true, "server_name": "trojan.example.com"},
			"detour":      "jump-socks",
		},
		{
			"type":        "socks",
			"tag":         "jump-socks",
			"server":      "127.0.0.1",
			"server_port": 1080,
			"version":     "5",
		},
		{
			"type":      "urltest",
			"tag":       "auto-group",
			"outbounds": []interface{}{"reality-node", "chained-node"},
			"url":       "https://www.gstatic.com/generate_204",
			"interval":  "5m",
		},
		{
			"type":      "selector",
			"tag":       "manual-group",
			"outbounds": []interface{}{"reality-node", "chained-node"},
			"default":   "reality-node",
		},
	}

	out, err := runSingboxCheck(t, binary, buildCheckableConfig(t, outbounds))
	if err != nil {
		t.Fatalf("sing-box check rejected the imported config: %v\n%s", err, out)
	}
}

// Санитайзы не просто «чистят на всякий случай»: без них ядро действительно
// отвергает конфиг. Тест фиксирует, что именно мы предотвращаем.
func TestUnsanitizedValuesAreRejectedByCore(t *testing.T) {
	binary := locateSingboxBinary(t)
	if binary == "" {
		t.Skip("bin/sing-box not found — skipping real-core validation")
	}

	cases := []struct {
		name     string
		outbound map[string]interface{}
		wantErr  string
	}{
		{
			name: "unknown uTLS fingerprint kills the whole config",
			outbound: map[string]interface{}{
				"type": "vless", "tag": "bad", "server": "e.com", "server_port": 443,
				"uuid": "b831381d-6324-4d53-ad4f-8cda48b30811",
				"tls": map[string]interface{}{
					"enabled": true,
					"utls":    map[string]interface{}{"enabled": true, "fingerprint": "HelloChrome_120"},
				},
			},
			wantErr: "fingerprint",
		},
		{
			name: "invalid REALITY public_key kills the whole config",
			outbound: map[string]interface{}{
				"type": "vless", "tag": "bad", "server": "e.com", "server_port": 443,
				"uuid": "b831381d-6324-4d53-ad4f-8cda48b30811",
				"tls": map[string]interface{}{
					"enabled": true,
					"reality": map[string]interface{}{"enabled": true, "public_key": "enabled"},
				},
			},
			wantErr: "public_key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []map[string]interface{}{tc.outbound}
			out, err := runSingboxCheck(t, binary, buildCheckableConfig(t, raw))
			if err == nil {
				t.Fatalf("expected the core to reject this config, but check passed:\n%s", out)
			}
			if !strings.Contains(strings.ToLower(out), tc.wantErr) {
				t.Logf("core rejected with: %s", out)
			}

			// А после санитайза — тот же outbound проходит.
			// Импорт использует ту же функцию, что и здесь.
			sanitized := map[string]interface{}{}
			for k, v := range tc.outbound {
				sanitized[k] = v
			}
			sanitizeForTest(t, sanitized)

			out2, err2 := runSingboxCheck(t, binary, buildCheckableConfig(t, []map[string]interface{}{sanitized}))
			if err2 != nil {
				t.Fatalf("sanitized outbound still rejected: %v\n%s", err2, out2)
			}
		})
	}
}

// sanitizeForTest вызывает ровно тот санитайзер, которым пользуется импорт.
func sanitizeForTest(t *testing.T, ob map[string]interface{}) {
	t.Helper()
	subscription.SanitizeSingboxOutboundMap(ob, "test")
}
