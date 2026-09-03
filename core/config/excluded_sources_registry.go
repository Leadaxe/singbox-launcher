// File excluded_sources_registry.go — источники, выпавшие из конфига на
// ПОСЛЕДНЕЙ сборке (SPEC 112-B часть B).
//
// SPEC 115: собственного хранилища у этого файла больше нет — он стал фасадом
// над типизированным отчётом сборки (build_report.go), где исключение
// источника живёт записью вида source_excluded. Так и должно быть: два
// независимых реестра давали бы два разных ответа на вопрос «что случилось на
// последней сборке», а раскраска Sources и окно «Итога» обязаны отвечать
// одинаково.
//
// Внешний контракт сохранён дословно: SetExcludedSources переписывает,
// AppendExcludedSources доливает, ExcludedSourceReason ищет по ULID.
//
// Зачем в памяти, а не в состоянии: исключение — свойство сборки, а не
// пользовательской настройки. Оно возникает и исчезает вместе с составом узлов
// (провайдер вернул хоп — исключения нет), и записанное в state.json пережило
// бы собственную причину.
//
// Читатели — UI: строка Wizard → Sources (пометка) и отчёт «Итога». Оба ходят
// сюда из горутин; сами виджеты трогать только через fyne.Do — это забота
// вызывающего.
package config

// SetExcludedSources переписывает записи об исключённых источниках итогом
// сборки.
//
// Зовётся ПОСЛЕ КАЖДОЙ сборки, в том числе успешной: чистая сборка обязана
// снять прежние пометки, иначе ⚠ переживёт свою причину. Пустой аргумент —
// штатный способ очистки.
//
// Стираются только записи вида source_excluded: остальные виды отчёта
// (снятые узлы, цепочки, naive) приходят от других поставщиков той же попытки,
// и обнулять их чужой перезаписью нельзя. Целиком попытку открывает
// StartBuildReport.
// Номер попытки обязателен: чужая (обогнанная или инвалидированная правкой
// модели) сборка не вправе ни стирать записи текущей, ни доливать свои.
func SetExcludedSources(gen BuildGeneration, list []SourceExclusion) {
	buildReportMu.Lock()
	if gen != buildReportGen {
		buildReportMu.Unlock()
		return
	}
	kept := buildReport[:0]
	for _, e := range buildReport {
		if e.Kind != BuildReportSourceExcluded {
			kept = append(kept, e)
		}
	}
	buildReport = kept
	buildReportMu.Unlock()
	AppendExcludedSources(gen, list)
}

// AppendExcludedSources доливает записи, не трогая уже лежащие (SPEC 113-B).
//
// Нужно потому, что исключений два поставщика и работают они в разное время:
// парсер знает про недоступный хоп-узел и пишет реестр сразу после сборки
// узлов; последний рубеж (core/build) узнаёт про исчезнувший селектор шаблона
// только когда сложен полный набор финальных тегов — то есть уже ПОСЛЕ
// SetExcludedSources. Перезапись там стёрла бы причины парсера, поэтому
// доливка. Повторная запись про тот же источник игнорируется: первая причина
// ближе к корню.
func AppendExcludedSources(gen BuildGeneration, list []SourceExclusion) {
	if len(list) == 0 {
		return
	}
	entries := make([]BuildReportEntry, 0, len(list))
	for _, e := range list {
		entries = append(entries, BuildReportEntry{
			Kind:        BuildReportSourceExcluded,
			Subject:     e.SourceLabel,
			SourceID:    e.SourceID,
			SourceLabel: e.SourceLabel,
			Reason:      e.Reason,
		})
	}
	AddBuildReportEntries(gen, entries)
}

// ExcludedSources — копия записей об исключённых источниках. Копия, а не сам
// слайс: читатель из UI не должен уметь испортить отчёт следующей сборке.
func ExcludedSources() []SourceExclusion {
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	var out []SourceExclusion
	for _, e := range buildReport {
		if e.Kind != BuildReportSourceExcluded {
			continue
		}
		out = append(out, SourceExclusion{
			SourceID: e.SourceID, SourceLabel: e.SourceLabel, Reason: e.Reason,
		})
	}
	return out
}

// ExcludedSourceReason — причина исключения источника с данным ULID; пусто,
// если источник в конфиг попал. Строка Wizard → Sources спрашивает именно так:
// у неё на руках id, а не позиция в сборке.
//
// Пустой sourceID никогда не совпадает: записи без id (конфиг собран не из
// состояния) привязать к строке не к чему, они живут только ради отчёта.
func ExcludedSourceReason(sourceID string) string {
	if sourceID == "" {
		return ""
	}
	buildReportMu.RLock()
	defer buildReportMu.RUnlock()
	for _, e := range buildReport {
		if e.Kind == BuildReportSourceExcluded && e.SourceID == sourceID {
			return e.Reason
		}
	}
	return ""
}
