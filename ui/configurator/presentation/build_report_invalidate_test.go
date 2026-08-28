package presentation

import (
	"testing"

	"singbox-launcher/core/config"
)

// SPEC 115 §2 — инвалидация отчёта сборки правкой модели.
//
// Проверяется именно ПРОВОДКА: что сигнал «модель изменилась» доходит до
// реестра. Логика самого реестра проверена в core/config; здесь важно, что
// MarkAsChanged её зовёт, — без этого вкладка «Итог» показывала бы отчёт
// сборки, сделанной ДО правки, а Save оставалась бы открытой.
func TestMarkAsChangedInvalidatesBuildReport(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	gen := config.StartBuildReport()
	config.AddBuildReportEntries(gen, []config.BuildReportEntry{
		{Kind: config.BuildReportSourceExcluded, Subject: "Proton NL", SourceID: "01SUB",
			SourceLabel: "Proton NL", Reason: "хоп не найден"},
	})
	config.FinishBuildReport(gen)

	p := &WizardPresenter{}
	p.MarkAsChanged()

	if config.BuildReportReady() {
		t.Error("после правки модели отчёт всё ещё считается готовым — Save осталась бы открытой")
	}
	if entries, _, _ := config.BuildReport(); len(entries) != 0 {
		t.Errorf("после правки модели в отчёте осталось %d записей", len(entries))
	}
	if reason := config.ExcludedSourceReason("01SUB"); reason != "" {
		t.Errorf("пометка ⚠ в списке источников пережила правку модели: %q", reason)
	}
}

// Отметка «сохранено» отчёт НЕ трогает: сохранение — это применение того же
// состояния, по которому отчёт и составлен, и гасить его здесь значило бы
// закрыть Save сразу после того, как ею воспользовались.
func TestMarkAsSavedKeepsBuildReport(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	gen := config.StartBuildReport()
	config.AddBuildReportEntries(gen, []config.BuildReportEntry{
		{Kind: config.BuildReportChainFailed, Subject: "цепочка", Reason: "позиция не найдена"},
	})
	config.FinishBuildReport(gen)

	p := &WizardPresenter{}
	p.MarkAsSaved()

	if !config.BuildReportReady() {
		t.Error("сохранение погасило отчёт, хотя состояние не менялось")
	}
}
