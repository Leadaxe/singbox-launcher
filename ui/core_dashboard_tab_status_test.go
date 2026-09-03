package ui

import "testing"

// SPEC 113-E, регресс: без контроллера updateRunningStatus обязана выходить
// сразу, а не после счётчика несовпадений.
//
// Раньше проверка `tab.controller == nil` стояла ВНУТРИ условия «состояние не
// то»: первые семь вызовов лишь крутили счётчик, а восьмой проваливался ниже —
// прямо в tab.controller.GetVPNButtonState() на nil-контроллере. Восьмой вызов
// здесь не случайность: панель обновляется от внешних событий, и сценарий
// «окно живо, контроллер ещё/уже не подключён» достижим.
func TestUpdateRunningStatusSurvivesNilController(t *testing.T) {
	tab := &CoreDashboardTab{pendingOp: true, pendingOpWantRun: true}
	for i := 0; i < 12; i++ {
		tab.updateRunningStatus() // паника здесь = регресс
	}
	if !tab.pendingOp {
		t.Error("операция в полёте не должна сниматься без ответа ядра")
	}
	if tab.pendingOpMismatchTicks != 0 {
		t.Errorf("счётчик несовпадений двигался без контроллера: %d", tab.pendingOpMismatchTicks)
	}
}
