package business

import (
	"encoding/json"
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Запись хранения папки («Copy JSON» строки источника) принимается полем Add:
// в корне становится НОВОЙ папкой со свежим ULID, внутренние ссылки едут на
// новый ULID, внешние (на Направления) остаются; внутри папки высыпается
// узлами. Проверяются данные, а не подписи (no-ui-format-tests).

const pastedFolderRecord = `{
  "kind": "folder",
  "tag": "",
  "enabled": true,
  "detour": {"tag": "WARP"},
  "id": "01OLDFOLDERID",
  "name": "Proton",
  "nodes": [
    {
      "kind": "server",
      "tag": "US",
      "enabled": true,
      "origin": {"kind": "uri", "raw": "` + fillTestURI + `#US"},
      "body": {"type": "vless", "server": "1.2.3.4", "server_port": 443, "uuid": "11111111-1111-1111-1111-111111111111"},
      "detour": {"tag": "WARP h3"}
    },
    {
      "kind": "server",
      "tag": "NL",
      "enabled": false,
      "body": {"type": "vless", "server": "5.6.7.8", "server_port": 443, "uuid": "11111111-1111-1111-1111-111111111111"},
      "detour": {"folder_id": "01OLDFOLDERID", "tag": "US"}
    }
  ],
  "replace": {"mode": "auto", "tag": "Proton best"}
}`

func TestPasteFolderRecord_RootBecomesNewFolder(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		moveTestFolder("01OLDFOLDERID", "Proton"),
	}}
	ctx := stubStaleUIUpdater{model: m}

	res, err := AppendURLsToSources(ctx, pastedFolderRecord)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if res.Folders != 1 || res.Added() != 1 {
		t.Fatalf("итог %+v, ожидалась одна папка", res)
	}
	if len(m.Sources) != 2 {
		t.Fatalf("источников %d, ожидалось 2", len(m.Sources))
	}
	f := m.Sources[1]
	if f.Kind != corestate.SourceKindFolder || f.Name != "Proton" {
		t.Fatalf("вторая запись не папка Proton: %+v", f.Node)
	}
	if strings.TrimSpace(f.ID) == "" || f.ID == "01OLDFOLDERID" {
		t.Fatalf("ULID папки не обновлён: %q", f.ID)
	}
	if f.Detour == nil || f.Detour.Tag != "WARP" || f.Detour.FolderID != "" {
		t.Fatalf("общий detour папки потерян: %+v", f.Detour)
	}
	if f.Replace == nil || f.Replace.Tag != "Proton best" {
		t.Fatalf("replace потерян: %+v", f.Replace)
	}
	if len(f.Nodes) != 2 {
		t.Fatalf("узлов %d, ожидалось 2", len(f.Nodes))
	}
	if f.Nodes[1].Enabled {
		t.Fatal("личный enabled узла NL потерян")
	}
	// Ссылка на соседа по папке переехала на новый ULID; на Направление — нет.
	if d := f.Nodes[1].Detour; d == nil || d.FolderID != f.ID || d.Tag != "US" {
		t.Fatalf("внутренняя ссылка не переписана: %+v (папка %s)", d, f.ID)
	}
	if d := f.Nodes[0].Detour; d == nil || d.FolderID != "" || d.Tag != "WARP h3" {
		t.Fatalf("внешняя ссылка испорчена: %+v", d)
	}
	var body map[string]any
	if err := json.Unmarshal(f.Nodes[0].Body, &body); err != nil || body["type"] != "vless" {
		t.Fatalf("тело узла не сохранено: %s", f.Nodes[0].Body)
	}
}

func TestPasteFolderRecord_IntoFolderFlattensNodes(t *testing.T) {
	m := fillTestModel(corestate.Node{Kind: corestate.SourceKindServer, Tag: "US", Enabled: true})

	res, err := AppendNodesToFolder(m, "01FOLDER", pastedFolderRecord, "")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if res.Added != 2 {
		t.Fatalf("добавлено %d, ожидалось 2", res.Added)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("в корне появились источники: %d", len(m.Sources))
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 3 {
		t.Fatalf("узлов %d, ожидалось 3", len(folder.Nodes))
	}
	// Тег US занят — приехавший уникализирован; ссылка соседа переехала на
	// ULID папки-адресата и на уникализированный тег.
	if folder.Nodes[1].Tag != "US-2" {
		t.Fatalf("тег %q, ожидался US-2", folder.Nodes[1].Tag)
	}
	if d := folder.Nodes[2].Detour; d == nil || d.FolderID != "01FOLDER" {
		t.Fatalf("внутренняя ссылка не переехала в папку-адресат: %+v", d)
	}
}

// Запись подписки в корне — новая подписка с дедупом по URL.
func TestPasteSubscriptionRecord_DedupByURL(t *testing.T) {
	rec := `{"kind":"subscription","tag":"","enabled":true,"id":"01SUB","url":"https://example.com/sub","tag_policy":{"prefix":"p:"}}`
	m := &wizardmodels.WizardModel{}
	ctx := stubStaleUIUpdater{model: m}

	res, err := AppendURLsToSources(ctx, rec)
	if err != nil || res.Subscriptions != 1 || len(m.Sources) != 1 {
		t.Fatalf("первая вставка: res=%+v err=%v sources=%d", res, err, len(m.Sources))
	}
	if m.Sources[0].ID == "01SUB" || m.Sources[0].TagPolicy == nil || m.Sources[0].TagPolicy.Prefix != "p:" {
		t.Fatalf("подписка легла неверно: %+v", m.Sources[0])
	}
	res, err = AppendURLsToSources(ctx, rec)
	if err != nil || res.Added() != 0 || res.Duplicates != 1 || len(m.Sources) != 1 {
		t.Fatalf("повтор: res=%+v err=%v sources=%d", res, err, len(m.Sources))
	}
}

// Одиночный sing-box outbound (с `type`) записью НЕ считается и идёт прежним путём.
func TestPasteRecord_SingboxOutboundUntouched(t *testing.T) {
	records, isRecord, err := carveSourceRecords(`{"type":"vless","tag":"x","server":"1.2.3.4","server_port":443,"uuid":"11111111-1111-1111-1111-111111111111"}`)
	if err != nil || isRecord || records != nil {
		t.Fatalf("outbound принят за запись: rec=%v isRecord=%v err=%v", records, isRecord, err)
	}
}
