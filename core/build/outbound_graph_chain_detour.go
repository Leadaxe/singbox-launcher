// File outbound_graph_chain_detour.go — правило 7 граф-санитайзера: цепочка
// как перезапись detour (SPEC 165 ядра, SPEC 189 лаунчера).
//
// Ядро с SPEC 165 при загрузке ставит каждому узлу, до которого цепочка
// доходит на позиции i ≥ 1 (члены групп любой вложенности тоже), detour на
// свой хоп «<цепочка>#<i−1>». Копий узла больше нет — у узла один экземпляр, и
// часть конфигов, которые прежнее ядро принимало, теперь валит загрузку ВСЕГО
// конфига (ALGORITHM ядра, §4.2):
//
//   - одно место на тег: узел или группа (все члены групп любой вложенности)
//     стоят в одной цепочке на одной позиции — общий первый хоп двух
//     цепочек тоже конфликт;
//   - кольцо: узел позиции ≥ 1 ходит через позицию ниже, и путь оттуда
//     возвращается к нему (`[A, Группа{A, X}]`, свой detour узла позиции 0);
//   - К5: у перехваченного узла фиксированный локальный порт;
//   - тег хопа `<цепочка>#<i>` занят, rewrite меняет detour.
//
// Поля привязки сокета (tcp_fast_open, routing_mark, …) ошибкой не являются:
// ядро показывает их в GetOutboundConfig записями lx.detour.
//
// Политика последнего рубежа та же, что у правил 1–6: деградировать один
// элемент с кодом в отчёт сборки, а не отдать ядру конфиг, который оно
// отвергнет: каждое нарушение снимает цепочку (из пары — ту, что позже по
// порядку записей). Сами проверки — config.ChainDetourProblems, их же зовёт
// редактор цепочки. Решение владельца 26.11: клиент не допускает колец и
// конфликтов; сверено с ядром d9fc75468.
//
// Правило действует, только когда ядро цели переписывает цепочки (версия не
// ниже границы `lx.detour` в registry/core_config.json или неизвестна): старое
// ядро клонирует узлы позиций, и для него эти конфиги законны.
package build

import (
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
)

// chainDetourRewriteActive — исполнит ли конфиг ядро, которое переписывает
// цепочку в detour. Граница — `min_core` блока `lx.detour` реестра: блок и
// перезапись пришли в ядро одной задачей (SPEC 165). Версия неизвестна —
// считается новой, как у гейта `max_core`: лаунчер ставит машинам ядро пина.
func chainDetourRewriteActive(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return true
	}
	reg, err := registry.Get()
	if err != nil {
		return true
	}
	f, ok := reg.CoreConfigFields()["lx.detour"]
	if !ok || f.MinCore == "" {
		return true
	}
	return nodeflow.CoreAtLeast(version, f.MinCore)
}

// applyChainDetourRules — одна итерация правила 7: первая проблема общей
// проверки config.ChainDetourProblems снимает свою цепочку с кодом. Возвращает
// true, если цепочка снята: фикспойнт вызовет снова на изменившемся графе.
func applyChainDetourRules(entries []*graphEntry, dropChain func(chain *graphEntry, code string, params map[string]string, why string)) bool {
	graph := make([]map[string]interface{}, 0, len(entries))
	byTag := make(map[string]*graphEntry, len(entries))
	for _, e := range entries {
		if e.dropped || e.m == nil || e.tag == "" {
			continue
		}
		graph = append(graph, e.m)
		byTag[e.tag] = e
	}
	problems := config.ChainDetourProblems(graph)
	if len(problems) == 0 {
		return false
	}
	p := problems[0]
	c := byTag[p.Chain]
	if c == nil {
		return false
	}
	dropChain(c, p.Code, p.Params, p.Reason)
	return true
}
