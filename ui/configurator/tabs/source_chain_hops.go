// File source_chain_hops.go — кандидаты позиций цепочки и их вид в списке
// (SPEC 110).
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
package tabs

import (
	"sort"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
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
	hopKindPending   = "pending"   // кэш узлов ещё не готов — судить рано
)

// chainHopCandidate — возможная позиция цепочки.
type chainHopCandidate struct {
	Tag  string
	Kind string
}

// Display — как строка выглядит в списке и в пикере.
//
// ТОЛЬКО тег. Позиция цепочки — это ссылка на тег, и именно его пользователь
// увидит в конфиге, в логе ядра и в списке прокси. Имя рядом с тегом не
// добавляет ничего: у server-источника тег и есть его имя, и строка
// удваивалась почти дословно («WARP (MASQUE) · WARP (MASQUE) H3»).
func (c chainHopCandidate) Display() string { return c.Tag }

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
	case hopKindPending:
		return locale.T("wizard.chain.kind_pending")
	default:
		return locale.T("wizard.chain.kind_unknown")
	}
}

// collectChainHopCandidates собирает всё, на что цепочка может сослаться.
//
// selfTag исключается: ядро отвергает цепочку, содержащую саму себя
// (`protocol/chain/chain.go:93`).
//
// Узлы берутся из готового кэша превью. Перестраивать его здесь нельзя:
// разбор всех подписок занимает секунды на живых конфигах, а окно правки
// открывается синхронно — пользователь смотрел бы на замерший интерфейс.
func collectChainHopCandidates(
	model *wizardmodels.WizardModel,
	parserConfig *config.ParserConfig,
	selfTag string,
) []chainHopCandidate {
	seen := make(map[string]bool, 64)
	var out []chainHopCandidate
	add := func(tag, kind string) {
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
		out = append(out, chainHopCandidate{Tag: tag, Kind: kind})
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
			add(d.Tag, hopKindDirection)
		}
	}

	// Встроенные теги шаблона: direct-out на позиции 0 — это «первый хоп
	// без прокси», осмысленный сценарий. Блокировка в цепочке смысла не
	// имеет и не предлагается.
	add("direct-out", hopKindBuiltin)

	if model != nil {
		// SPEC 110 T5: другие цепочки — законные позиции, но только первой.
		// Не прячем их из списка (сценарий рабочий), а помечаем видом,
		// чтобы форма могла предупредить о неверной позиции.
		for _, src := range model.Sources {
			if src.Type != corestate.SourceTypeChain || !src.Enabled {
				continue
			}
			add(src.Label, hopKindChain)
		}
		// Кэш превью НЕ перестраиваем: он парсит все подписки разом, а у
		// живых конфигов это сотни узлов — окно правки повисало на
		// открытии. Берём то, что уже есть; пусто — список позиций
		// покажет только Направления, и пользователь наполнит кэш,
		// открыв превью или обновив подписки.
		for _, n := range model.PreviewNodes {
			if n == nil {
				continue
			}
			kind := hopKindNode
			if n.Scheme == configtypes.SchemeGroup {
				kind = hopKindGroup
			}
			add(n.Tag, kind)
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

// chainNodesKnown — разобран ли кэш узлов.
//
// Пустой кэш и «в подписках нет ни одного узла» здесь неразличимы, и это
// осознанно: второе — тоже не повод объявлять позиции потерянными, пока
// подписки не загружены.
func chainNodesKnown(m *wizardmodels.WizardModel) bool {
	return m != nil && len(m.PreviewNodes) > 0
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
func describeChainHop(tag string, lookup map[string]chainHopCandidate, nodesKnown bool) chainHopCandidate {
	if c, ok := lookup[tag]; ok {
		return c
	}
	if !nodesKnown {
		// Кэш узлов ещё не разобран: в кандидатах пока только Направления,
		// и объявить позицию потерянной значило бы покрасить красным
		// рабочую цепочку — ровно то, что пользователь и увидел.
		return chainHopCandidate{Tag: tag, Kind: hopKindPending}
	}
	return chainHopCandidate{Tag: tag, Kind: hopKindUnknown}
}

// chainReferencedBy — кто из цепочек ссылается на цепочку с этим именем.
//
// Ключ — имя цепочки-цели, значение — имена тех, кто её использует
// позицией. Нужно, чтобы предупредить о разрыве ссылок при переименовании:
// цепочки указывают друг на друга по имени, и правка молча оставила бы
// позицию, указывающую в никуда.
func chainReferencedBy(m *wizardmodels.WizardModel) map[string][]string {
	if m == nil {
		return nil
	}
	var out map[string][]string
	for _, src := range m.Sources {
		if src.Type != corestate.SourceTypeChain || src.Chain == nil {
			continue
		}
		for _, hop := range src.Chain.Hops {
			if hop == "" || hop == src.Label {
				continue
			}
			if out == nil {
				out = make(map[string][]string, 2)
			}
			out[hop] = append(out[hop], src.Label)
		}
	}
	return out
}
