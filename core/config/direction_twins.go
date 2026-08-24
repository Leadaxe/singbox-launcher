// Package config: direction_twins.go — разворачивание парных auto-групп
// Направлений (SPEC 104, проход 0).
//
// Направление с включённым автовыбором эмитит в конфиг ДВЕ группы: свой
// `selector` и `urltest` с тем же составом узлов под тегом `<tag>-auto`.
// Двойник не хранится в состоянии: пользователь настраивает одно
// направление, а не два объекта, которые обязаны оставаться синхронными.
// Поэтому он разворачивается здесь — до всех проходов генератора, чтобы
// пройти и подсчёт валидности, и генерацию JSON наравне с остальными.
//
// Двойник — опция ВНУТРИ своего направления и только его (решение D-9,
// вариант А): в состав других направлений он не предлагается и целью
// правил не бывает. Отдельно стоящие urltest-группы шаблона
// (`auto-proxy-out`) — не двойники: они самостоятельные записи и живут по
// прежним правилам.
package config

import (
	"encoding/json"
	"fmt"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// twinSuffix — окончание тега парной urltest-группы.
//
// Зеркалит `group_templates.magic_nodes.auto.tpl` ("{parent_tag}-auto") и
// configtypes.Direction.AutoTag. Значение зашито намеренно: на совпадении
// формулы держится узнавание двойников по тегу.
const twinSuffix = "-auto"

// PrepareDirections раскрывает глобальные Направления перед генерацией
// (SPEC 104, проход 0): выключенные выпадают, у остальных разворачиваются
// парные auto-группы.
//
// Мутирует переданный ParserConfig — тем же образом и в том же месте, что
// и SubstituteParserConfigPlaceholders: вызывающие передают сюда копию,
// собранную для генерации, а не пользовательское состояние.
//
// twinOptions — опции auto-группы из `group_templates.auto.options`
// шаблона. Пакет config о шаблоне не знает и знать не должен, поэтому
// словарь приезжает параметром от слоя выше. nil = двойник получает только
// поля, заданные пользователем.
func PrepareDirections(parserConfig *ParserConfig, twinOptions map[string]interface{}) {
	if parserConfig == nil {
		return
	}
	out := parserConfig.ParserConfig.Outbounds[:0:0]
	for _, d := range parserConfig.ParserConfig.Outbounds {
		if d.Disabled {
			// Выключенное Направление не материализуется вовсе. Правила,
			// на него ссылавшиеся, чистит CleanDanglingOutboundsInRouteRules.
			debuglog.DebugLog("directions: %q выключено — пропускаем", d.Tag)
			continue
		}
		out = append(out, d)
	}
	parserConfig.ParserConfig.Outbounds = ExpandDirectionTwins(out, twinOptions)
}

// ExpandDirectionTwins возвращает список глобальных направлений, дополненный
// парными urltest-группами.
//
// Двойник ставится ПЕРЕД родителем: селектор ссылается на него в
// addOutbounds, а порядок в массиве outbounds для ядра не значим — зато
// конфиг читается сверху вниз, и группа появляется раньше ссылки на неё.
//
// Возвращает новый слайс; вход не мутируется (родителю дописывается ссылка
// на двойник, и мутация на месте испортила бы состояние пользователя).
func ExpandDirectionTwins(directions []configtypes.Direction, tmplAutoOptions map[string]interface{}) []configtypes.Direction {
	hasTwin := false
	for i := range directions {
		if directions[i].Auto != nil {
			hasTwin = true
			break
		}
	}
	if !hasTwin {
		// Ни одного двойника — отдаём вход как есть, чтобы конфиги без
		// автовыбора собирались байт-в-байт как раньше.
		return directions
	}

	// Занятые теги: двойник не должен затирать существующую запись с тем же
	// именем (например, руками созданный `vpn-1-auto` в шаблоне).
	used := make(map[string]bool, len(directions))
	for _, d := range directions {
		used[d.Tag] = true
	}

	out := make([]configtypes.Direction, 0, len(directions)*2)
	for _, d := range directions {
		if d.Auto == nil || d.Tag == "" {
			out = append(out, d)
			continue
		}
		twinTag := d.Tag + twinSuffix
		if used[twinTag] {
			debuglog.WarnLog("directions: %q уже занят — автогруппа для %q не создаётся", twinTag, d.Tag)
			out = append(out, d)
			continue
		}

		out = append(out, buildTwin(d, twinTag, tmplAutoOptions))

		// Родитель получает ссылку на двойника ПЕРВОЙ опцией: автовыбор —
		// то, ради чего его включали, и он должен быть на виду. Если
		// двойник окажется пустым, ссылка отфильтруется на проходе 3
		// вместе с прочими невалидными.
		parent := d
		parent.AddOutbounds = prependUnique(twinTag, parent.AddOutbounds)
		parent.TwinTag = twinTag
		out = append(out, parent)
	}
	return out
}

// buildTwin собирает запись двойника.
//
// Состав узлов — тот же фильтр, что у родителя: двойник меряет задержку тех
// же серверов, между которыми выбирает селектор. Опции берутся из шаблона
// (`group_templates.auto.options`) и перекрываются заданными полями Auto:
// настройка пользователя важнее умолчания, а незаданное поле означает
// «пусть решает шаблон», а не «ноль».
func buildTwin(parent configtypes.Direction, twinTag string, tmplAutoOptions map[string]interface{}) configtypes.Direction {
	a := parent.Auto
	options := make(map[string]interface{}, len(tmplAutoOptions)+8)
	for k, v := range tmplAutoOptions {
		options[k] = v
	}
	if a.URL != "" {
		options["url"] = a.URL
	}
	if a.Interval != "" {
		options["interval"] = a.Interval
	}
	// Значение может быть числом или ссылкой на переменную шаблона —
	// кладём как есть, подстановка идёт следующим шагом.
	if v := a.Tolerance.Value(); v != nil {
		options["tolerance"] = v
	}
	if a.IdleTimeout != "" {
		options["idle_timeout"] = a.IdleTimeout
	}
	if a.InterruptExistConnections != nil {
		options["interrupt_exist_connections"] = *a.InterruptExistConnections
	}

	// SPEC 088: round_robin — это тот же wire-тип urltest плюс `mode` и
	// `balancer{}`. При least_test не пишем ничего: конфиг остаётся
	// бит-в-бит апстримным.
	if a.Mode == configtypes.AutoModeRoundRobin {
		options["mode"] = configtypes.AutoModeRoundRobin
		sticky := a.StickyHash
		if len(sticky) == 0 {
			// Контракт ядра: пустой список неотличим от отсутствия поля и
			// схлопывается в дефолт ["process","domain"]. Чтобы ВЫКЛЮЧИТЬ
			// липкость, нужен sentinel ["none"].
			sticky = []string{"none"}
		}
		balancer := map[string]interface{}{
			"pool":        a.Pool,
			"sticky_hash": sticky,
		}
		if v := a.PoolTolerance.Value(); v != nil {
			balancer["pool_tolerance"] = v
		} else {
			balancer["pool_tolerance"] = 0
		}
		options["balancer"] = balancer
	}

	return configtypes.Direction{
		Tag:     twinTag,
		Type:    "urltest",
		Filters: parent.Filters,
		Options: options,
		Comment: parent.DisplayName() + " — auto-pick",

		// TwinOf помечает запись как производную: она не предлагается
		// опцией другим направлениям и не принимает в состав группы
		// подписок (urltest внутри urltest мерил бы уже выбранный группой
		// узел, а не сервер — частность, перенесённая из LxBox §322).
		TwinOf: parent.Tag,
	}
}

// prependUnique ставит tag первым в списке, не создавая дубля.
func prependUnique(tag string, list []string) []string {
	out := make([]string, 0, len(list)+1)
	out = append(out, tag)
	for _, t := range list {
		if t != tag {
			out = append(out, t)
		}
	}
	return out
}

// DirectionBuildOptions — то, что генератору нужно знать о Направлениях от
// шаблона (SPEC 104).
//
// Одна структура вместо россыпи параметров: точек вызова генератора четыре,
// и каждый новый аргумент пришлось бы протаскивать через все. Нулевое
// значение рабочее — направления соберутся из того, что задал пользователь,
// а запасной состав пустого направления не появится (нечем).
type DirectionBuildOptions struct {
	// TwinOptions — `group_templates.auto.options` шаблона: умолчания
	// парной urltest-группы, включая ссылки на переменные ("@urltest_url").
	TwinOptions map[string]interface{}

	// BlockTag / DirectTag — теги служебных outbound'ов принимающего
	// конфига (`group_templates.magic_nodes.{block,direct}`). Имена не
	// универсальны — в лаунчере это `block-out` и `direct-out`, — и
	// зашивать их в код значило бы сломать чужой шаблон.
	BlockTag  string
	DirectTag string
}

// isSelectorType — тип, у которого пустой состав можно заменить запасным.
//
// Пустой selector ядро отвергает, но заглушки в нём осмысленны; пустой
// urltest тоже отвергает, а мерить задержку до заглушек бессмысленно.
func isSelectorType(t string) bool { return t == "" || t == "selector" }

// emptyDirectionFallbackJSON собирает запасной состав пустого Направления:
// [block, direct] с default=block.
//
// Пусто, если шаблон не объявил тег блокировки: выдумывать `block-out` за
// автора шаблона нельзя — ссылка в никуда роняет весь конфиг.
func emptyDirectionFallbackJSON(d configtypes.Direction, opts DirectionBuildOptions) string {
	if opts.BlockTag == "" {
		return ""
	}
	members := []string{opts.BlockTag}
	if opts.DirectTag != "" {
		members = append(members, opts.DirectTag)
	}

	obType := d.Type
	if obType == "" {
		obType = "selector"
	}
	membersJSON, err := json.Marshal(members)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(`{"tag":%s,"type":%s,"default":%s,"outbounds":%s}`,
		marshalJSONString(d.Tag), marshalJSONString(obType),
		marshalJSONString(opts.BlockTag), string(membersJSON))
}

// warnEmptyDirection говорит пользователю, что Направление осталось без
// узлов, — но только когда виноват ЕГО фильтр.
//
// Пустой фильтр при нуле узлов — это «подписка не загрузилась», и валить
// это на настройку направления значит сбивать с толку: чинить надо
// подписку. Частность перенесена из LxBox (§200/§274) вместе со смыслом.
// Возвращает true, если предупреждение выдано — тогда направление
// попадает в OutboundGenerationResult.EmptyDirections и доезжает до UI.
func warnEmptyDirection(d configtypes.Direction, poolSize int) bool {
	body, _ := configtypes.DirectionFilterTag(d.Filters)
	if body == "" || poolSize == 0 {
		return false
	}
	debuglog.WarnLog(
		"directions: %q (%s) — фильтр %q не поймал ни одного узла из %d; трафик будет заблокирован (default). Проверьте отбор узлов.",
		d.DisplayName(), d.Tag, body, poolSize)
	return true
}

// dropGroupNodes убирает узлы, которые сами являются группами выбора
// (SPEC 094 A5: `selector`/`urltest` из импортированного конфига приходят
// обычными узлами со схемой SchemeGroup).
//
// Нужно только для auto-групп Направления: измерять задержку до группы
// бессмысленно — ответит тот узел, который она сейчас выбрала, и лидер
// urltest'а стал бы отражать чужой выбор, а не скорость сервера.
func dropGroupNodes(nodes []*configtypes.ParsedNode) []*configtypes.ParsedNode {
	out := make([]*configtypes.ParsedNode, 0, len(nodes))
	for _, n := range nodes {
		if n != nil && n.Scheme == configtypes.SchemeGroup {
			continue
		}
		out = append(out, n)
	}
	return out
}
