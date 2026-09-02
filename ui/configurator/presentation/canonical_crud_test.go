package presentation

// SPEC 117 §5.C — сценарии C1 (CRUD Направлений на canonical сквозь
// Save/Load) и C7 (update-конвейер читает Load-проекцию свежезагруженного
// state, куда UI сохранил canonical-путём).
//
// Операции повторяют ровно те canonical-мутации, которые выполняют
// обработчики outbounds_configurator (add / reorder / edit со сменой scope /
// toggle / delete): пакет мутирует model.GlobalOutbounds и
// model.Sources[i].Outbounds напрямую, Apply-шага не существует. Здесь
// проверяется контракт этих мутаций: canonical отражает каждую операцию
// немедленно, ревизия растёт, а CreateStateFromModel → Save → Load
// возвращает ровно этот состав (без обратных синков, снесённых в W4).
//
// Правило no-ui-format-tests соблюдено: ни одного ассерта на подписи,
// вёрстку или форматирование строк — только данные модели и состояния.

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// newCanonicalCRUDModel — модель с подпиской и server-URI (без шаблона:
// CreateStateFromModel с TemplateData == nil не трогает outbounds sync'ом,
// что изолирует проверку «Save пишет ровно canonical»).
func newCanonicalCRUDModel() *wizardmodels.WizardModel {
	m := wizardmodels.NewWizardModel()
	m.Sources = []wizardmodels.Source{
		{
			ID:    "01C1SUB00000000000000000",
			Node:  wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Proton NL",
			URL:   "https://example.com/sub",
		},
		{
			ID: "01C1SRV00000000000000000",
			Node: wizardmodels.Node{
				Kind: wizardmodels.SourceKindServer, Enabled: true, Tag: "warp-hop",
				Body:   json.RawMessage(`{"type":"vless","server":"host","server_port":443,"uuid":"uuid"}`),
				Origin: &wizardmodels.Origin{Kind: wizardmodels.OriginKindURI, Raw: "vless://uuid@host:443"},
			},
			Label: "WARP hop",
		},
	}
	return m
}

func globalTags(m *wizardmodels.WizardModel) []string {
	tags := make([]string, 0, len(m.GlobalOutbounds))
	for i := range m.GlobalOutbounds {
		tags = append(tags, m.GlobalOutbounds[i].Tag)
	}
	return tags
}

func TestDirectionsCRUD_CanonicalThroughSaveLoad(t *testing.T) {
	m := newCanonicalCRUDModel()
	p := NewWizardPresenter(m, &GUIState{}, nil)

	// --- add (global): как замыкание Add конфигуратора.
	rev := m.Revision
	m.GlobalOutbounds = append(m.GlobalOutbounds, configtypes.Direction{Tag: "vpn-1", Type: "selector"})
	m.BumpRevision()
	if m.Revision <= rev {
		t.Fatal("add: ревизия не выросла")
	}
	m.GlobalOutbounds = append(m.GlobalOutbounds, configtypes.Direction{Tag: "vpn-2", Type: "selector"})
	m.BumpRevision()

	// SPEC 118 W5: локальных Направлений источника нет — scope выбирать не
	// из чего, Направление всегда глобальное. Ветки local-add и смены scope
	// удалены вместе с предметом.

	// --- reorder (drag): vpn-2 наверх.
	outs := m.GlobalOutbounds
	moved := outs[1]
	rest := append(outs[:1:1], outs[2:]...)
	m.GlobalOutbounds = append([]configtypes.Direction{moved}, rest...)
	m.BumpRevision()
	if got := globalTags(m); got[0] != "vpn-2" || got[1] != "vpn-1" {
		t.Fatalf("reorder не отразился: %v", got)
	}

	// --- toggle: выключить vpn-1.
	for i := range m.GlobalOutbounds {
		if m.GlobalOutbounds[i].Tag == "vpn-1" {
			m.GlobalOutbounds[i].Disabled = true
		}
	}
	m.BumpRevision()

	// --- delete: убрать vpn-2.
	for i := range m.GlobalOutbounds {
		if m.GlobalOutbounds[i].Tag == "vpn-2" {
			m.GlobalOutbounds = append(m.GlobalOutbounds[:i], m.GlobalOutbounds[i+1:]...)
			break
		}
	}
	m.BumpRevision()

	wantGlobal := []string{"vpn-1"}
	if got := globalTags(m); len(got) != 1 || got[0] != wantGlobal[0] {
		t.Fatalf("canonical после CRUD = %v, ожидалось %v", got, wantGlobal)
	}

	// --- CreateStateFromModel → Save → Load: диск возвращает ровно canonical.
	sf := p.CreateStateFromModel("c1", "01C1STATE000000000000000")
	path := filepath.Join(t.TempDir(), "state.json")
	if err := sf.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := corestate.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Sources) != 2 {
		t.Fatalf("sources после roundtrip: %d, ожидалось 2", len(loaded.Sources))
	}
	for i := range m.Sources {
		if loaded.Sources[i].ID != m.Sources[i].ID {
			t.Errorf("source[%d].ID = %q, ожидался %q (ID обязан жить в canonical)", i, loaded.Sources[i].ID, m.Sources[i].ID)
		}
	}
	if len(loaded.Directions) != 1 || loaded.Directions[0].Tag != "vpn-1" {
		t.Fatalf("outbounds после roundtrip: %+v", loaded.Directions)
	}
	if !loaded.Directions[0].Disabled {
		t.Error("toggle (Disabled=true) потерян на Save/Load")
	}

	// --- C7: update-конвейер (loadParserConfigForUpdate → generate) читает
	// Load-проекцию state.ParserConfig свежезагруженного state. Гейт
	// «Proxies == nil → ошибка» не должен сработать, и проекция обязана
	// содержать ВСЕ источники, сохранённые UI canonical-путём.
	proxies := loaded.ParserConfig.ParserConfig.Proxies
	if proxies == nil {
		t.Fatal("Load-проекция пуста — гейт update-конвейера сработал бы на живом state")
	}
	if len(proxies) != len(m.Sources) {
		t.Fatalf("Load-проекция: %d proxies, ожидалось %d", len(proxies), len(m.Sources))
	}
	if proxies[0].Source != "https://example.com/sub" {
		t.Errorf("проекция подписки: Source = %q", proxies[0].Source)
	}
	// SPEC 118 Т5: узел едет в сборку КАНОНОМ (готовое тело), а не строкой
	// URI — конвейер сборки парсер тел не зовёт.
	if proxies[1].Canonical == nil || len(proxies[1].Canonical.Nodes) != 1 {
		t.Fatalf("проекция server-узла без канона: %+v", proxies[1].Canonical)
	}
	if got := proxies[1].Canonical.Nodes[0].Tag; got != "warp-hop" {
		t.Errorf("тег узла в проекции: %q", got)
	}
}
