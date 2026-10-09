// File tags.go — занятость корневого пространства финальных тегов.
//
// Корневое имя — тег верхнего узла (server / chain / auto), тег Направления
// и его `-auto`, тег замены свёрнутой папки/подписки и его `-auto`. Узел,
// вставший в корень именем, которое уже носит другая сущность, дал бы двух
// владельцев одного имени, и в сборке они спорили бы за него. Поэтому все,
// кто кладёт узел в корень вне UI (импорт бэкапа, правка состояния через
// Debug API), считают занятость здесь. Системные теги шаблона (`direct-out`,
// тег блокировки, outbound'ы `config`) состояние не знает — их добавляет
// вызывающий, у которого есть шаблон.
package state

import (
	"fmt"
	"strings"
)

// TakenRootTags — занятые имена КОРНЕВОГО пространства, известные из самого
// состояния: теги верхних узлов, Направлений и их `-auto`-двойников, теги
// замен свёрток и их `-auto`.
//
// `-auto` у свёртки ставится при любом режиме, а не только у both: режим
// меняется одной галкой, и имя, выданное узлу при режиме select, стало бы
// чужим при переключении.
func TakenRootTags(s *State) map[string]bool {
	taken := map[string]bool{}
	if s == nil {
		return taken
	}
	add := func(tag string) {
		if tag = strings.TrimSpace(tag); tag != "" {
			taken[tag] = true
		}
	}
	for i := range s.Sources {
		src := &s.Sources[i]
		switch src.Kind {
		case SourceKindServer, SourceKindChain, SourceKindAuto:
			add(src.NodeTagOrLabel())
		}
		if src.Replace != nil && strings.TrimSpace(src.Replace.Tag) != "" {
			add(src.Replace.Tag)
			add(src.Replace.Tag + "-auto")
		}
	}
	for i := range s.Directions {
		d := &s.Directions[i]
		add(d.Tag)
		if d.Auto != nil {
			add(d.AutoTag())
		}
	}
	return taken
}

// UniqueTag подбирает свободное имя вида `X`, `X-2`, `X-3` — та же форма
// суффикса, что у ручного добавления узла в Конфигураторе и у уникализации
// на эмиссии, чтобы имена из разных путей выглядели одинаково. Пустой тег
// возвращается как есть.
func UniqueTag(taken map[string]bool, tag string) string {
	if tag == "" || !taken[tag] {
		return tag
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", tag, n)
		if !taken[candidate] {
			return candidate
		}
	}
}
