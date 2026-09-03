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
	"strings"

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
	// уникализированы парсером; неразобранной записи тег даём здесь — ТОЙ ЖЕ
	// машиной (subscription.MakeIdentityUnique) и по ОДНОМУ счётчику на весь
	// материал, поэтому столкновение с соседом разводится общим правилом
	// «X, X-2, X-3», а не вторым, своим.
	idCounts := make(map[string]int, len(pb.Entries))
	for _, e := range pb.Entries {
		if e != nil {
			idCounts[e.RawTag] = 1
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
			out.Nodes = append(out.Nodes, unsupportedNodeFromRecord(r, idCounts, len(out.Nodes)+1))
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
		// Служебные узлы записи (релеи BYPASS) — ВСЕГДА отдельными узлами:
		// релей часть маршрута, и без него узел пошёл бы напрямую, то есть в
		// блокировку. Владелец дозванивается через первый из них полем Detour.
		// Видимость релея в выборе Направлений — отдельный вопрос, за него
		// отвечает Source.RelaysInDirections на стороне UI.
		if relays, detour := relayNodesFromEntry(subID, e, node.Tag, idCounts); len(relays) > 0 {
			if detour != nil {
				node.Detour = detour
			}
			out.Nodes = append(out.Nodes, node)
			out.Supported++
			out.Nodes = append(out.Nodes, relays...)
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
// Тег берётся ИЗ ЗАПИСИ — тем полем, которое в ней несёт имя (SPEC 116 W13):
// у URI-записи это фрагмент-подпись, у элемента JSON-тела — `tag`/`ps`/
// `remarks`/`name`, у анонса провайдера — сам его текст. Позиционный
// `unsupported-N` остаётся ровно для записи, у которой имени нет вовсе (или
// оно уже занято соседом): он не притворяется именем — запись узнаётся по
// исходнику, а тег ей нужен как идентичность (merge-ключ и адрес в
// контейнере).
//
// Имя из записи — ещё и условие merge-стабильности: у баннера тег = его
// подпись, поэтому при следующем fetch тот же баннер матчится по сырому тегу
// и не плодит вторую копию. Позиционный тег такой гарантии не даёт — он
// поедет от любой вставки выше по телу, поэтому остаётся ровно для безымянных.
//
// Столкновение имён разводит ОБЩАЯ машина уникализации контейнера
// (`X`, `X-2`, …) — та же, что у принятых узлов: два одинаковых баннера
// подряд обязаны именоваться тем же правилом, что два одноимённых сервера.
func unsupportedNodeFromRecord(r subscription.RejectedBodyRecord, idCounts map[string]int, pos int) state.Node {
	tag := clampUnsupportedTag(textnorm.NormalizeProxyDisplay(nameFromRejectedRecord(r)))
	if tag == "" {
		// Имени в записи нет вовсе — позиционный тег как идентичность. Он тоже
		// идёт через общий счётчик: `unsupported-3` мог прийти именем соседа.
		tag = fmt.Sprintf("unsupported-%d", pos)
	}
	return state.NewUnsupportedNode(subscription.MakeIdentityUnique(tag, idCounts), r.Reason, r.OriginKind, r.OriginRaw)
}

// maxUnsupportedTagRunes — потолок длины тега неразобранной записи.
//
// Анонс провайдера бывает не заголовком, а абзацем; тег — идентичность и
// строка списка, и класть в него килобайт чужого текста незачем. Обрезка
// стабильна (тот же текст → тот же тег), поэтому merge-ключ не плывёт, а
// полный исходник всё равно лежит в `origin.raw`.
const maxUnsupportedTagRunes = 120

func clampUnsupportedTag(tag string) string {
	runes := []rune(tag)
	if len(runes) <= maxUnsupportedTagRunes {
		return tag
	}
	return strings.TrimSpace(string(runes[:maxUnsupportedTagRunes])) + "…"
}

// nameFromRejectedRecord — как эту запись зовут у провайдера.
//
// Развилка по виду исходника, а не по причине отбраковки: имя лежит в РАЗНЫХ
// местах у разных форматов тела, и правило «взять поле имени» одно на все
// причины — сломанный узел и анонс провайдера называются одинаково.
func nameFromRejectedRecord(r subscription.RejectedBodyRecord) string {
	if r.Reason == subscription.RejectReasonProviderBanner {
		// Анонс провайдера — сам себе имя: его текст и есть подпись, которую
		// пользователь видел в списке. Гонять его через URI-разбор нельзя —
		// «Оплата: РФ/зарубеж» превратилась бы в обрезок до двоеточия.
		return r.OriginRaw
	}
	if r.OriginKind == state.OriginKindJSON {
		return subscription.NameFromOriginJSON(r.OriginRaw)
	}
	return subscription.LabelFromOriginURI(r.OriginRaw)
}
