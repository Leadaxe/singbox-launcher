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
// в форме. Список мест ниже обязан оставаться полным: появилась новая ссылка
// на тег Направления — её место здесь, рядом с остальными, а не отдельной
// правкой в вызывающем коде (тот же принцип, что у граф-санитайзера сборки).
package business

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/config/configtypes"
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
func RenameDirection(model *wizardmodels.WizardModel, oldTag, newTag string) int {
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if model == nil || oldTag == "" || newTag == "" || oldTag == newTag {
		return 0
	}

	renamed := 0
	oldAuto := oldTag + "-auto"
	newAuto := newTag + "-auto"

	// 1. Сам тег + ссылки в опциях других Направлений (addOutbounds).
	//
	// Двойник переименовываем вместе с родителем: `<tag>-auto` выводится
	// из тега на каждой сборке, и ссылка на старое имя двойника осталась
	// бы висеть на несуществующей группе.
	//
	// Правки ровно две и обе canonical (SPEC 117): GlobalOutbounds и
	// ссылки модели (хопы, detour, DNS). Legacy-вид
	// model.ParserConfig — одноразовая проекция и здесь не трогается:
	// четвёртой копии имени больше не существует.
	renameIn := func(dirs []configtypes.Direction) {
		for i := range dirs {
			d := &dirs[i]
			if d.Tag == oldTag {
				d.Tag = newTag
			}
			for j, opt := range d.AddOutbounds {
				switch opt {
				case oldTag:
					d.AddOutbounds[j] = newTag
					renamed++
				case oldAuto:
					d.AddOutbounds[j] = newAuto
					renamed++
				}
			}
		}
	}
	renameIn(model.GlobalOutbounds)

	// 2. Цели правил.
	for _, rs := range model.CustomRules {
		if rs == nil {
			continue
		}
		switch rs.SelectedOutbound {
		case oldTag:
			rs.SelectedOutbound = newTag
			renamed++
		case oldAuto:
			rs.SelectedOutbound = newAuto
			renamed++
		}
	}

	// 3. Маршрут по умолчанию. Два места хранения одного значения
	// (SelectedFinalOutbound + SettingsVars["route_final"]) синхронны по
	// построению — правим оба, иначе одно перебьёт другое на сохранении.
	switch model.SelectedFinalOutbound {
	case oldTag:
		model.SelectedFinalOutbound = newTag
		renamed++
	case oldAuto:
		model.SelectedFinalOutbound = newAuto
		renamed++
	}
	if model.SettingsVars != nil {
		switch model.SettingsVars["route_final"] {
		case oldTag:
			model.SettingsVars["route_final"] = newTag
		case oldAuto:
			model.SettingsVars["route_final"] = newAuto
		}
	}

	// 4. Outbound-переменные пресетов (preset.vars[].type == "outbound").
	//
	// Тип переменной здесь не проверяем: значение, совпавшее с тегом
	// Направления, и есть ссылка на него — а переменная другого типа со
	// значением ровно в тег означала бы то же самое.
	for _, ref := range model.PresetRefs {
		if ref == nil {
			continue
		}
		for name, val := range ref.Vars {
			switch val {
			case oldTag:
				ref.Vars[name] = newTag
				renamed++
			case oldAuto:
				ref.Vars[name] = newAuto
				renamed++
			}
		}
	}

	// 5. Позиции цепочек: хоп может вести в Направление. Ссылка корневого
	// пространства (FolderID пуст) — единственная форма, которой это
	// касается: хоп на узел папки адресуется её id и от переименования
	// Направления не зависит.
	for i := range model.Sources {
		hops := model.Sources[i].Hops
		for j := range hops {
			if hops[j].FolderID != "" {
				continue
			}
			switch hops[j].Tag {
			case oldTag:
				hops[j].Tag = newTag
				renamed++
			case oldAuto:
				hops[j].Tag = newAuto
				renamed++
			}
		}
	}

	// 6. Ссылки detour источников: цель дозвона тоже может быть
	// Направлением, и после переименования она повисла бы — на сборке это
	// fail-closed, то есть источник молча выпал бы из конфига.
	for i := range model.Sources {
		link := model.Sources[i].Detour
		if link == nil || link.FolderID != "" {
			continue
		}
		switch link.Tag {
		case oldTag:
			link.Tag = newTag
			renamed++
		case oldAuto:
			link.Tag = newAuto
			renamed++
		}
	}

	// 7. detour DNS-серверов.
	renamed += renameDNSDetour(model, oldTag, newTag, oldAuto, newAuto)

	return renamed
}

// renameDNSDetour переписывает поле detour в DNS-серверах.
//
// Серверы хранятся сырым JSON (форма записи задаётся шаблоном и ядром, а не
// нашей структурой), поэтому правим точечно: разбираем в map, меняем одно
// поле, собираем обратно. Сервер, который не разобрался или не ссылается на
// переименованное Направление, остаётся байт-в-байт прежним — переписывать
// чужой JSON целиком ради несделанной правки значит менять форматирование и
// порядок ключей на ровном месте.
func renameDNSDetour(model *wizardmodels.WizardModel, oldTag, newTag, oldAuto, newAuto string) int {
	renamed := 0
	for i, raw := range model.DNSServers {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		rawDetour, ok := obj["detour"]
		if !ok {
			continue
		}
		var detour string
		if err := json.Unmarshal(rawDetour, &detour); err != nil {
			continue
		}
		var replacement string
		switch detour {
		case oldTag:
			replacement = newTag
		case oldAuto:
			replacement = newAuto
		default:
			continue
		}
		encoded, err := json.Marshal(replacement)
		if err != nil {
			continue
		}
		obj["detour"] = encoded
		updated, err := json.Marshal(obj)
		if err != nil {
			continue
		}
		model.DNSServers[i] = updated
		renamed++
	}
	return renamed
}
