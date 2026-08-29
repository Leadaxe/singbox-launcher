// File fetch_writeback_test.go — запись результата fetch'а не откатывает
// правки пользователя (SPEC 118 W6, хвост ревью W3).
//
// Ручное обновление подписки работает на снимке: снимок → горутина → запись
// обратно. Пока горутина летит, пользователь вправе править ту же запись, и
// запись снимка ЦЕЛИКОМ откатывала бы её правки к моменту старта fetch'а.
package business

import (
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func modelWithSub() *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			ID: "SUB", URL: "https://old.example/sub", Name: "старое имя"},
	}}
}

// Модель не трогали — снимок пишется целиком, как раньше.
func TestApplyFetchSnapshotWholeRecordWhenUntouched(t *testing.T) {
	m := modelWithSub()
	rev := m.Revision

	snap := m.Sources[0]
	snap.Nodes = []wizardmodels.Node{{Kind: wizardmodels.SourceKindServer, Tag: "n1", Enabled: true}}
	snap.UpdateStatus = &corestate.SubUpdateStatus{LastStatus: "ok"}

	if !ApplyFetchSnapshot(m, &snap, rev) {
		t.Fatal("запись не выполнена")
	}
	if len(m.Sources[0].Nodes) != 1 {
		t.Errorf("узлы fetch'а не доехали: %+v", m.Sources[0].Nodes)
	}
	if m.Sources[0].UpdateStatus == nil || m.Sources[0].UpdateStatus.LastStatus != "ok" {
		t.Error("диагностика fetch'а не доехала")
	}
}

// Модель ПРАВИЛИ, пока летел fetch: правка новее, и её обязаны сохранить, а
// результат fetch'а — занести поверх.
func TestApplyFetchSnapshotKeepsUserEditsMadeDuringFetch(t *testing.T) {
	m := modelWithSub()
	rev := m.Revision
	snap := m.Sources[0] // снимок ДО правок

	// Пользователь успел переименовать источник, сменить URL и тег-политику.
	m.Sources[0].Name = "новое имя"
	m.Sources[0].URL = "https://new.example/sub"
	m.Sources[0].TagPolicy = &corestate.TagPolicy{Prefix: "[NL] "}
	m.BumpRevision()

	// Горутина вернулась со свежим составом.
	snap.Nodes = []wizardmodels.Node{{Kind: wizardmodels.SourceKindServer, Tag: "n1", Enabled: true}}
	snap.UpdateStatus = &corestate.SubUpdateStatus{LastStatus: "ok"}

	if !ApplyFetchSnapshot(m, &snap, rev) {
		t.Fatal("запись не выполнена")
	}

	live := m.Sources[0]
	if live.Name != "новое имя" {
		t.Errorf("подпись откатилась к %q — правка пользователя потеряна", live.Name)
	}
	if live.URL != "https://new.example/sub" {
		t.Errorf("URL откатился к %q", live.URL)
	}
	if live.TagPolicy == nil || live.TagPolicy.Prefix != "[NL] " {
		t.Errorf("тег-политика откатилась: %+v", live.TagPolicy)
	}
	// Результат fetch'а при этом доехал: иначе обновление просто пропало бы.
	if len(live.Nodes) != 1 {
		t.Errorf("узлы fetch'а не доехали: %+v", live.Nodes)
	}
	if live.UpdateStatus == nil || live.UpdateStatus.LastStatus != "ok" {
		t.Error("диагностика fetch'а не доехала")
	}
}

// Источник удалили, пока летел fetch — воскрешать его записью снимка нельзя:
// удаление осознанно, а fetch про него ничего не знает.
func TestApplyFetchSnapshotDroppedSource(t *testing.T) {
	m := modelWithSub()
	rev := m.Revision
	snap := m.Sources[0]

	m.Sources = nil
	m.BumpRevision()

	if ApplyFetchSnapshot(m, &snap, rev) {
		t.Fatal("удалённый источник воскрешён записью снимка")
	}
	if len(m.Sources) != 0 {
		t.Errorf("источники = %d, ожидали 0", len(m.Sources))
	}
}

// Слайс мог реаллоцироваться (пользователь добавил источник) — запись ищет
// по ID, а не по индексу снимка, иначе попала бы в чужую запись.
func TestApplyFetchSnapshotFindsByIDAfterReorder(t *testing.T) {
	m := modelWithSub()
	rev := m.Revision
	snap := m.Sources[0]

	// Новый источник встал ПЕРЕД старым: индекс 0 теперь чужой.
	m.Sources = append([]wizardmodels.Source{
		{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Tag: "fresh"}, ID: "NEW"},
	}, m.Sources...)
	m.BumpRevision()

	snap.Nodes = []wizardmodels.Node{{Kind: wizardmodels.SourceKindServer, Tag: "n1", Enabled: true}}
	if !ApplyFetchSnapshot(m, &snap, rev) {
		t.Fatal("запись не выполнена")
	}
	if len(m.Sources[0].Nodes) != 0 {
		t.Error("результат fetch'а уехал в чужую запись")
	}
	if len(m.Sources[1].Nodes) != 1 {
		t.Errorf("результат fetch'а не нашёл свою запись: %+v", m.Sources[1].Nodes)
	}
}
