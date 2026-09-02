// Package business — SPEC 077 → SPEC 118: выбор цели дозвона (detour) для
// окна источника.
//
// В модели v7 у цели ОДНА форма — `NodeLink` (features/directions.md §6):
// либо ссылка корневого пространства финальных тегов (Направление, replace-тег
// свёрнутой папки, верхний узел), либо пара «id папки + сырой тег узла в ней».
// Прежней тройни detour_tag / detour_node_source_id / detour_node_hash
// больше нет — здесь остался один выбор с одним значением.
package business

import (
	"strconv"
	"strings"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// detourExcludedBuiltins are service/auto outbounds that make no sense as a
// proxy-chain hop and are never offered as detour targets (SPEC 077):
//   - direct-out / reject / drop — built-in service outbounds;
//   - auto-proxy-out — the default template's urltest auto-select group.
//
// Note: auto-proxy-out is the default template's tag; a custom template that
// renames its auto group would have that group leak in (acceptable — chaining
// through an urltest group still works, it just isn't filtered).
var detourExcludedBuiltins = map[string]struct{}{
	wizardmodels.DefaultOutboundTag: {}, // direct-out
	wizardmodels.RejectActionName:   {}, // reject
	"drop":                          {},
	"auto-proxy-out":                {},
}

// DetourChoice — что означает одна опция пикера: готовая ссылка модели.
//
// Финальный конфиговый тег в выборе не хранится: он вычисляется на каждой
// сборке (тег-политика папки), и хранимый протухал бы от её правки — ровно
// тот класс багов, ради которого SPEC 112 снёс контент-хеш.
type DetourChoice struct {
	// Link — ссылка, которая уедет в Source.Detour. nil = «без detour».
	Link *corestate.NodeLink
	// Label — подпись выбранного узла для текстов формы.
	Label string
}

// detourNodeMarker prefixes single-node options so they read differently from
// group tags in the same dropdown.
const detourNodeMarker = "» "

// DetourOptions строит опции пикера «Detour server» и текущий выбор.
//
// Предлагаются только устойчивые, осмысленные цели:
//   - Направления и активные группы пресетов (GetAvailableOutbounds), кроме
//     служебных (detourExcludedBuiltins);
//   - верхние узлы-серверы (кроме самого источника).
//
// options[0] — всегда noneLabel (сбрасывает detour). Повисший прежний выбор
// (цели больше нет) дописывается в конец, чтобы остаться видимым и стираемым.
func DetourOptions(model *wizardmodels.WizardModel, source *corestate.Source, noneLabel string) (options []string, selected string) {
	options, selected, _ = DetourOptionsWithNodes(model, source, noneLabel)
	return options, selected
}

// DetourOptionsWithNodes — то же плюс карта «показанная строка → её смысл».
func DetourOptionsWithNodes(model *wizardmodels.WizardModel, source *corestate.Source, noneLabel string) (options []string, selected string, choices map[string]DetourChoice) {
	h := detourHolder{}
	if source != nil {
		h.cur = source.Detour
		h.selfID = source.ID
		if source.Replace != nil {
			h.replaceTag = source.Replace.Tag
		}
	}
	return detourOptionsFor(model, h, noneLabel)
}

// DetourOptionsForNode — тот же пикер для узла ВНУТРИ контейнера (папка,
// подписка).
//
// Отдельного резолва здесь нет и быть не должно: ссылка (NodeLink) у всех
// носителей одна и та же — пара «id папки + сырой тег», где пустой FolderID
// означает корневое пространство (features/directions.md). Разными были лишь
// три входа — текущая ссылка, свои replace-теги и собственный ID для
// самоисключения, — и они вынесены в detourHolder. Вторая реализация пикера
// разъехалась бы с первой на первой же правке правил fail-closed.
//
// folderID — контейнер, в котором лежит узел (его ID); rawTag — сырой тег
// самого узла: через себя цепочку не строят.
func DetourOptionsForNode(model *wizardmodels.WizardModel, node *corestate.Node, folderID, rawTag, noneLabel string) (options []string, selected string, choices map[string]DetourChoice) {
	h := detourHolder{selfFolderID: folderID, selfRawTag: rawTag}
	if node != nil {
		h.cur = node.Detour
	}
	return detourOptionsFor(model, h, noneLabel)
}

// detourHolder — носитель detour глазами пикера: всё, что ему нужно знать о
// том, КТО выбирает цель. Источник и узел контейнера различаются только этим.
type detourHolder struct {
	// cur — действующая ссылка носителя.
	cur *corestate.NodeLink
	// selfID — ID носителя-источника (пусто у узла контейнера).
	selfID string
	// replaceTag — тег свёртки носителя-папки (пусто у остальных).
	replaceTag string
	// selfFolderID / selfRawTag — адрес носителя-узла внутри контейнера.
	selfFolderID, selfRawTag string
}

func detourOptionsFor(model *wizardmodels.WizardModel, holder detourHolder, noneLabel string) (options []string, selected string, choices map[string]DetourChoice) {
	options = []string{noneLabel}
	choices = map[string]DetourChoice{}
	inOptions := map[string]struct{}{noneLabel: {}}

	// Свои replace-теги: свёрнутая папка не может ходить через собственную
	// же замену — это ссылка на саму себя.
	own := map[string]struct{}{}
	if holder.replaceTag != "" {
		own[holder.replaceTag] = struct{}{}
		own[holder.replaceTag+"-auto"] = struct{}{}
	}

	for _, tag := range GetAvailableOutbounds(model) {
		if _, isBuiltin := detourExcludedBuiltins[tag]; isBuiltin {
			continue // service/auto outbound — never a chain hop
		}
		if _, isOwn := own[tag]; isOwn {
			continue
		}
		options = append(options, tag)
		inOptions[tag] = struct{}{}
		choices[tag] = DetourChoice{Link: &corestate.NodeLink{Tag: tag}, Label: tag}
	}

	cur := holder.cur
	selectedDisplay := ""

	// Верхние узлы-серверы: они тоже законные цели (SPEC 112-A) — их тег
	// корневой, уникализация ему не нужна.
	if model != nil {
		for i := range model.Sources {
			s := &model.Sources[i]
			if s.Kind != wizardmodels.SourceKindServer {
				continue
			}
			// Выключенный узел в конфиг не идёт, и целью detour быть не может:
			// выбор указал бы в пустоту, а сборка уронила бы носителя
			// fail-closed. Список целей обязан совпадать с тем, что реально
			// попадёт в config.json — то же правило, что у Направлений
			// (GetAvailableOutbounds пропускает Disabled).
			//
			// Действующий выбор при этом не теряется: ветка ниже показывает
			// его отдельной строкой, даже если цель погасла, — иначе человек
			// не узнал бы, куда ведёт fail-closed на сборке.
			if !s.Enabled {
				continue
			}
			if holder.selfID != "" && s.ID == holder.selfID {
				continue // цепочка через самого себя
			}
			// Узел контейнера тоже не ходит через себя: у корневого узла
			// адрес — пустой FolderID и его сырой тег.
			if holder.selfFolderID == "" && holder.selfRawTag != "" &&
				strings.TrimSpace(s.NodeTagOrLabel()) == holder.selfRawTag {
				continue
			}
			nodeTag := strings.TrimSpace(s.NodeTagOrLabel())
			if nodeTag == "" {
				continue // безымянный узел адресовать нечем
			}
			label := s.Label
			if label == "" {
				label = nodeTag
			}
			display := detourNodeMarker + label
			for n := 2; ; n++ {
				if _, dup := choices[display]; !dup {
					break
				}
				display = detourNodeMarker + label + " (" + strconv.Itoa(n) + ")"
			}
			options = append(options, display)
			choices[display] = DetourChoice{Link: &corestate.NodeLink{Tag: nodeTag}, Label: label}
			if selectedDisplay == "" && cur != nil && cur.FolderID == "" && cur.Tag == nodeTag {
				selectedDisplay = display
			}
		}
	}

	selected = noneLabel
	switch {
	case cur == nil || strings.TrimSpace(cur.Tag) == "":
		// без detour
	case selectedDisplay != "":
		selected = selectedDisplay
	case cur.FolderID == "":
		// Ссылка корневого пространства: либо живая опция выше, либо повисшая.
		selected = cur.Tag
		if _, ok := inOptions[selected]; !ok {
			options = append(options, selected) // dangling — keep visible/clearable
			choices[selected] = DetourChoice{Link: &corestate.NodeLink{Tag: cur.Tag}, Label: cur.Tag}
		}
	default:
		// Ссылка на узел ПАПКИ. Узлы папок пикер не перечисляет (их сотни),
		// но повисший/действующий выбор обязан остаться видимым и стираемым:
		// иначе пользователь не узнал бы, куда ведёт fail-closed на сборке.
		label := cur.Tag
		if model != nil {
			if folder := findSourceByID(model, cur.FolderID); folder != nil && folder.Name != "" {
				label = folder.Name + " / " + cur.Tag
			}
		}
		display := detourNodeMarker + label
		for n := 2; ; n++ {
			if _, dup := choices[display]; !dup {
				break
			}
			display = detourNodeMarker + label + " (" + strconv.Itoa(n) + ")"
		}
		options = append(options, display)
		link := *cur
		choices[display] = DetourChoice{Link: &link, Label: label}
		selected = display
	}
	return options, selected, choices
}

// findSourceByID — источник модели по ULID (nil, если такого нет).
func findSourceByID(model *wizardmodels.WizardModel, id string) *corestate.Source {
	if model == nil || id == "" {
		return nil
	}
	for i := range model.Sources {
		if model.Sources[i].ID == id {
			return &model.Sources[i]
		}
	}
	return nil
}
