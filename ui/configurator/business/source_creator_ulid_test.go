package business

// SPEC 117 §5.B / риск Р3 — аудит создателей Source: обратного синка Save,
// который раньше доминтовывал пустые ID, больше нет, поэтому ULID обязан
// рождаться в момент создания источника. Создатели на этом слое:
// AppendURLsToSources (подписка / server-URI / sing-box JSON) и
// AppendManualConfigJSON. Создатель цепочки (source_tab.go addChainAction) и
// импорт бэкапа (core/backup) минтят там же — второй покрыт
// core/backup/import_ulid_test.go, первый живёт в Fyne-обработчике и
// проверяется тем, что литерал создания содержит MakeULID.

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// assertFreshULIDs — у каждого добавленного источника ULID (26 знаков),
// уникальный в пределах модели.
func assertFreshULIDs(t *testing.T, model *wizardmodels.WizardModel, from int) {
	t.Helper()
	seen := map[string]bool{}
	for _, src := range model.Sources {
		if seen[src.ID] {
			t.Errorf("duplicate ULID %q", src.ID)
		}
		seen[src.ID] = true
	}
	for i := from; i < len(model.Sources); i++ {
		if len(model.Sources[i].ID) != 26 {
			t.Errorf("source %d (%s) created without ULID: id=%q",
				i, model.Sources[i].Type, model.Sources[i].ID)
		}
	}
}

func TestAppendURLsToSources_IssuesULIDs(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	ctx := stubStaleUIUpdater{model: model}

	input := "https://example.invalid/sub\n" +
		"vless://uuid@host.example:443?security=tls#node-a\n"
	if err := AppendURLsToSources(ctx, input); err != nil {
		t.Fatalf("AppendURLsToSources: %v", err)
	}
	if len(model.Sources) != 2 {
		t.Fatalf("sources = %d, want 2 (subscription + server)", len(model.Sources))
	}
	assertFreshULIDs(t, model, 0)
}

func TestAppendURLsToSources_JSONNodes_IssueULIDs(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	ctx := stubStaleUIUpdater{model: model}

	input := `{"outbounds":[{"type":"vless","tag":"js-node","server":"1.2.3.4","server_port":443,"uuid":"u"}]}`
	if err := AppendURLsToSources(ctx, input); err != nil {
		t.Fatalf("AppendURLsToSources(json): %v", err)
	}
	if len(model.Sources) == 0 {
		t.Fatal("no sources added from sing-box JSON")
	}
	assertFreshULIDs(t, model, 0)
}

func TestAppendManualConfigJSON_IssuesULID(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	ctx := stubStaleUIUpdater{model: model}

	body := []byte(`{"type":"someproto","tag":"manual","server":"10.0.0.1","server_port":443}`)
	if err := AppendManualConfigJSON(ctx, body, "manual"); err != nil {
		t.Fatalf("AppendManualConfigJSON: %v", err)
	}
	if len(model.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(model.Sources))
	}
	assertFreshULIDs(t, model, 0)
}
