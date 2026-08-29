// File convert_v7.go — конвертеры границы «модель v7 ↔ контракт 0.11»
// (SPEC 118 Т9).
//
// Контракт бэкапа НЕ меняется: он общий с LxBox-стороной и живёт своей
// версией. Модель приложения переехала на v7 (SPEC 118), где нет ни
// disabled-карты, ни свёртки, ни detour-тройни, ни строковых хопов. Значит
// перевод одного в другое обязан быть ЯВНЫМ и в одном месте — здесь, а не
// россыпью по export.go/import.go.
//
// Что во что:
//
//	node.enabled=false ⇄ sub.disabled{сырой тег: 0}
//	FolderReplace      ⇄ fold{mode, auto}
//	NodeLink (detour)  ⇄ тройня detour_node_source_id + detour_node_tag
//	[]NodeLink (hops)  ⇄ []string (теги)
//	TagPolicy          ⇄ tag{prefix, postfix}     (mask не пишется)
//	Node.Tag           ⇄ server.node_tag / chain.tag
//
// Материализованные nodes[] в бэкап НЕ уезжают: контракт 0.11 их не несёт, и
// после импорта подписка фетчится заново.
package backup

import (
	"encoding/json"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// ── экспорт: v7 → 0.11 ───────────────────────────────────────────

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

// exportNodeLinkRef — NodeLink модели в detour-тройню контракта.
//
// Ссылка на узел папки едет парой «id папки + сырой тег»: ровно та адресация,
// которую тройня и описывала. Ссылка корневого пространства (FolderID пуст)
// едет одним тегом — так же, как её писала переходная форма.
func exportNodeLinkRef(link *state.NodeLink) SourceRef {
	if link == nil {
		return SourceRef{}
	}
	if link.FolderID == "" {
		return SourceRef{DetourNodeTag: link.Tag, DetourNodeLabel: link.Tag}
	}
	return SourceRef{
		DetourNodeSourceID: link.FolderID,
		DetourNodeTag:      link.Tag,
		DetourNodeLabel:    link.Tag,
	}
}

// exportHops — позиции цепочки в строковые теги контракта.
//
// Хоп на узел папки теряет адрес папки: контракт 0.11 знает только строку.
// Это названная цена (BACKUP.md §10) — на импорте такой хоп резолвится по
// живому индексу, а не резолвнувшийся уходит fail-closed с warning.
func exportHops(hops []state.NodeLink) []string {
	if len(hops) == 0 {
		return nil
	}
	out := make([]string, 0, len(hops))
	for _, h := range hops {
		if strings.TrimSpace(h.Tag) == "" {
			continue
		}
		out = append(out, h.Tag)
	}
	return out
}

// exportChainSpec — форма цепочки контракта: настройки из тела узла плюс
// позиции строками.
func exportChainSpec(src state.Source) *configtypes.SourceChain {
	return configtypes.ChainFromBody(src.Body, exportHops(src.Hops))
}

// ── импорт: 0.11 → v7 ────────────────────────────────────────────

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

// replaceTagSurvivesExport — переживёт ли ЯВНЫЙ тег замены круг «экспорт →
// импорт».
//
// В v7 `replace.tag` — обычное поле, которое пользователь задаёт руками
// (вкладка Replace, W5). В контракте 0.11 места для него нет: там свёртка
// несла только режим, а тег был ПОЗИЦИОННЫМ ДЕРИВАТИВОМ — «префикс тегов
// подписки, а если он пуст, то `<номер>:`» плюс `select`. Импорт другого
// источника тега не имеет и обязан воспроизвести ту же формулу
// (backupReplaceTag), иначе правила ТОГО ЖЕ файла уедут в никуда.
//
// Значит тег, не совпавший с деривативом, круг не переживает: на приёмнике
// группа будет называться иначе, а правила, метившие в старое имя, приедут
// выключенными. Молчать об этом нельзя (П6) — экспорт называет расхождение.
func replaceTagSurvivesExport(src state.Source, index int) (derived string, ok bool) {
	if src.Replace == nil {
		return "", true
	}
	want := strings.TrimSpace(src.Replace.Tag)
	prefix := ""
	if src.TagPolicy != nil {
		prefix = src.TagPolicy.Prefix
	}
	derived = legacyFoldPrefix(prefix, index) + "select"
	if want == "" || want == derived {
		return derived, true
	}
	return derived, false
}

// legacyFoldPrefix — префикс групп прежней свёртки: тег-префикс подписки с
// позиционным умолчанием «<номер>:». Формула воспроизведена байт-в-байт (в
// т. ч. TrimSpace: старый движок обрезал префикс, и `"[P] "` давал `[P]select`)
// — по этим тегам ссылались правила живых состояний.
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
// сторон один — сборка конфига (chain_hop_missing). Предупреждать здесь
// значило бы обвешать каждый нормальный restore ложной тревогой и разойтись
// с общим корпусом, который считает такие файлы чистыми.
//
// Индекс строится по СЫРЫМ тегам узлов контейнеров; корневые узлы
// (server/chain/auto), replace-теги и Направления живут в корневом
// пространстве и адреса не требуют.
func resolveImportedHops(sources []state.Source, directions []configtypes.Direction) {
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
				if tag == "" {
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
			if id, ok := byTag[tag]; ok && !ambiguous[tag] {
				hop.FolderID = id
			}
		}
	}
}
