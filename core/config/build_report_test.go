package config

import "testing"

// SPEC 115 §2 — отчёт попытки сборки: наполнение всеми видами записей,
// группировка снятых узлов по источнику, инвалидация при правке модели.
//
// Формулировки строк тестами НЕ фиксируются (в проекте они запрещены):
// проверяются виды, привязки и счётчики.

// Отчёт принимает все пять видов записей и отдаёт их обратно вместе с
// привязкой к источнику. До SPEC 115 реестр знал ровно один вид (исключение
// источника), и остальные причины — несобравшаяся цепочка, снятые naive-узлы,
// пропавшая цель detour — доходили только до лога.
func TestBuildReportAcceptsEveryKind(t *testing.T) {
	t.Cleanup(ResetBuildReport)
	gen := StartBuildReport()

	AddBuildReportEntries(gen, []BuildReportEntry{
		{Kind: BuildReportSourceExcluded, Subject: "Proton NL", SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"},
		{Kind: BuildReportNodesDropped, Subject: "Big Sub", SourceID: "01BIG", SourceLabel: "Big Sub", Reason: "цель detour исчезла", NodeCount: 500},
		{Kind: BuildReportChainFailed, Subject: "двойной прыжок", Reason: "позиция не найдена"},
		{Kind: BuildReportNaiveDegraded, Subject: "naive", Reason: "ядро без with_naive_outbound", NodeCount: 3},
		{Kind: BuildReportTargetMissing, Subject: "vpn-select", SourceID: "01BIG", SourceLabel: "Big Sub", Reason: "селектор шаблона выключен"},
	})

	entries, ready, _ := BuildReport()
	if len(entries) != 5 {
		t.Fatalf("в отчёте %d записей, ожидалось 5", len(entries))
	}
	if ready {
		t.Error("отчёт объявлен готовым без FinishBuildReport — Save открылся бы посреди сборки")
	}

	seen := make(map[BuildReportKind]int, 5)
	for _, e := range entries {
		seen[e.Kind]++
	}
	for _, kind := range []BuildReportKind{
		BuildReportSourceExcluded, BuildReportNodesDropped,
		BuildReportChainFailed, BuildReportNaiveDegraded, BuildReportTargetMissing,
	} {
		if seen[kind] != 1 {
			t.Errorf("вид %q попал в отчёт %d раз, ожидался 1", kind, seen[kind])
		}
	}

	FinishBuildReport(gen)
	if !BuildReportReady() {
		t.Error("после FinishBuildReport отчёт не считается готовым — Save не откроется никогда")
	}
}

// Снятые узлы группируются ПО ИСТОЧНИКУ и несут число: у подписки на 500 узлов
// сломанный переход снимает их все разом, и без числа пометка не отличает
// потерю одного узла от потери всей подписки.
func TestBuildReportGroupsDroppedNodesBySource(t *testing.T) {
	t.Cleanup(ResetBuildReport)
	gen := StartBuildReport()

	AddBuildReportEntries(gen, []BuildReportEntry{
		{Kind: BuildReportNodesDropped, Subject: "Big Sub", SourceID: "01BIG", SourceLabel: "Big Sub", Reason: "цель detour исчезла", NodeCount: 500},
		{Kind: BuildReportNodesDropped, Subject: "Small Sub", SourceID: "01SML", SourceLabel: "Small Sub", Reason: "кольцо ссылок", NodeCount: 2},
		// Повтор той же записи — второй поставщик той же попытки. Должен быть
		// проглочен, иначе строка Sources показала бы «снято 500» дважды.
		{Kind: BuildReportNodesDropped, Subject: "Big Sub", SourceID: "01BIG", SourceLabel: "Big Sub", Reason: "другая формулировка", NodeCount: 1},
	})

	entries, _, _ := BuildReport()
	if len(entries) != 2 {
		t.Fatalf("в отчёте %d записей о снятых узлах, ожидалось 2 (по одной на источник)", len(entries))
	}

	if n, reason := DroppedNodesForSource("01BIG"); n != 500 || reason == "" {
		t.Errorf("у 01BIG снято %d узлов (причина %q), ожидалось 500 с причиной", n, reason)
	}
	if n, _ := DroppedNodesForSource("01SML"); n != 2 {
		t.Errorf("у 01SML снято %d узлов, ожидалось 2", n)
	}
	if n, _ := DroppedNodesForSource("01NONE"); n != 0 {
		t.Errorf("незатронутый источник получил %d снятых узлов", n)
	}
	if n, _ := DroppedNodesForSource(""); n != 0 {
		t.Error("пустой source_id совпал с записью — строка без ULID получила бы чужую пометку")
	}
}

// Инвалидация: правка модели Мастера стирает отчёт целиком и закрывает Save.
// Без этого пользователь чинит источник, а пометка ⚠ и открытая Save
// продолжают отвечать про конфигурацию, которой уже нет.
func TestResetBuildReportInvalidatesEverything(t *testing.T) {
	t.Cleanup(ResetBuildReport)
	gen := StartBuildReport()
	AddBuildReportEntries(gen, []BuildReportEntry{
		{Kind: BuildReportSourceExcluded, Subject: "Proton NL", SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"},
		{Kind: BuildReportNodesDropped, Subject: "Big Sub", SourceID: "01BIG", SourceLabel: "Big Sub", Reason: "цель detour исчезла", NodeCount: 7},
	})
	FinishBuildReport(gen)

	ResetBuildReport()

	if entries, ready, _ := BuildReport(); len(entries) != 0 || ready {
		t.Fatalf("после правки модели отчёт жив: %d записей, готов=%v", len(entries), ready)
	}
	if got := ExcludedSourceReason("01SUB"); got != "" {
		t.Errorf("пометка ⚠ пережила правку модели: %q", got)
	}
	if n, _ := DroppedNodesForSource("01BIG"); n != 0 {
		t.Errorf("пометка «снято N» пережила правку модели: %d", n)
	}
	if BuildReportReady() {
		t.Error("гейт Save остался открытым после правки модели")
	}
}

// Новая попытка перезаписывает отчёт: записи прошлой сборки не имеют права
// смешаться с записями текущей (SPEC 115 §2, «одна попытка — один реестр»).
func TestStartBuildReportDiscardsPreviousAttempt(t *testing.T) {
	t.Cleanup(ResetBuildReport)

	gen := StartBuildReport()
	AddBuildReportEntries(gen, []BuildReportEntry{
		{Kind: BuildReportChainFailed, Subject: "старая цепочка", Reason: "ядро без with_lx_chain"},
	})
	FinishBuildReport(gen)

	StartBuildReport()
	if entries, ready, _ := BuildReport(); len(entries) != 0 || ready {
		t.Fatalf("новая попытка унесла с собой прошлую: %d записей, готов=%v", len(entries), ready)
	}
}

// Фасад исключений (SPEC 112-B) переписывает ТОЛЬКО свой вид записей: снятые
// узлы и цепочки приходят от других поставщиков той же попытки, и обнулять их
// чужой перезаписью нельзя.
func TestSetExcludedSourcesKeepsOtherKinds(t *testing.T) {
	t.Cleanup(ResetBuildReport)
	gen := StartBuildReport()

	AddBuildReportEntries(gen, []BuildReportEntry{
		{Kind: BuildReportNodesDropped, Subject: "Big Sub", SourceID: "01BIG", SourceLabel: "Big Sub", Reason: "цель detour исчезла", NodeCount: 4},
		{Kind: BuildReportChainFailed, Subject: "цепочка", Reason: "позиция не найдена"},
	})
	SetExcludedSources(gen, []SourceExclusion{{SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"}})

	if got := ExcludedSourceReason("01SUB"); got == "" {
		t.Fatal("исключение источника не доехало до отчёта")
	}
	if n, _ := DroppedNodesForSource("01BIG"); n != 4 {
		t.Errorf("перезапись исключений снесла запись о снятых узлах (осталось %d)", n)
	}

	// Чистая сборка снимает ⚠, но чужие виды по-прежнему не трогает.
	SetExcludedSources(gen, nil)
	if got := ExcludedSourceReason("01SUB"); got != "" {
		t.Errorf("пометка ⚠ пережила чистую сборку: %q", got)
	}
	if n, _ := DroppedNodesForSource("01BIG"); n != 4 {
		t.Errorf("чистая сборка исключений снесла запись о снятых узлах (осталось %d)", n)
	}
}
