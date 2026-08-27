package tabs

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 113-E: правка тега узла БУФЕРИЗУЕТСЯ до Save.
//
// Тег — единственная идентичность узла (SPEC 112), и его смена обязана идти в
// паре со сбросом ссылок; сброс делается только на Save. Раньше поле писало
// новый тег в модель посимвольно, а Cancel его не откатывал: окно закрывалось,
// тег в модели оставался новым, ссылающиеся источники выпадали на следующей
// сборке fail-closed — по правке, от которой пользователь отказался.
//
// Здесь проверяется контракт буфера: правка живёт в scratch, а в модель её
// переносит только applyProxyEditToSource (= путь Save).

func serverSourceWithTag(tag string) wizardmodels.Source {
	return wizardmodels.Source{
		ID:      "01SRV0000000000000000000",
		Type:    wizardmodels.SourceTypeServer,
		Enabled: true,
		Label:   "WARP hop",
		NodeTag: tag,
		URI:     "vless://uuid@host:443",
	}
}

// Cancel: буфер правили, Save не жали — модель обязана остаться прежней.
func TestNodeTagEditIsBufferedUntilSave(t *testing.T) {
	src := serverSourceWithTag("hop")
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{src}}
	scratch := m.Sources[0].ToProxySourceV4()

	// Ровно то, что делает nodeTagEntry.OnChanged: пишет в буфер, и только.
	scratch.TagMask = "hop-renamed"

	if got := m.Sources[0].NodeTagOrLabel(); got != "hop" {
		t.Fatalf("тег в модели = %q — правка утекла мимо Save", got)
	}
}

// Save: буфер доезжает до модели.
func TestNodeTagEditReachesModelOnSave(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{serverSourceWithTag("hop")}}
	scratch := m.Sources[0].ToProxySourceV4()
	scratch.TagMask = "hop-renamed"

	applyProxyEditToSource(&scratch, &m.Sources[0])

	if got := m.Sources[0].NodeTagOrLabel(); got != "hop-renamed" {
		t.Fatalf("тег в модели = %q, ожидался hop-renamed", got)
	}
}

// Очистка поля — тоже правка: applyProxyEditToSource пустую маску игнорирует,
// поэтому у именующих узел источников её применяет applyClearedNodeTag.
func TestClearedNodeTagReachesModelOnSave(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{serverSourceWithTag("hop")}}
	scratch := m.Sources[0].ToProxySourceV4()
	scratch.TagMask = ""

	applyProxyEditToSource(&scratch, &m.Sources[0])
	applyClearedNodeTag(m, true, 0, scratch.TagMask)

	if m.Sources[0].NodeTag != "" {
		t.Fatalf("тег = %q, очистка поля не доехала", m.Sources[0].NodeTag)
	}
	// Пустой NodeTag читается как «тег равен подписи» — прежнее поведение.
	if got := m.Sources[0].NodeTagOrLabel(); got != "WARP hop" {
		t.Errorf("NodeTagOrLabel = %q, ожидался откат на подпись", got)
	}
}

// У подписки пустая маска значит «маски нет», а не «сотри тег»: чужой ветке
// applyClearedNodeTag делать нечего.
func TestClearedNodeTagSkipsNonIdentityOwner(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{{
		ID: "01SUB", Type: wizardmodels.SourceTypeSubscription, Enabled: true,
		Label: "Proton NL", NodeTag: "leftover", URL: "https://example.com/sub",
	}}}

	applyClearedNodeTag(m, false, 0, "")

	if m.Sources[0].NodeTag != "leftover" {
		t.Fatalf("тег подписки затёрт: %q", m.Sources[0].NodeTag)
	}
}

// Выход за границы модели не должен ничего ронять: окно живёт своей жизнью, а
// источник за это время могли удалить из списка.
func TestClearedNodeTagOutOfRangeIsNoop(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{serverSourceWithTag("hop")}}
	applyClearedNodeTag(m, true, 7, "")
	applyClearedNodeTag(nil, true, 0, "")
	if m.Sources[0].NodeTag != "hop" {
		t.Fatalf("тег = %q, ожидался нетронутым", m.Sources[0].NodeTag)
	}
}
