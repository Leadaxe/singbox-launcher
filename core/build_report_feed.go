// File build_report_feed.go — перевод итогов конвейера в записи отчёта
// сборки (SPEC 115 §2).
//
// Живёт в `core`, а не в `core/build` или `core/config`, потому что сшивает
// оба: санитайзер отдаёт свои потери типом core/build, отчёт живёт в
// core/config, и зависимость между ними намеренно односторонняя (core/build
// про core/config не знает). Общий потребитель у них ровно один — этот пакет,
// и он же единственный, кого зовут и боевая сборка (rebuild.go), и сборка в
// памяти на вкладке «Итог».
//
// Одна функция на всех: разъехавшись, боевой отчёт и отчёт «Итога» дали бы
// пользователю два разных ответа про одну конфигурацию — тот самый класс
// расхождения превью и боевой сборки, который у проекта уже был.
package core

import (
	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
)

// FeedBuildReportFromParser кладёт в отчёт всё, что знает ПАРСЕРНАЯ стадия:
// исключённые источники, несобравшиеся цепочки, деградацию naive.
//
// Зовётся сразу после генерации узлов, до последнего рубежа: причины парсера
// ближе к корню, и попасть в отчёт они должны первыми.
// gen — номер СВОЕЙ попытки (из config.StartBuildReport). Чужая, обогнанная
// попытка не вправе дописывать в текущий отчёт: её причины относятся к другой
// конфигурации.
func FeedBuildReportFromParser(gen config.BuildGeneration, res *config.OutboundGenerationResult) {
	if res == nil {
		return
	}
	// Исключения источников идут прежним фасадом: он и переписывает записи
	// своего вида, и чистая сборка обязана снять пометки предыдущей.
	config.SetExcludedSources(gen, res.ExcludedSources)

	entries := make([]config.BuildReportEntry, 0,
		len(res.BrokenChains)+len(res.ParseFailedSources)+1)

	// SPEC 115: источник, не давший ни одного узла. До этого вида записи такой
	// источник сообщал о себе одним WARN в логе («source returned zero nodes
	// (counted as failed)»), а в UI — ничем: строка Sources показывала галку и
	// прежний счётчик узлов. Ровно тот же парадокс, что был у исключённых
	// источников, только причина другая — чинить надо саму подписку.
	//
	// Субъект — подпись источника, как у source_excluded: список отчёта читают
	// по левому краю, и там должно стоять имя того, что сломалось.
	for _, s := range res.ParseFailedSources {
		entries = append(entries, config.BuildReportEntry{
			Kind:        config.BuildReportSourceParseFailed,
			Subject:     s.SourceLabel,
			SourceID:    s.SourceID,
			SourceLabel: s.SourceLabel,
			Reason:      s.Reason,
		})
	}

	// Цепочка, не ставшая узлом (SPEC 110): причина уже сформулирована
	// chain_nodes.go, здесь она только меняет адресата — из лога в отчёт.
	for _, c := range res.BrokenChains {
		entries = append(entries, config.BuildReportEntry{
			Kind:    config.BuildReportChainFailed,
			Subject: c.Name,
			Reason:  c.Reason,
		})
	}

	// naive без поддержки в ядре: узлы сняты, конфиг собран. Молчание тут
	// читалось бы как баг парсера — «узлы были, узлов нет».
	if res.SkippedNaiveNodes > 0 {
		entries = append(entries, config.BuildReportEntry{
			Kind:      config.BuildReportNaiveDegraded,
			Subject:   "naive",
			Reason:    res.SkippedNaiveReason,
			NodeCount: res.SkippedNaiveNodes,
		})
	}

	config.AddBuildReportEntries(gen, entries)
}

// feedParserDiagnosticsOnFailure открывает попытку и кладёт в неё причины
// разбора, когда сборка ПРОВАЛИЛАСЬ.
//
// Обычный путь отчёта проходит через успешную сборку, и до SPEC 115 это
// казалось достаточным: нет конфига — не о чем и отчитываться. На деле
// наоборот. Когда единственный источник отвечает отказом («подписка
// неактивна»), узлов не набирается ни одного, генератор возвращает ошибку —
// и раньше вместе с ней на пол летели уже собранные причины. Пользователь
// видел в списке источник без единой пометки: Preview причину показывал, а
// строку красит отчёт, до которого управление не доходило.
//
// Отдельная функция, а не голая пара вызовов на каждой развилке: выходов из
// сборки много, и забыть один из них — значит вернуть ровно тот же немой
// источник.
func feedParserDiagnosticsOnFailure(res *config.OutboundGenerationResult) {
	if res == nil {
		return
	}
	FeedBuildReportFromParser(config.StartBuildReport(), res)
}

// FeedBuildReportFromSanitizer кладёт в отчёт потери ПОСЛЕДНЕГО РУБЕЖА:
// узлы, снятые граф-санитайзером за недоступную цель detour.
//
// На одну потерю приходятся ДВЕ записи, когда пропавшая цель известна, и они
// отвечают на разные вопросы. nodes_dropped — что потерял пользователь
// («Источник X: снято N узлов»); по ней раскрашивается строка Sources, и она
// же есть у потери без внятной цели (кольцо ссылок). target_missing — ЧЕГО не
// хватило в сборке; субъект там сама цель, потому что чинить надо её, а не
// пострадавший источник. Схлопнув их в одну, пришлось бы выбирать между «что
// сломалось» и «что чинить».
func FeedBuildReportFromSanitizer(gen config.BuildGeneration, list []build.SourceExclusion) {
	if len(list) == 0 {
		return
	}
	entries := make([]config.BuildReportEntry, 0, len(list)*2)
	for _, e := range list {
		debuglog.WarnLog("build report: у источника %q снято %d узлов: %s",
			e.SourceLabel, e.DroppedNodes, e.Reason)
		entries = append(entries, config.BuildReportEntry{
			Kind:        config.BuildReportNodesDropped,
			Subject:     e.SourceLabel,
			SourceID:    e.SourceID,
			SourceLabel: e.SourceLabel,
			Reason:      e.Reason,
			NodeCount:   e.DroppedNodes,
		})
		if e.MissingTarget != "" {
			entries = append(entries, config.BuildReportEntry{
				Kind:        config.BuildReportTargetMissing,
				Subject:     e.MissingTarget,
				SourceID:    e.SourceID,
				SourceLabel: e.SourceLabel,
				Reason:      e.Reason,
			})
		}
	}
	config.AddBuildReportEntries(gen, entries)
}
