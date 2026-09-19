package debugapi

// SPEC 127 W2.9 — перенос настроек через debug API.
//
// Проверяется свойство, ради которого ручки и заведены: снимок, снятый
// снаружи, ВОССТАНАВЛИВАЕТ настройку на другой машине. Поэтому центральный
// тест — круг export → import в пустое состояние → export, и сравнение по
// БАЙТАМ: равенство отдельных полей пропустило бы ровно то, что теряется
// на переносе чаще всего, — поле, о котором забыли и в проверке.
//
// Из сравнения вынуто ровно два поля, и оба — не про содержимое переноса:
// номера оси (импорт вправе их переписать, `stripAPIAxisNums`) и `exported_at`
// (`stripAPIExportedAt`). Сравниваются РАЗНЫЕ экспорты, а отметка времени
// берётся часами в момент каждого из них.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/backup"
	"singbox-launcher/core/build"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
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
	if stripAPIExportedAt(stripAPIAxisNums(string(second))) != stripAPIExportedAt(stripAPIAxisNums(string(first))) {
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
	if stripAPIExportedAt(string(third)) != stripAPIExportedAt(string(second)) {
		t.Fatalf("круг не байт-идентичен:\n--- до ---\n%s\n--- после ---\n%s", second, third)
	}
}

// stripAPIExportedAt — `exported_at` вне сравнения кругов.
//
// Поле штампуется временем ЭКСПОРТА (`now.UTC()` в `backup.Export10`), а круг
// сравнивает два РАЗНЫХ экспорта. Совпадали они только пока оба укладывались
// в одну секунду: стоило второму перешагнуть границу секунды — и байтовое
// равенство ломалось на ровном месте. Инвариант круга про содержимое бэкапа,
// а не про часы, поэтому отметка времени снимается так же, как номера оси.
func stripAPIExportedAt(doc string) string {
	const key = `"exported_at": "`
	i := strings.Index(doc, key)
	if i < 0 {
		return doc
	}
	j := strings.IndexByte(doc[i+len(key):], '"')
	if j < 0 {
		return doc
	}
	return doc[:i+len(key)] + "<TS>" + doc[i+len(key)+j:]
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

// Писатель один — 1.0 (D-110): /backup/formats говорит ровно это, а экспорт
// без ?format и с ?format=1.0 отдаёт один и тот же файл формата 1.0.
func TestBackupFormatsWriteOnly10(t *testing.T) {
	ff := &fakeFacade{stateValue: backupTestState()}
	base, _ := newTestServer(t, ff)

	var formats struct {
		Reads   []int    `json:"reads"`
		Writes  []string `json:"writes"`
		Default string   `json:"default"`
	}
	if status, raw := doJSON(t, authedReq(t, "GET", base+"/backup/formats", nil), &formats); status != 200 {
		t.Fatalf("formats status %d: %s", status, raw)
	}
	// Читаются оба маркера (файлы 0.x у пользователей на руках), пишется один.
	if len(formats.Reads) != 2 || formats.Reads[0] != 1 || formats.Reads[1] != 2 {
		t.Errorf("reads = %v, ожидалось [1 2]", formats.Reads)
	}
	if len(formats.Writes) != 1 || formats.Writes[0] != "1.0" || formats.Default != "1.0" {
		t.Errorf("writes/default = %v/%q, ожидалось [1.0]/1.0", formats.Writes, formats.Default)
	}

	implicit, resp := fetchBackup(t, base, "")
	if resp.StatusCode != 200 {
		t.Fatalf("export без ?format: status %d: %s", resp.StatusCode, implicit)
	}
	var head struct {
		LxBackup int `json:"lx_backup"`
	}
	if err := json.Unmarshal(implicit, &head); err != nil || head.LxBackup != 2 {
		t.Fatalf("экспорт без ?format отдал не файл 1.0: %v lx_backup=%d", err, head.LxBackup)
	}
	explicit, _ := fetchBackup(t, base, "?format=1.0")
	// Это тоже два разных экспорта — `exported_at` у них вправе разойтись на
	// границе секунды, а сравниваем мы формат файла, а не часы.
	if stripAPIExportedAt(string(implicit)) != stripAPIExportedAt(string(explicit)) {
		t.Errorf("файл без ?format не совпал с файлом ?format=1.0")
	}
}

// Файл 0.12 читается тем же POST: импорт не спрашивает формат и не заставляет
// вызывающего его знать. Писателя 0.12 у сборки нет (D-110), поэтому файл —
// тот, что снимали прежние релизы лаунчера.
func TestBackupImportAcceptsLegacyFormat(t *testing.T) {
	legacy := []byte(`{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.6", "platform": "darwin"},
  "exported_at": "2026-09-10T00:00:00Z",
  "subscriptions": [{"id": "01SUB0000000000000000000", "url": "https://example-1.com/sub", "label": "Main"}],
  "servers": [{"id": "01SRV0000000000000000000", "node_tag": "ts-node", "uri": "trojan://pw@1.2.3.4:443#ts-node"}],
  "rules": [{"kind": "inline", "name": "Work", "num": 1000, "outbound": "direct",
             "match": {"domain_suffix": ["example.com"]}}]
}`)

	dst := &fakeFacade{stateValue: state.New()}
	dstBase, _ := newTestServer(t, dst)
	var out struct {
		OK      bool   `json:"ok"`
		Format  string `json:"format"`
		Applied struct {
			Sources int `json:"sources"`
			Rules   int `json:"rules"`
		} `json:"applied"`
	}
	if status, raw := doJSON(t, authedReq(t, "POST", dstBase+"/backup/import", legacy), &out); status != 200 {
		t.Fatalf("import 0.12 status %d: %s", status, raw)
	}
	if !out.OK || out.Format != "0.12" {
		t.Errorf("ответ импорта: ok=%v format=%q, ожидался 0.12", out.OK, out.Format)
	}
	if out.Applied.Sources != 2 || out.Applied.Rules != 1 {
		t.Errorf("применено источников %d, правил %d — ожидалось 2 и 1", out.Applied.Sources, out.Applied.Rules)
	}
	if dst.savedState == nil || len(dst.savedState.Sources) != 2 {
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
	// 0.12 — не «неизвестный», а снятый с записи формат (D-110): скрипт,
	// написанный под окно двух писателей, обязан узнать именно это — и что
	// импорт такие файлы по-прежнему читает.
	var rejected struct {
		Error string `json:"error"`
	}
	if status, raw := doJSON(t, authedReq(t, "GET", base+"/backup/export?format=0.12", nil), &rejected); status != 400 {
		t.Errorf("?format=0.12 дал %d, ожидался 400: %s", status, raw)
	}
	if rejected.Error != "format 0.12 is no longer written; import still reads it" {
		t.Errorf("?format=0.12: текст отказа %q", rejected.Error)
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

// Направления уезжают в файл телом ПОСЛЕ слияния с шаблоном и пресетом.
//
// Дефект, пойманный на живом лаунчере 15.09.2026: ссылочная запись state —
// это tag+ref+updates, писатель брал её как есть, и в файл уезжал один тег.
// На другой машине proxy-out приезжал без фильтра !RU и собирал в себя
// российские узлы; так же терялись фильтр vpn ② из USER-патча и тело
// Направления пресета. Путь проверяется целиком, через API: слитый вид
// (/state/outbounds/resolved) → файл → импорт в тот же лаунчер (ничего не
// меняет) и в чистый с тем же шаблоном (без дублей, состав тот же).
func TestBackupExportCarriesMergedDirections(t *testing.T) {
	// Тег блокировки в шаблоне НЕ по умолчанию: include_block обязан
	// ставиться по нему — по тому же тегу, что у галки формы Направления.
	const blockTag = "reject-out"
	td := &template.TemplateData{
		ParserConfig: `{"ParserConfig":{"outbounds":[
			{"tag":"proxy-out","type":"selector","options":{"interrupt_exist_connections":true},
			 "addOutbounds":["direct-out"],
			 "auto":{"url":"@urltest_url","interval":"@urltest_interval"}},
			{"tag":"vpn ②","type":"selector","options":{"default":"proxy-out"},
			 "addOutbounds":["direct-out","proxy-out"]}
		]}}`,
		Presets: []template.Preset{{
			ID: "ru-direct",
			Outbounds: []template.PresetOutbound{
				{Mode: "update", Tag: "proxy-out", Filters: map[string]interface{}{"tag": "!/(🇷🇺)/i"}},
				{Mode: "add", Tag: "ru VPN 🇷🇺", Type: "selector",
					Filters:      map[string]interface{}{"tag": "/(🇷🇺)/i"},
					AddOutbounds: []string{"direct-out"}},
			},
		}},
		RawTemplate: json.RawMessage(`{"group_templates":{"magic_nodes":{"block":{"source":"preset","tag":"` + blockTag + `"}}}}`),
	}
	target := template.LocalTarget()

	rule := presetRule("ru-direct", nil, true)
	num := state.UserRuleNumStart
	rule.Num = &num
	st := state.New()
	st.Rules = []state.Rule{rule}
	st.Directions = []configtypes.Direction{
		{Tag: "proxy-out", Ref: configtypes.RefTemplate},
		{Tag: "vpn ②", Ref: configtypes.RefTemplate, Updates: []configtypes.OutboundUpdate{{
			Ref: configtypes.RefUser,
			Patch: map[string]interface{}{
				"filters":      map[string]interface{}{"tag": "!/(🔥|Proton)/i"},
				"addOutbounds": []interface{}{"direct-out", "proxy-out", blockTag},
			},
		}}},
		{Tag: "local-net", Type: "selector",
			Filters:      map[string]interface{}{"tag": "/(LAN)/i"},
			AddOutbounds: []string{"direct-out"}},
	}
	// Синхронизация Save визарда: докладывает патч пресета в proxy-out и
	// заводит тонкую запись пресета — ровно форма живого state.
	build.SyncOutboundsWithTemplate(st.Rules, &st.Directions, td.Presets, build.TemplateOutboundTags(td), target)
	for _, d := range st.Directions {
		if d.Ref != "" && (d.Filters != nil || d.AddOutbounds != nil) {
			t.Fatalf("предусловие: ссылочная запись %q хранит тело: %+v", d.Tag, d)
		}
	}

	src := &fakeFacade{stateValue: st, templateValue: td}
	srcBase, _ := newTestServer(t, src)

	// Ожидание — слитый вид, записанный литералом: отбор и опции селектора.
	want := []struct {
		tag     string
		filter  string
		invert  bool
		options []string
	}{
		{"proxy-out", "(🇷🇺)", true, []string{"direct-out"}},
		{"vpn ②", "(🔥|Proton)", true, []string{"direct-out", "proxy-out", blockTag}},
		{"local-net", "(LAN)", false, []string{"direct-out"}},
		{"ru VPN 🇷🇺", "(🇷🇺)", false, []string{"direct-out"}},
	}
	merged := resolvedDirectionsOverAPI(t, srcBase)
	for _, w := range want {
		m, ok := merged[w.tag]
		body, invert := configtypes.DirectionFilterTag(m.Filters)
		if !ok || body != w.filter || invert != w.invert || !reflect.DeepEqual(m.AddOutbounds, w.options) {
			t.Fatalf("предусловие: слитый вид %q не тот: %+v", w.tag, m)
		}
	}

	raw, resp := fetchBackup(t, srcBase, "?format=1.0")
	if resp.StatusCode != 200 {
		t.Fatalf("export status %d: %s", resp.StatusCode, raw)
	}
	file, _, err := backup.Parse(raw)
	if err != nil || file.V10 == nil {
		t.Fatalf("экспорт отдал не файл 1.0: %v\n%s", err, raw)
	}
	if len(file.V10.Directions) != len(want) {
		t.Fatalf("Направлений в файле %d, ожидалось %d:\n%s", len(file.V10.Directions), len(want), raw)
	}
	for i, w := range want {
		got := file.V10.Directions[i]
		var include []string
		for _, tag := range w.options {
			if tag != "direct-out" && tag != blockTag {
				include = append(include, tag)
			}
		}
		if got.Tag != w.tag || got.Filter != w.filter || got.Invert != w.invert ||
			got.IncludeDirect != containsTag(w.options, "direct-out") ||
			got.IncludeBlock != containsTag(w.options, blockTag) ||
			!reflect.DeepEqual(got.Include, include) {
			t.Errorf("Направление %q уехало не слитым телом: %+v", w.tag, got)
		}
	}
	// Двойник и опции селектора ссылочной записи — тоже тело шаблона.
	if proxy := file.V10.Directions[0]; proxy.Auto == nil || proxy.InterruptExistConnections == nil {
		t.Errorf("proxy-out уехал без автовыбора или опций шаблона: %+v", proxy)
	}

	// Импорт в тот же лаунчер: все теги заняты (§9) — ничего не применяется,
	// и состояние остаётся байт в байт прежним.
	before, err := st.MarshalV8()
	if err != nil {
		t.Fatal(err)
	}
	var same struct {
		Applied struct {
			Directions int `json:"directions"`
		} `json:"applied"`
		Warnings []backupWarningView `json:"warnings"`
	}
	if status, body := doJSON(t, authedReq(t, "POST", srcBase+"/backup/import", raw), &same); status != 200 {
		t.Fatalf("импорт в тот же лаунчер: status %d: %s", status, body)
	}
	exists := 0
	for _, w := range same.Warnings {
		if w.Code == backup.WarnBackupDirectionExists {
			exists++
		}
	}
	if same.Applied.Directions != 0 || exists != len(want) {
		t.Errorf("импорт в тот же лаунчер: применено %d, backup_direction_exists %d из %d",
			same.Applied.Directions, exists, len(want))
	}
	after, err := src.savedState.MarshalV8()
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("импорт своего же файла изменил состояние:\n--- до ---\n%s\n--- после ---\n%s", before, after)
	}

	// Чистый лаунчер с тем же шаблоном. После импорта — тот же проход, что у
	// загрузки визарда (миграция в ссылочную форму + синхронизация): дубль
	// появился бы именно там, если бы прямая запись не узналась шаблоном.
	dst := &fakeFacade{stateValue: state.New(), templateValue: td}
	dstBase, _ := newTestServer(t, dst)
	if status, body := doJSON(t, authedReq(t, "POST", dstBase+"/backup/import", raw), nil); status != 200 {
		t.Fatalf("импорт в чистый лаунчер: status %d: %s", status, body)
	}
	got := dst.savedState
	build.MigrateOutboundsToReferencedShape(&got.Directions, got.Rules, td, target)
	build.SyncOutboundsWithTemplate(got.Rules, &got.Directions, td.Presets, build.TemplateOutboundTags(td), target)
	seen := map[string]int{}
	for _, d := range got.Directions {
		seen[d.Tag]++
	}
	if len(got.Directions) != len(want) {
		t.Errorf("после импорта в чистый лаунчер Направлений %d, ожидалось %d: %v", len(got.Directions), len(want), seen)
	}
	back := resolvedDirectionsOverAPI(t, dstBase)
	for _, w := range want {
		if seen[w.tag] != 1 {
			t.Errorf("Направление %q после импорта встречается %d раз", w.tag, seen[w.tag])
		}
		m := back[w.tag]
		body, invert := configtypes.DirectionFilterTag(m.Filters)
		// Тег блокировки импорт пишет своим именем (importDirection), а не
		// шаблонным, — это вход, и в эту сверку он не входит.
		if body != w.filter || invert != w.invert ||
			!reflect.DeepEqual(sortedOptions(m.AddOutbounds, blockTag, "block-out"), sortedOptions(w.options, blockTag)) {
			t.Errorf("Направление %q на чистом лаунчере собирается не так: %+v", w.tag, m)
		}
	}
}

// resolvedDirectionsOverAPI — слитый вид Направлений по тегу.
func resolvedDirectionsOverAPI(t *testing.T, base string) map[string]configtypes.Direction {
	t.Helper()
	var res struct {
		Outbounds []configtypes.Direction `json:"outbounds"`
	}
	if status, raw := doJSON(t, authedReq(t, "GET", base+"/state/outbounds/resolved", nil), &res); status != 200 {
		t.Fatalf("resolved status %d: %s", status, raw)
	}
	out := make(map[string]configtypes.Direction, len(res.Outbounds))
	for _, d := range res.Outbounds {
		out[d.Tag] = d
	}
	return out
}

func containsTag(list []string, tag string) bool {
	for _, x := range list {
		if x == tag {
			return true
		}
	}
	return false
}

// sortedOptions — опции селектора множеством, без перечисленных тегов.
func sortedOptions(list []string, drop ...string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if !containsTag(drop, x) {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
