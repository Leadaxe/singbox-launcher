// File subscription_merge.go — merge свежеразобранного тела подписки в
// материализованные nodes[] контейнера (SPEC 118 W3, PLAN §3.1; SPEC 116 W1;
// правила — features/sources.md «Подписка: fetch» / «Наполнение папки»,
// features/state.md «Материализация»).
//
// Merge живёт в state, а не в fetch-сервисе: правила принадлежат модели
// (тот же уровень, что normalizeSourceShape), а fetch-сервис лишь скачал и
// разобрал тело. Ключ merge — СЫРОЙ (уникализированный парсером) тег: на нём
// висят enabled, detour и все ссылки NodeLink, поэтому обновление подписки
// их не рвёт.
//
// Ядро merge одно на два контейнера — разница только в ПОЛИТИКЕ исчезнувшего
// узла и в том, какие старые узлы вообще участвуют в сопоставлении:
//
//   - подписка (MergeSubscriptionNodes): участвуют все узлы; исчезнувший
//     УДАЛЯЕТСЯ — состав подписки принадлежит провайдеру;
//   - папка (MergeFolderNodesFromSubscription): участвуют только узлы с
//     origin.subUrl == залитого URL; исчезнувший РАЗЫМЕНОВЫВАЕТСЯ (origin.subUrl
//     обнуляется), но остаётся на своём месте — папка территория пользователя,
//     подписка из неё не удаляет (features/sources.md §«Наполнение папки»).
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
	// «остался за капом»). Для папки тем же запретом закрыто разыменование.
	Truncated bool
}

// refreshMergedNode переносит пользовательские пометки со старого узла на
// свежий: enabled и (только у Server) detour. Общая половина обоих merge.
//
// Возвращает warning, если detour потерян сменой вида узла у провайдера.
func refreshMergedNode(fresh *Node, old *Node) string {
	if old.IsUnsupported() && !fresh.IsUnsupported() {
		// Запись починилась у провайдера: узел оживает ОБЫЧНЫМ, на своём
		// месте. `enabled=false` неразобранной записи — не выбор пользователя,
		// а её собственная невозможность (включение у неё запрещено), и
		// перенести эту отметку на живой узел значило бы выключить его за
		// пользователя. Свежий узел приходит включённым — как всякий новый.
		return ""
	}
	fresh.Enabled = old.Enabled
	if fresh.IsUnsupported() {
		// Обратный случай: узел сломался у провайдера. Включённой
		// неразобранная запись не бывает — модель этой формы не держит.
		fresh.Enabled = false
	}
	if old.Detour == nil {
		return ""
	}
	if fresh.Kind == SourceKindServer {
		d := *old.Detour
		fresh.Detour = &d
		return ""
	}
	// detour существует только у Server (SPEC Т2): узел сменил вид у
	// провайдера — пометка теряется, но не молча.
	return fmt.Sprintf("node %q: detour dropped — node kind changed to %q", fresh.Tag, fresh.Kind)
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
		// Копия ПОВЕРХНОСТНАЯ: Origin/Detour/Hops/Group — указатели и слайсы,
		// общие с материалом вызывающего. Писать по ним на месте нельзя;
		// merge и не пишет — refreshMergedNode сажает detour на клон, а
		// setNodeSubURL — origin.
		n := fresh
		freshTags[n.Tag] = true
		if old, ok := oldByTag[n.Tag]; ok {
			// Совпавший: пользовательские пометки живут, содержимое — свежее.
			if w := refreshMergedNode(&n, old); w != "" {
				warns = append(warns, w)
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

// MergeFolderNodesFromSubscription заливает свежий материал подписки subURL в
// nodes[] ПАПКИ (SPEC 116 §Д1, критерий A5). Тот же merge по сырому тегу, но
// с политикой папки:
//
//   - участвуют только узлы папки с origin.subUrl == subURL; узлы с другим
//     subUrl или без него не сопоставляются, не двигаются и не трогаются
//     вовсе — папка может держать вручную созданные узлы и заливки из
//     нескольких подписок;
//   - совпал → body/origin.raw освежены, enabled/detour/ПОЗИЦИЯ сохранены
//     (порядок папки задаёт пользователь, а не провайдер);
//   - новый → добавлен включённым в хвост, origin.subUrl проставлен;
//   - исчезнувший у провайдера → НЕ удаляется: origin.subUrl обнуляется
//     (узел становится ручным) + warning;
//   - Truncated=true → разыменование не выполняется вовсе («исчез»
//     неотличим от «остался за капом», SPEC 113-A);
//   - trusted=false → nodes[] не трогаются вообще (как у подписки);
//   - Auto, приехавший заливкой, переуказывает members на узлы ПАПКИ
//     (repointFolderAutoMembers); член без копии в папке — prune с warning.
//
// PendingDisabled у папки не существует (поле подписочное) — не трогается.
//
// Возвращает (changed, warnings) в том же смысле, что MergeSubscriptionNodes.
func MergeFolderNodesFromSubscription(folder *Source, subURL string, res *SubFetchMaterial, trusted bool) (bool, []string) {
	if folder == nil || folder.Kind != SourceKindFolder || subURL == "" {
		return false, nil
	}
	if !trusted || res == nil {
		return false, nil
	}

	var warns []string
	before, _ := json.Marshal(folder.Nodes)

	// Индекс участвующих: только узлы этой заливки. Тег уникален в рамках
	// контейнера, но узел с тем же тегом и ЧУЖИМ subUrl участником не
	// становится — переписать его заливкой значило бы украсть чужой узел.
	ownedIdx := make(map[string]int, len(folder.Nodes))
	for i := range folder.Nodes {
		if nodeSubURL(&folder.Nodes[i]) == subURL {
			ownedIdx[folder.Nodes[i].Tag] = i
		}
	}

	nodes := folder.Nodes
	freshTags := make(map[string]bool, len(res.Nodes))
	// touched — сырые теги узлов, реально приехавших ЭТОЙ заливкой (совпавшие
	// освежены + добавленные). Отвергнутые коллизией сюда не попадают: чужой
	// узел с тем же тегом заливка не трогает и его members не переписывает.
	touched := make(map[string]bool, len(res.Nodes))
	var added []Node
	for _, fresh := range res.Nodes {
		// Поверхностная копия — см. MergeSubscriptionNodes. Здесь это особенно
		// важно: материал заливки — живые узлы подписки-источника, и правка
		// origin на месте отвязала бы их от провайдера.
		n := fresh
		freshTags[n.Tag] = true
		if idx, ok := ownedIdx[n.Tag]; ok {
			// Совпавший: содержимое свежее, пометки и ПОЗИЦИЯ старые.
			if w := refreshMergedNode(&n, &nodes[idx]); w != "" {
				warns = append(warns, w)
			}
			setNodeSubURL(&n, subURL)
			nodes[idx] = n
			touched[n.Tag] = true
			continue
		}
		if nodeTagTaken(nodes, n.Tag) || nodeTagTaken(added, n.Tag) {
			// Тег занят чужим узлом папки (ручным или заливкой другой
			// подписки): подменять его нельзя, а второй узел с тем же сырым
			// тегом порвал бы идентичность в контейнере. Узел деградирует
			// с warning — перенос решает пользователь.
			warns = append(warns, fmt.Sprintf("node %q: raw tag already taken in folder by another node — not merged", n.Tag))
			continue
		}
		// Новый: включён, помечен родной подпиской, в хвост.
		n.Enabled = true
		setNodeSubURL(&n, subURL)
		touched[n.Tag] = true
		added = append(added, n)
	}

	// Исчезнувшие у провайдера: разыменование, не удаление. При truncated
	// не выполняется вовсе.
	if !res.Truncated {
		for i := range nodes {
			if nodeSubURL(&nodes[i]) != subURL {
				continue
			}
			if freshTags[nodes[i].Tag] {
				continue
			}
			setNodeSubURL(&nodes[i], "")
			warns = append(warns, fmt.Sprintf("node %q: disappeared at provider — kept in folder, unlinked from subscription", nodes[i].Tag))
		}
	}

	if len(added) > 0 {
		nodes = append(nodes, added...)
	}
	folder.Nodes = nodes

	// Auto приехал из ЧУЖОГО контейнера: его members адресуют подписку-источник
	// (features/sources.md §Auto — «Auto при копии/заливке в папку переуказывает
	// members на узлы папки»). Пройти обязательно ПОСЛЕ того, как состав папки
	// сложился целиком: член мог приехать этой же заливкой и лечь в хвост позже
	// самой группы.
	warns = append(warns, repointFolderAutoMembers(folder, subURL, touched)...)

	after, _ := json.Marshal(folder.Nodes)
	return !bytes.Equal(before, after), warns
}

// repointFolderAutoMembers переуказывает members Auto-узлов, приехавших этой
// заливкой, на узлы ПАПКИ (совпадение по сырому тегу); член, чьей копии в
// папке не оказалось, — prune с warning (features/sources.md §Auto).
//
// Почему это ядро merge, а не отдельный проход UI: решение «член не попал»
// принимается ровно тем знанием, которым владеет merge — состав папки ПОСЛЕ
// заливки. Пересчитывать его снаружи значило бы завести вторую копию правил
// коллизии тега, а с ней и расхождение.
//
// Трогаются только узлы `touched` — приехавшие этой заливкой. Ручной Auto
// папки и заливка другой подписки свои members уже настроили, и «поправить»
// их здесь означало бы стереть чужое решение.
//
// Копией члена считается узел папки, принадлежащий ЭТОЙ ЖЕ заливке
// (`origin.subUrl == subURL`), а не любой одноимённый: тег, отбитый коллизией,
// занят ЧУЖИМ узлом (ручным или заливкой другой подписки), и увести член на
// него значило бы молча подменить состав группы соседним узлом пользователя.
// Такой член — как раз тот, «чья копия в папку не попала».
//
// Prune, а не fail-closed: битый член выпадает, группа живёт (features/
// sources.md §Auto). Группа, потерявшая всех членов, здесь НЕ удаляется —
// пустую группу отбрасывает эмиссия с собственным warning, а узел из папки
// молча вынести нельзя: папка территория пользователя.
func repointFolderAutoMembers(folder *Source, subURL string, touched map[string]bool) []string {
	if folder == nil || len(touched) == 0 {
		return nil
	}
	inFolder := make(map[string]bool, len(folder.Nodes))
	for i := range folder.Nodes {
		if nodeSubURL(&folder.Nodes[i]) == subURL {
			inFolder[folder.Nodes[i].Tag] = true
		}
	}

	var warns []string
	for i := range folder.Nodes {
		n := &folder.Nodes[i]
		if n.Kind != SourceKindAuto || n.Group == nil || !touched[n.Tag] {
			continue
		}
		if nodeSubURL(n) != subURL {
			continue
		}
		kept := make([]NodeLink, 0, len(n.Group.Members))
		for _, mem := range n.Group.Members {
			if !inFolder[mem.Tag] {
				warns = append(warns, fmt.Sprintf(
					"group %q: member %q has no copy in the folder — pruned", n.Tag, mem.Tag))
				continue
			}
			kept = append(kept, NodeLink{FolderID: folder.ID, Tag: mem.Tag})
		}
		// Members копируются в НОВЫЙ слайс: узел — поверхностная копия материала
		// вызывающего, и правка на месте переписала бы состав группы САМОЙ
		// подписки (тот же довод, что у setNodeSubURL).
		g := *n.Group
		g.Members = kept
		// Умолчание селектора — тоже ссылка на члена: снявшийся с состава
		// default роняет селектор, поэтому он снимается вместе с членом
		// (features/sources.md §Auto — «обязан входить в состав»).
		if g.Default != "" && !inFolder[g.Default] {
			warns = append(warns, fmt.Sprintf(
				"group %q: default %q has no copy in the folder — cleared", n.Tag, g.Default))
			g.Default = ""
		}
		n.Group = &g
	}
	return warns
}

// nodeSubURL — subUrl узла ("" у ручного узла и узла без origin).
func nodeSubURL(n *Node) string {
	if n == nil || n.Origin == nil {
		return ""
	}
	return n.Origin.SubURL
}

// setNodeSubURL проставляет/обнуляет origin.subUrl. Пустой url на узле без
// origin — не повод рождать пустой origin.
//
// Origin ПЕРЕСАЖИВАЕТСЯ на клон, а не правится на месте: Node — структура с
// указателем Origin, поэтому копия узла (`n := fresh`) делит origin с
// оригиналом. Заливка папки работает как раз на таких копиях материала
// вызывающего (SubFetchMaterial.Nodes — живые узлы подписки), и правка на
// месте переписала бы origin.subUrl САМОЙ подписки, отвязав её узлы от
// провайдера. Клон рвёт это разделение; поля Origin — только строки, так что
// поверхностного копирования структуры достаточно.
func setNodeSubURL(n *Node, url string) {
	if n == nil {
		return
	}
	if n.Origin == nil {
		if url == "" {
			return
		}
		n.Origin = &Origin{SubURL: url}
		return
	}
	o := *n.Origin
	o.SubURL = url
	n.Origin = &o
}

// nodeTagTaken — сырой тег уже занят каким-то узлом набора.
func nodeTagTaken(nodes []Node, tag string) bool {
	for i := range nodes {
		if nodes[i].Tag == tag {
			return true
		}
	}
	return false
}
