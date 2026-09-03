// File source_node_counts.go — счёт узлов на источник для списка Sources.
//
// Показывает, сколько узлов даёт источник и сколько из них реально пойдёт
// в конфиг: подписка на полсотни серверов, у которой половина выключена
// галками, выглядит в списке одинаково с полной, и понять это можно было
// только открыв её.
//
// SPEC 118 W5: считается ПРЯМО из материализованных `nodes[]` — разбирать
// нечего, состав узлов лежит в состоянии. Ленивый кэш с тремя состояниями
// («не готов / готов и пуст / есть») схлопнулся: данные есть всегда.
package business

import (
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// EnsureSourceNodeCounts считает счётчики, если их ещё нет.
//
// Возвращает true, если счёт был выполнен (значит список надо перерисовать).
// Повторные вызовы бесплатны.
//
// Кэш живёт до InvalidateSourceNodeCounts: его снимает всё, что меняет
// СОСТАВ узлов (правка источника, обновление подписки, снятая галка ноды).
func EnsureSourceNodeCounts(model *wizardmodels.WizardModel) bool {
	if model == nil || model.SourceNodeCounts != nil {
		return false
	}

	counts := make(map[int]wizardmodels.SourceNodeCount, len(model.Sources))
	for idx := range model.Sources {
		src := &model.Sources[idx]
		var c wizardmodels.SourceNodeCount
		switch src.Kind {
		case wizardmodels.SourceKindFolder, wizardmodels.SourceKindSubscription:
			c.Total = len(src.Nodes)
			for i := range src.Nodes {
				if src.Nodes[i].Enabled {
					c.Enabled++
				}
			}
		default:
			// Узловой источник — ровно один узел, и его судьбу решает
			// собственный enabled.
			c.Total = 1
			if src.Enabled {
				c.Enabled = 1
			}
		}
		if c.Total == 0 {
			continue
		}
		counts[idx] = c
	}
	model.SourceNodeCounts = counts
	return true
}

// InvalidateSourceNodeCounts снимает кэш счётчиков.
func InvalidateSourceNodeCounts(model *wizardmodels.WizardModel) {
	if model != nil {
		model.SourceNodeCounts = nil
	}
}
