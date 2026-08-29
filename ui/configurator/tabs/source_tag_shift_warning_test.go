// File source_tag_shift_warning_test.go — предупреждение о протухании
// ручного выбора в селекторах (SPEC 118 Т8, features/directions.md §10).
//
// Одну ссылку лаунчер переписать не может: выбор селекторов в кэше живого
// ядра (cache.db) адресован тегами. Значит любая операция, меняющая финальный
// тег, обязана предупредить — и ровно она, а не любая правка подряд: диалог
// на каждое сохранение приучают закрывать не читая.
package tabs

import (
	"testing"

	corestate "singbox-launcher/core/state"
)

func shapeOf(prefix, postfix, replaceTag, mode string) sourceTagShape {
	src := &corestate.Source{}
	if prefix != "" || postfix != "" {
		src.TagPolicy = &corestate.TagPolicy{Prefix: prefix, Postfix: postfix}
	}
	if replaceTag != "" {
		src.Replace = &corestate.FolderReplace{Tag: replaceTag, Mode: mode}
	}
	return tagShapeOf(src)
}

// Правка, не трогающая теги, предупреждения не даёт.
func TestStaleSelectionSilentWithoutTagChange(t *testing.T) {
	sh := shapeOf("[NL] ", "", "nl", corestate.FolderReplaceManual)
	if got := staleSelectionAfterEdit(sh, sh); !got.Empty() {
		t.Errorf("правка без смены тегов дала предупреждение: %+v", got)
	}
}

// Смена префикса переименовывает КАЖДЫЙ узел контейнера — протухает выбор
// члена в любой группе, куда эти узлы входят.
func TestStaleSelectionOnTagPolicyChange(t *testing.T) {
	before := shapeOf("[NL] ", "", "", "")
	after := shapeOf("[Netherlands] ", "", "", "")

	got := staleSelectionAfterEdit(before, after)
	if !got.NodesRenamed {
		t.Error("смена префикса не объявлена переименованием узлов")
	}
	if len(got.GroupTags) != 0 {
		t.Errorf("теги групп не менялись, но названы: %v", got.GroupTags)
	}

	// Постфикс — та же операция над тем же финальным тегом.
	if got := staleSelectionAfterEdit(shapeOf("", "", "", ""), shapeOf("", " ✅", "", "")); !got.NodesRenamed {
		t.Error("смена постфикса не объявлена переименованием узлов")
	}
}

// Смена тега замены уводит выбор В САМОЙ группе; при режиме both — ещё и у
// `-auto`-двойника. Названы теги СТАРОЙ формы: именно их помнит кэш.
func TestStaleSelectionOnReplaceTagChange(t *testing.T) {
	before := shapeOf("", "", "de", corestate.FolderReplaceBoth)
	after := shapeOf("", "", "germany", corestate.FolderReplaceBoth)

	got := staleSelectionAfterEdit(before, after)
	if got.NodesRenamed {
		t.Error("тег-политика не менялась, но узлы объявлены переименованными")
	}
	want := []string{"de", "de-auto"}
	if len(got.GroupTags) != len(want) {
		t.Fatalf("теги групп = %v, ожидали %v", got.GroupTags, want)
	}
	for i := range want {
		if got.GroupTags[i] != want[i] {
			t.Errorf("тег групп[%d] = %q, ожидали %q (кэш помнит СТАРЫЕ имена)", i, got.GroupTags[i], want[i])
		}
	}
}

// Смена режима свёртки меняет СОСТАВ тегов: manual → both рождает двойника,
// both → manual его убивает. Выбор не переживает ни того, ни другого.
func TestStaleSelectionOnReplaceModeChange(t *testing.T) {
	both := shapeOf("", "", "de", corestate.FolderReplaceBoth)
	manual := shapeOf("", "", "de", corestate.FolderReplaceManual)

	if got := staleSelectionAfterEdit(both, manual); len(got.GroupTags) != 2 {
		t.Errorf("both → manual: теги = %v, ожидали оба старых", got.GroupTags)
	}
	if got := staleSelectionAfterEdit(manual, both); len(got.GroupTags) != 1 {
		t.Errorf("manual → both: теги = %v, ожидали один старый", got.GroupTags)
	}
}

// Появление и исчезновение свёртки — тоже смена тегов.
func TestStaleSelectionOnReplaceAppearsAndDisappears(t *testing.T) {
	none := shapeOf("", "", "", "")
	folded := shapeOf("", "", "de", corestate.FolderReplaceManual)

	// Свёртка снята: тег «de» из конфига исчез, выбор в нём протух.
	if got := staleSelectionAfterEdit(folded, none); len(got.GroupTags) != 1 || got.GroupTags[0] != "de" {
		t.Errorf("снятие свёртки: теги = %v, ожидали [de]", got.GroupTags)
	}
	// Свёртка включена: старых тегов не было — называть нечего, но само
	// событие смены состава групп предупреждения не требует (кэш о них
	// ничего не помнит).
	if got := staleSelectionAfterEdit(none, folded); len(got.GroupTags) != 0 {
		t.Errorf("включение свёртки назвало теги, которых кэш не видел: %v", got.GroupTags)
	}
}
