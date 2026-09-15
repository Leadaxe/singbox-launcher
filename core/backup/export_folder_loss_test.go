package backup

// SPEC 116 W9, критерий A9: экспорт НИКОГДА не отдаёт успешный файл, молча
// потерявший запись.
//
// В формате 1.0 папка — своя запись sources[] с составом в nodes[], и
// теряться у неё больше нечему. Целиком не едет только корневая
// провайдерская группа — вид, которого union sources[] не выражает, — и тест
// смотрит на данные, из которых UI обязан собрать фразу «группа N и её M
// узлов в файл не попали»: код, имя, вид и объём.

import (
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// folderLossState — состояние с папкой, провайдерской группой, подпиской и
// пустой папкой: все четыре ветки экспорта разом.
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

// Состав папки ЕДЕТ: запись папки несёт своих членов в nodes[]. Именно эта
// потеря делала файл негодным к восстановлению (SPEC 116), и её отсутствие —
// главное, что здесь проверяется.
func TestExportCarriesFolderMembers(t *testing.T) {
	b, warns, err := Export10(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	for _, w := range warns {
		if w.Code == WarnBackupSourceKindUnsupported && w.Kind == string(state.SourceKindFolder) {
			t.Errorf("папка объявлена неподдержанной, хотя её состав едет: %v", w)
		}
	}
	var folder *Source10
	for i := range b.Sources {
		if b.Sources[i].Kind == state.SourceKindFolder && b.Sources[i].Name == "Работа" {
			folder = &b.Sources[i]
		}
	}
	if folder == nil {
		t.Fatalf("папка не поехала в файл: %+v", b.Sources)
	}
	// Порядок членов нормативен: приёмник собирает папку в порядке записей.
	want := []string{"n-1", "n-2", "n-3"}
	if len(folder.Nodes) != len(want) {
		t.Fatalf("в файл поехало %d членов папки, ожидалось %d: %+v", len(folder.Nodes), len(want), folder.Nodes)
	}
	for i := range want {
		if folder.Nodes[i].Tag != want[i] {
			t.Errorf("член %d = %q, ожидался %q (порядок нормативен)", i, folder.Nodes[i].Tag, want[i])
		}
	}
	// Выключенный узел едет выключенным: пользователь его настроил.
	if folder.Nodes[1].Enabled {
		t.Errorf("выключенный член приехал включённым: %+v", folder.Nodes[1])
	}
}

// Пустая папка — запись без состава, а не потеря: предупреждать не о чем.
func TestExportEmptyFolderIsSilent(t *testing.T) {
	b, warns, err := Export10(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	for _, w := range warns {
		if w.Detail == "Пустая" || strings.HasPrefix(w.Detail, "Пустая:") {
			t.Errorf("пустая папка объявлена потерей: %v", w)
		}
	}
	found := false
	for _, rec := range b.Sources {
		if rec.Kind == state.SourceKindFolder && rec.Name == "Пустая" {
			found = true
		}
	}
	if !found {
		t.Error("пустая папка не поехала в файл записью — её имя и настройки потерялись бы молча")
	}
}

// Провайдерская группа отличима от папки: код у потери общий, слова разные.
func TestExportDistinguishesAutoFromFolder(t *testing.T) {
	_, warns, err := Export10(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	for _, w := range warns {
		if w.Kind == string(state.SourceKindAuto) {
			if w.Code != WarnBackupSourceKindUnsupported {
				t.Errorf("группа названа кодом %q", w.Code)
			}
			if w.Detail != "provider-auto" {
				t.Errorf("группа названа %q, ожидался её тег", w.Detail)
			}
			return
		}
	}
	t.Fatal("провайдерская группа выпала без предупреждения")
}
