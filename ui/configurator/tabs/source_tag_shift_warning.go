// File source_tag_shift_warning.go — предупреждение о протухании ручного
// выбора в селекторах при смене финальных тегов (SPEC 118 Т8,
// features/directions.md §10).
//
// Ручной выбор пользователя в селекторе (Направление, замена свёрнутой папки,
// провайдерская Auto-группа) в модели НЕ хранится: его помнит кэш живого ядра
// (cache.db) парой «тег группы → тег члена». Двусторонней синхронизации с
// Clash API нет — и это осознанное решение модели, а не пробел.
//
// Следствие: любая операция, меняющая ФИНАЛЬНЫЙ тег, оставляет в кэше ссылку
// на имя, которого больше нет, и выбор сбивается на умолчание. Лаунчер эту
// ссылку переписать не может — она на чужой стороне (а у Remote-машины ещё и
// на чужой машине). Единственное, что он может и обязан, — предупредить.
//
// Две операции окна источника меняют финальные теги:
//
//   - правка тег-политики (prefix/postfix): финальный тег КАЖДОГО узла
//     контейнера считается по ней, и смена префикса переименовывает разом
//     всю подписку;
//   - правка `replace.tag` (и режима свёртки): это тег самой группы, и
//     смена уводит выбор В НЕЙ, а при режиме both — ещё и у `-auto`-двойника.
//
// Предупреждение — существующим механизмом: тот же информирующий диалог, что
// у сброса ссылок при переименовании узла (showDetourRefsResetDialog).
// Отменять в нём нечего: правка уже сохранена, а выбор в кэше — не наша
// собственность.
package tabs

import (
	"strings"

	corestate "singbox-launcher/core/state"
)

// sourceTagShape — то в источнике, от чего зависят финальные теги его узлов
// и групп. Снимок делается на открытии окна и сравнивается на сохранении.
type sourceTagShape struct {
	prefix      string
	postfix     string
	replaceTag  string
	replaceMode string
}

// tagShapeOf — снимок формы тегов источника.
func tagShapeOf(src *corestate.Source) sourceTagShape {
	var sh sourceTagShape
	if src == nil {
		return sh
	}
	if src.TagPolicy != nil {
		sh.prefix = strings.TrimSpace(src.TagPolicy.Prefix)
		sh.postfix = strings.TrimSpace(src.TagPolicy.Postfix)
	}
	if src.Replace != nil {
		sh.replaceTag = strings.TrimSpace(src.Replace.Tag)
		sh.replaceMode = src.Replace.Mode
	}
	return sh
}

// staleSelectionScope — ЧТО именно протухнет в кэше ядра.
type staleSelectionScope struct {
	// NodesRenamed — переименованы узлы контейнера (сменилась тег-политика):
	// протухает выбор ЧЛЕНА в любой группе, куда эти узлы входят.
	NodesRenamed bool
	// GroupTags — теги групп, чьё ИМЯ сменилось: протухает выбор в них самих.
	// Порядок — селектор, затем `-auto`-двойник (формула твинов).
	GroupTags []string
}

// Empty — предупреждать не о чем.
func (s staleSelectionScope) Empty() bool {
	return !s.NodesRenamed && len(s.GroupTags) == 0
}

// staleSelectionAfterEdit — что протухнет в кэше ядра из-за правки источника.
//
// Сравниваются именно ФОРМЫ тегов, а не источники целиком: правка URL, скипа,
// подписи или галок узлов финальных тегов не трогает и выбор не сбивает —
// предупреждать о ней значило бы приучить закрывать диалог не читая.
//
// Появление и исчезновение свёртки — тоже смена: группы `<tag>`/`<tag>-auto`
// либо родились, либо исчезли, и выбор в них не переживает ни того, ни
// другого. Названы теги СТАРОЙ формы: именно их помнит кэш.
func staleSelectionAfterEdit(before, after sourceTagShape) staleSelectionScope {
	var out staleSelectionScope

	if before.prefix != after.prefix || before.postfix != after.postfix {
		out.NodesRenamed = true
	}

	oldTags := replaceTagsOfShape(before)
	newTags := replaceTagsOfShape(after)
	if !sameStrings(oldTags, newTags) {
		// Протухают теги, которые кэш ПОМНИТ, — то есть прежние. Новых он
		// ещё не видел, и говорить о них нечего.
		out.GroupTags = oldTags
	}
	return out
}

// replaceTagsOfShape — теги замены по снимку формы (формула twins:
// both → селектор + `<tag>-auto`).
func replaceTagsOfShape(sh sourceTagShape) []string {
	if sh.replaceTag == "" {
		return nil
	}
	if sh.replaceMode == corestate.FolderReplaceBoth {
		return []string{sh.replaceTag, sh.replaceTag + "-auto"}
	}
	return []string{sh.replaceTag}
}

// sameStrings — равенство списков тегов по порядку и составу.
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
