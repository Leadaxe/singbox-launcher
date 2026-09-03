// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл outbound.go содержит функции для работы с outbounds:
//   - GetAvailableOutbounds - список доступных outbound тегов из canonical (GlobalOutbounds); мемо по ревизии модели
//   - EnsureDefaultAvailableOutbounds - обеспечивает наличие обязательных outbounds (direct-out, reject, drop)
//   - EnsureFinalSelected - обеспечивает выбранный final outbound в модели
//
// Эти функции работают с WizardModel (чистыми данными), без зависимостей от GUI.
// Используются в презентере при обновлении опций outbound для правил маршрутизации.
//
// Используется в:
//   - presentation/presenter_methods.go - RefreshOutboundOptions вызывает GetAvailableOutbounds и EnsureFinalSelected
//   - business/create_config.go - GetAvailableOutbounds используется при генерации конфигурации
package business

import (
	"sort"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// ResolveMergedOutbound — возвращает merged view одного global outbound по tag.
// Для referenced entries (ref != "") выполняет тот же pipeline что parseAndPreview
// делает для emit: deep-copy outbounds slice → MergeOutboundUpdatesInPlace
// (резолвит base из template/preset + flatten Updates). Returns копию body
// готовую для display в Edit dialog.
//
// SPEC 058-R-N: единая точка merge — не дублируем resolve логику в UI.
//
// Returns nil если model/template отсутствуют или entry с таким tag не найден.
func ResolveMergedOutbound(model *wizardmodels.WizardModel, tag string) *config.Direction {
	if model == nil || model.TemplateData == nil {
		return nil
	}
	// Найдём индекс в model.GlobalOutbounds (canonical view).
	idx := -1
	for i := range model.GlobalOutbounds {
		if model.GlobalOutbounds[i].Tag == tag {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	// Deep-copy entry чтобы Merge не мутировал state. Wrap в minimal ParserConfig
	// чтобы переиспользовать MergeOutboundUpdatesInPlace.
	entry := model.GlobalOutbounds[idx]
	if len(entry.Updates) > 0 {
		entry.Updates = append([]config.OutboundUpdate(nil), entry.Updates...)
	}
	pc := &config.ParserConfig{}
	pc.ParserConfig.Outbounds = []config.Direction{entry}
	build.MergeOutboundUpdatesInPlace(pc, model.TemplateData, model.Target)
	if len(pc.ParserConfig.Outbounds) == 0 {
		return nil
	}
	merged := pc.ParserConfig.Outbounds[0]
	return &merged
}

// GetAvailableOutbounds возвращает список доступных outbound тегов из модели.
// Читает canonical `model.GlobalOutbounds` (SPEC 117); результат кэшируется
// по ревизии модели (сброс — InvalidateNodePool / любая мутация).
func GetAvailableOutbounds(model *wizardmodels.WizardModel) []string {
	tags := map[string]struct{}{
		wizardmodels.DefaultOutboundTag: {},
		wizardmodels.RejectActionName:   {},
		"drop":                          {}, // Always include "drop" in available options
	}

	if model == nil {
		return sortedOutboundTagSlice(tags)
	}

	// Мемо валидно, пока ревизия модели не ушла вперёд (0 = пусто): любая
	// правка Направлений и preset-ref'ов поднимает ревизию, поэтому один
	// ключ покрывает все входы.
	if model.AvailableOutboundsMemoRev != 0 && model.AvailableOutboundsMemoRev == model.Revision && len(model.AvailableOutboundsMemoTags) > 0 {
		out := make([]string, len(model.AvailableOutboundsMemoTags))
		copy(out, model.AvailableOutboundsMemoTags)
		return out
	}

	// Направления (SPEC 104). Выключенные пропускаем: список целей обязан
	// совпадать с тем, что реально попадёт в config.json, иначе правило
	// укажет в никуда.
	//
	// Парные auto-группы (`<tag>-auto`) в список НЕ идут: двойник —
	// опция внутри своего направления, а не самостоятельная цель
	// (решение D-9А). В addOutbounds его тоже нет — он разворачивается
	// только на сборке.
	for i := range model.GlobalOutbounds {
		outbound := &model.GlobalOutbounds[i]
		if outbound.Disabled {
			continue
		}
		if outbound.Tag != "" {
			tags[outbound.Tag] = struct{}{}
		}
		for _, extra := range outbound.AddOutbounds {
			tags[extra] = struct{}{}
		}
	}
	// Теги ЗАМЕН свёрнутых папок целями правил не предлагаются: такая цель
	// живёт по чужим правилам жизненного цикла — исчезает вместе с папкой, а
	// правило молча указывает в никуда. Замена это группировка и сахар к
	// Направлению, а не самостоятельная цель (SPEC 108 S3). Осиротевшие
	// ссылки из старых состояний сбрасываются при загрузке
	// (state.resetForeignRuleTargets).

	// SPEC 056: добавляем теги от preset.outbounds[] mode=add активных
	// preset-ref'ов (mode=update не вводит новых тегов, только патчит
	// существующие). Без этого UI Rules tab не предложит "ru VPN 🇷🇺" из
	// ru-inside, и пользователь не сможет выбрать его в своих правилах.
	for _, tag := range collectActivePresetOutboundTags(model) {
		tags[tag] = struct{}{}
	}

	// Глобальные outbound'ы ШАБЛОНА (`parser_config.outbounds[]`).
	//
	// Без этого в списке не было `block-out` — единственной цели «заблокировать
	// соединение», которую шаблон объявляет: пользователь физически не мог
	// выбрать блокировку, хотя outbound в конфиг уезжал и работал. Список
	// собирался только из подписок и пресетов, а объявления шаблона в него не
	// попадали вовсе.
	//
	// `direct-out` тоже объявлен здесь, но он уже добавлен как умолчание —
	// map сам снимет дубль.
	if model.TemplateData != nil {
		for _, ob := range model.TemplateData.GlobalOutbounds() {
			if ob.Tag != "" {
				tags[ob.Tag] = struct{}{}
			}
		}
	}

	result := sortedOutboundTagSlice(tags)
	model.AvailableOutboundsMemoRev = model.Revision
	model.AvailableOutboundsMemoTags = append([]string(nil), result...)
	return result
}

// collectActivePresetOutboundTags возвращает outbound-теги от mode="add"
// entries активных (Enabled) preset-ref'ов в model.PresetRefs.
//
// Семантика (SPEC 056):
//   - Только mode="" (default add) и mode="add" вводят новые tag'и;
//     mode="update" патчит existing — не возвращает.
//   - Per-entry if/if_or фильтруется по varsMap (user override + preset.vars[].Default).
//   - @var в Tag-поле резолвится (rare, обычно tag — литерал).
//
// Дедуп делает caller (sortedOutboundTagSlice). Возвращает nil если нет
// active preset-refs или ни один не имеет preset.outbounds[].
func collectActivePresetOutboundTags(model *wizardmodels.WizardModel) []string {
	if model == nil || model.TemplateData == nil || len(model.PresetRefs) == 0 {
		return nil
	}
	presetByID := make(map[string]*wizardtemplate.Preset, len(model.TemplateData.Presets))
	for i := range model.TemplateData.Presets {
		presetByID[model.TemplateData.Presets[i].ID] = &model.TemplateData.Presets[i]
	}

	var out []string
	for _, ref := range model.PresetRefs {
		if ref == nil || !ref.Enabled {
			continue
		}
		preset, ok := presetByID[ref.Ref]
		if !ok || len(preset.Outbounds) == 0 {
			continue
		}

		// Build varsMap: user override или preset.vars[].Default.
		varsMap := make(map[string]string, len(preset.Vars))
		for _, v := range preset.Vars {
			if val, has := ref.Vars[v.Name]; has && val != "" {
				varsMap[v.Name] = val
			} else {
				varsMap[v.Name] = v.Default
			}
		}

		for _, ob := range preset.Outbounds {
			mode := ob.Mode
			if mode == "" {
				mode = "add"
			}
			if mode != "add" {
				continue
			}
			if !evalPresetOutboundIf(ob.If, ob.IfOr, varsMap) {
				continue
			}
			tag := ob.Tag
			if strings.HasPrefix(tag, "@") {
				if val, has := varsMap[tag[1:]]; has {
					tag = val
				}
			}
			if tag != "" {
				out = append(out, tag)
			}
		}
	}
	return out
}

// evalPresetOutboundIf — preset outbound if/if_or activation. Delegates to the
// shared template.EvalIf (single source of truth with the build pipeline) so UI
// preview and server-side emit never diverge.
func evalPresetOutboundIf(ifList, ifOrList []string, varsMap map[string]string) bool {
	return wizardtemplate.EvalIf(ifList, ifOrList, varsMap)
}

func sortedOutboundTagSlice(tags map[string]struct{}) []string {
	result := make([]string, 0, len(tags))
	for tag := range tags {
		// SPEC 110 T6: `<chain>#<i>` — рантайм-звенья цепочки. В конфиге их
		// нет, и правило на такой тег не даст ядру стартовать. Прийти сюда
		// они могут из импортированного конфига или чужого шаблона.
		if config.ChainInternalTag(tag) {
			continue
		}
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

// AllDirectionTags — теги ВСЕХ Направлений, включая выключенные.
//
// Отдельно от GetAvailableOutbounds намеренно: тот отдаёт цели, которые
// реально попадут в config.json (выключенные пропущены), а для защиты правил
// от необратимого сброса (SPEC 108 S5) выключенное Направление — временно
// снятая своя цель, а не чужая.
func AllDirectionTags(model *wizardmodels.WizardModel) []string {
	if model == nil {
		return nil
	}
	var out []string
	for i := range model.GlobalOutbounds {
		if tag := model.GlobalOutbounds[i].Tag; tag != "" {
			out = append(out, tag)
		}
	}
	// Теги ЗАМЕН свёрнутых папок сюда НЕ входят намеренно: цели чужого
	// жизненного цикла — ровно то, ради чьего сброса resetForeignRuleTargets
	// и существует (SPEC 108 S5). Включить их значило бы защитить от сброса
	// осиротевшие ссылки на `AL:select`.
	return out
}

// EnsureDefaultAvailableOutbounds обеспечивает наличие дефолтных outbounds в списке.
func EnsureDefaultAvailableOutbounds(outbounds []string) []string {
	if len(outbounds) == 0 {
		return []string{wizardmodels.DefaultOutboundTag, wizardmodels.RejectActionName}
	}
	return outbounds
}

// EnsureFinalSelected обеспечивает, что final outbound выбран из доступных опций.
func EnsureFinalSelected(model *wizardmodels.WizardModel, options []string) {
	options = EnsureDefaultAvailableOutbounds(options)
	preferred := model.SelectedFinalOutbound
	if preferred == "" && model.TemplateData != nil && model.TemplateData.DefaultFinal != "" {
		preferred = model.TemplateData.DefaultFinal
	}
	if preferred == "" {
		preferred = wizardmodels.DefaultOutboundTag
	}
	if !containsString(options, preferred) {
		if model.TemplateData != nil && model.TemplateData.DefaultFinal != "" && containsString(options, model.TemplateData.DefaultFinal) {
			preferred = model.TemplateData.DefaultFinal
		} else if containsString(options, wizardmodels.DefaultOutboundTag) {
			preferred = wizardmodels.DefaultOutboundTag
		} else {
			preferred = options[0]
		}
	}
	model.SelectedFinalOutbound = preferred
}

// containsString проверяет, содержит ли слайс строк целевую строку.
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
