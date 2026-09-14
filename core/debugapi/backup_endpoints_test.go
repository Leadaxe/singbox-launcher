package debugapi

// SPEC 127 W2.9 — перенос настроек через debug API.
//
// Проверяется свойство, ради которого ручки и заведены: снимок, снятый
// снаружи, ВОССТАНАВЛИВАЕТ настройку на другой машине. Поэтому центральный
// тест — круг export → import в пустое состояние → export, и сравнение по
// БАЙТАМ: равенство отдельных полей пропустило бы ровно то, что теряется
// на переносе чаще всего, — поле, о котором забыли и в проверке.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// backupTestState — состояние, в котором есть все сущности, доступные форме
// 1.0: подписка с identity/skip/fold, папка с политикой тегов и составом,
// корневой узел с секциями, цепочка с адресным хопом, Направление, правила
// трёх видов, DNS обоих видов, warp.
//
// Полное состояние, а не «пара записей»: круг легко удержать на пустом файле
// и легко потерять на одном поле, которое едет только у одного вида записи.
func backupTestState() *state.State {
	send := false
	auto := true
	st := state.New()
	st.Directions = []configtypes.Direction{
		{Tag: "vpn-de", Type: "selector", AddOutbounds: []string{"direct-out"}},
	}
	sub := state.Source{
		ID:   "01SUB0000000000000000000",
		Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
		URL:  "https://example-1.com/sub", Name: "Main", MaxNodes: 200,
		TagPolicy:       &state.TagPolicy{Prefix: "[A] "},
		Update:          &state.UpdateSpec{IntervalHours: 12, AutoRefresh: &auto},
		Skip:            []map[string]string{{"field": "tag", "contains": "trial"}},
		PendingDisabled: []string{"node-a"},
	}
	sub.SetIdentity("Happ/1.0", "7c9e6679-7425-40de-944b-e07fc1f90ae7", &send, nil)

	sections := &state.NodeSections{}
	nodeRule := state.NewInlineRule(
		state.SelfPlaceholderBraced+" net",
		map[string]interface{}{"ip_cidr": []interface{}{"100.64.0.0/10"}},
		state.SelfPlaceholder)
	nodeRule.Enabled = true
	nodeNum := state.NodeRuleDefaultNum
	nodeRule.Num = &nodeNum
	sections.Rules = []state.Rule{nodeRule}
	sections.SetDNS(
		[]state.DNSServer{{
			Kind: state.DNSServerKindUser, Tag: "ts-dns", Enabled: true,
			Body: map[string]interface{}{"type": "tailscale", "endpoint": state.SelfPlaceholder},
		}},
		[]state.DNSRule{{
			Kind: state.DNSRuleKindUser, Enabled: true,
			Body: map[string]interface{}{"domain_suffix": []interface{}{".ts.net"}, "server": "ts-dns"},
		}})

	st.Sources = []state.Source{
		sub,
		{
			ID: "01SRV0000000000000000000",
			Node: state.Node{
				Kind: state.SourceKindServer, Enabled: true, Tag: "ts-node",
				Origin:   &state.Origin{Kind: state.OriginKindURI, Raw: "trojan://pw@1.2.3.4:443#ts-node"},
				Sections: sections,
			},
		},
		{
			ID: "01FLD0000000000000000000", Name: "Proton",
			TagPolicy: &state.TagPolicy{Prefix: "[P] "},
			Node:      state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Nodes: []state.Node{{
				Kind: state.SourceKindServer, Enabled: true, Tag: "Amsterdam",
				Body: json.RawMessage(`{"type":"trojan","server":"ams.example","server_port":443}`),
			}},
		},
		{
			ID: "01CHN0000000000000000000",
			Node: state.Node{
				Kind: state.SourceKindChain, Enabled: true, Tag: "relay",
				Hops: []state.NodeLink{{FolderID: "01FLD0000000000000000000", Tag: "Amsterdam"}},
			},
		},
	}

	srs := state.NewSrsRule("Ads", []string{"https://example.com/ads.srs"}, "reject")
	srs.Enabled = true
	srsNum := state.UserRuleNumStart + 2
	srs.Num = &srsNum
	inline := inlineRule("Work", map[string]interface{}{
		"domain_suffix": []interface{}{"example.com"},
	}, "vpn-de")
	inlineNum := state.UserRuleNumStart + 1
	inline.Num = &inlineNum
	preset := presetRule("traffic-processing", map[string]string{"mode": "on"}, true)
	presetNum := state.UserRuleNumStart
	preset.Num = &presetNum
	st.Rules = []state.Rule{preset, inline, srs}

	st.Vars = []state.SettingVar{
		{Name: "log_level", Value: "debug"},
		{Name: "route_final", Value: "vpn-de"},
	}
	st.DNS.Final = "google_dot"
	st.DNS.Strategy = "ipv4_only"
	st.DNS.Servers = []state.DNSServer{
		{Kind: state.DNSServerKindTemplate, Tag: "google_dot", Enabled: true},
		{Kind: state.DNSServerKindUser, Tag: "my_dns", Enabled: true,
			Body: map[string]interface{}{"type": "udp", "server": "10.0.0.1"}},
	}
	st.DNS.Rules = []state.DNSRule{{
		Kind: state.DNSRuleKindUser, Enabled: true,
		Body: map[string]interface{}{"domain_suffix": []interface{}{"example.com"}, "server": "my_dns"},
	}}
	st.WarpAccounts = &state.WarpAccountsSection{
		WG: &state.WarpWGAccount{PrivateKey: "priv", PeerPublic: "pub", ClientV4: "172.16.0.2"},
	}
	return st
}

// fetchBackup снимает файл заданным форматом и отдаёт тело плюс заголовки.
func fetchBackup(t *testing.T, base, query string) ([]byte, *http.Response) {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/backup/export"+query, nil))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes(), resp
}

// Круг через API: export 1.0 → import в ПУСТОЕ состояние → export.
//
// Пустое состояние на приёмнике — не удобство теста, а сам смысл переноса:
// файл обязан описывать настройку целиком, а не быть дельтой к тому, что уже
// стояло. Номера оси импорт перенумеровывает (§9 п. 7), поэтому сравнение
// идёт со стёртыми номерами, а тождество байт в байт проверяется со ВТОРОГО
// круга — ровно та же пара утверждений, что в core/backup (purity_test.go).
func TestBackupExportImportRoundTripOverAPI(t *testing.T) {
	src := &fakeFacade{stateValue: backupTestState()}
	srcBase, _ := newTestServer(t, src)

	first, resp := fetchBackup(t, srcBase, "?format=1.0")
	if resp.StatusCode != 200 {
		t.Fatalf("export status %d: %s", resp.StatusCode, first)
	}
	if cd := resp.Header.Get("Content-Disposition"); !bytes.Contains([]byte(cd), []byte("lx-backup-")) {
		t.Errorf("Content-Disposition без имени файла: %q", cd)
	}
	var head struct {
		LxBackup int `json:"lx_backup"`
	}
	if err := json.Unmarshal(first, &head); err != nil {
		t.Fatalf("тело ответа — не файл бэкапа: %v\n%s", err, first)
	}
	if head.LxBackup != 2 {
		t.Fatalf("?format=1.0 отдал файл с lx_backup=%d", head.LxBackup)
	}

	// Приёмник: пустое состояние, своя сборка.
	dst := &fakeFacade{stateValue: state.New()}
	dstBase, _ := newTestServer(t, dst)

	var imported struct {
		OK            bool `json:"ok"`
		ConfigRebuilt bool `json:"config_rebuilt"`
		Applied       struct {
			Sources int `json:"sources"`
			Rules   int `json:"rules"`
		} `json:"applied"`
		Warnings []backupWarningView `json:"warnings"`
	}
	status, raw := doJSON(t, authedReq(t, "POST", dstBase+"/backup/import", first), &imported)
	if status != 200 || !imported.OK {
		t.Fatalf("import status %d: %s", status, raw)
	}
	if imported.Applied.Sources == 0 || imported.Applied.Rules == 0 {
		t.Fatalf("импорт ничего не применил: %s", raw)
	}
	// Импорт обязан довести состояние до диска и до config.json тем же путём,
	// что UI: без Save следующий запуск не увидел бы перенесённой настройки.
	if dst.savedState == nil {
		t.Error("импорт не сохранил состояние")
	}
	if src.rebuilds != 0 {
		t.Errorf("экспорт дёрнул пересборку конфига (%d раз) — чтение состояния ничего не пересобирает", src.rebuilds)
	}
	if dst.rebuilds != 1 || !imported.ConfigRebuilt {
		t.Errorf("импорт не пересобрал конфиг: rebuilds=%d, config_rebuilt=%v", dst.rebuilds, imported.ConfigRebuilt)
	}

	second, resp2 := fetchBackup(t, dstBase, "?format=1.0")
	if resp2.StatusCode != 200 {
		t.Fatalf("re-export status %d: %s", resp2.StatusCode, second)
	}
	if stripAPIAxisNums(string(second)) != stripAPIAxisNums(string(first)) {
		t.Fatalf("перенос через API потерял не только номера оси:\n--- отправлено ---\n%s\n--- получено обратно ---\n%s", first, second)
	}

	// Второй круг — тождество байт в байт: состояние на приёмнике уже
	// размечено, и дальше файл обязан не дрейфовать никогда.
	dst2 := &fakeFacade{stateValue: state.New()}
	dst2Base, _ := newTestServer(t, dst2)
	if status, raw := doJSON(t, authedReq(t, "POST", dst2Base+"/backup/import", second), nil); status != 200 {
		t.Fatalf("второй импорт status %d: %s", status, raw)
	}
	third, _ := fetchBackup(t, dst2Base, "?format=1.0")
	if string(third) != string(second) {
		t.Fatalf("круг не байт-идентичен:\n--- до ---\n%s\n--- после ---\n%s", second, third)
	}
}

// stripAPIAxisNums — см. пояснение к кругу выше: номера оси импорт вправе
// переписать, всё прочее обязано доехать дословно.
func stripAPIAxisNums(doc string) string {
	out := make([]byte, 0, len(doc))
	i := 0
	for i < len(doc) {
		const key = `"num": `
		if len(doc)-i >= len(key) && doc[i:i+len(key)] == key {
			out = append(out, key...)
			out = append(out, 'N')
			i += len(key)
			for i < len(doc) && doc[i] >= '0' && doc[i] <= '9' {
				i++
			}
			continue
		}
		out = append(out, doc[i])
		i++
	}
	return string(out)
}

// Умолчание формата — та же константа, что у UI: ручка без ?format отдаёт
// ровно то, что дал бы чекбокс в исходном положении.
func TestBackupExportDefaultFormatMatchesConstant(t *testing.T) {
	ff := &fakeFacade{stateValue: backupTestState()}
	base, _ := newTestServer(t, ff)

	implicit, _ := fetchBackup(t, base, "")

	var formats struct {
		Reads   []int    `json:"reads"`
		Writes  []string `json:"writes"`
		Default string   `json:"default"`
	}
	if status, raw := doJSON(t, authedReq(t, "GET", base+"/backup/formats", nil), &formats); status != 200 {
		t.Fatalf("formats status %d: %s", status, raw)
	}
	if len(formats.Reads) != 2 || len(formats.Writes) != 2 {
		t.Fatalf("сборка обязана читать оба формата и писать оба: %+v", formats)
	}
	explicit, _ := fetchBackup(t, base, "?format="+formats.Default)
	if string(implicit) != string(explicit) {
		t.Errorf("файл без ?format не совпал с файлом формата по умолчанию (%s)", formats.Default)
	}
}

// Файл 0.12 читается тем же POST: импорт не спрашивает формат и не заставляет
// вызывающего его знать.
func TestBackupImportAcceptsLegacyFormat(t *testing.T) {
	src := &fakeFacade{stateValue: backupTestState()}
	srcBase, _ := newTestServer(t, src)
	legacy, resp := fetchBackup(t, srcBase, "?format=0.12")
	if resp.StatusCode != 200 {
		t.Fatalf("export 0.12 status %d", resp.StatusCode)
	}
	var head struct {
		LxBackup int `json:"lx_backup"`
	}
	if err := json.Unmarshal(legacy, &head); err != nil || head.LxBackup != 1 {
		t.Fatalf("?format=0.12 отдал не файл 0.x: %v lx_backup=%d", err, head.LxBackup)
	}
	// Экспорт 0.12 теряет то, чему в старом формате нет дома, — и обязан
	// назвать потерю, а не отдать файл молча (П6).
	if resp.Header.Get("X-Backup-Warnings") == "" {
		t.Error("экспорт 0.12 богатого состояния не назвал ни одной потери в X-Backup-Warnings")
	}

	dst := &fakeFacade{stateValue: state.New()}
	dstBase, _ := newTestServer(t, dst)
	var out struct {
		OK     bool   `json:"ok"`
		Format string `json:"format"`
	}
	if status, raw := doJSON(t, authedReq(t, "POST", dstBase+"/backup/import", legacy), &out); status != 200 {
		t.Fatalf("import 0.12 status %d: %s", status, raw)
	}
	if out.Format != "0.12" {
		t.Errorf("ответ импорта назвал формат %q, ожидался 0.12", out.Format)
	}
	if len(dst.savedState.Sources) == 0 {
		t.Error("импорт файла 0.12 не привёз источников")
	}
}

// Свежая установка: файла состояния ещё нет, а импорт обязан сработать.
//
// Это самый частый сценарий переноса — «новая машина, вот файл», — и 404 на
// нём означал бы, что восстановиться можно только поверх уже настроенного
// лаунчера. Экспорт в той же ситуации по-прежнему отвечает 404: снимать
// нечего, и пустой файл был бы враньём о содержимом машины.
func TestBackupImportOnFreshInstall(t *testing.T) {
	src := &fakeFacade{stateValue: backupTestState()}
	srcBase, _ := newTestServer(t, src)
	file, _ := fetchBackup(t, srcBase, "?format=1.0")

	fresh := &fakeFacade{stateLoadErr: state.ErrNotFound}
	freshBase, _ := newTestServer(t, fresh)

	if status, raw := doJSON(t, authedReq(t, "GET", freshBase+"/backup/export", nil), nil); status != 404 {
		t.Errorf("экспорт без state.json дал %d, ожидался 404: %s", status, raw)
	}

	var out struct {
		OK      bool `json:"ok"`
		Applied struct {
			Sources int `json:"sources"`
		} `json:"applied"`
	}
	if status, raw := doJSON(t, authedReq(t, "POST", freshBase+"/backup/import", file), &out); status != 200 {
		t.Fatalf("импорт на свежей установке дал %d: %s", status, raw)
	}
	if !out.OK || out.Applied.Sources == 0 {
		t.Fatalf("импорт на свежей установке ничего не привёз: %+v", out)
	}
	if fresh.savedState == nil || len(fresh.savedState.Sources) == 0 {
		t.Fatal("импорт на свежей установке не создал состояния")
	}
	if fresh.savedState.Version != state.SchemaVersion {
		t.Errorf("созданное состояние написано схемой %d, ожидалась %d",
			fresh.savedState.Version, state.SchemaVersion)
	}
}

// Конверт: тот же файл плюс предупреждения в одном JSON-теле.
func TestBackupExportEnvelope(t *testing.T) {
	ff := &fakeFacade{stateValue: backupTestState()}
	base, _ := newTestServer(t, ff)
	plain, _ := fetchBackup(t, base, "?format=1.0")

	var env struct {
		Format   string              `json:"format"`
		FileName string              `json:"file_name"`
		File     json.RawMessage     `json:"file"`
		Warnings []backupWarningView `json:"warnings"`
	}
	if status, raw := doJSON(t, authedReq(t, "GET", base+"/backup/export?format=1.0&envelope=1", nil), &env); status != 200 {
		t.Fatalf("envelope status %d: %s", status, raw)
	}
	if env.Format != "1.0" || env.FileName == "" {
		t.Errorf("конверт без формата или имени файла: %+v", env)
	}
	// Файл в конверте и файл в теле — одно и то же содержимое. Сравнение по
	// разобранному JSON, а не по байтам: конверт переносит документ, а не
	// форматирование (отступы внутри него задаёт writeJSON).
	var a, b any
	if err := json.Unmarshal(env.File, &a); err != nil {
		t.Fatalf("file в конверте не JSON: %v", err)
	}
	if err := json.Unmarshal(plain, &b); err != nil {
		t.Fatalf("тело без конверта не JSON: %v", err)
	}
	if !jsonEqualAPI(a, b) {
		t.Error("файл в конверте отличается от файла в теле")
	}
}

func jsonEqualAPI(a, b any) bool {
	ra, err1 := json.Marshal(a)
	rb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ra) == string(rb)
}

// Отказы: неизвестный формат — про запрос, не про сервер; пустое тело импорта
// не должно молча «применяться».
func TestBackupEndpointsRejectBadInput(t *testing.T) {
	ff := &fakeFacade{stateValue: backupTestState()}
	base, _ := newTestServer(t, ff)

	if status, raw := doJSON(t, authedReq(t, "GET", base+"/backup/export?format=2.0", nil), nil); status != 400 {
		t.Errorf("неизвестный формат дал %d: %s", status, raw)
	}
	if status, _ := doJSON(t, authedReq(t, "POST", base+"/backup/import", []byte("  ")), nil); status != 400 {
		t.Errorf("пустое тело импорта дало %d, ожидался 400", status)
	}
	if status, _ := doJSON(t, authedReq(t, "POST", base+"/backup/import", []byte(`{"hello":1}`)), nil); status != 400 {
		t.Errorf("чужой JSON дал %d, ожидался 400", status)
	}
	if status, _ := doJSON(t, authedReq(t, "POST", base+"/backup/export", nil), nil); status != 405 {
		t.Errorf("POST на экспорт дал %d, ожидался 405", status)
	}
	if status, _ := doJSON(t, authedReq(t, "GET", base+"/backup/import", nil), nil); status != 405 {
		t.Errorf("GET на импорт дал %d, ожидался 405", status)
	}
	// Импорт не должен был тронуть состояние ни в одном из отказов.
	if ff.savedState != nil || ff.rebuilds != 0 {
		t.Error("отказ импорта всё же записал состояние или пересобрал конфиг")
	}
}

// Маршрут не может быть заведён мимо /help (SPEC 078): реестр — единственный
// источник истины и для роутера, и для документации.
func TestBackupEndpointsDocumentedInHelp(t *testing.T) {
	base, _ := newTestServer(t, &fakeFacade{})
	var help struct {
		Endpoints []endpointView `json:"endpoints"`
	}
	if status, raw := doJSON(t, authedReq(t, "GET", base+"/help", nil), &help); status != 200 {
		t.Fatalf("help status %d: %s", status, raw)
	}
	want := map[string]bool{
		"/backup/export":  false,
		"/backup/import":  false,
		"/backup/formats": false,
	}
	for _, e := range help.Endpoints {
		if _, ok := want[e.Path]; ok {
			want[e.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("%s не описан в /help", path)
		}
	}
}
