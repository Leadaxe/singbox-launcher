package business

import (
	"strings"
	"testing"

	"singbox-launcher/core/config"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 115 (фикс-раунд) — сборка «Итога» и парсерная стадия обязаны попасть в
// ОДНУ попытку отчёта.
//
// Блокер, который эти тесты закрывают: «Итог» строился прямо из
// model.GeneratedOutbounds, которые наполняет только ParseAndPreview. Вход в
// Мастера сразу на «Итог» давал успешную сборку БЕЗ прокси-нод и открытую
// Save; вход мимо вкладки Направлений после правки — отчёт из одних
// санитайзерных записей, объявленный полным.

// finalReportModel — модель с настоящим шаблоном: сборка «Итога» идёт полным
// конвейером (ForPreview=false), и подменённый шаблон проверял бы не то.
func finalReportModel(t *testing.T) *wizardmodels.WizardModel {
	t.Helper()
	execDir := findProjectRoot(t)
	td, err := wizardtemplate.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}
	model := wizardmodels.NewWizardModel()
	model.TemplateData = td
	model.ExecDir = execDir
	model.ParserConfigJSON = strings.TrimSpace(td.ParserConfig)
	// SPEC 117: гейты «нечего собирать» смотрят на canonical model.Sources —
	// эмулируем добавленную подписку (генератор в тестах замокан).
	model.Sources = append(model.Sources, wizardmodels.Source{
		ID:      "01TESTFINALREPORT00000000",
		Type:    wizardmodels.SourceTypeSubscription,
		Enabled: true,
		URL:     "https://example.com/sub",
	})
	model.RulesLibraryMerged = true
	model.Target = wizardtemplate.LocalTarget()
	ApplyWizardDNSTemplate(model)
	ApplyDNSVarsFromSettingsToModel(model)
	return model
}

// Вход на «Итог» с непройденной парсерной стадией: после разбора в отчёте
// лежат ПАРСЕРНЫЕ виды записей, а собранный конфиг несёт узлы.
//
// До фикса сборка на пустом кэше проходила успешно и молча: конфиг без единой
// прокси-ноды валиден, Save открывалась по этому вранью.
func TestFinalBuildContinuesParserAttempt(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	model := finalReportModel(t)
	model.PreviewNeedsParse = true

	// Парсерная стадия — та самая, что зовёт вкладка «Итог» перед сборкой.
	// Её результат несёт причины всех трёх парсерных видов.
	gen := &registryGenMock{out: &config.OutboundGenerationResult{
		OutboundsJSON: []string{`{"type":"direct","tag":"node-from-subscription"}`},
		ExcludedSources: []config.SourceExclusion{
			{SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"},
		},
		BrokenChains: []config.ChainDegradation{
			{Tag: "hop2", Name: "двойной прыжок", Reason: "позиция не найдена"},
		},
		SkippedNaiveNodes:  3,
		SkippedNaiveReason: "ядро собрано без with_naive_outbound",
	}}
	if err := ParseAndPreview(stubStaleUIUpdater{model: model}, gen); err != nil {
		t.Fatalf("ParseAndPreview: %v", err)
	}
	if model.BuildReportGen == 0 {
		t.Fatal("парсерная стадия не оставила номер попытки — сборка «Итога» не найдёт, куда доливать свои записи")
	}

	text, buildGen, err := BuildFinalReportConfig(model)
	if err != nil {
		t.Fatalf("BuildFinalReportConfig: %v", err)
	}
	if buildGen != model.BuildReportGen {
		t.Errorf("сборка «Итога» ушла в попытку %d, парсерная стадия открыла %d — половины конвейера разъехались",
			buildGen, model.BuildReportGen)
	}

	// Узлы в конфиге: ради этого парсерная стадия и гоняется. Пустая секция
	// между парсер-маркерами прошла бы валидацию и не дала бы ни одной прокси.
	if !strings.Contains(text, "node-from-subscription") {
		t.Error("собранный конфиг не содержит разобранных узлов — Save открылась бы на конфиге без прокси")
	}

	entries, ready, regGen := config.BuildReport()
	if !ready {
		t.Fatal("отчёт не объявлен готовым после успешной сборки — Save не откроется никогда")
	}
	if regGen != buildGen {
		t.Errorf("в реестре лежит попытка %d, сборка вернула %d", regGen, buildGen)
	}
	seen := make(map[config.BuildReportKind]bool, len(entries))
	for _, e := range entries {
		seen[e.Kind] = true
	}
	for _, kind := range []config.BuildReportKind{
		config.BuildReportSourceExcluded,
		config.BuildReportChainFailed,
		config.BuildReportNaiveDegraded,
	} {
		if !seen[kind] {
			t.Errorf("парсерный вид %q не доехал до отчёта — «Итог» показал бы половину причин и объявил её полной", kind)
		}
	}
}

// Без парсерной стадии сборка «Итога» ОТКАЗЫВАЕТСЯ, а не собирает конфиг без
// нод: успех тут был бы враньём, на котором открывается Save.
func TestFinalBuildRefusesWithoutParserAttempt(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	model := finalReportModel(t)
	model.BuildReportGen = 0

	if _, _, err := BuildFinalReportConfig(model); err == nil {
		t.Fatal("сборка «Итога» прошла без парсерной стадии — отчёт был бы из одних санитайзерных записей")
	}
}

// Попытку, обогнанную другим писателем реестра (фоновое авто-обновление
// подписок), сборка «Итога» не объявляет готовой: в реестре лежит уже не её
// отчёт.
func TestFinalBuildRefusesSupersededAttempt(t *testing.T) {
	t.Cleanup(config.ResetBuildReport)

	model := finalReportModel(t)
	model.GeneratedOutbounds = []string{`{"type":"direct","tag":"node"}`}
	model.PreviewNeedsParse = false
	model.BuildReportGen = config.StartBuildReport()

	// Чужой писатель перехватил реестр, пока «Итог» собирался.
	config.StartBuildReport()

	if _, _, err := BuildFinalReportConfig(model); err == nil {
		t.Fatal("сборка обогнанной попытки объявлена успешной — Save открылась бы на чужом отчёте")
	}
	if config.BuildReportReady() {
		t.Error("обогнанная попытка объявила готовым чужой отчёт")
	}
}
