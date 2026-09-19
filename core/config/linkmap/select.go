package linkmap

import (
	"sort"

	"singbox-launcher/core/config/registry"
)

// Candidate — кандидат выбора: что-то, у чего есть `detect` и порядок.
//
// Интерфейс, а не конкретный тип, потому что выбирают одинаково на ОБОИХ
// уровнях: вид источника (source_kinds.json) и форма секции (forms[]). Если бы
// уровни выбирали по-своему, «порядок значим» пришлось бы чинить дважды.
type Candidate interface {
	DetectSpec() *registry.Detect
	OrderKey() int
	Name() string
}

// SourceCandidate — вид источника уровня документа.
type SourceCandidate struct{ Kind registry.SourceKind }

func (c SourceCandidate) DetectSpec() *registry.Detect { return c.Kind.Detect }
func (c SourceCandidate) OrderKey() int                { return c.Kind.Priority }
func (c SourceCandidate) Name() string                 { return c.Kind.SourceKind }

// FormCandidate — форма секции-маппера. Порядок задаёт позиция в массиве:
// у форм своего priority нет, и вводить его незачем — их немного и они
// перечислены рядом.
type FormCandidate struct {
	Form  registry.Form
	Index int
}

func (c FormCandidate) DetectSpec() *registry.Detect { return c.Form.Detect }
func (c FormCandidate) OrderKey() int                { return c.Index }
func (c FormCandidate) Name() string                 { return c.Form.ID }

// SelectResult — исход выбора.
type SelectResult struct {
	// Index — позиция победителя в исходном списке; -1, если не выбран никто.
	Index int
	// Matched — имена ВСЕХ сработавших кандидатов, по порядку. Линтер
	// требует, чтобы на корпусе их было ровно один: два — тихое перекрытие,
	// ноль — нераспознанный источник.
	Matched []string
	// ByDefault — победила ветка «всё остальное».
	ByDefault bool
}

// Select выбирает кандидата по предикатам.
//
// Правило неоднозначности (SPEC 133 §3A.3): совпало несколько — побеждает
// меньший OrderKey; не совпало ничего — берётся ветка `default`; нет и её —
// Index = -1, и вызывающий обязан выдать код `source_unrecognized`.
//
// Ветка `default` НИКОГДА не конкурирует с настоящими предикатами, даже если
// её порядок меньше: иначе «всё остальное» выигрывало бы у точного признака,
// и порядок в файле стал бы ловушкой.
func Select(cands []Candidate, c *Content) SelectResult {
	res := SelectResult{Index: -1}

	order := make([]int, 0, len(cands))
	for i := range cands {
		order = append(order, i)
	}
	sort.SliceStable(order, func(a, b int) bool {
		return cands[order[a]].OrderKey() < cands[order[b]].OrderKey()
	})

	fallback := -1
	for _, i := range order {
		spec := cands[i].DetectSpec()
		if spec != nil && spec.Default {
			if fallback < 0 {
				fallback = i
			}
			continue
		}
		if Matches(spec, c) {
			if res.Index < 0 {
				res.Index = i
			}
			res.Matched = append(res.Matched, cands[i].Name())
		}
	}

	if res.Index < 0 && fallback >= 0 {
		res.Index = fallback
		res.ByDefault = true
	}
	return res
}

// SelectSource выбирает вид источника уровня документа.
func SelectSource(set *registry.MapperSet, c *Content) (registry.SourceKind, SelectResult) {
	kinds := set.SourceKindsByPriority()
	cands := make([]Candidate, 0, len(kinds))
	for _, k := range kinds {
		cands = append(cands, SourceCandidate{Kind: k})
	}
	res := Select(cands, c)
	if res.Index < 0 {
		return registry.SourceKind{}, res
	}
	return kinds[res.Index], res
}

// SelectURI находит секцию `uri`, чей detect опознаёт текст ссылки.
//
// Имён схем здесь нет и быть не может: схему выбирает РЕЕСТР своим detect, а
// не префикс ссылки — написание и схема разные вещи (`hy2://` → hysteria2,
// `socks5://` → socks, `naive+quic://` → naive).
//
// Две секции на один текст — ошибка реестра, а не повод гадать: движок
// отказывается вести такую ссылку, и вызывающий получает «схема не
// поддержана». Линтер корпуса ловит такой случай отдельно.
//
// Прежде функция звалась SelectLiveURI и спрашивала ещё и атрибут `live`:
// пока схемы переводились волнами, в конвейере жили два пути, и флаг отвечал,
// какой из них ведёт эту схему. Переведены все — атрибут снят вместе с
// развилкой.
func SelectURI(plans *PlanSet, text string) (string, *Plan, bool) {
	return SelectKind(plans, "uri", text)
}

// SelectKind находит секцию НАЗВАННОГО вида, чей detect опознаёт текст.
//
// Общая форма SelectURI: вид источника — параметр, потому что называть его
// движок не вправе (его называет вызывающий, знающий, откуда приехал текст).
// Текст `.conf` приезжает файлом, схемы в нём нет вовсе, и выбирает секцию
// предикат по ini — ровно так же, как у ссылки выбирает предикат по тексту.
func SelectKind(plans *PlanSet, kind, text string) (string, *Plan, bool) {
	if plans == nil {
		return "", nil, false
	}
	content := NewContent(text)
	hit, plan := "", (*Plan)(nil)
	for _, scheme := range plans.Schemes() {
		p, ok := plans.Plan(scheme, kind)
		if !ok || p.Mapper == nil || p.Mapper.Detect == nil {
			continue
		}
		if !Matches(p.Mapper.Detect, content) {
			continue
		}
		if hit != "" {
			return "", nil, false
		}
		hit, plan = scheme, p
	}
	if hit == "" {
		return "", nil, false
	}
	return hit, plan, true
}

// SelectForm выбирает форму секции-маппера.
func SelectForm(m *registry.Mapper, c *Content) (registry.Form, SelectResult) {
	if m == nil || len(m.Forms) == 0 {
		return registry.Form{}, SelectResult{Index: -1}
	}
	cands := make([]Candidate, 0, len(m.Forms))
	for i, f := range m.Forms {
		cands = append(cands, FormCandidate{Form: f, Index: i})
	}
	res := Select(cands, c)
	if res.Index < 0 {
		return registry.Form{}, res
	}
	return m.Forms[res.Index], res
}
