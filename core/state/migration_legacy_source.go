// File migration_legacy_source.go — ВХОД миграции: легаси-поля источника v6.
//
// SPEC 118 W5: в каноническом `Source` этих полей больше нет (SPEC Т6). Но
// одноразовая миграция v6→v7 обязана их прочитать — иначе живые состояния
// приедут в v7 без отметок, свёрток, детуров и цепочек. Поэтому легаси-форма
// живёт ЗДЕСЬ, рядом с миграцией, отдельным «сайдкаром»: параллельный
// `Source`-у слайс, который умирает вместе с объектом миграции и никогда не
// доезжает ни до сборки, ни до диска.
//
// Правило границы: ни один прод-путь не имеет права читать этот тип. Всё, что
// из него нужно системе, миграция переносит в канон (nodes[], Node.Tag,
// NodeLink, Replace) — а что не переносится, попадает в отчёт потерь.
//
// Санкционированное исключение grep-инвариантов SPEC §4.A («читатели
// миграции — единственное исключение»).
package state

import (
	"encoding/json"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// legacySourceV6 — легаси-поля одного источника v6, снятые с sourceV6 при
// структурном переносе (adoptConnectionsV6). Индекс совпадает с индексом
// State.Sources: сайдкар и канон идут парой.
type legacySourceV6 struct {
	// Label — прежнее отображаемое имя (у подписки уезжает в Name).
	Label string
	// NodeTag — прежний системный тег узла server/chain (пусто → Label).
	NodeTag string
	// URI / ConfigJSON — тело одиночного сервера до материализации.
	URI        string
	ConfigJSON json.RawMessage
	// Chain — строковые хопы цепочки (финальные теги старой машины).
	Chain *configtypes.SourceChain
	// Outbounds — локальные Направления источника (упразднены классом).
	Outbounds []configtypes.Direction
	// ExcludeFromGlobal / ExposeGroupTagsToGlobal — флаги SPEC 108-эпохи.
	ExcludeFromGlobal       bool
	ExposeGroupTagsToGlobal bool
	// Fold — свёртка подписки (SPEC 108) → FolderReplace.
	Fold *legacyFold
	// Detour-тройня + tag-ссылка (SPEC 077 / 112-A) → NodeLink.
	DetourTag          string
	DetourNodeSourceID string
	DetourNodeTag      string
	DetourNodeHash     string
	DetourNodeLabel    string
	// DisabledNodes — карта выключенных узлов подписки (SPEC 094 D4) →
	// Node.Enabled=false.
	DisabledNodes map[string]int64
	// TagMask — прежняя маска тегов: у server/chain она хранила ТЕГ узла,
	// у подписки была шаблоном (упразднён с warning).
	TagMask string
	// MetaHistory — прежняя fetch-история из SubscriptionMeta v6
	// (канонический дом — SubUpdateStatus).
	MetaHistory legacySubMetaHistory
}

// legacySubMetaHistory — fetch-история v6-меты подписки.
type legacySubMetaHistory struct {
	URLAtFetch        string
	LastFetchedAt     string
	LastStatus        string
	ErrorCount        int
	LastErrorMsg      string
	LastErrorURL      string
	HTTPStatusCode    int
	RawBodyBytes      int64
	NodesCountFetched int
	Truncated         bool
}

// IsEmpty — истории не было (пустышку в SubUpdateStatus не плодим).
func (h legacySubMetaHistory) IsEmpty() bool {
	return h.URLAtFetch == "" && h.LastFetchedAt == "" && h.LastStatus == "" &&
		h.ErrorCount == 0 && h.LastErrorMsg == "" && h.LastErrorURL == "" &&
		h.HTTPStatusCode == 0 && h.RawBodyBytes == 0 &&
		h.NodesCountFetched == 0 && !h.Truncated
}

// ── свёртка v6 (прежний configtypes.SourceFold) ──────────────────

// Режимы прежней свёртки подписки.
const (
	legacyFoldModeSelect     = "select"
	legacyFoldModeAuto       = "auto"
	legacyFoldModeSelectAuto = "select_auto"
)

// legacyFold — прежняя свёртка подписки в группу (SPEC 108). Читается только
// миграцией; канонический наследник — FolderReplace.
type legacyFold struct {
	Mode string                     `json:"mode,omitempty"`
	Auto *configtypes.DirectionAuto `json:"auto,omitempty"`
}

// EffectiveMode — режим с учётом умолчания: неизвестное значение читается как
// простой селектор (иначе узлы свёрнутой подписки пропали бы молча).
func (f *legacyFold) EffectiveMode() string {
	if f == nil {
		return ""
	}
	switch f.Mode {
	case legacyFoldModeAuto, legacyFoldModeSelectAuto:
		return f.Mode
	default:
		return legacyFoldModeSelect
	}
}

// HasSelect / HasAuto — режим порождает селектор / автогруппу.
func (f *legacyFold) HasSelect() bool {
	m := f.EffectiveMode()
	return m == legacyFoldModeSelect || m == legacyFoldModeSelectAuto
}

func (f *legacyFold) HasAuto() bool {
	m := f.EffectiveMode()
	return m == legacyFoldModeAuto || m == legacyFoldModeSelectAuto
}

// Теги групп прежней свёртки: `<PFX>auto` и `<PFX>select`, где PFX — префикс
// тегов подписки с позиционным умолчанием «<номер>:». Формулу воспроизводим
// байт-в-байт: по этим тегам живые состояния ссылались из правил, и
// миграция обязана материализовать РОВНО их (иначе правила уедут в никуда).
func legacyFoldTagPrefix(tagPrefix string, sourceIndex int) string {
	if p := strings.TrimSpace(tagPrefix); p != "" {
		return p
	}
	return strconv.Itoa(sourceIndex+1) + ":"
}

func legacyFoldAutoTag(tagPrefix string, sourceIndex int) string {
	return legacyFoldTagPrefix(tagPrefix, sourceIndex) + "auto"
}

func legacyFoldSelectTag(tagPrefix string, sourceIndex int) string {
	return legacyFoldTagPrefix(tagPrefix, sourceIndex) + "select"
}
