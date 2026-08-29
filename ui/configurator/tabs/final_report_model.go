// File final_report_model.go — логика вкладки «Итог» без единого виджета
// (SPEC 115 §1, §3).
//
// Отделено от вёрстки намеренно: гейт кнопки Save и состав отчёта — это то,
// что обязано быть проверено тестами, а тесты на вёрстку и формулировки в
// проекте запрещены. Всё, что здесь лежит, — чистые функции от состояния
// сборки; ни одна не трогает fyne.
package tabs

import (
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/locale"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	finalTabHintText  = "Entering this tab runs a full build in memory: sources are parsed, references and chains are resolved, the outbound graph is checked. Nothing is written and the core is not restarted — the build only produces the report below. Save writes the state and rebuilds the working config.json."
	finalCleanText    = "The build passed with no warnings."
	finalBuildingText = "Building the config in memory…"
	// Разбор подписок не дал ни одного узла. Это ОШИБКА сборки, а не чистый
	// отчёт: конфиг без прокси-нод собирается и проходит валидацию, поэтому
	// молчание тут открыло бы Save на пустышке.
	finalNoNodesText = "The sources produced no nodes, so the build would contain no proxies. Check the Sources tab."
)

// finalBuildState — состояние вкладки «Итог» между заходами.
//
// Три исхода, а не два: «сборки ещё не было / идёт» ≠ «собралось» ≠
// «упало». У каждого своя поверхность (прелоадер, отчёт, текст ошибки) и своя
// судьба у кнопки Save.
type finalBuildState struct {
	// running — сборка идёт прямо сейчас.
	running bool
	// done — сборка дошла до конца и отчёт показан.
	done bool
	// err — сборка упала; отчёта нет, показывается текст ошибки.
	err error
	// gen — номер попытки, чей отчёт показан на экране.
	//
	// Реестр один на процесс, и писателей у него несколько: пока пользователь
	// читает отчёт, фоновое авто-обновление подписок вправе открыть свою
	// попытку. Признака «отчёт готов» тогда мало — он относился бы уже к чужой
	// сборке, и Save осталась бы открытой на итоге, которого никто не видел.
	gen config.BuildGeneration
}

// saveButtonVisible — гейт кнопки Save (SPEC 115 §1).
//
// Чистая функция состояния: Save открывается ТОЛЬКО после того, как сборка
// прошла и отчёт показан. Смысл гейта — не «защитить от ошибки», а порядок
// чтения: сохранение пересобирает боевой config.json, и человек обязан
// сначала увидеть, что именно соберётся, а уже потом решать.
//
// reportReadyFor — предикат «в реестре готов отчёт ИМЕННО этой попытки»
// (config.BuildReportReadyFor). Правка модели сбрасывает попытку, чужой
// писатель её перехватывает — в обоих случаях открытая Save обязана закрыться
// обратно. Без второго условия кнопка пережила бы собственное основание.
func saveButtonVisible(st finalBuildState, reportReadyFor func(config.BuildGeneration) bool) bool {
	if st.running || st.err != nil || !st.done {
		return false
	}
	if reportReadyFor == nil {
		return false
	}
	return reportReadyFor(st.gen)
}

// finalReportLine — одна строка отчёта: текст плюс источник, к которому она
// относится (пусто — переходить некуда).
type finalReportLine struct {
	Text     string
	SourceID string
}

// finalReportLines разворачивает отчёт в строки для показа.
//
// Порядок — по видам, от «источник потерян целиком» к «часть узлов снята»:
// сначала то, что стоило пользователю всего источника, потом частичные
// потери. Внутри вида сохраняется порядок записей (он детерминирован в
// сборке), поэтому список не прыгает между заходами на вкладку.
func finalReportLines(entries []config.BuildReportEntry) []finalReportLine {
	// Источник, не давший ни одного узла, идёт ПЕРВЫМ: он объясняет остальные
	// записи (у пропавшей подписки следом рвутся ссылки на её узлы), и читать
	// список сверху вниз надо начиная с корня.
	order := map[config.BuildReportKind]int{
		config.BuildReportSourceParseFailed: 0,
		config.BuildReportSourceExcluded:    1,
		config.BuildReportTargetMissing:     2,
		config.BuildReportNodesDropped:      3,
		config.BuildReportChainFailed:       4,
		// Деградации обновления и эмиссии — частичные потери у источника,
		// который работает: после всего, что стоило источника целиком.
		config.BuildReportFetchDegraded: 5,
		config.BuildReportEmitDegraded:  6,
		config.BuildReportNaiveDegraded: 7,
	}
	idx := make([]int, len(entries))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return order[entries[idx[a]].Kind] < order[entries[idx[b]].Kind]
	})

	out := make([]finalReportLine, 0, len(entries))
	for _, i := range idx {
		e := entries[i]
		out = append(out, finalReportLine{
			Text:     finalReportEntryText(e),
			SourceID: e.SourceID,
		})
	}
	return out
}

// finalReportEntryText — человекочитаемая строка одной записи.
//
// Субъект впереди, причина после тире: список читают глазами по левому краю,
// и там должно стоять имя того, что сломалось, а не общая для десятка записей
// формулировка причины.
func finalReportEntryText(e config.BuildReportEntry) string {
	subject := strings.TrimSpace(e.Subject)
	if subject == "" {
		subject = strings.TrimSpace(e.SourceLabel)
	}
	switch e.Kind {
	case config.BuildReportSourceParseFailed:
		return locale.Tf("Source %q produced no nodes: %s", subject, e.Reason)
	case config.BuildReportSourceExcluded:
		return locale.Tf("Source %q excluded from the config: %s", subject, e.Reason)
	case config.BuildReportNodesDropped:
		return locale.Tf("Source %q: %d node(s) dropped — %s", subject, e.NodeCount, e.Reason)
	case config.BuildReportChainFailed:
		return locale.Tf("Chain %q did not build: %s", subject, e.Reason)
	case config.BuildReportNaiveDegraded:
		return locale.Tf("%d naive node(s) skipped: %s", e.NodeCount, e.Reason)
	case config.BuildReportTargetMissing:
		return locale.Tf("Detour target %q is missing from the build: %s", subject, e.Reason)
	case config.BuildReportEmitDegraded, config.BuildReportFetchDegraded:
		// Причина уже сформулирована целиком (она называет и узел, и что с
		// ним) — субъект добавляется ТОЛЬКО как адрес, где чинить. Без
		// источника фраза остаётся сама собой, а не «: причина» без левого
		// края (SPEC 116 W12, фикс 3).
		if subject == "" {
			return e.Reason
		}
		// Склейка, а не фраза: обе половины уже переведены, переводить
		// пунктуацию между ними не за чем.
		return fmt.Sprintf("%s — %s", subject, e.Reason)
	}
	return fmt.Sprintf("%s: %s", subject, e.Reason)
}

// finalBuildStatusText — явный статус сборки над списком (SPEC 116 W12,
// фикс 4).
//
// До этой волны исход читался косвенно: пустой список означал «собралось
// чисто», непустой — «собралось с предупреждениями», а провал показывался
// одной красной строкой ВНУТРИ того же списка. Три разных исхода, отличимых
// только по содержимому, — и пользователь, пробежав список глазами, не знал
// главного: конфиг вообще собрался или нет.
//
// Чистая функция: вёрстка берёт у неё текст и важность.
func finalBuildStatusText(err error, warnings int) (string, bool) {
	if err != nil {
		return locale.Tf("Build FAILED: %v", err), true
	}
	if warnings == 0 {
		return locale.T("Build OK"), false
	}
	return locale.Tf("Build OK, %d warning(s)", warnings), false
}

// finalReportText — весь отчёт одним текстом для кнопки «Копировать».
//
// Обязан совпадать с тем, что видно на экране: копия, отличающаяся от
// показанного, бесполезна ровно там, где нужна больше всего — в переписке с
// поддержкой.
func finalReportText(lines []finalReportLine) string {
	if len(lines) == 0 {
		return locale.T(finalCleanText)
	}
	var sb strings.Builder
	sb.WriteString(locale.T("Build warnings:"))
	for _, l := range lines {
		sb.WriteString("\n• ")
		sb.WriteString(l.Text)
	}
	return sb.String()
}

// sourceIndexByID — позиция источника в модели по его ULID; -1, если такого
// нет.
//
// Логическая половина перехода «показать источник» (SPEC 115 §3): вкладка
// Sources умеет прокручиваться к строке по индексу, а запись отчёта знает
// только ULID. Разъехаться эти два представления могут легко — источник могли
// удалить между сборкой и кликом, — поэтому промах здесь законный исход, а не
// ошибка.
func sourceIndexByID(ids []string, sourceID string) int {
	if strings.TrimSpace(sourceID) == "" {
		return -1
	}
	for i, id := range ids {
		if id == sourceID {
			return i
		}
	}
	return -1
}
