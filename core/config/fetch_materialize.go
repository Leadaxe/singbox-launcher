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
)

// SubscriptionFetchMaterial — материализованный итог разбора тела при fetch.
type SubscriptionFetchMaterial struct {
	// Nodes — принятые узлы (server + auto) в порядке тела провайдера;
	// merge-ключ — их сырой уникализированный тег.
	Nodes []state.Node
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
	for _, e := range pb.Entries {
		if e == nil || e.Node == nil {
			continue
		}
		node, convErr := canonicalNodeFromEntry(subID, e)
		if convErr != nil {
			// Битая запись — деградация записи с warning, не подписки.
			out.Warnings = append(out.Warnings, fmt.Sprintf("node %q not emittable — dropped: %v", e.RawTag, convErr))
			continue
		}
		out.Nodes = append(out.Nodes, node)
	}
	return out, nil
}
