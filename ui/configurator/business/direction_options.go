// File direction_options.go — опции Направления: только объявленные корневые
// имена (NODE_LINK.md §8, решение владельца 15.09.2026).
//
// Направление на узлы не ссылается. Узлы в него набирает фильтр, а опции
// (addOutbounds) — это имена корня без узла за ними:
//
//   - теги Направлений и их парные `-auto`;
//   - теги свёрток (replace) и их `-auto`;
//   - системные и шаблонные теги: `direct-out`, тег блокировки шаблона,
//     outbound'ы и endpoint'ы самого шаблона, теги активных пресетов.
//
// Узел в опциях протухал бы от правки tag_policy и переноса, а перепись
// ссылок узла за ним не ходит. Форма Направления другого и не предлагает;
// здесь закрыт второй вход — сырой JSON.
package business

import (
	"fmt"
	"strings"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Отказы сохранения опций: ключ локали — английский текст.
const (
	directionOptionNodeText    = "node %q cannot be added to a Direction directly — use a filter"
	directionOptionUnknownText = "option %q not found"
)

// DeclaredRootNames — объявленные корневые имена модели (состав — в шапке
// файла). Выключенные Направления входят: это временно снятое своё имя.
//
// Не зовёт GetAvailableOutbounds: тот сам фильтрует опции по этому множеству.
func DeclaredRootNames(model *wizardmodels.WizardModel) map[string]bool {
	names := map[string]bool{wizardmodels.DefaultOutboundTag: true}
	add := func(tag string) {
		if tag = strings.TrimSpace(tag); tag != "" {
			names[tag] = true
		}
	}
	if model == nil {
		return names
	}
	for i := range model.GlobalOutbounds {
		d := &model.GlobalOutbounds[i]
		add(d.Tag)
		if d.Auto != nil && strings.TrimSpace(d.Tag) != "" {
			add(d.AutoTag())
		}
	}
	for _, tag := range ModelReplaceTags(model) {
		add(tag)
	}
	for _, tag := range collectActivePresetOutboundTags(model) {
		add(tag)
	}
	if td := model.TemplateData; td != nil {
		add(td.DirectionBlockTag())
		for _, tag := range td.SystemOutboundTags() {
			add(tag)
		}
		for _, d := range td.GlobalOutbounds() {
			add(d.Tag)
			if d.Auto != nil && strings.TrimSpace(d.Tag) != "" {
				add(d.AutoTag())
			}
		}
	}
	return names
}

// ValidateDirectionOptions — опции, которые Направление вправе сохранить.
//
// dirTag и hasAuto — тег и автовыбор САМОЙ сохраняемой записи: сырой JSON
// может переименовать Направление или включить ему `-auto` той же правкой, и
// модель об этом ещё не знает. Первая непринятая опция возвращается ошибкой
// на языке пользователя: узел — «используйте фильтр», прочее — «не найден».
func ValidateDirectionOptions(model *wizardmodels.WizardModel, dirTag string, hasAuto bool, options []string) error {
	declared := DeclaredRootNames(model)
	if tag := strings.TrimSpace(dirTag); tag != "" {
		declared[tag] = true
		if hasAuto {
			declared[tag+"-auto"] = true
		}
	}
	var nodes map[string]bool
	for _, raw := range options {
		opt := strings.TrimSpace(raw)
		if opt == "" || declared[opt] {
			continue
		}
		if nodes == nil {
			nodes = modelNodeNames(model)
		}
		if nodes[opt] {
			return fmt.Errorf(locale.T(directionOptionNodeText), opt)
		}
		return fmt.Errorf(locale.T(directionOptionUnknownText), opt)
	}
	return nil
}

// modelNodeNames — имена, под которыми пользователь мог вписать УЗЕЛ: теги
// корневых узлов, сырые и финальные теги членов контейнеров (без суффикса
// уникализации) и финальные теги собранного пула, если он есть. Нужны только
// для текста отказа: отказ случится и без них.
func modelNodeNames(model *wizardmodels.WizardModel) map[string]bool {
	out := map[string]bool{}
	if model == nil {
		return out
	}
	for _, tag := range ModelRootNodeTags(model) {
		out[tag] = true
	}
	for i := range model.Sources {
		src := &model.Sources[i]
		if src.Kind != corestate.SourceKindFolder && src.Kind != corestate.SourceKindSubscription {
			continue
		}
		for j := range src.Nodes {
			n := &src.Nodes[j]
			if raw := strings.TrimSpace(n.Tag); raw != "" {
				out[raw] = true
			}
			if final, ok := corestate.NodeLinkFinalTag(src.TagPolicy, n.Tag); ok {
				out[final] = true
			}
		}
	}
	for _, n := range model.NodePool {
		if n != nil && strings.TrimSpace(n.Tag) != "" {
			out[strings.TrimSpace(n.Tag)] = true
		}
	}
	return out
}
