// File chain_hops.go — кандидаты позиций цепочки и их вид в списке
// (SPEC 110, фаза 3).
//
// Позиция цепочки — это тег любого outbound'а: узла подписки, группы,
// которую подписка экспортировала, другого Направления или встроенного
// тега шаблона. Вписывать теги руками нельзя (как участники группы DNS и
// детур): имена приходят из шаблона и подписок, и опечатка в теге — это
// ссылка в никуда, на которой ядро не стартует вовсе.
//
// Для чтения списка важен не только тег, но и ЧТО за ним стоит: группа
// выбирает участника на лету, а `direct` на позиции ≥ 1 означает «хопа
// здесь нет». Поэтому у каждого кандидата есть вид, и он показан в строке.
package outbounds_configurator

import (
	"sort"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Виды позиции — определяют подпись справа в строке списка.
const (
	hopKindNode      = "node"      // узел подписки
	hopKindGroup     = "group"     // группа, экспортированная подпиской
	hopKindDirection = "direction" // другое Направление
	hopKindChain     = "chain"     // другая цепочка (только позиция 0, T5)
	hopKindBuiltin   = "builtin"   // встроенный тег шаблона (direct-out и т. п.)
	hopKindUnknown   = "unknown"   // тег, которого больше нет
)

// chainHopCandidate — возможная позиция цепочки.
type chainHopCandidate struct {
	Tag  string
	Kind string
	// Label — человеческое имя (имя Направления, метка узла). Пусто, если
	// совпадает с тегом.
	Label string
}

// Display — как строка выглядит в списке и в пикере.
func (c chainHopCandidate) Display() string {
	if c.Label != "" && c.Label != c.Tag {
		return c.Label + "  ·  " + c.Tag
	}
	return c.Tag
}

// KindText — подпись вида, локализованная.
func (c chainHopCandidate) KindText() string {
	switch c.Kind {
	case hopKindNode:
		return locale.T("wizard.chain.kind_node")
	case hopKindGroup:
		return locale.T("wizard.chain.kind_group")
	case hopKindDirection:
		return locale.T("wizard.chain.kind_direction")
	case hopKindChain:
		return locale.T("wizard.chain.kind_chain")
	case hopKindBuiltin:
		return locale.T("wizard.chain.kind_builtin")
	default:
		return locale.T("wizard.chain.kind_unknown")
	}
}

// collectChainHopCandidates собирает всё, на что цепочка может сослаться.
//
// selfTag исключается: ядро отвергает цепочку, содержащую саму себя
// (`protocol/chain/chain.go:93`).
//
// Кэш превью перестраивается перед чтением узлов — тем же приёмом, что во
// флаг-пикере: без этого список пуст, пока пользователь не открыл вкладку
// превью, и выбирать оказывается не из чего.
func collectChainHopCandidates(
	model *wizardmodels.WizardModel,
	parserConfig *config.ParserConfig,
	selfTag string,
) []chainHopCandidate {
	seen := make(map[string]bool, 64)
	var out []chainHopCandidate
	add := func(tag, kind, label string) {
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == selfTag || seen[tag] {
			return
		}
		// SPEC 110 T6: рантайм-звенья `<chain>#<i>` в конфиге не существуют —
		// ссылка на них не даст ядру стартовать.
		if config.ChainInternalTag(tag) {
			return
		}
		seen[tag] = true
		out = append(out, chainHopCandidate{Tag: tag, Kind: kind, Label: label})
	}

	// Направления — сначала: это самые осмысленные позиции, и пользователь
	// думает о маршруте именно в них.
	if parserConfig != nil {
		for _, d := range parserConfig.ParserConfig.Outbounds {
			if d.Disabled {
				// Выключенное Направление в конфиг не попадёт, а ссылка на
				// него не даст стартовать ядру.
				continue
			}
			kind := hopKindDirection
			if d.IsChain() {
				// SPEC 110 T5: вложенная цепочка допустима только на
				// позиции 0. Не прячем её из списка — сценарий рабочий, —
				// но помечаем, чтобы форма могла предупредить о неверной
				// позиции.
				kind = hopKindChain
			}
			add(d.Tag, kind, d.DisplayName())
		}
	}

	// Встроенные теги шаблона: direct-out на позиции 0 — это «первый хоп
	// без прокси», осмысленный сценарий. Блокировка в цепочке смысла не
	// имеет и не предлагается.
	add("direct-out", hopKindBuiltin, "")

	if model != nil {
		_, _ = wizardbusiness.RebuildPreviewCache(model)
		for _, n := range model.PreviewNodes {
			if n == nil {
				continue
			}
			kind := hopKindNode
			if n.Scheme == configtypes.SchemeGroup {
				kind = hopKindGroup
			}
			add(n.Tag, kind, n.Label)
		}
	}

	// Направления и встроенные — в порядке объявления (он осмыслен),
	// узлы — по алфавиту: их сотни, и порядок подписки для выбора бесполезен.
	head := 0
	for head < len(out) && (out[head].Kind == hopKindDirection ||
		out[head].Kind == hopKindChain || out[head].Kind == hopKindBuiltin) {
		head++
	}
	tail := out[head:]
	sort.SliceStable(tail, func(i, j int) bool { return tail[i].Display() < tail[j].Display() })
	return out
}

// chainHopLookup — быстрый доступ к кандидату по тегу.
func chainHopLookup(cands []chainHopCandidate) map[string]chainHopCandidate {
	m := make(map[string]chainHopCandidate, len(cands))
	for _, c := range cands {
		m[c.Tag] = c
	}
	return m
}

// describeChainHop — вид позиции, уже лежащей в цепочке.
//
// Тег, которого больше нет среди кандидатов (подписка обновилась, узел
// исчез), помечается неизвестным, а не выбрасывается молча: цепочка со
// ссылкой в никуда не соберётся, и пользователь должен увидеть, ЧТО именно
// пропало, — иначе позиция просто исчезнет из списка и маршрут поменяется
// без его ведома.
func describeChainHop(tag string, lookup map[string]chainHopCandidate) chainHopCandidate {
	if c, ok := lookup[tag]; ok {
		return c
	}
	return chainHopCandidate{Tag: tag, Kind: hopKindUnknown}
}

// tagOf — тег записи, безопасно для nil (новая запись ещё не создана).
func tagOf(d *config.Direction) string {
	if d == nil {
		return ""
	}
	return d.Tag
}

// chainSupportedForList — умеет ли ядро цепочки; для пометок в списке
// Направлений.
//
// Отдельно от формы, потому что список перерисовывается часто, а вердикт
// уже кэширован по (mtime, size) бинаря — повторный вызов не запускает
// `sing-box version` заново.
func chainSupportedForList() bool {
	supported, _ := config.ChainSupportedByCore()
	return supported
}
