// File source_chain_hops.go — кандидаты позиций цепочки и их вид в списке
// (SPEC 110, SPEC 118 Т8).
//
// Позиция цепочки — это ССЫЛКА (NodeLink), а не строка. Ссылка бывает двух
// видов: корневая (финальный тег Направления, замены свёрнутой папки,
// верхнего узла или системный тег шаблона) и папочная — пара «id папки +
// СЫРОЙ тег узла в ней». Разница не формальная: финальный тег узла папки
// вычисляется её тег-политикой на каждой сборке, и сохранённый в состоянии
// протух бы от правки префикса — ровно тот класс багов, ради которого
// SPEC 112 снёс контент-хэш.
//
// Вписывать теги руками нельзя (как участников группы DNS и детур): имена
// приходят из шаблона и подписок, и опечатка — ссылка в никуда, на которой
// ядро не стартует вовсе.
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
	hopKindNode      = "node"      // узел подписки или папки
	hopKindGroup     = "group"     // группа, экспортированная подпиской
	hopKindDirection = "direction" // другое Направление
	hopKindChain     = "chain"     // другая цепочка (только позиция 0, T5)
	hopKindBuiltin   = "builtin"   // встроенный тег шаблона (direct-out и т. п.)
	hopKindUnknown   = "unknown"   // тег, которого больше нет
	hopKindPending   = "pending"   // пул узлов ещё не собран — судить рано
)

// chainHopCandidate — возможная позиция цепочки.
type chainHopCandidate struct {
	// Link — то, что уедет в модель. Единственная форма хранения позиции.
	Link corestate.NodeLink
	// Tag — как позиция называется В КОНФИГЕ (финальный тег). Показывается
	// пользователю и им же адресуются проверки формы: reality-узлы, узлы со
	// своим detour, типы протоколов — всё это считается по эмитированному
	// пулу, где ключ — финальный тег.
	Tag  string
	Kind string
	// Below — цепочка объявлена НИЖЕ редактируемой по списку источников.
	// Сборка разрешает ссылки только на цепочки выше (ссылки вперёд
	// отвергаются — так циклы невозможны по построению), и форма обязана
	// предупредить, а не дать молча собрать деградирующую позицию.
	Below bool
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
		return locale.T("node")
	case hopKindGroup:
		return locale.T("group")
	case hopKindDirection:
		return locale.T("direction")
	case hopKindChain:
		return locale.T("chain")
	case hopKindBuiltin:
		return locale.T("built-in")
	case hopKindPending:
		return locale.T("loading…")
	default:
		return locale.T("not found")
	}
}

// hopLinkKey — ключ ссылки для сравнения и карт.
//
// Пара (folderId, tag) склеивается через '\x00': разделитель, которого не
// бывает ни в ULID папки, ни в теге, — иначе папка «a» с узлом «b/c» и папка
// «a/b» с узлом «c» получили бы один ключ.
func hopLinkKey(l corestate.NodeLink) string {
	return l.FolderID + "\x00" + l.Tag
}

// collectChainHopCandidates собирает всё, на что цепочка может сослаться.
//
// selfTag исключается: ядро отвергает цепочку, содержащую саму себя
// (`protocol/chain/chain.go:93`).
//
// SPEC 118 Т8: узлы берутся из пула, собранного эмиссией материализованных
// `nodes[]` — того же кода, что и на сборке. Узлы КОНТЕЙНЕРОВ (папок и
// подписок) адресуются папочной ссылкой, узлы верхнего уровня — корневой:
// как их адресует резолв сборки, так их и предлагает форма.
func collectChainHopCandidates(
	model *wizardmodels.WizardModel,
	selfTag string,
) []chainHopCandidate {
	seen := make(map[string]bool, 64)
	var out []chainHopCandidate
	add := func(link corestate.NodeLink, displayTag, kind string) {
		displayTag = strings.TrimSpace(displayTag)
		link.Tag = strings.TrimSpace(link.Tag)
		if displayTag == "" || link.Tag == "" || displayTag == selfTag {
			return
		}
		key := hopLinkKey(link)
		if seen[key] {
			return
		}
		// SPEC 110 T6: рантайм-звенья `<chain>#<i>` в конфиге не существуют —
		// ссылка на них не даст ядру стартовать.
		if config.ChainInternalTag(displayTag) {
			return
		}
		seen[key] = true
		out = append(out, chainHopCandidate{Link: link, Tag: displayTag, Kind: kind})
	}
	addRoot := func(tag, kind string) {
		add(corestate.NodeLink{Tag: tag}, tag, kind)
	}

	// Направления — сначала: это самые осмысленные позиции, и пользователь
	// думает о маршруте именно в них. Теги — из canonical GlobalOutbounds
	// (SPEC 117): кандидаты позиций это теги Направлений, проекция не нужна.
	if model != nil {
		for i := range model.GlobalOutbounds {
			d := &model.GlobalOutbounds[i]
			if d.Disabled {
				// Выключенное Направление в конфиг не попадёт, а ссылка на
				// него не даст стартовать ядру.
				continue
			}
			addRoot(d.Tag, hopKindDirection)
		}
	}

	// Встроенные теги шаблона: direct-out на позиции 0 — это «первый хоп
	// без прокси», осмысленный сценарий. Блокировка в цепочке смысла не
	// имеет и не предлагается.
	addRoot("direct-out", hopKindBuiltin)

	if model != nil {
		// SPEC 110 T5: другие цепочки — законные позиции, но только первой.
		// Не прячем их из списка (сценарий рабочий), а помечаем видом,
		// чтобы форма могла предупредить о неверной позиции. Цепочки НИЖЕ
		// редактируемой помечаются отдельно: сборка разрешает ссылки только
		// вверх по списку.
		belowSelf := false
		for _, src := range model.Sources {
			if src.Kind != corestate.SourceKindChain {
				continue
			}
			if src.NodeTagOrLabel() == selfTag {
				belowSelf = true
				continue
			}
			if !src.Enabled {
				continue
			}
			addRoot(src.NodeTagOrLabel(), hopKindChain)
			if belowSelf && len(out) > 0 && out[len(out)-1].Tag == src.NodeTagOrLabel() {
				out[len(out)-1].Below = true
			}
		}

		// Замены свёрнутых папок: свёрнутая папка представлена в пуле
		// Направлений (и в целях ссылок) ТОЛЬКО своими replace-тегами — её
		// узлы под своими именами адресовать нельзя, и предлагать их значило
		// бы вести к fail-closed.
		for i := range model.Sources {
			src := &model.Sources[i]
			if src.Replace == nil || !src.Enabled {
				continue
			}
			for _, tag := range chainReplaceTags(src.Replace) {
				addRoot(tag, hopKindGroup)
			}
		}

		// Узлы. Пул НЕ перестраиваем: он эмитит все узлы всех источников, а у
		// живых конфигов это сотни — окно правки повисало бы на открытии.
		// Берём то, что уже есть; пусто — список позиций покажет только
		// Направления, а фон дозагрузит остальное (см. окно источника).
		folderIDs := chainFolderIDsBySourceIndex(model)
		for _, n := range model.NodePool {
			if n == nil {
				continue
			}
			kind := hopKindNode
			if n.Scheme == configtypes.SchemeGroup {
				kind = hopKindGroup
			}
			folderID, isFolderNode := folderIDs[n.SourceIndex]
			if !isFolderNode {
				// Верхний узел: его финальный тег и есть адрес в корне.
				addRoot(n.Tag, kind)
				continue
			}
			if folderID == "" {
				// Свёрнутая папка либо папка без id: её узлы адресовать
				// нечем — за неё отвечают replace-теги выше.
				continue
			}
			raw := strings.TrimSpace(n.IdentityTag)
			if raw == "" {
				raw = strings.TrimSpace(n.Tag)
			}
			add(corestate.NodeLink{FolderID: folderID, Tag: raw}, n.Tag, kind)
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

// chainReplaceTags — теги замены свёрнутой папки.
//
// Делегирует канонической формуле сборки (config.FolderReplaceTags), а не
// повторяет её: разойдись они — и форма предлагала бы позицией тег, которого
// в конфиге нет (или прятала бы существующий). Перевод формы нужен только
// потому, что у модели и у сборки разные экземпляры одной структуры.
func chainReplaceTags(r *corestate.FolderReplace) []string {
	if r == nil {
		return nil
	}
	return config.FolderReplaceTags(&configtypes.FolderReplace{Mode: r.Mode, Tag: r.Tag})
}

// chainFolderIDsBySourceIndex — «индекс источника → id папки» для источников,
// чьи узлы живут в ПАПОЧНОМ пространстве тегов.
//
// Свёрнутая папка (`replace != nil`) в карту не попадает вовсе с пустым id:
// её узлы не адресуются ссылками — конфиг видит только теги замены, и хоп на
// узел свёрнутой папки был бы fail-closed.
func chainFolderIDsBySourceIndex(model *wizardmodels.WizardModel) map[int]string {
	if model == nil {
		return nil
	}
	out := make(map[int]string, len(model.Sources))
	for i := range model.Sources {
		src := &model.Sources[i]
		if src.Kind != corestate.SourceKindFolder && src.Kind != corestate.SourceKindSubscription {
			continue
		}
		if src.Replace != nil {
			out[i] = "" // свёрнута — узлы не адресуются
			continue
		}
		out[i] = src.ID
	}
	return out
}

// chainNodesKnown — собран ли пул узлов.
//
// Пустой пул и «в подписках нет ни одного узла» здесь неразличимы, и это
// осознанно: второе — тоже не повод объявлять позиции потерянными, пока
// подписки не загружены.
func chainNodesKnown(m *wizardmodels.WizardModel) bool {
	return m != nil && len(m.NodePool) > 0
}

// chainHopLookup — быстрый доступ к кандидату по его ссылке.
func chainHopLookup(cands []chainHopCandidate) map[string]chainHopCandidate {
	m := make(map[string]chainHopCandidate, len(cands))
	for _, c := range cands {
		m[hopLinkKey(c.Link)] = c
	}
	return m
}

// describeChainHop — вид позиции, уже лежащей в цепочке.
//
// Ссылка, которой больше нет среди кандидатов (подписка обновилась, узел
// исчез), помечается неизвестной, а не выбрасывается молча: цепочка со
// ссылкой в никуда не соберётся, и пользователь должен увидеть, ЧТО именно
// пропало, — иначе позиция просто исчезнет из списка и маршрут поменяется
// без его ведома.
func describeChainHop(link corestate.NodeLink, lookup map[string]chainHopCandidate, nodesKnown bool) chainHopCandidate {
	if c, ok := lookup[hopLinkKey(link)]; ok {
		return c
	}
	// Показать нечего, кроме самой ссылки: финального тега у неё нет (его
	// вычисляет сборка по живой папке), поэтому в строке остаётся сырой тег.
	fallback := chainHopCandidate{Link: link, Tag: link.Tag, Kind: hopKindUnknown}
	if !nodesKnown {
		// Пул узлов ещё не собран: в кандидатах пока только Направления,
		// и объявить позицию потерянной значило бы покрасить красным
		// рабочую цепочку — ровно то, что пользователь и увидел.
		fallback.Kind = hopKindPending
	}
	return fallback
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
		if src.Kind != corestate.SourceKindChain || len(src.Hops) == 0 {
			continue
		}
		for _, hop := range src.Hops {
			// Ссылка на узел ПАПКИ (FolderID ≠ "") цепочку по имени не
			// адресует: её переименование такой хоп не задевает.
			if hop.FolderID != "" || hop.Tag == "" || hop.Tag == src.NodeTagOrLabel() {
				continue
			}
			if out == nil {
				out = make(map[string][]string, 2)
			}
			out[hop.Tag] = append(out[hop.Tag], src.NodeTagOrLabel())
		}
	}
	return out
}
