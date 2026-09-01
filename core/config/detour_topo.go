// File detour_topo.go — топологический порядок резолва detour-на-узел и
// каскад fail-closed (SPEC 113-B).
//
// # Почему порядок вообще важен
//
// Резолв ссылки «source_id + identity-тег» происходит на проходе 2 сборки, по
// списку источников в том порядке, в котором они лежат в состоянии. Порядок в
// состоянии — пользовательский, к зависимостям отношения не имеющий: источник
// B, задетуренный на узел источника A, имеет полное право стоять ВЫШЕ A.
//
// Пока каждая ссылка проверялась независимо, это было безразлично: цель либо
// есть среди узлов, либо нет. Но с каскадом (SPEC 113-B) исход зависит от
// СОСЕДА: если A выпал fail-closed, его узлы из конфига исчезли, и хоп, на
// который смотрит B, больше не существует — значит B обязан выпасть тоже.
// Ответ на «существует ли цель» становится верным только после того, как
// решена судьба источника-цели. Отсюда порядок: цель раньше ссылающегося.
//
// # Форма графа
//
// У источника не больше ОДНОЙ исходящей node-ссылки (пикер пишет одну; поля
// detour_node_source_id/detour_node_tag скалярные). Значит граф — набор
// функциональных цепочек, каждая из которых либо упирается в источник без
// ссылки, либо замыкается в кольцо. Топологическая сортировка такого графа
// тривиальна и не требует ни Кана, ни рекурсии: идём по единственному ребру,
// пока не упрёмся в уже посещённое.
//
// # Кольца
//
// Кольцо A↔B ядро отвергает целиком («dependency not found»), а прежде оно
// доезжало до граф-санитайзера и разрывалось СНЯТИЕМ detour — то есть узлы
// одного из источников начинали ходить напрямую. Для detour это запрещено
// (SPEC 113: «detour — управление анонимностью, не балансировка»): участники
// кольца выпадают ВСЕ, каждый со своей причиной, называющей цикл.
package config

import (
	"fmt"
)

// detourEdge — единственная исходящая node-ссылка источника: индекс
// источника-цели. targetKnown=false, когда цель по ссылке не находится вовсе
// (источник удалён, tag-only ссылка не разрешается) — такое ребро в граф не
// входит, исход решается обычным резолвом.
type detourEdge struct {
	target      int
	targetKnown bool
}

// detourSourceOrder — порядок обхода источников со ссылками и множество
// участников колец.
//
// order — индексы refSources, в котором цель встречается РАНЬШЕ ссылающегося.
// inCycle — источники, чьи рёбра замкнулись в кольцо: их резолв не начинается
// вовсе, они выпадают fail-closed все разом.
//
// Источники, не входящие в refSources (без ссылки), в order не попадают: их
// резолвить нечего, но рёбра на них учитываются как терминалы.
func detourSourceOrder(refSources []int, edges map[int]detourEdge) (order []int, inCycle map[int]bool) {
	inCycle = make(map[int]bool)
	isRef := make(map[int]bool, len(refSources))
	for _, i := range refSources {
		isRef[i] = true
	}

	const (
		white = 0 // не посещён
		grey  = 1 // на текущем пути
		black = 2 // разобран
	)
	color := make(map[int]int, len(refSources))
	order = make([]int, 0, len(refSources))

	for _, start := range refSources {
		if color[start] != white {
			continue
		}
		// Идём по единственному исходящему ребру, накапливая путь. Упёрлись в
		// grey — вершины пути ОТ этой точки и до конца образуют кольцо.
		path := make([]int, 0, len(refSources))
		cur := start
		for {
			if color[cur] == grey {
				// Кольцо: всё от первого вхождения cur до конца пути.
				at := -1
				for k, v := range path {
					if v == cur {
						at = k
						break
					}
				}
				if at >= 0 {
					for _, v := range path[at:] {
						inCycle[v] = true
					}
				}
				break
			}
			if color[cur] == black {
				break // упёрлись в уже разобранную цепочку
			}
			color[cur] = grey
			path = append(path, cur)

			e, ok := edges[cur]
			if !ok || !e.targetKnown {
				break // ссылка никуда не ведёт — терминал, исход решит резолв
			}
			if !isRef[e.target] {
				// Цель сама ни на кого не ссылается: её судьба уже известна
				// (она либо есть, либо её нет), обходить дальше нечего.
				color[e.target] = black
				break
			}
			cur = e.target
		}
		// Разворот пути даёт «цель раньше ссылающегося»: путь строился
		// ссылающийся → цель.
		for k := len(path) - 1; k >= 0; k-- {
			color[path[k]] = black
			if !inCycle[path[k]] {
				order = append(order, path[k])
			}
		}
	}
	return order, inCycle
}

// detourCascadeReason — причина выпадения источника, чей хоп жил в другом
// источнике, который сам выпал из конфига. Называет обе стороны человеческими
// именами (SPEC 112-A): без имени промежуточного источника цепочка выпадений
// выглядит как случайная пропажа.
func detourCascadeReason(hopName, targetSourceName string) string {
	if hopName == "" {
		return fmt.Sprintf("the hop's source (%s) is excluded from the config", targetSourceName)
	}
	return fmt.Sprintf("hop %q lived in source %q, which is excluded from the config", hopName, targetSourceName)
}

// detourCycleReason — причина выпадения участника кольца.
func detourCycleReason(selfName, targetName string) string {
	return fmt.Sprintf("circular link: %q dials through %q, and the hop chain brings traffic back to %q",
		selfName, targetName, selfName)
}
