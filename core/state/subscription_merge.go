// File subscription_merge.go — merge свежеразобранного тела подписки в её
// материализованные nodes[] (SPEC 118 W3, PLAN §3.1; правила —
// features/sources.md «Подписка: fetch», features/state.md «Материализация»).
//
// Merge живёт в state, а не в fetch-сервисе: правила принадлежат модели
// (тот же уровень, что normalizeSourceShape), а fetch-сервис лишь скачал и
// разобрал тело. Ключ merge — СЫРОЙ (уникализированный парсером) тег: на нём
// висят enabled, detour и все ссылки NodeLink, поэтому обновление подписки
// их не рвёт.
package state

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// SubFetchMaterial — материализованный итог одного достоверного разбора тела:
// узлы в порядке тела провайдера + признак обрезки капом.
type SubFetchMaterial struct {
	Nodes []Node
	// Truncated — разбор упёрся в кап max_nodes: обновлять и добавлять можно,
	// удалять «исчезнувших» — запрещено (SPEC 113-A: «исчез» неотличим от
	// «остался за капом»).
	Truncated bool
}

// MergeSubscriptionNodes сливает свежий материал в sub.Nodes по сырому тегу:
//
//   - совпавший тег — body/origin/состав группы освежаются из свежего тела,
//     а пользовательские enabled и detour живут;
//   - новый тег — узел добавляется включённым;
//   - исчезнувший тег — удаляется (состав подписки принадлежит провайдеру),
//     НО при truncated удаление запрещено — узел остаётся как был;
//   - порядок nodes[] после merge = порядок свежего тела (выжившие занимают
//     позицию тела, а не старую); удержанные truncated-узлы — в хвосте в
//     прежнем относительном порядке.
//
// trusted=false — недостоверный ответ (ошибка сети, пустое тело, обрыв
// разбора): nodes[] не трогаются ВООБЩЕ (SPEC 113-A), вызывающий пишет
// только updateStatus.
//
// PendingDisabled (вердикт O2): одноразовые отметки выключения импорта
// бэкапа применяются по сырым тегам на первом достоверном fetch и стираются;
// при truncated несматченные теги переживают до следующего fetch.
//
// Возвращает (changed, warnings): changed — состав/содержимое nodes[]
// реально изменились (сигнал «конфиг устарел»), warnings — деградации merge
// (потерянный detour при смене вида узла, несматченные pending-отметки).
func MergeSubscriptionNodes(sub *Source, res *SubFetchMaterial, trusted bool) (bool, []string) {
	if sub == nil || sub.Kind != SourceKindSubscription {
		return false, nil
	}
	if !trusted || res == nil {
		return false, nil
	}

	var warns []string
	before, _ := json.Marshal(sub.Nodes)

	oldByTag := make(map[string]*Node, len(sub.Nodes))
	for i := range sub.Nodes {
		oldByTag[sub.Nodes[i].Tag] = &sub.Nodes[i]
	}

	merged := make([]Node, 0, len(res.Nodes))
	freshTags := make(map[string]bool, len(res.Nodes))
	for _, fresh := range res.Nodes {
		n := fresh // копия: не мутируем материал вызывающего
		freshTags[n.Tag] = true
		if old, ok := oldByTag[n.Tag]; ok {
			// Совпавший: пользовательские пометки живут, содержимое — свежее.
			n.Enabled = old.Enabled
			if n.Kind == SourceKindServer && old.Detour != nil {
				d := *old.Detour
				n.Detour = &d
			} else if old.Detour != nil && n.Kind != SourceKindServer {
				// detour существует только у Server (SPEC Т2): узел сменил вид
				// у провайдера — пометка теряется, но не молча.
				warns = append(warns, fmt.Sprintf("node %q: detour dropped — node kind changed to %q", n.Tag, n.Kind))
			}
		}
		merged = append(merged, n)
	}

	// Исчезнувшие: удаляются; при truncated — удерживаются в хвосте.
	if res.Truncated {
		for i := range sub.Nodes {
			if !freshTags[sub.Nodes[i].Tag] {
				merged = append(merged, sub.Nodes[i])
			}
		}
	}

	sub.Nodes = merged

	// Одноразовые отметки выключения импорта бэкапа (O2).
	if len(sub.PendingDisabled) > 0 {
		var remaining []string
		for _, tag := range sub.PendingDisabled {
			matched := false
			for i := range sub.Nodes {
				if sub.Nodes[i].Tag == tag {
					sub.Nodes[i].Enabled = false
					matched = true
				}
			}
			if matched {
				continue
			}
			if res.Truncated {
				// Узел мог остаться за капом — отметку не выбрасываем.
				remaining = append(remaining, tag)
			} else {
				warns = append(warns, fmt.Sprintf("pending disable mark %q matched no node — dropped", tag))
			}
		}
		sub.PendingDisabled = remaining
	}

	after, _ := json.Marshal(sub.Nodes)
	return !bytes.Equal(before, after), warns
}
