// File folder_replaces.go — свёртка папки (FolderReplace) на сборке
// (SPEC 118 W4, Т5; features/directions.md §5).
//
// Свёрнутая папка отдаёт Направлениям не свои узлы, а ЗАМЕНУ: селектор,
// авто-группу или пару «селектор + авто-двойник». Как и твины Направлений,
// замена не хранится в состоянии — она разворачивается здесь, на каждой
// сборке: пользователь настраивает один объект, а не два, обязанных
// оставаться синхронными.
//
// # Чем отличается от умершего PrepareSourceFolds
//
//   - тег замены ЯВНЫЙ (`replace.tag`), а не позиционный дериватив: он не
//     зависит от места папки в списке и переживает перестановку источников;
//   - маркеры `WIZARD:auto`/`WIZARD:selector` в comment не пишутся вовсе —
//     пул кандидатов Направлений решает ПРАВИЛО (outbound_filter.go), а не
//     метка в тексте;
//   - `ExcludeFromGlobal`/`ExposeGroupTagsToGlobal` не выставляются: тем же
//     правилом пула узлы свёрнутой папки из него уходят, а её replace-теги
//     приходят.
//
// Узлы свёрнутой папки при этом эмитятся в outbounds как обычно и остаются
// законными целями цепочек, detour и членами Auto (§5 features).
package config

import (
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// PrepareFolderReplaces разворачивает свёртки папок в локальные группы.
//
// Мутирует переданный ParserConfig — как PrepareDirections и
// PrepareSourceFolds, вызывается по копии, собранной для генерации.
//
// tmplAutoOptions — те же `group_templates.auto.options` шаблона, что у
// твинов Направлений: авто-группа замены и авто-группа Направления — одна и
// та же настройка на двух уровнях.
func PrepareFolderReplaces(parserConfig *ParserConfig, tmplAutoOptions map[string]interface{}) {
	if parserConfig == nil {
		return
	}
	for i := range parserConfig.ParserConfig.Proxies {
		ps := &parserConfig.ParserConfig.Proxies[i]
		if ps.Disabled || ps.Canonical == nil || ps.Canonical.Replace == nil {
			continue
		}
		groups := buildReplaceGroups(*ps.Canonical.Replace, tmplAutoOptions)
		if len(groups) == 0 {
			continue
		}
		ps.Outbounds = append(ps.Outbounds, groups...)
		debuglog.DebugLog("SPEC 118: папка %d свёрнута в %d группу(ы), режим %s",
			i+1, len(groups), ps.Canonical.Replace.Mode)
	}
}

// buildReplaceGroups собирает записи групп одной свёртки.
//
// Порядок как у твинов Направления: авто-группа идёт ПЕРЕД селектором —
// селектор ссылается на неё в addOutbounds, и конфиг читается сверху вниз.
func buildReplaceGroups(replace configtypes.FolderReplace, tmplAutoOptions map[string]interface{}) []configtypes.Direction {
	selectTag := replace.Tag
	if selectTag == "" {
		debuglog.WarnLog("replace: свёртка без тега — замена не создаётся")
		return nil
	}
	autoTag := selectTag
	if replace.Mode == configtypes.FolderReplaceBoth {
		// Двойник получает деривативный тег той же формулой, что твины
		// Направлений (direction_twins.go): на совпадении формулы держится
		// узнавание пар по тегу.
		autoTag = selectTag + twinSuffix
	}

	hasAuto := replace.Mode == configtypes.FolderReplaceAuto || replace.Mode == configtypes.FolderReplaceBoth
	hasSelect := replace.Mode == configtypes.FolderReplaceManual || replace.Mode == configtypes.FolderReplaceBoth

	var out []configtypes.Direction

	if hasAuto {
		auto := replace.Strategy
		if auto == nil {
			auto = &configtypes.DirectionAuto{}
		}
		// buildTwin делает ровно нужное: сливает шаблонные опции с
		// пользовательскими и раскрывает round_robin в mode+balancer.
		// Фильтров у замены нет — пул замены это ВСЯ папка (§5 features).
		parent := configtypes.Direction{Tag: selectTag, Auto: auto}
		twin := buildTwin(parent, autoTag, tmplAutoOptions)
		// TwinOf у группы замены не ставим: он помечает производную запись
		// Направления и меняет её обработку в проходах 1–3. Здесь запись —
		// самостоятельная локальная группа.
		twin.TwinOf = ""
		twin.Filters = nil
		// Авто-состав замены исключает Auto-узлы своей папки (§5 features):
		// измеритель поверх чужой группы мерил бы её выбор, а не маршрут.
		twin.NoGroupMembers = true
		twin.Comment = "folder replace auto"
		out = append(out, twin)
	}

	if hasSelect {
		sel := configtypes.Direction{
			Tag:     selectTag,
			Type:    "selector",
			Filters: map[string]interface{}{},
			// Собственных селекторных полей у replace нет: селекторная
			// половина наследует опции из шаблонных настроек групп (§5).
			Options: map[string]interface{}{
				"interrupt_exist_connections": true,
			},
			Comment: "folder replace select",
		}
		if hasAuto {
			// Авто-двойник — первой опцией и умолчанием: ради него режим
			// both и выбирают.
			sel.AddOutbounds = []string{autoTag}
			sel.Options["default"] = autoTag
		}
		out = append(out, sel)
	}

	return out
}

// FolderReplaceTags — теги замены одной свёртки: селекторный и/или
// авто-двойник. Пусто, если свёртки нет.
//
// Одна формула на всех потребителей (пул кандидатов, гард занятости, реестр
// известных целей): разойдись они — и Направление смогло бы занять имя
// замены, дав ядру два outbound'а с одним тегом.
func FolderReplaceTags(replace *configtypes.FolderReplace) []string {
	if replace == nil || replace.Tag == "" {
		return nil
	}
	switch replace.Mode {
	case configtypes.FolderReplaceAuto:
		return []string{replace.Tag}
	case configtypes.FolderReplaceBoth:
		return []string{replace.Tag, replace.Tag + twinSuffix}
	default: // manual
		return []string{replace.Tag}
	}
}

// FolderReplacePoolTag — тег, которым свёрнутая папка представлена В ПУЛЕ
// кандидатов Направлений: ИТОГ свёртки.
//
// При both это селектор (авто-двойник — его внутренняя опция, а не второй
// кандидат); при auto — авто-группа; при manual — селектор.
func FolderReplacePoolTag(replace *configtypes.FolderReplace) string {
	if replace == nil {
		return ""
	}
	return replace.Tag
}

// FolderReplaceGroupTags — теги замен, которые считаются ГРУППОВЫМИ
// кандидатами: их исключают твины Направлений (авто-измеритель поверх чужой
// группы мерил бы чужой выбор, а не свой маршрут — §4 features).
//
// Групповые — все теги замены: и селектор, и авто-двойник.
func FolderReplaceGroupTags(proxies []ProxySource) map[string]bool {
	out := make(map[string]bool)
	for i := range proxies {
		if proxies[i].Canonical == nil {
			continue
		}
		for _, tag := range FolderReplaceTags(proxies[i].Canonical.Replace) {
			out[tag] = true
		}
	}
	return out
}
