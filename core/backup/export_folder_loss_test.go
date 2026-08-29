package backup

// SPEC 116 W9, критерий A9: экспорт НИКОГДА не отдаёт успешный файл, молча
// потерявший папку.
//
// Тест смотрит не на текст (это дело UI), а на данные, из которых UI обязан
// собрать фразу «папка N и её M узлов в файл не попали»: код, имя, вид и
// объём. Раньше вид папки приклеивался к началу Detail, а числа не было
// вовсе — предупреждение звучало как оговорка формата, хотя означало, что
// восстанавливаться из этого файла нельзя.

import (
	"testing"

	"singbox-launcher/core/state"
)

// folderLossState — состояние с папкой, провайдерской группой, подпиской и
// пустой папкой: все четыре ветки switch'а экспорта разом.
func folderLossState() *state.State {
	return &state.State{Sources: []state.Source{
		{
			ID:   "01SUB0000000000000000000",
			Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			Name: "Provider",
		},
		{
			ID:   "01FLD0000000000000000000",
			Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Name: "Работа",
			Nodes: []state.Node{
				{Kind: state.SourceKindServer, Tag: "n-1", Enabled: true},
				{Kind: state.SourceKindServer, Tag: "n-2", Enabled: false},
				{Kind: state.SourceKindServer, Tag: "n-3", Enabled: true},
			},
		},
		{
			ID:   "01FLD0000000000000001111",
			Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Name: "Пустая",
		},
		{
			ID:   "01AUT0000000000000000000",
			Node: state.Node{Kind: state.SourceKindAuto, Enabled: true, Tag: "provider-auto"},
		},
	}}
}

// Папка называется предупреждением поимённо, вместе с числом своих узлов.
func TestExportNamesLostFolderWithNodeCount(t *testing.T) {
	_, warns, err := Export(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var folder *Warning
	for i := range warns {
		if warns[i].Code == WarnBackupSourceKindUnsupported &&
			warns[i].Kind == string(state.SourceKindFolder) &&
			warns[i].Detail == "Работа" {
			folder = &warns[i]
			break
		}
	}
	if folder == nil {
		t.Fatalf("папка выпала из файла без предупреждения: %v", warns)
	}
	// Выключенный узел тоже потерян: пользователь его настроил и увидит
	// пропажу после restore ровно так же, как включённый.
	if folder.Nodes != 3 {
		t.Errorf("объём потери назван как %d узлов, в папке 3", folder.Nodes)
	}
	if folder.Kind != string(state.SourceKindFolder) {
		t.Errorf("вид записи потерян: %q — UI не сможет отличить папку от группы", folder.Kind)
	}
}

// Пустая папка — тоже потеря, но без числа: терять нечего кроме самой папки.
func TestExportNamesEmptyFolderWithoutCount(t *testing.T) {
	_, warns, err := Export(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	for _, w := range warns {
		if w.Detail == "Пустая" {
			if w.Nodes != 0 {
				t.Errorf("у пустой папки насчитано %d узлов", w.Nodes)
			}
			return
		}
	}
	t.Fatal("пустая папка выпала молча: пустота — воля пользователя, а не отсутствие записи")
}

// Провайдерская группа отличима от папки: код у потери общий, слова разные.
func TestExportDistinguishesAutoFromFolder(t *testing.T) {
	_, warns, err := Export(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	for _, w := range warns {
		if w.Kind == string(state.SourceKindAuto) {
			if w.Detail != "provider-auto" {
				t.Errorf("группа названа %q, ожидался её тег", w.Detail)
			}
			return
		}
	}
	t.Fatal("провайдерская группа выпала без предупреждения")
}

// Потерянные ЦЕЛИКОМ записи идут первыми (§O1=А, «первой строкой»): под
// списком переименованных тегов замены папка была бы прочитана последней или
// не прочитана вовсе.
func TestExportPutsWholeRecordLossesFirst(t *testing.T) {
	s := folderLossState()
	// Подписка с неверифицируемым тегом замены даёт warning «приехало иначе»
	// и стоит в списке источников ПЕРЕД папкой — то есть в порядке обхода
	// попала бы наверх.
	s.Sources[0].Replace = &state.FolderReplace{
		Mode: state.FolderReplaceManual,
		Tag:  "совершенно-своё-имя",
	}

	_, warns, err := Export(s, ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(warns) < 2 {
		t.Fatalf("ожидались и потеря записи, и переименование тега: %v", warns)
	}
	// Хвост списка обязан содержать переименование — иначе тест проверяет не
	// сортировку, а отсутствие второго предупреждения.
	var sawDerived bool
	for _, w := range warns {
		if w.Code == WarnBackupReplaceTagDerived {
			sawDerived = true
		}
	}
	if !sawDerived {
		t.Fatalf("тег замены не дал предупреждения — сортировать нечего: %v", warns)
	}
	if warns[0].Code != WarnBackupSourceKindUnsupported {
		t.Errorf("первым идёт %q, а потеря записи целиком — ниже: %v", warns[0].Code, warns)
	}
}
