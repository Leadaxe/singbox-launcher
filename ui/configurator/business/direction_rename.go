// File direction_rename.go — переименование Направления вместе со всеми
// ссылками на его тег.
//
// У Направления ровно одно имя — тег (контракт 0.9.0), поэтому «переименовать»
// значит сменить тег. А тег это ССЫЛОЧНОЕ имя: на него смотрят правила,
// route.final, опции других Направлений, detour DNS-серверов, позиции цепочек
// и outbound-переменные пресетов. Сменить его в одном месте и не тронуть
// остальные — значит оставить ссылки в никуда: правило молча уедет на
// умолчание, цепочка не соберётся, DNS-сервер потеряет маршрут.
//
// Поэтому переименование — ОДНА операция над всей моделью, а не правка поля
// в форме. Список мест один на все корневые имена (root_name_refs.go):
// появилась новая ссылка на корневое имя — её место там, а не отдельной
// правкой здесь или в вызывающем коде.
package business

import (
	"strings"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// DirectionTagTaken — занят ли тег кем-то, кроме владельца exceptTag
// (передайте "" при создании нового Направления).
//
// Проверяем по ВСЕМ целям, а не только по Направлениям: тег обязан быть
// уникален среди всего, на что может сослаться правило. Совпади он с узлом
// подписки или служебным `direct-out` — сборка склеила бы две разные
// сущности под одним именем, и какая из них попадёт в конфиг, зависело бы
// от порядка обхода.
func DirectionTagTaken(model *wizardmodels.WizardModel, tag, exceptTag string) bool {
	tag = strings.TrimSpace(tag)
	exceptTag = strings.TrimSpace(exceptTag)
	if model == nil || tag == "" {
		return false
	}
	// Своё же имя не занято: открыть форму и сохранить, не трогая тег, —
	// обычный сценарий, а не конфликт.
	if tag == exceptTag {
		return false
	}
	// Canonical список Направлений — единственный актуальный (SPEC 117):
	// формы правят его же, отдельного «более свежего» вида больше нет.
	dirs := model.GlobalOutbounds
	for i := range dirs {
		if dirs[i].Tag == exceptTag {
			continue
		}
		if dirs[i].Tag == tag {
			return true
		}
		// Парная auto-группа не хранится в состоянии, но занимает имя на
		// сборке: Направление с тегом `X-auto` рядом с Направлением `X`
		// столкнулось бы с его двойником.
		if dirs[i].Auto != nil && dirs[i].AutoTag() == tag {
			return true
		}
	}
	// SPEC 118 W4: ЕДИНЫЙ гард занятости (features/directions.md §8) —
	// replace-теги свёрнутых папок, их `-auto`-двойники и верхние узлы.
	// Направление `x` рядом с папкой, чья замена зовётся `x`, дало бы два
	// `x-auto`, и ядро отвергло бы весь конфиг: частная проверка «занят ли
	// тег среди Направлений» этого не видит по построению.
	//
	// Твин проверяем парой: занять `x`, когда чужой `x-auto` уже есть,
	// значит завести вторую группу с тем же именем на следующей сборке.
	owners := ModelTagOwners(model)
	// Собственные притязания владельца снимаем: открыть форму `x` и
	// сохранить, не трогая тег, — обычный сценарий, а не конфликт с самим
	// собой (то же и для его твина `x-auto`).
	if exceptTag != "" {
		delete(owners, exceptTag)
		delete(owners, exceptTag+"-auto")
	}
	if _, taken := owners[tag]; taken {
		return true
	}
	if _, taken := owners[tag+"-auto"]; taken {
		return true
	}
	// Служебные цели и всё, что уже предлагается целью правил (узлы,
	// теги пресетов, объявления шаблона).
	for _, known := range GetAvailableOutbounds(model) {
		if known == tag {
			return true
		}
	}
	return false
}

// RenameDirection меняет тег Направления с oldTag на newTag и переписывает
// все ссылки на него. Возвращает число переписанных ссылок (сам тег
// Направления не считается).
//
// No-op при пустых аргументах или oldTag == newTag. Вызывающий обязан
// заранее проверить newTag через DirectionTagTaken: молча слить два
// Направления в одно здесь было бы хуже, чем отказать в форме.
//
// Ссылки переписывает общий обход корневых имён (root_name_refs.go): цели
// правил, route.final, опции (addOutbounds), options.default и литеральный
// preferredDefault Направлений — в теле собственной записи и в USER-патче,
// переменные пресетов, detour DNS и NodeLink — detour корневых записей И
// членов папок, позиции цепочек в корне И в папках, члены корневых групп.
// Прежний частный список видел только addOutbounds собственных записей и
// корневые записи, и хоп цепочки в папке, detour члена папки, опция в
// USER-патче или умолчание селектора повисали на старом имени (NODE_LINK.md
// §9.3 п. 7).
func RenameDirection(model *wizardmodels.WizardModel, oldTag, newTag string) int {
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if model == nil || oldTag == "" || newTag == "" || oldTag == newTag {
		return 0
	}

	// Сам тег. Правка canonical (SPEC 117): GlobalOutbounds; legacy-вид
	// model.ParserConfig — одноразовая проекция и здесь не трогается.
	for i := range model.GlobalOutbounds {
		if model.GlobalOutbounds[i].Tag == oldTag {
			model.GlobalOutbounds[i].Tag = newTag
		}
	}

	// Двойник переименовываем вместе с родителем: `<tag>-auto` выводится из
	// тега на каждой сборке, и ссылка на старое имя двойника осталась бы
	// висеть на несуществующей группе.
	_, renamed := editRootNameRefs(model, rootRenames(map[string]string{
		oldTag:           newTag,
		oldTag + "-auto": newTag + "-auto",
	}))
	return renamed
}
