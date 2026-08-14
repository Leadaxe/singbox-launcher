package services

import (
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// writeMachineRuleSet кладёт файл в .srs каталог машины — туда, откуда
// CollectDeployResources берёт тела ресурсов.
func writeMachineRuleSet(t *testing.T, execDir, machineID, name, body string) {
	t.Helper()
	dir := platform.GetRuleSetsDirFor(execDir, constants.ConfigTargetRemote, machineID)
	if err := os.MkdirAll(dir, platform.DefaultDirMode); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), platform.DefaultFileMode); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// Собираем ровно те файлы, на которые ссылается конфиг: список заливаемого
// берётся из rule_set[], а не из содержимого каталога.
func TestCollectDeployResourcesPicksReferenced(t *testing.T) {
	execDir := t.TempDir()
	const machineID = "m1"
	writeMachineRuleSet(t, execDir, machineID, "geosite-ru.srs", "RU")
	// Лишний файл в каталоге: конфиг на него не ссылается, значит и заливать
	// его незачем.
	writeMachineRuleSet(t, execDir, machineID, "unused.srs", "NOPE")

	config := []byte(`{
		// комментарий: конфиг у нас jsonc
		"route": {
			"rule_set": [
				{"tag": "ru", "type": "local", "path": "/etc/sing-box/resources/geosite-ru.srs"},
				{"tag": "remote-src", "type": "remote", "url": "https://example.invalid/x.srs"},
				{"tag": "manual", "type": "local", "path": "/opt/custom/manual.srs"}
			]
		}
	}`)

	got, err := CollectDeployResources(execDir, machineID, config)
	if err != nil {
		t.Fatalf("CollectDeployResources: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("собрано %d ресурсов, ожидался 1: %v", len(got), keysOf(got))
	}
	if string(got["geosite-ru.srs"]) != "RU" {
		t.Fatalf("тело ресурса = %q, ожидалось %q", got["geosite-ru.srs"], "RU")
	}
}

// Ссылка на отсутствующий файл — ошибка, а не тихий пропуск: конфиг со
// ссылкой в пустоту уронил бы ядро на той стороне.
func TestCollectDeployResourcesMissingFileFails(t *testing.T) {
	execDir := t.TempDir()
	config := []byte(`{"route":{"rule_set":[
		{"tag":"ru","type":"local","path":"/etc/sing-box/resources/absent.srs"}
	]}}`)

	if _, err := CollectDeployResources(execDir, "m1", config); err == nil {
		t.Fatal("ожидалась ошибка на отсутствующий rule-set, получено nil")
	}
}

// Битый конфиг: разбор обязан вернуть ошибку, а не пустой набор — иначе
// деплой прошёл бы «успешно», не залив ничего.
func TestCollectDeployResourcesBrokenConfigFails(t *testing.T) {
	if _, err := CollectDeployResources(t.TempDir(), "m1", []byte("{not json")); err == nil {
		t.Fatal("ожидалась ошибка разбора конфига, получено nil")
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
