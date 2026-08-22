package build

// Материализация каналов в outbound-группы (SPEC 104).
//
// Канал — именованная точка выбора, на которую ссылаются правила. Ссылаться
// напрямую на узел подписки нельзя: его тег генерируется при разборе и
// меняется на каждом обновлении, так что правило разваливалось бы само собой.
//
// На сборке канал превращается в `selector` со списком подходящих узлов и,
// если включён автовыбор, в парную группу `urltest`.
//
// Логика перенесена из LxBox (§125/§201/§267/§322) вместе с её выстраданными
// частностями — каждая из них исправленный баг, а не стиль.

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// ChannelBuildInput — вход материализации.
type ChannelBuildInput struct {
	// Channels — каналы состояния в порядке пользователя.
	Channels []corestate.Channel
	// NodeTags — итоговые теги узлов, доступных каналам, в порядке конфига.
	NodeTags []string
	// GroupTags — теги узлов, которые сами являются группами выбора
	// (urltest подписки). В urltest-двойник канала они не попадают.
	GroupTags map[string]bool
	// Templates — описание групп из шаблона.
	Templates template.ChannelGroupTemplates
	// DirectTag / BlockTag — теги служебных outbound'ов принимающего
	// конфига. Берутся из шаблона: имена там не универсальны
	// (`direct-out`, `block-out`).
	DirectTag string
	BlockTag  string
}

// ChannelBuildResult — результат материализации.
type ChannelBuildResult struct {
	// Groups — готовые группы в порядке: канал, его auto, следующий канал…
	Groups []map[string]interface{}
	// Warnings — то, что пользователь должен узнать: фильтр, не поймавший
	// ни одного узла, и чем это обернётся.
	Warnings []string
	// EmptyChannels — метки каналов, оставшихся без узлов (для UI).
	EmptyChannels []string
}

// BuildChannelGroups превращает каналы в outbound-группы.
func BuildChannelGroups(in ChannelBuildInput) ChannelBuildResult {
	var res ChannelBuildResult

	for _, ch := range in.Channels {
		if !ch.Enabled || ch.Tag == "" {
			continue
		}

		members := selectChannelNodes(ch, in.NodeTags)

		// Узел автовыбора внутрь urltest не идёт: urltest поверх urltest
		// мерил бы задержку уже выбранного узла, а не сервера.
		autoMembers := make([]string, 0, len(members))
		for _, tag := range members {
			if !in.GroupTags[tag] {
				autoMembers = append(autoMembers, tag)
			}
		}
		emitAuto := ch.Auto != nil && len(autoMembers) > 0

		selectorOutbounds := make([]string, 0, len(members)+3)
		selectorOutbounds = append(selectorOutbounds, members...)
		if ch.IncludeDirect && in.DirectTag != "" {
			selectorOutbounds = append(selectorOutbounds, in.DirectTag)
		}
		if ch.IncludeBlock && in.BlockTag != "" {
			selectorOutbounds = append(selectorOutbounds, in.BlockTag)
		}
		if emitAuto {
			selectorOutbounds = append(selectorOutbounds, ch.AutoTag())
		}

		// Пустая группа — фатальна для ядра, поэтому у канала без единой
		// опции должен быть запасной состав. Первым идёт блокировка:
		// заблокировать безопаснее, чем выпустить трафик мимо VPN, когда
		// пользователь ожидал туннель.
		emptyFallback := len(selectorOutbounds) == 0
		if emptyFallback {
			if in.BlockTag != "" {
				selectorOutbounds = append(selectorOutbounds, in.BlockTag)
			}
			if in.DirectTag != "" {
				selectorOutbounds = append(selectorOutbounds, in.DirectTag)
			}
			if len(selectorOutbounds) == 0 {
				// Ни блокировки, ни direct в шаблоне — канал не построить.
				res.Warnings = append(res.Warnings,
					"Channel "+ch.DisplayLabel()+" ("+ch.Tag+"): no members and no fallback outbounds — skipped")
				continue
			}
		}

		// Предупреждаем, только когда виноват ИМЕННО фильтр: пустой фильтр
		// при нулевом числе узлов — это отсутствие подписки, а не ошибка
		// настройки, и кричать о ней бессмысленно.
		if len(members) == 0 && ch.NodeFilter != "" && len(in.NodeTags) > 0 {
			effect := "traffic is blocked (default)"
			if !emptyFallback && len(selectorOutbounds) > 0 && selectorOutbounds[0] == in.DirectTag {
				// Пользователь сам включил direct-опцию: врать про
				// блокировку нельзя — трафик пойдёт мимо VPN.
				effect = "traffic goes direct (no VPN hop)"
			}
			res.Warnings = append(res.Warnings,
				"Channel "+ch.DisplayLabel()+" ("+ch.Tag+"): node filter matched no nodes — "+effect+". Check its node filter.")
			res.EmptyChannels = append(res.EmptyChannels, ch.DisplayLabel())
		}

		selector := map[string]interface{}{
			"tag":       ch.Tag,
			"type":      groupType(in.Templates.Channel, "selector"),
			"outbounds": selectorOutbounds,
		}
		applyGroupOptions(selector, in.Templates.Channel.Options)
		selector["interrupt_exist_connections"] = ch.InterruptExistConnections
		if emptyFallback && in.BlockTag != "" {
			selector["default"] = in.BlockTag
		} else if def := matchDefault(ch, members); def != "" {
			selector["default"] = def
		}
		res.Groups = append(res.Groups, selector)

		if emitAuto {
			auto := map[string]interface{}{
				"tag":       ch.AutoTag(),
				"type":      groupType(in.Templates.Auto, "urltest"),
				"outbounds": autoMembers,
			}
			applyGroupOptions(auto, in.Templates.Auto.Options)
			applyChannelAuto(auto, ch.Auto)
			res.Groups = append(res.Groups, auto)
		}
	}

	return res
}

// selectChannelNodes отбирает узлы канала по фильтру, сохраняя порядок
// конфига: он осмыслен (порядок подписки), а сортировка по алфавиту
// перемешала бы локации.
func selectChannelNodes(ch corestate.Channel, nodeTags []string) []string {
	if ch.NodeFilter == "" {
		out := make([]string, len(nodeTags))
		copy(out, nodeTags)
		return out
	}
	re, err := regexp.Compile(ch.NodeFilter)
	if err != nil {
		// Невалидное выражение не должно ронять сборку: канал получает все
		// узлы, как при пустом фильтре. Иначе опечатка в поле фильтра
		// оставляла бы пользователя без конфига целиком.
		out := make([]string, len(nodeTags))
		copy(out, nodeTags)
		return out
	}
	out := make([]string, 0, len(nodeTags))
	for _, tag := range nodeTags {
		matched := re.MatchString(tag)
		if ch.NodeFilterInvert {
			matched = !matched
		}
		if matched {
			out = append(out, tag)
		}
	}
	return out
}

// matchDefault выбирает узел по умолчанию — первый совпавший с DefaultFilter.
func matchDefault(ch corestate.Channel, members []string) string {
	if ch.DefaultFilter == "" || len(members) == 0 {
		return ""
	}
	re, err := regexp.Compile(ch.DefaultFilter)
	if err != nil {
		return ""
	}
	for _, tag := range members {
		if re.MatchString(tag) {
			return tag
		}
	}
	return ""
}

func groupType(spec template.ChannelGroupSpec, fallback string) string {
	if t := strings.TrimSpace(spec.Type); t != "" {
		return t
	}
	return fallback
}

// applyGroupOptions переносит поля группы из шаблона.
//
// Значения кладутся как есть, включая ссылки на переменные ("@urltest_url"):
// подстановка — задача движка шаблонов, и делать её здесь значило бы завести
// вторую реализацию.
func applyGroupOptions(dst map[string]interface{}, options map[string]json.RawMessage) {
	if len(options) == 0 {
		return
	}
	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Strings(keys) // детерминированный порядок обхода
	for _, k := range keys {
		var v interface{}
		if err := json.Unmarshal(options[k], &v); err != nil {
			continue
		}
		dst[k] = v
	}
}

// applyChannelAuto накладывает пользовательские параметры автовыбора поверх
// шаблонных — настройка канала важнее умолчания.
func applyChannelAuto(dst map[string]interface{}, auto *corestate.ChannelAuto) {
	if auto == nil {
		return
	}
	if auto.URL != "" {
		dst["url"] = auto.URL
	}
	if auto.Interval != "" {
		dst["interval"] = auto.Interval
	}
	if auto.Tolerance > 0 {
		dst["tolerance"] = auto.Tolerance
	}
	if auto.IdleTimeout != "" {
		dst["idle_timeout"] = auto.IdleTimeout
	}
	dst["interrupt_exist_connections"] = auto.InterruptExistConnections
}

// buildChannelGroupStrings материализует каналы контекста в готовые
// JSON-строки outbound-групп.
//
// Возвращает пустой список, когда каналов нет: конфиг тогда собирается ровно
// как до SPEC 104, и шаблон без секции `group_templates` не замечает разницы.
func buildChannelGroupStrings(ctx BuildContext, cache *ParsedCache) []string {
	if len(ctx.Channels) == 0 {
		return nil
	}

	templates, ok := ctx.Template.ChannelTemplates()
	if !ok {
		// Шаблон не описывает каналы — материализовать нечем. Молча
		// пропускаем: это не ошибка пользователя, а несовместимый шаблон.
		return nil
	}

	nodeTags, groupTags := cacheNodeTags(cache)
	res := BuildChannelGroups(ChannelBuildInput{
		Channels:  ctx.Channels,
		NodeTags:  nodeTags,
		GroupTags: groupTags,
		Templates: templates,
		DirectTag: templateMagicTag(templates, "direct"),
		BlockTag:  templateMagicTag(templates, "block"),
	})

	out := make([]string, 0, len(res.Groups))
	for _, g := range res.Groups {
		raw, err := json.Marshal(g)
		if err != nil {
			continue
		}
		out = append(out, string(raw))
	}
	return out
}

// templateMagicTag достаёт тег служебной опции (direct/block) из шаблона.
//
// Имена не универсальны — в лаунчере это `direct-out` и `block-out`, — и
// зашивать их в код значило бы сломать чужой шаблон.
func templateMagicTag(t template.ChannelGroupTemplates, name string) string {
	node, ok := t.MagicNodes[name]
	if !ok {
		return ""
	}
	return node.ResolveTag("")
}

// cacheNodeTags возвращает теги узлов кэша и множество тех из них, что сами
// являются группами выбора.
func cacheNodeTags(cache *ParsedCache) ([]string, map[string]bool) {
	if cache == nil {
		return nil, nil
	}
	tags := make([]string, 0, len(cache.Outbounds))
	groups := make(map[string]bool)
	for _, raw := range cache.Outbounds {
		var probe struct {
			Tag  string `json:"tag"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(raw), &probe); err != nil || probe.Tag == "" {
			continue
		}
		tags = append(tags, probe.Tag)
		if probe.Type == "urltest" || probe.Type == "selector" {
			groups[probe.Tag] = true
		}
	}
	return tags, groups
}
