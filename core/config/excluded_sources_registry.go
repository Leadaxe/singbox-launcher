// File excluded_sources_registry.go — реестр источников, выпавших из конфига
// на ПОСЛЕДНЕЙ сборке (SPEC 112-B часть B).
//
// Зачем отдельное хранилище, а не поле состояния: исключение — свойство
// сборки, а не пользовательской настройки. Оно возникает и исчезает вместе с
// составом узлов (провайдер вернул хоп — исключения нет), и записанное в
// state.json пережило бы собственную причину: строка источника носила бы ⚠ до
// следующего сохранения. Поэтому реестр живёт в памяти процесса и целиком
// переписывается каждой сборкой.
//
// Читатели — UI: строка Wizard → Sources (пометка) и тост после сборки на
// вкладке Local. Оба ходят сюда из горутин, отсюда мьютекс; сами виджеты
// трогать только через fyne.Do — это забота вызывающего.
package config

import "sync"

var (
	excludedSourcesMu sync.RWMutex
	excludedSources   []SourceExclusion
)

// SetExcludedSources переписывает реестр итогом сборки.
//
// Зовётся ПОСЛЕ КАЖДОЙ сборки, в том числе успешной: чистая сборка обязана
// снять прежние пометки, иначе ⚠ переживёт свою причину. Пустой аргумент —
// штатный способ очистки.
func SetExcludedSources(list []SourceExclusion) {
	excludedSourcesMu.Lock()
	defer excludedSourcesMu.Unlock()
	if len(list) == 0 {
		excludedSources = nil
		return
	}
	excludedSources = append([]SourceExclusion(nil), list...)
}

// AppendExcludedSources доливает записи в реестр, не трогая уже лежащие
// (SPEC 113-B).
//
// Нужно потому, что исключений теперь два поставщика и работают они в разное
// время: парсер знает про недоступный хоп-узел и пишет реестр сразу после
// сборки узлов; последний рубеж (core/build) узнаёт про исчезнувший селектор
// шаблона только когда сложен полный набор финальных тегов — то есть уже
// ПОСЛЕ SetExcludedSources. Перезапись там стёрла бы причины парсера, поэтому
// доливка. Повторная запись про тот же источник игнорируется: первая причина
// ближе к корню.
func AppendExcludedSources(list []SourceExclusion) {
	if len(list) == 0 {
		return
	}
	excludedSourcesMu.Lock()
	defer excludedSourcesMu.Unlock()
	for _, e := range list {
		dup := false
		for _, have := range excludedSources {
			if have.SourceID == e.SourceID && have.SourceLabel == e.SourceLabel {
				dup = true
				break
			}
		}
		if !dup {
			excludedSources = append(excludedSources, e)
		}
	}
}

// ExcludedSources — копия реестра. Копия, а не сам слайс: читатель из UI не
// должен уметь испортить его следующей сборке.
func ExcludedSources() []SourceExclusion {
	excludedSourcesMu.RLock()
	defer excludedSourcesMu.RUnlock()
	if len(excludedSources) == 0 {
		return nil
	}
	return append([]SourceExclusion(nil), excludedSources...)
}

// ExcludedSourceReason — причина исключения источника с данным ULID; пусто,
// если источник в конфиг попал. Строка Wizard → Sources спрашивает именно так:
// у неё на руках id, а не позиция в сборке.
//
// Пустой sourceID никогда не совпадает: записи без id (конфиг собран не из
// состояния) привязать к строке не к чему, они живут только ради тоста.
func ExcludedSourceReason(sourceID string) string {
	if sourceID == "" {
		return ""
	}
	excludedSourcesMu.RLock()
	defer excludedSourcesMu.RUnlock()
	for _, e := range excludedSources {
		if e.SourceID == sourceID {
			return e.Reason
		}
	}
	return ""
}
