// File source_record_paste.go — приём ЗАПИСИ ХРАНЕНИЯ источника (канон v7,
// то, что лежит в state.json) как входа Add.
//
// # Зачем
//
// «Copy JSON» в меню строки источника и блок «Storage record» в Overview
// отдают запись целиком: kind, nodes[], detour, tag_policy, replace. До этого
// файла поле Add такой текст не узнавало: у записи нет ни `type` (признак
// sing-box outbound), ни `outbounds[]` (признак конфига), и документ уходил в
// построчный классификатор, где ни одна строка не ссылка. Итог — «no valid
// URLs to add», причём в корне ошибка тонула в debug-логе.
//
// Эмиттер и парсер ходят парой ([[emitter-parser-pairing]]): раз лаунчер
// сам отдаёт запись в буфер, он обязан принимать её обратно. Здесь запись
// разбирается ДО carveSingboxJSON — sing-box outbound с полем `kind` не
// существует, поэтому одно это поле отличает запись от чужого JSON.
//
// # Что происходит с записью
//
//   - папка → новая папка с НОВЫМ ULID: узлы, detour, tag_policy, replace,
//     enabled едут как есть. Ссылки NodeLink внутри записи, адресующие её
//     прежний ULID (detour узла на соседа по папке, хопы, члены группы),
//     переписываются на новый — иначе они бы указали в пустоту (или, хуже,
//     в оригинал, если тот ещё жив);
//   - подписка → новая подписка с новым ULID, URL/UA/HWID/tag_policy
//     сохранены; дедуп по URL — как у обычной подписочной строки. История
//     fetch (update_status) не копируется: она про оригинал;
//   - server / chain / auto / unsupported → УЗЕЛ, куда положить — решает
//     вызывающий (корень или папка), ровно как со всяким другим узлом.
//
// Куда кладётся контейнер, тоже решает вызывающий: в корне папка становится
// папкой, а внутри папки (вложенных папок нет) её узлы высыпаются в открытую
// папку; подписка внутри папки — отказ, как и подписочная строка.
package business

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

// carveSourceRecords выкусывает запись (или массив записей) хранения.
//
// Возвращает записи и признак «вход был записью»: он нужен, чтобы отличить
// «не запись» (вход идёт дальше по обычному пути) от «запись, но битая»
// (пользователь обязан увидеть ошибку). Массив, где записи перемешаны с чем-то
// ещё, — ошибка, а не «взять что узнали»: такой документ человек собрал не
// руками лаунчера, и молча выбросить половину значит соврать про результат.
func carveSourceRecords(input string) (records []corestate.Source, isRecord bool, err error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil, false, nil
	}

	var raws []json.RawMessage
	if trimmed[0] == '{' {
		raws = []json.RawMessage{json.RawMessage(trimmed)}
	} else if uerr := json.Unmarshal([]byte(trimmed), &raws); uerr != nil || len(raws) == 0 {
		return nil, false, nil
	}

	records = make([]corestate.Source, 0, len(raws))
	for i, raw := range raws {
		if !looksLikeSourceRecord(raw) {
			if i == 0 {
				return nil, false, nil // не наш JSON — пусть разбирается обычный путь
			}
			return nil, true, fmt.Errorf("record %d is not a launcher source record", i+1)
		}
		var src corestate.Source
		if uerr := json.Unmarshal(raw, &src); uerr != nil {
			return nil, true, fmt.Errorf("source record %d: %w", i+1, uerr)
		}
		normalizePastedRecord(&src)
		records = append(records, src)
	}
	return records, true, nil
}

// looksLikeSourceRecord — объект с полем `kind` из числа известных видов.
// sing-box outbound такого поля не несёт, а `type` у записи не бывает —
// этого достаточно, чтобы не перепутать.
func looksLikeSourceRecord(raw json.RawMessage) bool {
	var probe struct {
		Kind string          `json:"kind"`
		Type json.RawMessage `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || len(probe.Type) > 0 {
		return false
	}
	switch corestate.SourceKind(probe.Kind) {
	case corestate.SourceKindServer, corestate.SourceKindChain, corestate.SourceKindAuto,
		corestate.SourceKindFolder, corestate.SourceKindSubscription, corestate.SourceKindUnsupported:
		return true
	}
	return false
}

// normalizePastedRecord приводит вставленную запись к состоянию «новая»:
// свежий ULID, внутренние ссылки на прежний ULID переписаны, история fetch
// и одноразовые отметки сброшены, узел без тела материализован из origin.
func normalizePastedRecord(src *corestate.Source) {
	oldID := strings.TrimSpace(src.ID)
	src.ID = corestate.MakeULID()
	src.UpdateStatus = nil
	src.PendingDisabled = nil
	src.Label = ""

	switch src.Kind {
	case corestate.SourceKindFolder, corestate.SourceKindSubscription:
		// У контейнера тег не живёт: запись папки несёт "tag": "" всегда,
		// но чужая рука могла вписать туда что угодно.
		src.Tag = ""
		src.Body = nil
		for i := range src.Nodes {
			normalizePastedNode(&src.Nodes[i])
		}
		if oldID != "" {
			repointRecordLinks(src, oldID, src.ID)
		}
	default:
		src.Nodes = nil
		normalizePastedNode(&src.Node)
	}
}

// normalizePastedNode — узел записи: пустой kind читается как server (так
// писали старые состояния), тело без материализации собирается из origin.
func normalizePastedNode(n *corestate.Node) {
	if n.Kind == "" {
		n.Kind = corestate.SourceKindServer
	}
	if n.Kind != corestate.SourceKindServer || len(n.Body) > 0 {
		return
	}
	if n.Origin == nil || strings.TrimSpace(n.Origin.Raw) == "" {
		return
	}
	mat, err := config.MaterializeServerNode(n.Origin.Raw, nil)
	if err != nil {
		debuglog.WarnLog("AddSources: pasted record node %q has no body and origin did not parse: %v", n.Tag, err)
		return
	}
	n.Body = mat.Body
}

// repointRecordLinks переписывает ссылки ВНУТРИ записи с прежнего ULID
// контейнера на новый. Ссылки на другие контейнеры и на корень (folder_id
// пуст) не трогаются: они адресуют то, что в модели, возможно, есть.
func repointRecordLinks(src *corestate.Source, oldID, newID string) {
	fix := func(link *corestate.NodeLink) {
		if link != nil && strings.TrimSpace(link.FolderID) == oldID {
			link.FolderID = newID
		}
	}
	fixNode := func(n *corestate.Node) {
		fix(n.Detour)
		for j := range n.Hops {
			fix(&n.Hops[j])
		}
		if n.Group != nil {
			for j := range n.Group.Members {
				fix(&n.Group.Members[j])
			}
		}
	}
	fixNode(&src.Node)
	for i := range src.Nodes {
		fixNode(&src.Nodes[i])
	}
}

// splitPastedRecords раскладывает записи на контейнеры и узлы для
// parsedSourceInput. Узел без тега получает заглушку `server-N` тем же
// правилом, что и безымянная ссылка.
func splitPastedRecords(records []corestate.Source, res *parsedSourceInput, next *int) {
	for i := range records {
		rec := records[i]
		switch rec.Kind {
		case corestate.SourceKindFolder, corestate.SourceKindSubscription:
			res.Containers = append(res.Containers, rec)
		default:
			node := rec.Node
			unnamed := strings.TrimSpace(node.Tag) == ""
			if unnamed {
				*next++
				node.Tag = fmt.Sprintf("server-%d", *next)
			}
			res.Nodes = append(res.Nodes, node)
			res.URIOf = append(res.URIOf, "")
			res.Unnamed = append(res.Unnamed, unnamed)
		}
	}
}

// flattenRecordNodesInto — узлы контейнера-записи для вставки В ПАПКУ:
// вложенных папок нет, поэтому папка-запись высыпается узлами, а её
// внутренние ссылки переезжают на ULID папки-адресата (соседи по папке
// остаются соседями).
func flattenRecordNodesInto(rec corestate.Source, dstFolderID string) []corestate.Node {
	if len(rec.Nodes) == 0 {
		return nil
	}
	copyRec := rec
	copyRec.Nodes = make([]corestate.Node, len(rec.Nodes))
	for i := range rec.Nodes {
		copyRec.Nodes[i] = cloneCanonicalNodeForMove(rec.Nodes[i])
	}
	repointRecordLinks(&copyRec, strings.TrimSpace(rec.ID), strings.TrimSpace(dstFolderID))
	return copyRec.Nodes
}
