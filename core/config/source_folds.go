// Package config: source_folds.go — материализация свёрнутых подписок
// (SPEC 108, проход 0).
//
// Свёрнутая подписка отдаёт в Направления не свои узлы, а ОДНУ группу:
// селектор, urltest или селектор с парной urltest-группой. Группы не
// хранятся в состоянии — они разворачиваются здесь, на каждой сборке, ровно
// как парные auto-группы Направлений (direction_twins.go): пользователь
// настраивает одну свёртку, а не два объекта, обязанных оставаться
// синхронными.
//
// Включение существующих механизмов, а не новый конвейер:
//   - узлы подписки уходят из общего пула через ExcludeFromGlobal
//     (outbound_filter.go);
//   - тег группы попадает в кандидаты Направлений через
//     ExposeGroupTagsToGlobal + маркер WIZARD:* в comment
//     (outbound_validity.go).
//
// Оба флага выставляются здесь, на копии конфига, собранной для генерации.
package config

import (
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// Маркеры групп в comment (SPEC 026): по ним collectExposeTagCandidates
// узнаёт, какие локальные записи можно выносить в Направления.
const (
	foldMarkerAuto   = "WIZARD:auto"
	foldMarkerSelect = "WIZARD:selector"
)

// PrepareSourceFolds разворачивает свёртки подписок в локальные группы.
//
// Мутирует переданный ParserConfig — как и PrepareDirections, вызывается по
// копии, собранной для генерации, а не по пользовательскому состоянию.
//
// tmplAutoOptions — те же `group_templates.auto.options` шаблона, что и у
// двойников Направлений: автогруппа подписки и автогруппа Направления —
// одна и та же настройка на двух уровнях.
func PrepareSourceFolds(parserConfig *ParserConfig, tmplAutoOptions map[string]interface{}) {
	if parserConfig == nil {
		return
	}
	for i := range parserConfig.ParserConfig.Proxies {
		ps := &parserConfig.ParserConfig.Proxies[i]
		if ps.Fold == nil || ps.Disabled {
			continue
		}
		// SPEC 118 W4: у канонического источника свёртку разворачивает
		// PrepareFolderReplaces — по явному тегу и без маркеров. Мостовой
		// Fold у него доживает лишь как отражение replace (TEMPORARY BRIDGE),
		// и второй разворот дал бы дубль тега.
		if ps.Canonical != nil {
			continue
		}
		groups := buildFoldGroups(*ps.Fold, ps.TagPrefix, i, tmplAutoOptions)
		if len(groups) == 0 {
			continue
		}

		// Записи с прежними маркерами выкидываем: состояние, записанное до
		// SPEC 108 и не прошедшее миграцию (например, приехавшее из
		// импорта), иначе дало бы дубль тега.
		ps.Outbounds = append(dropFoldGroups(ps.Outbounds), groups...)

		// Узлы свёрнутой подписки в общем списке не нужны — ради этого
		// свёртку и включают.
		ps.ExcludeFromGlobal = true
		// А её группа, наоборот, обязана стать кандидатом в Направления.
		ps.ExposeGroupTagsToGlobal = true

		debuglog.DebugLog("SPEC 108: подписка %d свёрнута в %d группу(ы), режим %s",
			i+1, len(groups), ps.Fold.EffectiveMode())
	}
}

// buildFoldGroups собирает записи групп для одной свёртки.
//
// Порядок как у двойников Направления: автогруппа идёт ПЕРЕД селектором —
// селектор ссылается на неё в addOutbounds, и конфиг читается сверху вниз.
func buildFoldGroups(
	fold configtypes.SourceFold,
	tagPrefix string,
	sourceIndex int,
	tmplAutoOptions map[string]interface{},
) []configtypes.Direction {
	autoTag := configtypes.FoldAutoTag(tagPrefix, sourceIndex)
	selectTag := configtypes.FoldSelectTag(tagPrefix, sourceIndex)
	// TEMPORARY BRIDGE (SPEC 118 W2-W4), удаляется в W5: свёртка,
	// мигрированная в FolderReplace, несёт МАТЕРИАЛИЗОВАННЫЕ теги — они
	// стабильны при перестановке источников и совпадают со ссылками,
	// переписанными миграцией.
	if fold.SelectTagOverride != "" {
		selectTag = fold.SelectTagOverride
	}
	if fold.AutoTagOverride != "" {
		autoTag = fold.AutoTagOverride
	}

	var out []configtypes.Direction

	if fold.HasAuto() {
		auto := fold.Auto
		if auto == nil {
			auto = &configtypes.DirectionAuto{}
		}
		// buildTwin делает ровно то, что нужно: сливает шаблонные опции с
		// пользовательскими и раскрывает round_robin в mode+balancer.
		// Фильтров у группы подписки нет — в неё входят все её узлы.
		parent := configtypes.Direction{Tag: selectTag, Auto: auto}
		twin := buildTwin(parent, autoTag, tmplAutoOptions)
		// TwinOf у группы подписки не ставим: он помечает производную
		// запись Направления и меняет её обработку в проходах 1–3
		// (исключение из пула, отказ от expose-кредита). Здесь запись —
		// самостоятельная локальная группа.
		twin.TwinOf = ""
		twin.Filters = nil
		// Маркер ставим ТОЛЬКО когда автогруппа — самостоятельный итог
		// свёртки. При select_auto она внутренняя опция селектора, и
		// маркер сделал бы её вторым кандидатом в Направления: пользователь
		// получил бы в списке и подписку, и её же автовыбор — выбор,
		// которого он не просил. Комментарий без маркера сохраняем: он
		// читается человеком на вкладке JSON.
		if fold.EffectiveMode() == configtypes.FoldModeAuto {
			twin.Comment = "local auto " + foldMarkerAuto
		} else {
			twin.Comment = "local auto"
		}
		out = append(out, twin)
	}

	if fold.HasSelect() {
		sel := configtypes.Direction{
			Tag:     selectTag,
			Type:    "selector",
			Filters: map[string]interface{}{},
			Options: map[string]interface{}{
				"interrupt_exist_connections": true,
			},
			Comment: "local select " + foldMarkerSelect,
		}
		if fold.HasAuto() {
			// Автогруппа — первой опцией и умолчанием: ради неё режим и
			// выбирали (S7).
			sel.AddOutbounds = []string{autoTag}
			sel.Options["default"] = autoTag
		}
		out = append(out, sel)
	}

	return out
}

// dropFoldGroups убирает записи с маркерами локальных групп.
func dropFoldGroups(outbounds []configtypes.Direction) []configtypes.Direction {
	if len(outbounds) == 0 {
		return outbounds
	}
	kept := make([]configtypes.Direction, 0, len(outbounds))
	for _, ob := range outbounds {
		if commentHasWizardLocalOutboundMarker(ob.Comment) {
			continue
		}
		kept = append(kept, ob)
	}
	return kept
}
