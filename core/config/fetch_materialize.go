// File fetch_materialize.go — материализация тела подписки при fetch
// (SPEC 118 W3, PLAN §3.1).
//
// Fetch — единственное место разбора тела подписки в модели v7: скачанное
// (и уже декодированное фетчером) тело гонится через чистый парсер
// subscription.ParseSubscriptionBody, принятые записи конвертируются в
// канонические узлы state.Node тем же кодом, что и миграция W2
// (canonicalNodeFromEntry) — body свежего fetch и body мигрированных узлов
// обязаны совпадать байт-в-байт.
//
// Живёт в пакете config (не в core и не в state): эмиттеры outbound-JSON и
// парсер подписок — здешние соседи, а state их импортировать не может
// (цикл — тот же довод, что у migrate_materialize.go).
package config

import (
	"fmt"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/textnorm"
)

// SubscriptionFetchMaterial — материализованный итог разбора тела при fetch.
type SubscriptionFetchMaterial struct {
	// Nodes — узлы в порядке тела провайдера; merge-ключ — их сырой
	// уникализированный тег. Кроме принятых (server + auto) сюда попадают
	// НЕРАЗОБРАННЫЕ записи узлами kind=unsupported (SPEC 116 W11) — на своих
	// позициях, с исходником и причиной.
	Nodes []state.Node
	// Supported — сколько из Nodes реально собрались (без unsupported).
	// Достоверность ответа считается по НИМ: тело, из которого не родилось
	// ни одного собравшегося узла, недостоверно, сколько бы отбракованных
	// строк в нём ни было (SPEC 113-A — так выглядит HTML вместо подписки).
	Supported int
	// Truncated — разбор упёрся в кап: merge не удаляет «исчезнувших».
	Truncated bool
	// Warnings — per-record деградации разбора (битые записи, потерянные
	// группы-члены, обрезка капом); персистятся в updateStatus.
	Warnings []string
}

// MaterializeSubscriptionBody разбирает ДЕКОДИРОВАННОЕ тело подписки в
// канонические узлы v7.
//
// capN — уже разрешённый кап (настройка подписки → дефолт настроек
// приложения); ≤0 → аварийный потолок-константа (клэмп 3000 — внутри
// парсера). Ошибка (пустое тело / обрыв разбора) возвращается вызывающему —
// это признак НЕДОСТОВЕРНОГО ответа: nodes[] трогать нельзя (SPEC 113-A).
func MaterializeSubscriptionBody(subID string, decodedBody []byte, skip []map[string]string, capN int) (*SubscriptionFetchMaterial, error) {
	pb, parseErr := subscription.ParseSubscriptionBody(decodedBody, skip, capN)
	if pb == nil {
		return nil, parseErr
	}
	out := &SubscriptionFetchMaterial{
		Truncated: pb.Truncated,
		Warnings:  append([]string(nil), pb.Warnings...),
	}
	if parseErr != nil {
		return out, parseErr
	}

	// Теги узлов (и принятых, и неразобранных) обязаны быть уникальны в рамках
	// контейнера: сырой тег — идентичность и merge-ключ. Принятые уже
	// уникализированы парсером; неразобранной записи тег даём здесь, поэтому
	// занятость считаем по одному множеству на весь материал.
	taken := make(map[string]bool, len(pb.Entries))
	for _, e := range pb.Entries {
		if e != nil {
			taken[e.RawTag] = true
		}
	}

	// Отбракованные записи вставляются на СВОИ позиции: rejectedAt[k] — те,
	// что стояли после k-й принятой записи.
	rejectedAt := make(map[int][]subscription.RejectedBodyRecord, len(pb.Rejected))
	for _, r := range pb.Rejected {
		rejectedAt[r.After] = append(rejectedAt[r.After], r)
	}

	accepted := 0
	flushRejected := func() {
		for _, r := range rejectedAt[accepted] {
			node := unsupportedNodeFromRecord(r, taken, len(out.Nodes)+1)
			taken[node.Tag] = true
			out.Nodes = append(out.Nodes, node)
		}
	}

	flushRejected()
	for _, e := range pb.Entries {
		if e == nil || e.Node == nil {
			continue
		}
		accepted++
		node, convErr := canonicalNodeFromEntry(subID, e)
		if convErr != nil {
			// Запись разобралась, но собрать из неё outbound нечем: та же
			// неразобранная запись, только причина обнаружилась позже. Узлом
			// kind=unsupported она остаётся на своём месте — и со своим тегом,
			// который парсер уже уникализировал.
			reason := fmt.Sprintf("not emittable: %v", convErr)
			out.Warnings = append(out.Warnings, fmt.Sprintf("node %q not emittable — dropped: %v", e.RawTag, convErr))
			out.Nodes = append(out.Nodes, state.NewUnsupportedNode(e.RawTag, reason, e.OriginKind, e.OriginRaw))
			flushRejected()
			continue
		}
		out.Nodes = append(out.Nodes, node)
		out.Supported++
		flushRejected()
	}
	return out, nil
}

// unsupportedNodeFromRecord — неразобранная запись тела как узел контейнера.
//
// Тег берётся ИЗ ЗАПИСИ (подпись во фрагменте share-URI — единственное имя,
// которое пользователь у неё видел), а когда её нет или она уже занята —
// позиционный `unsupported-N`, где N — место записи в составе. Позиционный
// тег не притворяется именем: запись всё равно узнаётся по исходнику, а тег
// ей нужен как идентичность (merge-ключ и адрес в контейнере).
func unsupportedNodeFromRecord(r subscription.RejectedBodyRecord, taken map[string]bool, pos int) state.Node {
	tag := textnorm.NormalizeProxyDisplay(subscription.LabelFromOriginURI(r.OriginRaw))
	if tag == "" || taken[tag] {
		tag = fmt.Sprintf("unsupported-%d", pos)
	}
	for taken[tag] {
		pos++
		tag = fmt.Sprintf("unsupported-%d", pos)
	}
	return state.NewUnsupportedNode(tag, r.Reason, r.OriginKind, r.OriginRaw)
}
