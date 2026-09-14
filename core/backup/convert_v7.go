// File convert_v7.go — конвертеры границы «модель состояния ↔ формы
// контракта» (SPEC 118 Т9, SPEC 127 §6.0).
//
// В модели приложения (v7, затем v8) нет ни disabled-карты, ни свёртки в форме
// контракта, ни detour-тройни, ни строковых хопов. Значит перевод одного в
// другое обязан быть ЯВНЫМ и в одном месте — здесь, а не россыпью по
// экспорту и импорту.
//
// Экспорт (писатель 1.0 — единственный с v1.6.0, D-110) переводит здесь ровно
// два поля тонкого слоя:
//
//	node.enabled=false + PendingDisabled → sub.disabled{сырой тег: 0}
//	FolderReplace                        → fold{mode, auto}
//
// Импорт переводит обратно свёртку (оба входа) и всё, что называла иначе
// форма 0.x (legacy-вход):
//
//	fold{mode, auto}                    → FolderReplace
//	тройня detour_node_source_id + tag  → NodeLink (detour)
//	[]string (теги хопов)               → []NodeLink (+ резолв по живому индексу)
//	chain{…}                            → Node.Body
//	tag{prefix, postfix, mask}          → TagPolicy (mask — потеря с warning)
//
// Материализованные nodes[] подписки в бэкап НЕ уезжают ни в каком формате:
// после импорта подписка фетчится заново.
package backup

import (
	"encoding/json"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// ── экспорт: состояние → поля тонкого слоя 1.0 ───────────────────

// exportFold — FolderReplace модели в свёртку контракта.
func exportFold(r *state.FolderReplace) *Fold {
	if r == nil {
		return nil
	}
	out := &Fold{}
	switch r.Mode {
	case state.FolderReplaceAuto:
		out.Mode = "auto"
	case state.FolderReplaceBoth:
		out.Mode = "select_auto"
	default:
		out.Mode = "select"
	}
	if r.Strategy != nil {
		out.Auto = r.Strategy.Clone()
	}
	return out
}

// exportDisabledMap — отметки выключения по СЫРЫМ тегам узлов (identity в
// рамках источника). Значение — время подтверждения; в v7 его больше нет
// (карта времён умерла вместе с TTL), поэтому пишется 0: контракт числа
// требует, а смысла в нём для приёмника нет.
//
// PendingDisabled (отметки, ещё не сматченные с узлом) уезжают тем же
// списком: они ровно того же рода, и терять их на экспорте значило бы
// потерять выбор пользователя на первом же круге «экспорт → импорт».
func exportDisabledMap(src state.Source) map[string]int64 {
	out := map[string]int64{}
	for i := range src.Nodes {
		// Неразобранная запись (kind=unsupported, SPEC 116 W11) выключена не
		// пользователем, а собственной невозможностью: в файл её отметка не
		// едет — приёмник иначе выключил бы у себя ЖИВОЙ одноимённый узел.
		if src.Nodes[i].IsUnsupported() {
			continue
		}
		if !src.Nodes[i].Enabled && strings.TrimSpace(src.Nodes[i].Tag) != "" {
			out[src.Nodes[i].Tag] = 0
		}
	}
	for _, tag := range src.PendingDisabled {
		if strings.TrimSpace(tag) != "" {
			out[tag] = 0
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ── импорт: формы контракта → состояние ──────────────────────────

// importFold — свёртка контракта в FolderReplace модели.
//
// tag замены контракт не несёт (в 0.11 он был позиционным деривативом), и
// выдумывать его нельзя: тег даёт вызывающий из имени/индекса источника.
func importFold(f *Fold, replaceTag string) *state.FolderReplace {
	if f == nil {
		return nil
	}
	out := &state.FolderReplace{Tag: replaceTag}
	switch f.Mode {
	case "auto":
		out.Mode = state.FolderReplaceAuto
	case "select_auto":
		out.Mode = state.FolderReplaceBoth
	default:
		out.Mode = state.FolderReplaceManual
	}
	if f.Auto != nil {
		out.Strategy = f.Auto.Clone()
	}
	return out
}

// importNodeLinkRef — detour-тройня контракта в NodeLink модели.
//
// Пустая тройня при непустом detour_tag — ссылка на ГРУППУ (прежний
// DetourTag): в v7 у неё та же форма, что у ссылки корневого пространства.
//
// `detour_node_source_id` здесь ложится в folder_id как есть, даже когда это
// id СЕРВЕРА или цепочки файла (`servers[].id`, `chains[].id`): в 0.12 сервер
// был сам себе источником. Адрес такой ссылке дописывает слияние, когда
// известно, куда лёг узел (mergedInfo.rewriteLinks): корневой узел — `{tag:
// тег здесь}`, член папки — парой (NODE_LINK.md §7.4).
func importNodeLinkRef(ref SourceRef) *state.NodeLink {
	tag := strings.TrimSpace(ref.DetourNodeTag)
	if tag == "" {
		tag = strings.TrimSpace(ref.DetourNodeLabel)
	}
	if tag == "" {
		if t := strings.TrimSpace(ref.DetourTag); t != "" {
			return &state.NodeLink{Tag: t}
		}
		return nil
	}
	return &state.NodeLink{FolderID: strings.TrimSpace(ref.DetourNodeSourceID), Tag: tag}
}

// importHops — строковые теги контракта в позиции модели.
//
// Резолв по живому индексу — забота вызывающего (он видит весь набор
// источников); здесь строка становится ссылкой корневого пространства, а это
// ровно то, чем она в 0.11 и была.
func importHops(hops []string) []state.NodeLink {
	if len(hops) == 0 {
		return nil
	}
	out := make([]state.NodeLink, 0, len(hops))
	for _, h := range hops {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		out = append(out, state.NodeLink{Tag: h})
	}
	return out
}

// importChainBody — тело узла-цепочки из формы контракта (позиции в тело не
// едут: их дом — Node.Hops).
func importChainBody(c *configtypes.SourceChain) json.RawMessage {
	if c == nil {
		return nil
	}
	return configtypes.ChainBody(c)
}

// importMaskTag — `tag.mask` контракта.
//
// Маска умерла классом (SPEC 118 W5): в v7 у контейнера остались только
// prefix/postfix, а у узла — собственный тег. Правило перевода зависит от
// того, ЧЬЯ маска:
//
//   - у одиночного узла (server/chain) маска несла ровно имя этого узла и
//     становится `Node.tag` — так её и читает миграция состояния
//     (`migration_v6_to_v7.go`, шаг 3). В контракте 0.11 у секций servers[] и
//     chains[] поля `tag` нет вовсе: имя узла едет отдельным ключом
//     (`node_tag` / `tag`), и никакой маски там не бывает — этот случай на
//     границе бэкапа невыразим по построению;
//   - у ПОДПИСКИ маска была шаблоном имени для КАЖДОЙ ноды (`{$label}` и
//     прочие подстановки), и prefix/postfix её не заменяют. Перевести нечем —
//     возвращаем строку, чтобы вызывающий назвал потерю warning'ом, а не
//     подставил шаблон тегом.
func importMaskTag(tp *TagPolicy) string {
	if tp == nil {
		return ""
	}
	return strings.TrimSpace(tp.Mask)
}

// legacyFoldPrefix — префикс групп прежней свёртки: тег-префикс подписки с
// позиционным умолчанием «<номер>:» (D-081; номер — индекс записи в секции
// subscriptions[], а не позиция среди всех источников). Формула воспроизведена
// байт-в-байт (в т. ч. TrimSpace: старый движок обрезал префикс, и `"[P] "`
// давал `[P]select`) — по этим тегам ссылались правила живых состояний, а файл
// 0.x и файл 1.0 без `fold_tag` имени группы иначе не несут.
func legacyFoldPrefix(tagPrefix string, index int) string {
	if p := strings.TrimSpace(tagPrefix); p != "" {
		return p
	}
	return strconv.Itoa(index+1) + ":"
}

// foldDerivedDirectionTags — теги локальных Направлений, которые породила
// свёртка, а не пользователь.
//
// Старая свёртка эмитила пару `<PFX>select` / `<PFX>auto` и клала её в
// `outbounds[]` источника. В v7 эта пара — не Направления, а FolderReplace, и
// импортировать её вторым способом значило бы получить два владельца одного
// тега. Всё, что в эту пару не попало, — настоящее локальное Направление
// пользователя, и оно упразднено классом (warning вызывающего).
func foldDerivedDirectionTags(sub Subscription, index int) map[string]bool {
	if sub.Fold == nil {
		return nil
	}
	prefix := ""
	if sub.Tag != nil {
		prefix = sub.Tag.Prefix
	}
	prefix = legacyFoldPrefix(prefix, index)
	return map[string]bool{
		prefix + "select": true,
		prefix + "auto":   true,
	}
}

// resolveImportedHops — второй проход по цепочкам: строковый хоп получает
// адрес папки, если тег нашёлся среди узлов приехавших подписок/папок.
//
// Первый проход (importHops) кладёт голую строку — ссылку корневого
// пространства, ровно ту, чем хоп в 0.11 и был. Здесь он поднимается до
// адресной ссылки там, где адрес известен. Нерезолвнутый остаётся
// `NodeLink{"", тег}` — на сборке такая позиция уходит fail-closed и роняет
// цепочку целиком, что и есть требуемое поведение.
//
// ПОЧЕМУ БЕЗ WARNING'А. Нерезолвнутый хоп на импорте — норма, а не потеря:
// у только что импортированной подписки nodes[] пусты (контракт их не несёт),
// и КАЖДЫЙ хоп в узел подписки был бы «не найден» до первого обновления.
// Контракт говорит об этом прямым текстом (backup.schema.json, chains[]):
// достижимость позиций на импорте не проверяется, рубеж валидации у обеих
// сторон один — config build (chain_hop_missing). Предупреждать здесь
// значило бы обвешать каждый нормальный restore ложной тревогой и разойтись
// с общим корпусом, который считает такие файлы чистыми.
//
// Индекс строится по СЫРЫМ тегам узлов контейнеров; корневые узлы
// (server/chain/auto), replace-теги и Направления живут в корневом
// пространстве и адреса не требуют.
//
// Хоп цепочки, приехавшей ЭТИМ файлом, сперва ищется в пространстве файла:
// среди членов папок файла под их тегом в файле (merged.landed.legacyTags) и
// узлов подписок, приехавших файлом. Член, которого слияние уникализировало
// или узнало по телу под другим тегом, находится по прежнему имени и не
// уводит позицию на здешнего тёзку (NODE_LINK.md §7.2). Члены, добавленные
// файлом под другим тегом, в общий индекс не входят: их здешнее имя файлу не
// принадлежит.
func resolveImportedHops(sources []state.Source, directions []configtypes.Direction, merged *mergedInfo) {
	byTag := map[string]string{}   // сырой тег узла контейнера → id контейнера
	ambiguous := map[string]bool{} // тот же тег в двух контейнерах — адрес не выбираем
	rootTags := map[string]bool{}

	for i := range sources {
		src := &sources[i]
		switch src.Kind {
		case state.SourceKindFolder, state.SourceKindSubscription:
			for j := range src.Nodes {
				// Неразобранная запись целью хопа быть не может (сборка её не
				// эмитит вовсе) — и адресом контейнера её тег тоже не служит:
				// иначе хоп «уехал» бы в узел, которого в конфиге нет.
				if src.Nodes[j].IsUnsupported() {
					continue
				}
				tag := strings.TrimSpace(src.Nodes[j].Tag)
				if tag == "" || merged.landed.renamed[state.NodeLink{FolderID: src.ID, Tag: src.Nodes[j].Tag}] {
					continue
				}
				if prev, seen := byTag[tag]; seen && prev != src.ID {
					ambiguous[tag] = true
					continue
				}
				byTag[tag] = src.ID
			}
			if src.Replace != nil {
				if t := strings.TrimSpace(src.Replace.Tag); t != "" {
					rootTags[t] = true
					if src.Replace.Mode == state.FolderReplaceBoth {
						rootTags[t+"-auto"] = true
					}
				}
			}
		default:
			if t := strings.TrimSpace(src.NodeTagOrLabel()); t != "" {
				rootTags[t] = true
			}
		}
	}
	for _, d := range directions {
		if t := strings.TrimSpace(d.Tag); t != "" {
			rootTags[t] = true
		}
	}

	fromFile := map[int]bool{}
	for _, ln := range merged.linked {
		if ln.at.node < 0 {
			fromFile[ln.at.src] = true
		}
	}
	fileTier := make(map[string][]state.NodeLink, len(merged.landed.legacyTags))
	for tag, hits := range merged.landed.legacyTags {
		fileTier[tag] = append([]state.NodeLink(nil), hits...)
	}
	for i := range sources {
		src := &sources[i]
		if !fromFile[i] || src.Kind != state.SourceKindSubscription || src.ID == "" {
			continue
		}
		for j := range src.Nodes {
			tag := strings.TrimSpace(src.Nodes[j].Tag)
			if tag == "" || src.Nodes[j].IsUnsupported() {
				continue
			}
			fileTier[tag] = append(fileTier[tag], state.NodeLink{FolderID: src.ID, Tag: src.Nodes[j].Tag})
		}
	}
	for i := range sources {
		src := &sources[i]
		if src.Kind != state.SourceKindChain {
			continue
		}
		for h := range src.Hops {
			hop := &src.Hops[h]
			if hop.FolderID != "" {
				continue
			}
			tag := strings.TrimSpace(hop.Tag)
			if tag == "" || rootTags[tag] {
				continue
			}
			if hits, inFile := fileTier[tag]; inFile && fromFile[i] {
				if len(hits) == 1 {
					*hop = hits[0]
				}
				continue
			}
			if id, ok := byTag[tag]; ok && !ambiguous[tag] {
				hop.FolderID = id
			}
		}
	}
}
