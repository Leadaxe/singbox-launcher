package tabs

// Поведенческие тесты пути Save окна источника (SPEC 118 W3, фиксы
// адверсариального ревью, блокеры 1 и 2): тумблер узла в окне пишет ОБА
// представления включённости и переживает fetch; Save переносит runtime-поля
// fetch'а из живой записи модели и применяет оконные правки журналом, а не
// слепым снимком.
//
// Тестируется реальный UI-путь данных: cloneSource (открытие окна) →
// setNodeEnabled (тумблер) → mergeEditedSourceIntoModel (Save) →
// state.MergeSubscriptionNodes (fetch). Presenter/Fyne в data-части не
// участвуют — поэтому она и вынесена из applySourceEditToModel.

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func applyTestNode(tag string) wizardmodels.Node {
	return wizardmodels.Node{
		Kind:    wizardmodels.SourceKindServer,
		Tag:     tag,
		Enabled: true,
		Body:    json.RawMessage(`{"type":"vless"}`),
	}
}

func applyTestModel(nodes ...wizardmodels.Node) *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{Sources: []wizardmodels.Source{{
		Node:  wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
		ID:    "01APPLYTESTSUB000000000000",
		Name:  "sub",
		URL:   "https://example.invalid/sub",
		Nodes: nodes,
	}}}
}

// Блокер 1 — выключение узла в окне переживает Save и последующий fetch;
// включение обратно — тоже. До фикса тумблер писал только легаси-карту,
// merge читал канон, и выключение молча откатывалось первым же fetch'ем.
func TestWindowToggleSurvivesSaveAndFetch(t *testing.T) {
	m := applyTestModel(applyTestNode("A"), applyTestNode("B"))

	// Окно: рабочая копия + журнал; тумблер B → off.
	scratch := cloneSource(&m.Sources[0])
	enabledEdits := map[string]bool{}
	setNodeEnabled(&scratch, "B", false)
	enabledEdits["B"] = false
	if scratch.Nodes[1].Enabled {
		t.Fatal("setNodeEnabled не опустил канонический enabled в рабочей копии")
	}
	if _, off := scratch.DisabledNodes["B"]; !off {
		t.Fatal("setNodeEnabled не записал мостовую карту")
	}

	// Save.
	mergeEditedSourceIntoModel(m, 0, &scratch, enabledEdits)
	live := &m.Sources[0]
	if live.Nodes[1].Enabled {
		t.Fatal("Save потерял выключение узла")
	}

	// Fetch: провайдер прислал то же тело.
	freshBody := func() *state.SubFetchMaterial {
		return &state.SubFetchMaterial{Nodes: []state.Node{applyTestNode("A"), applyTestNode("B")}}
	}
	state.MergeSubscriptionNodes(live, freshBody(), true)
	if live.Nodes[1].Enabled {
		t.Fatal("выключение узла откатилось fetch'ем")
	}
	if _, off := live.DisabledNodes["B"]; !off {
		t.Fatalf("мостовая карта рассинхронизирована: %v", live.DisabledNodes)
	}

	// Включение обратно тем же UI-путём — тоже переживает fetch.
	scratch2 := cloneSource(live)
	setNodeEnabled(&scratch2, "B", true)
	mergeEditedSourceIntoModel(m, 0, &scratch2, map[string]bool{"B": true})
	live = &m.Sources[0]
	if !live.Nodes[1].Enabled {
		t.Fatal("Save потерял включение узла")
	}
	state.MergeSubscriptionNodes(live, freshBody(), true)
	if !live.Nodes[1].Enabled {
		t.Fatal("включение узла откатилось fetch'ем")
	}
	if _, off := live.DisabledNodes["B"]; off {
		t.Fatalf("мостовая карта держит снятую отметку: %v", live.DisabledNodes)
	}
}

// Блокер 2, сценарий «Fetch now в окне → Save»: runtime-поля fetch'а
// (nodes[], updateStatus, pending_disabled), появившиеся в живой записи
// после открытия окна, не затираются снимком открытия; оконные правки
// обычных полей при этом живут.
func TestSaveKeepsRuntimeFieldsFromLiveRecord(t *testing.T) {
	m := applyTestModel() // на момент открытия окна узлов нет
	scratch := cloneSource(&m.Sources[0])
	scratch.Name = "renamed" // обычная оконная правка

	// Пока окно открыто: one-shot «Fetch now» наполнил живую запись.
	m.Sources[0].Nodes = []wizardmodels.Node{applyTestNode("A")}
	m.Sources[0].UpdateStatus = &wizardmodels.SubUpdateStatus{LastStatus: "ok"}
	m.Sources[0].PendingDisabled = []string{"beyond-cap"}

	mergeEditedSourceIntoModel(m, 0, &scratch, nil)
	live := &m.Sources[0]
	if live.Name != "renamed" {
		t.Fatal("оконная правка потеряна на Save")
	}
	if len(live.Nodes) != 1 || live.Nodes[0].Tag != "A" {
		t.Fatalf("Save затёр свежие nodes[] снимком открытия: %+v", live.Nodes)
	}
	if live.UpdateStatus == nil || live.UpdateStatus.LastStatus != "ok" {
		t.Fatalf("Save затёр updateStatus: %+v", live.UpdateStatus)
	}
	if len(live.PendingDisabled) != 1 || live.PendingDisabled[0] != "beyond-cap" {
		t.Fatalf("Save затёр pending_disabled: %v", live.PendingDisabled)
	}
}

// Блокер 2, сценарий «фоновый fetch во время окна»: свежие узлы живой
// записи выживают, И оконный тумблер применяется поверх них — по журналу
// правок, а не слепым снимком карты на момент открытия.
func TestSaveAppliesWindowTogglesOverFreshFetch(t *testing.T) {
	m := applyTestModel(applyTestNode("A"), applyTestNode("B"))
	scratch := cloneSource(&m.Sources[0])
	enabledEdits := map[string]bool{}
	setNodeEnabled(&scratch, "B", false)
	enabledEdits["B"] = false

	// Фоновый fetch, пока окно открыто: свежие тела + новый узел C.
	m.Sources[0].Nodes = []wizardmodels.Node{
		applyTestNode("A"), applyTestNode("B"), applyTestNode("C"),
	}

	mergeEditedSourceIntoModel(m, 0, &scratch, enabledEdits)
	live := &m.Sources[0]
	if len(live.Nodes) != 3 {
		t.Fatalf("свежие nodes[] фонового fetch'а потеряны: %+v", live.Nodes)
	}
	if live.Nodes[1].Enabled {
		t.Fatal("оконный тумблер не применён поверх свежего fetch'а")
	}
	if !live.Nodes[0].Enabled || !live.Nodes[2].Enabled {
		t.Fatal("тумблер зацепил чужие узлы")
	}
	if _, off := live.DisabledNodes["B"]; !off {
		t.Fatalf("мостовая карта не отражает тумблер: %v", live.DisabledNodes)
	}
}
