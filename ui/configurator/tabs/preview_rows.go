// File preview_rows.go — строки списка узлов на вкладке Preview окна
// источника (SPEC 116 W11).
//
// # Зачем отдельная модель строки
//
// До W11 список рисовался прямо по узлам ЭМИССИИ (`[]*config.ParsedNode`), и
// это было честно ровно потому, что состав контейнера и состав эмиссии
// совпадали. С W11 они расходятся: неразобранная запись живёт в `nodes[]`
// узлом kind=unsupported, а эмиссия её не создаёт вовсе — она пропускается
// ДО тег-машины, чтобы не съесть ни {$num}, ни слот уникализации у соседей.
//
// Значит строке нужен свой тип: у одних строк за спиной эмитированный узел
// (протокол, транспорт, JSON, операции), у других — только запись состояния
// (исходник и причина). Развилку «а это точно узел?» внутри каждого
// обработчика заводить нельзя — она разъехалась бы по десятку мест.
//
// Порядок строк — порядок `nodes[]` контейнера: он и есть состав, который
// пользователь двигает и правит. Эмитированные узлы подставляются в него по
// СЫРОМУ тегу (идентичность в контейнере, SPEC 112).
package tabs

import (
	"strings"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/nodewarn"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// previewRow — одна строка списка узлов.
type previewRow struct {
	// Node — эмитированный узел; nil у неразобранной записи.
	Node *config.ParsedNode
	// RawTag — сырой тег: идентичность узла в контейнере, адрес всех
	// операций и ключ отметки enabled.
	RawTag string
	// Unsupported — запись, которую разобрать не удалось: чекбокса у неё нет,
	// а вместо протокола в подстроке едет причина.
	Unsupported bool
	// Service — узел служебный (релей провайдера, SPEC 120): строка помечена
	// шестерёнкой, в выборе Направлений его нет.
	Service bool
	// Reason — причина отбраковки (только у Unsupported).
	Reason string
	// OriginRaw — исходник записи байт в байт (у Unsupported — единственное,
	// по чему её можно узнать и починить).
	OriginRaw string
	// Warnings — коды деградаций, применённых конвейером к телу узла
	// (SPEC 131 §6). Берутся ИЗ СОСТОЯНИЯ, а не из эмитированного узла:
	// эмиссия их не возвращает, а показывать их надо у любой строки состава,
	// в том числе у выключенной, которую эмиссия не выпустила.
	//
	// Живут рядом с Reason, но это разные вещи (Л20): Reason — английский
	// текст отбракованной записи, Warnings — коды ВЫЖИВШЕГО узла, которые
	// переводит реестр.
	Warnings []corestate.NodeWarning
	// GroupAlive/GroupCounted — честный размер пула авто-группы: члены,
	// которые СЕЙЧАС резолвятся по модели (annotatePreviewGroupRows).
	// Counted=false — строку не считали (нет доступа к модели): подстрока
	// падает на заявленный состав.
	GroupAlive   int
	GroupCounted bool
}

// buildPreviewRows раскладывает состав контейнера в строки списка.
//
// emitted — узлы того же источника после эмиссии (в них финальные теги и
// разобранные поля); stateNodes — материализованный состав `nodes[]`.
//
// Узловой источник (server/chain/auto) состава не имеет: у него stateNodes
// пуст, и строки строятся прямо по эмиссии.
func buildPreviewRows(stateNodes []wizardmodels.Node, emitted []*config.ParsedNode) []previewRow {
	if len(stateNodes) == 0 {
		rows := make([]previewRow, 0, len(emitted))
		for _, n := range emitted {
			rows = append(rows, previewRow{Node: n, RawTag: config.NodeIdentity(n)})
		}
		return rows
	}

	// Эмитированные — по сырому тегу. Узлы без идентичности (группы: у них
	// её нет по построению, SPEC 112) кладутся в отдельную очередь и
	// раздаются по порядку: состав контейнера и эмиссия идут одним порядком,
	// поэтому n-я безымянная строка — это n-й безымянный узел.
	byRaw := make(map[string]*config.ParsedNode, len(emitted))
	var anonymous []*config.ParsedNode
	for _, n := range emitted {
		raw := config.NodeIdentity(n)
		if raw == "" {
			anonymous = append(anonymous, n)
			continue
		}
		if _, dup := byRaw[raw]; !dup {
			byRaw[raw] = n
		}
	}
	nextAnonymous := 0

	rows := make([]previewRow, 0, len(stateNodes))
	for i := range stateNodes {
		sn := &stateNodes[i]
		raw := strings.TrimSpace(sn.Tag)
		if sn.IsUnsupported() {
			row := previewRow{RawTag: raw, Unsupported: true, Reason: sn.Reason}
			if sn.Origin != nil {
				row.OriginRaw = sn.Origin.Raw
			}
			rows = append(rows, row)
			continue
		}
		node := byRaw[raw]
		if node == nil && nextAnonymous < len(anonymous) {
			node = anonymous[nextAnonymous]
			nextAnonymous++
		}
		// Исходник берётся из СОСТОЯНИЯ, а не из эмитированного узла:
		// ParsedNode происхождение не несёт (эмиссия читает его, но обратно не
		// кладёт), а «из чего сделан узел» пользователю нужно у любой строки.
		row := previewRow{Node: node, RawTag: raw, Service: sn.Service, Warnings: sn.Warnings}
		if sn.Origin != nil {
			row.OriginRaw = sn.Origin.Raw
		}
		// node == nil — узел есть в составе, но эмиссия его не выпустила:
		// выключен либо не собрался. Строка обязана остаться — иначе снятая
		// галка прятала бы узел от пользователя, который её и снял.
		rows = append(rows, row)
	}
	return rows
}

// annotatePreviewGroupRows — считает живых членов авто-групп по МОДЕЛИ.
//
// Эмиссия одного источника резолва ссылок не делает (проход 2 идёт только на
// сборке конфига), поэтому по эмитированному узлу группа всегда выглядит
// «заявленной». А заявленный член может быть мёртв: его источник выключен или
// удалён, узел исчез из состава, выключен или сам стал unsupported. Считаем
// то, что резолвится сейчас, — группа с нулём живых членов на сборке пуста и
// обязана показываться сломанной (⚠), а не «типа всё нормально» (обкатка,
// заход 3).
//
// rows и stateNodes идут одним порядком (контракт buildPreviewRows); длины
// расходятся только у узлового источника без состава — там групп-ссылок нет.
//
// ownerID — ID источника, чьё превью рисуется. Он нужен ровно для одной
// развилки: ВЫКЛЮЧЕННЫЙ источник. Прежде членство считалось живым только у
// включённого источника, и у выключенной подписки все её авто-группы
// объявлялись сломанными («⚠ [0] fastest — no working members», «⚠ 8 node
// error(s)» в шапке), хотя с узлами ничего не случилось: группа выключена
// ВМЕСТЕ со своими членами, одним и тем же тумблером. Ложная тревога — и
// ровно про то место, где пользователь сам всё выключил.
//
// Поэтому член СВОЕГО источника считается по собственному состоянию, а
// тумблер источника из счёта уходит. Член ЧУЖОГО выключенного источника
// по-прежнему не живой: включи такую группу — и на сборке она останется без
// членов, здесь это правда, а не артефакт показа.
func annotatePreviewGroupRows(
	rows []previewRow,
	stateNodes []wizardmodels.Node,
	sources []corestate.Source,
	ownerID string,
) {
	if len(rows) != len(stateNodes) {
		return
	}
	ownerID = strings.TrimSpace(ownerID)
	var byID map[string]*corestate.Source
	for i := range rows {
		sn := &stateNodes[i]
		if sn.Group == nil || rows[i].Unsupported {
			continue
		}
		if byID == nil {
			byID = make(map[string]*corestate.Source, len(sources))
			for j := range sources {
				byID[sources[j].ID] = &sources[j]
			}
		}
		alive := 0
		for _, m := range sn.Group.Members {
			memberOwner := strings.TrimSpace(m.FolderID)
			src := byID[memberOwner]
			if src == nil {
				continue
			}
			// Свой источник: его тумблер гасит и группу, и членов разом, и
			// спрашивать о нём здесь незачем — см. шапку.
			ownSource := ownerID != "" && memberOwner == ownerID
			if !src.Enabled && !ownSource {
				continue
			}
			for k := range src.Nodes {
				n := &src.Nodes[k]
				if n.Tag == m.Tag && n.Enabled && !n.IsUnsupported() {
					alive++
					break
				}
			}
		}
		rows[i].GroupAlive = alive
		rows[i].GroupCounted = true
	}
}

// previewRowsSupported — сколько строк несут собравшийся узел (счётчик
// «сколько серверов дал источник»).
func previewRowsSupported(rows []previewRow) int {
	n := 0
	for i := range rows {
		if !rows[i].Unsupported {
			n++
		}
	}
	return n
}

// previewRowsUnsupported — сколько записей источника разобрать не удалось.
func previewRowsUnsupported(rows []previewRow) int {
	n := 0
	for i := range rows {
		if rows[i].Unsupported {
			n++
		}
	}
	return n
}

// previewRowsBroken — сводка ⚠ состава: неразобранные записи плюс авто-группы
// без единого живого члена (после annotatePreviewGroupRows).
func previewRowsBroken(rows []previewRow) int {
	n := 0
	for i := range rows {
		if rows[i].Unsupported || (rows[i].GroupCounted && rows[i].GroupAlive == 0) {
			n++
		}
	}
	return n
}

// previewRowsWarned — сколько строк несут ПРОБЛЕМУ конвейера: код уровня
// error или warning (SPEC 131 §6).
//
// Считается ОТДЕЛЬНО от previewRowsBroken: сломанная запись в конфиг не
// поедет, а помеченный узел поедет — просто не таким, каким его прислал
// провайдер. Одно число на оба факта врало бы про оба.
//
// info в это число НЕ входит: ⚠ в шапке зовёт разбираться, а info говорит
// «разбираться не с чем». Счётчик по `len(Warnings) > 0` ставил в шапку «⚠
// 12» у источника, где все двенадцать кодов были «к сведению».
func previewRowsWarned(rows []previewRow) int {
	n := 0
	for i := range rows {
		if nodewarn.HasProblems(rows[i].Warnings) {
			n++
		}
	}
	return n
}

// previewRowsInfoOnly — сколько строк несут ТОЛЬКО «к сведению».
//
// Своё число, а не разность: узел с error и info попадает в previewRowsWarned
// и здесь не считается вовсе — про него уже сказано сильнее.
func previewRowsInfoOnly(rows []previewRow) int {
	n := 0
	for i := range rows {
		if nodewarn.InfoOnly(rows[i].Warnings) {
			n++
		}
	}
	return n
}
