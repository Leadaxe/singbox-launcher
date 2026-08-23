// File source_node_counts.go — счёт узлов на источник для списка Sources.
//
// Показывает, сколько узлов даст источник и сколько из них реально пойдёт
// в конфиг: подписка на полсотни серверов, у которой половина выключена
// галками, выглядит в списке одинаково с полной, и понять это можно было
// только открыв её.
//
// Считается ЛЕНИВО и кэшируется. Разбор всех подписок занимает секунды на
// живых конфигах (сотни узлов), а строка списка перерисовывается на каждое
// движение мыши — считать на месте значило бы подвесить вкладку. По той же
// причине счёт не запускается сам при загрузке: пока пользователь не
// открыл Sources, эта цифра ему не нужна.
package business

import (
	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// EnsureSourceNodeCounts считает счётчики, если их ещё нет.
//
// Возвращает true, если счёт был выполнен (значит список надо перерисовать).
// Повторные вызовы бесплатны — в этом и смысл кэша.
//
// Кэш живёт до InvalidateSourceNodeCounts: его снимает всё, что меняет
// СОСТАВ узлов (правка источника, обновление подписки, снятая галка ноды).
func EnsureSourceNodeCounts(model *wizardmodels.WizardModel) bool {
	if model == nil || model.SourceNodeCounts != nil {
		return false
	}
	timing := debuglog.StartTiming("sourceNodeCounts")
	defer timing.EndWithDefer()

	// Кэш превью — источник истины: он уже разобран тем же парсером, что
	// и сборка конфига. Пустой кэш заполняем здесь же, раз пользователь
	// открыл Sources и цифры ему нужны.
	if len(model.PreviewNodes) == 0 {
		if _, err := RebuildPreviewCache(model); err != nil {
			debuglog.WarnLog("sourceNodeCounts: превью не собрано: %v", err)
			return false
		}
	}

	counts := make(map[int]wizardmodels.SourceNodeCount, len(model.Sources))
	for idx := range model.Sources {
		nodes := model.PreviewNodesBySource[idx]
		if len(nodes) == 0 {
			continue
		}
		disabled := model.Sources[idx].DisabledNodes
		c := wizardmodels.SourceNodeCount{Total: len(nodes)}
		for _, n := range nodes {
			if len(disabled) > 0 {
				// Отметка «нода выключена» хранится по хешу идентичности,
				// а не по тегу: теги подписок пересоздаются на каждом
				// обновлении (SPEC 094 D4).
				if h := config.NodeIdentityHash(n); h != "" {
					if _, off := disabled[h]; off {
						continue
					}
				}
			}
			c.Enabled++
		}
		counts[idx] = c
	}
	model.SourceNodeCounts = counts
	return true
}

// InvalidateSourceNodeCounts снимает кэш счётчиков.
//
// Зовётся вместе с InvalidatePreviewCache: счётчики выведены из превью, и
// пережить его они не могут — иначе список показывал бы числа от прошлого
// состава подписок.
func InvalidateSourceNodeCounts(model *wizardmodels.WizardModel) {
	if model != nil {
		model.SourceNodeCounts = nil
	}
}
