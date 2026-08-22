// File source_fold_migrate.go — разворачивание прежних флагов подписки в
// Fold (SPEC 108), одноразово при загрузке состояния.
//
// Правило миграции: встретили старую комбинацию — привели к новой, обратно
// не сохраняем. Прежние локальные группы (`<PFX>auto` / `<PFX>select` с
// маркерами WIZARD:*) при этом удаляются из Source.Outbounds: с SPEC 108
// они не хранятся, а разворачиваются на сборке из Fold.
package state

import (
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// Маркеры локальных групп в comment (SPEC 026). Дублируются здесь, потому
// что state — leaf-пакет и ui/configurator/business импортировать не может;
// значения зашиты в старых state.json и меняться уже не будут.
const (
	wizardMarkerAuto         = "WIZARD:auto"
	wizardMarkerSelect       = "WIZARD:selector"
	wizardMarkerSelectLegacy = "WIZARD:select"
)

func commentIsWizardAuto(comment string) bool {
	return strings.Contains(comment, wizardMarkerAuto)
}

func commentIsWizardSelect(comment string) bool {
	return strings.Contains(comment, wizardMarkerSelect) || strings.Contains(comment, wizardMarkerSelectLegacy)
}

// migrateSourceFolds разворачивает прежние флаги подписок в Fold.
//
// Таблица (SPEC 108 §Миграция):
//
//	группы + Expose (с Exclude или без) → Fold по составу групп
//	группы без Expose                   → Fold нет, группы удаляются
//	                                      (они не доезжали до конфига)
//	Exclude без групп                   → Fold нет, флаг остаётся как есть
//
// Идемпотентна: у подписки с уже выставленным Fold только вычищаются
// остатки групп.
func migrateSourceFolds(s *State) {
	if s == nil {
		return
	}
	for i := range s.Connections.Sources {
		src := &s.Connections.Sources[i]
		if src.Type != SourceTypeSubscription {
			continue
		}

		hasAuto, hasSelect := false, false
		for _, ob := range src.Outbounds {
			if commentIsWizardAuto(ob.Comment) {
				hasAuto = true
			}
			if commentIsWizardSelect(ob.Comment) {
				hasSelect = true
			}
		}

		if src.Fold == nil && (hasAuto || hasSelect) && src.ExposeGroupTagsToGlobal {
			mode := configtypes.FoldModeSelect
			switch {
			case hasAuto && hasSelect:
				mode = configtypes.FoldModeSelectAuto
			case hasAuto:
				mode = configtypes.FoldModeAuto
			}
			src.Fold = &configtypes.SourceFold{Mode: mode}
			if hasAuto {
				// Параметры прежней urltest-группы переносим, чтобы
				// пользовательские url/interval/tolerance не сбросились
				// на шаблонные молча.
				src.Fold.Auto = autoFromLegacyGroup(src.Outbounds)
			}
			debuglog.DebugLog("SPEC 108: подписка %q свёрнута в группу (режим %s) из прежних флагов", src.ID, mode)
		}

		// Группы больше не хранятся в состоянии независимо от исхода:
		// с Fold они разворачиваются на сборке, без Fold — не доезжали до
		// конфига и раньше.
		if hasAuto || hasSelect {
			src.Outbounds = dropWizardGroups(src.Outbounds)
		}

		// Флаг Expose выражен свёрткой и больше не нужен. ExcludeFromGlobal
		// сохраняем: без групп он выражал «узлы подписки мимо общего
		// списка», а свёрткой это не описывается (см. SPEC 108 §Открытое).
		if src.Fold != nil {
			src.ExposeGroupTagsToGlobal = false
			src.ExcludeFromGlobal = false
		}
	}
}

// autoFromLegacyGroup достаёт параметры прежней urltest-группы подписки.
// Неизвестные и невыразимые ключи отбрасываются: форма настроек у свёртки
// та же, что у Направления, и лишнему в ней места нет.
func autoFromLegacyGroup(outbounds []configtypes.Direction) *configtypes.DirectionAuto {
	auto := &configtypes.DirectionAuto{}
	for _, ob := range outbounds {
		if !commentIsWizardAuto(ob.Comment) {
			continue
		}
		for k, v := range ob.Options {
			switch k {
			case "url":
				if s, ok := v.(string); ok {
					auto.URL = s
				}
			case "interval":
				if s, ok := v.(string); ok {
					auto.Interval = s
				}
			case "idle_timeout":
				if s, ok := v.(string); ok {
					auto.IdleTimeout = s
				}
			case "tolerance":
				if n, ok := numberValue(v); ok {
					auto.Tolerance = configtypes.NewTemplateInt(n)
				}
			case "interrupt_exist_connections":
				if b, ok := v.(bool); ok {
					flag := b
					auto.InterruptExistConnections = &flag
				}
			}
		}
		break
	}
	return auto
}

// numberValue приводит значение из распакованного JSON к int. json.Unmarshal
// в interface{} даёт float64, но состояние могло прийти и из кода, где лежал
// int — принимаем обе формы.
func numberValue(v interface{}) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	}
	return 0, false
}

func dropWizardGroups(outbounds []configtypes.Direction) []configtypes.Direction {
	kept := outbounds[:0]
	for _, ob := range outbounds {
		if commentIsWizardAuto(ob.Comment) || commentIsWizardSelect(ob.Comment) {
			continue
		}
		kept = append(kept, ob)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}
