package tabs

import (
	"errors"
	"strings"
	"testing"

	"singbox-launcher/core/config"
)

// SPEC 115 — логика вкладки «Итог». Вёрстка и формулировки НЕ проверяются
// (тесты на них в проекте запрещены): здесь только гейт Save, порядок записей
// в отчёте и логическая половина перехода «показать источник».

// Гейт Save (SPEC 115 §1): кнопка открывается ТОЛЬКО когда сборка прошла и
// отчёт готов. Смысл — порядок чтения: сохранение пересобирает боевой
// config.json, и пользователь обязан сначала увидеть, что соберётся.
func TestSaveButtonGate(t *testing.T) {
	// Реестр в тесте подменяется предикатом: «готов отчёт попытки 7».
	// Так проверяется именно гейт, а не глобальное состояние реестра.
	readyFor7 := func(gen config.BuildGeneration) bool { return gen == 7 }
	readyForNobody := func(config.BuildGeneration) bool { return false }

	cases := []struct {
		name  string
		state finalBuildState
		ready func(config.BuildGeneration) bool
		want  bool
	}{
		{"до первой сборки", finalBuildState{}, readyForNobody, false},
		{"сборка идёт", finalBuildState{running: true}, readyForNobody, false},
		{
			// Отчёт от ПРОШЛОЙ сборки ещё лежит в реестре, но текущая идёт:
			// открытая кнопка предложила бы сохранить итог, к которому отчёт
			// на экране уже не относится.
			"сборка идёт поверх прошлого отчёта", finalBuildState{running: true, gen: 7}, readyFor7, false,
		},
		{"сборка упала", finalBuildState{err: errors.New("template data not available"), gen: 7}, readyFor7, false},
		{
			// Правка модели сбросила отчёт — кнопка обязана закрыться
			// обратно, а не пережить собственное основание.
			"отчёт инвалидирован правкой", finalBuildState{done: true, gen: 7}, readyForNobody, false,
		},
		{
			// Реестр готов, но это отчёт ЧУЖОЙ попытки: пока пользователь читал
			// свою, фоновое авто-обновление подписок перехватило реестр.
			// Признака «готов» тут мало — Save открылась бы на итоге, которого
			// никто не видел.
			"реестр перехвачен другим писателем", finalBuildState{done: true, gen: 6}, readyFor7, false,
		},
		{"сборка прошла, отчёт показан", finalBuildState{done: true, gen: 7}, readyFor7, true},
	}
	for _, tc := range cases {
		if got := saveButtonVisible(tc.state, tc.ready); got != tc.want {
			t.Errorf("%s: гейт Save = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

// Отчёт разворачивается в строки без потерь и в порядке «сначала то, что
// стоило источника целиком». Записи без source_id остаются в списке, но
// перехода не получают — переходить к удалённому источнику некуда.
func TestFinalReportLinesOrderAndSourceLinks(t *testing.T) {
	entries := []config.BuildReportEntry{
		{Kind: config.BuildReportNaiveDegraded, Subject: "naive", Reason: "ядро без with_naive_outbound", NodeCount: 3},
		{Kind: config.BuildReportNodesDropped, Subject: "Big Sub", SourceID: "01BIG", SourceLabel: "Big Sub", Reason: "цель detour исчезла", NodeCount: 500},
		{Kind: config.BuildReportSourceExcluded, Subject: "Proton NL", SourceID: "01SUB", SourceLabel: "Proton NL", Reason: "хоп не найден"},
		{Kind: config.BuildReportChainFailed, Subject: "двойной прыжок", Reason: "позиция не найдена"},
	}

	lines := finalReportLines(entries)
	if len(lines) != len(entries) {
		t.Fatalf("строк %d, записей %d — отчёт что-то потерял", len(lines), len(entries))
	}
	if lines[0].SourceID != "01SUB" {
		t.Errorf("первой строкой идёт %q, ожидался исключённый целиком источник 01SUB", lines[0].SourceID)
	}
	if lines[1].SourceID != "01BIG" {
		t.Errorf("второй строкой идёт %q, ожидался источник со снятыми узлами 01BIG", lines[1].SourceID)
	}
	// Цепочка и naive источника не имеют — перехода у них быть не должно.
	for _, l := range lines[2:] {
		if l.SourceID != "" {
			t.Errorf("запись без источника получила переход на %q", l.SourceID)
		}
	}
	for i, l := range lines {
		if l.Text == "" {
			t.Errorf("строка %d пуста — запись отчёта не дошла до пользователя", i)
		}
	}
}

// Чистая сборка даёт пустой список строк: «предупреждений нет» — тоже ответ,
// и подменять его молчанием нельзя.
func TestFinalReportLinesCleanBuild(t *testing.T) {
	if lines := finalReportLines(nil); len(lines) != 0 {
		t.Fatalf("чистая сборка дала %d строк отчёта", len(lines))
	}
	if finalReportText(nil) == "" {
		t.Error("копия отчёта чистой сборки пуста — в буфер уехало бы ничего")
	}
}

// Копия отчёта совпадает с показанным: копия, отличающаяся от экрана,
// бесполезна ровно там, где нужна больше всего — в переписке с поддержкой.
func TestFinalReportTextCoversEveryLine(t *testing.T) {
	lines := []finalReportLine{{Text: "первая"}, {Text: "вторая"}, {Text: "третья"}}
	text := finalReportText(lines)
	for _, l := range lines {
		if !strings.Contains(text, l.Text) {
			t.Errorf("строка %q не попала в копируемый текст", l.Text)
		}
	}
}

// Логическая половина перехода «показать источник» (SPEC 115 §3): выбор
// позиции по ULID. Источник могли удалить между сборкой и кликом — промах
// здесь законный исход, а не ошибка.
func TestSourceIndexByID(t *testing.T) {
	ids := []string{"01A", "01B", "01C"}
	if got := sourceIndexByID(ids, "01B"); got != 1 {
		t.Errorf("позиция 01B = %d, ожидалась 1", got)
	}
	if got := sourceIndexByID(ids, "01GONE"); got != -1 {
		t.Errorf("удалённый источник разрешился в позицию %d — переход увёл бы в чужую строку", got)
	}
	if got := sourceIndexByID(ids, ""); got != -1 {
		t.Errorf("пустой ULID разрешился в позицию %d", got)
	}
	if got := sourceIndexByID(nil, "01A"); got != -1 {
		t.Errorf("пустой список источников дал позицию %d", got)
	}
}
