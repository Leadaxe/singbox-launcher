// File source_fold.go — свёртка подписки в группу (SPEC 108).
//
// Подписка на полсотни узлов не обязана приезжать в список Направлений
// полусотней строк. Свёрнутая подписка отдаёт вместо своих узлов ОДНУ
// запись — селектор, автогруппу или селектор с автогруппой, — а узлы
// остаются в конфиге только как её состав.
//
// Заменяет четыре прежних флага (`Local auto`, `Local select`,
// `Exclude from global`, `Expose tags`), из восьми комбинаций которых
// осмысленной была одна. Флаги читаются бессрочно и разворачиваются в Fold
// при загрузке состояния (state.migrateSourceFold), обратно не пишутся.
package configtypes

import (
	"strconv"
	"strings"
)

// Режимы свёртки в SourceFold.Mode.
const (
	// FoldModeSelect — узлы заменяются селектором `<PFX>select`: ручной
	// выбор среди узлов подписки.
	FoldModeSelect = "select"
	// FoldModeAuto — узлы заменяются urltest-группой `<PFX>auto`.
	FoldModeAuto = "auto"
	// FoldModeSelectAuto — селектор с парной автогруппой: `<PFX>auto`
	// стоит первой опцией селектора и его умолчанием.
	FoldModeSelectAuto = "select_auto"
)

// SourceFold — свёртка подписки в группу. nil = не свёрнута (узлы попадают
// в Направления по отдельности, как было до SPEC 108).
type SourceFold struct {
	// Mode — во что заменяются узлы. Пустое значение читается как
	// FoldModeSelect: свёрнутая подписка обязана дать хоть какую-то
	// группу, иначе её узлы просто исчезли бы из конфига.
	Mode string `json:"mode,omitempty"`

	// Auto — параметры автогруппы; применяется при Mode = auto |
	// select_auto. Форма та же, что у Направления (SPEC 104), намеренно:
	// это одна и та же настройка на двух уровнях, и вторая её реализация
	// разъехалась бы с первой на первой же правке.
	Auto *DirectionAuto `json:"auto,omitempty"`
}

// EffectiveMode возвращает режим с учётом умолчания.
func (f *SourceFold) EffectiveMode() string {
	if f == nil {
		return ""
	}
	switch f.Mode {
	case FoldModeAuto, FoldModeSelectAuto:
		return f.Mode
	default:
		// Неизвестное значение (правка руками, состояние от будущей
		// версии) трактуем как простой селектор, а не отбрасываем: узлы
		// свёрнутой подписки иначе пропали бы из конфига молча.
		return FoldModeSelect
	}
}

// HasSelect — режим порождает селектор.
func (f *SourceFold) HasSelect() bool {
	m := f.EffectiveMode()
	return m == FoldModeSelect || m == FoldModeSelectAuto
}

// HasAuto — режим порождает автогруппу.
func (f *SourceFold) HasAuto() bool {
	m := f.EffectiveMode()
	return m == FoldModeAuto || m == FoldModeSelectAuto
}

// Теги групп свёрнутой подписки: `<PFX>auto` и `<PFX>select`.
//
// Схема унаследована от прежних локальных групп (SPEC 026) намеренно: она
// работает, и её смена сломала бы ссылки в живых конфигах (S6).
const (
	foldAutoSuffix   = "auto"
	foldSelectSuffix = "select"
)

// EffectiveTagPrefix — префикс тегов подписки с учётом умолчания
// «<номер>:». sourceIndex — индекс подписки в parser_config.proxies,
// нумерация в умолчании с единицы.
func EffectiveTagPrefix(tagPrefix string, sourceIndex int) string {
	if p := strings.TrimSpace(tagPrefix); p != "" {
		return p
	}
	return strconv.Itoa(sourceIndex+1) + ":"
}

// FoldAutoTag / FoldSelectTag — теги групп свёрнутой подписки.
func FoldAutoTag(tagPrefix string, sourceIndex int) string {
	return EffectiveTagPrefix(tagPrefix, sourceIndex) + foldAutoSuffix
}

func FoldSelectTag(tagPrefix string, sourceIndex int) string {
	return EffectiveTagPrefix(tagPrefix, sourceIndex) + foldSelectSuffix
}
