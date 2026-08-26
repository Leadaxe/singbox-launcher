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
