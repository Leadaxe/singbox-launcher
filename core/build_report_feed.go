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
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
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

	// SPEC 118 W4: деградации ЭМИССИИ из материализованных nodes[]
	// (нерезолвнутая позиция цепочки, выпавший член Auto, снятое умолчание,
	// столкновение тегов в гарде). Раньше их место занимали причины разбора;
	// разбор теперь живёт только в fetch, а сборка отчитывается за то, что
	// не смогла собрать из уже принятого.
	//
	// SPEC 116 W12 (фикс 3): у записи есть АДРЕСАТ. Раньше субъектом стояла
	// строка "emission", и пользователь, прочитав «узел X исключён», шёл
	// искать глазами, у какого из сорока источников этот узел живёт — притом
	// что в момент рождения предупреждения источник был известен точно.
	// Теперь ULID едет в SourceID, и строка Sources встаёт с ⚠ у виновника —
	// ровно как у деградаций подписки (fetch_degraded).
	for _, w := range res.EmissionWarnings {
		subject := strings.TrimSpace(w.SourceLabel)
		if subject == "" {
			subject = strings.TrimSpace(w.DirectionTag)
		}
		entries = append(entries, config.BuildReportEntry{
			Kind:        config.BuildReportEmitDegraded,
			Subject:     subject,
			SourceID:    w.SourceID,
			SourceLabel: w.SourceLabel,
			Reason:      w.Text,
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

// FeedBuildReportFromFetchStatus кладёт в отчёт деградации ПОСЛЕДНЕГО
// ОБНОВЛЕНИЯ подписок, прочитанные ИЗ СОСТОЯНИЯ (SPEC 118 W4, Т3/Р8).
//
// # Почему из состояния, а не из разбора
//
// В модели v7 тело подписки разбирается один раз — при fetch, — и его
// per-record деградации (битая запись, потерянный член группы, обрезка
// капом, недостоверный ответ) персистятся в `update_status.warnings`.
// Сборка тел не читает вовсе, поэтому заново узнать эти причины ей неоткуда:
// единственный честный источник — состояние.
//
// Это и закрывает риск Р8 (двухфазность отчёта «Итога»): строки появляются
// сразу после fetch, а не после следующей пересборки, потому что читатель
// синхронно смотрит в state, а не ждёт своей стадии конвейера.
//
// Подписка, ни разу не сфетченная (nodes[] пуст, успехов не было), — тоже
// строка отчёта, а не отказ сборки (SPEC Т3).
func FeedBuildReportFromFetchStatus(gen config.BuildGeneration, sources []state.Source) {
	var entries []config.BuildReportEntry
	for i := range sources {
		src := &sources[i]
		if src.Kind != state.SourceKindSubscription || !src.Enabled {
			continue
		}
		label := src.Name
		if label == "" {
			label = src.Label
		}
		if label == "" {
			label = src.URL
		}
		st := src.UpdateStatus

		// «Никогда не фетчилось» — предупреждение, не отказ: конфиг идёт из
		// того, что есть у остальных источников.
		if len(src.Nodes) == 0 && (st == nil || st.LastSuccessAt == "") {
			entries = append(entries, config.BuildReportEntry{
				Kind:        config.BuildReportFetchDegraded,
				Subject:     label,
				SourceID:    src.ID,
				SourceLabel: label,
				Reason:      "подписка ещё ни разу не обновлялась — узлов нет; нажмите Update",
			})
			continue
		}
		if st == nil {
			continue
		}

		// Последнее обновление ПРОВАЛИЛОСЬ, а узлы в конфиг всё-таки едут —
		// от прошлого успеха. Молчать тут нельзя: пользователь видит полный
		// список узлов и считает подписку свежей, хотя провайдер мог давно
		// сменить или отозвать половину. Отказом это не является — сборка
		// честно идёт из последнего известного успеха, поэтому вид записи
		// тот же fetch_degraded, что у остальных деградаций обновления.
		if st.LastStatus == subUpdateStatusErr {
			entries = append(entries, config.BuildReportEntry{
				Kind:        config.BuildReportFetchDegraded,
				Subject:     label,
				SourceID:    src.ID,
				SourceLabel: label,
				Reason:      lastFetchFailedReason(st),
				NodeCount:   len(src.Nodes),
			})
		}

		for _, w := range st.Warnings {
			reason := w.Message
			if reason == "" {
				reason = w.Kind
			}
			subject := label
			if w.Tag != "" {
				// Адресация (folderId, tag): субъект — конкретный узел, а
				// пометка строки Sources остаётся за SourceID.
				subject = label + " → " + w.Tag
			}
			entries = append(entries, config.BuildReportEntry{
				Kind:        config.BuildReportFetchDegraded,
				Subject:     subject,
				SourceID:    src.ID,
				SourceLabel: label,
				Reason:      reason,
				NodeCount:   w.Count,
			})
		}
	}
	if len(entries) == 0 {
		return
	}
	config.AddBuildReportEntries(gen, entries)
}

// subUpdateStatusErr — значение SubUpdateStatus.LastStatus для провала.
const subUpdateStatusErr = "err"

// lastFetchFailedReason — формулировка провалившегося обновления: что
// сломалось и ЧЕМ при этом собран конфиг.
//
// Дата последнего успеха — половина смысла записи: «обновление провалилось»
// без неё читается как «узлов нет», а узлы есть, просто старые. Когда успеха
// не было вовсе, вторая половина опускается — врать про несуществующую дату
// хуже, чем не сказать.
func lastFetchFailedReason(st *state.SubUpdateStatus) string {
	msg := strings.TrimSpace(st.LastErrorMsg)
	if msg == "" {
		msg = "причина не сохранена"
	}
	reason := "последнее обновление провалилось: " + msg
	if at := strings.TrimSpace(st.LastSuccessAt); at != "" {
		reason += "; конфиг собран из узлов от " + at
	}
	return reason
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
