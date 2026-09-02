package backup

// SPEC 116 W9, критерий A9: экспорт НИКОГДА не отдаёт успешный файл, молча
// потерявший запись.
//
// С контрактом 0.12 папка больше не теряется: её члены-серверы едут
// записями servers[] с пометкой folder, и потерей остаются лишь настройки
// САМОЙ папки (backup_local_only_dropped). Провайдерская группа — вид,
// которого контракт по-прежнему не знает: она уезжает целиком, и тест
// смотрит на данные, из которых UI обязан собрать фразу «группа N и её M
// узлов в файл не попали»: код, имя, вид и объём.

import (
	"strings"
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

// Состав папки ЕДЕТ (контракт 0.12): члены становятся записями servers[] с
// пометкой folder, и потери состава больше нет. Именно эта потеря делала
// файл негодным к восстановлению, и её отсутствие — главное, что здесь
// проверяется.
func TestExportCarriesFolderMembers(t *testing.T) {
	b, warns, err := Export(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	for _, w := range warns {
		if w.Code == WarnBackupSourceKindUnsupported && w.Kind == string(state.SourceKindFolder) {
			t.Errorf("папка объявлена неподдержанной, хотя её состав едет: %v", w)
		}
	}
	var got []string
	for _, srv := range b.Servers {
		if srv.Folder == "Работа" {
			got = append(got, srv.NodeTag)
		}
	}
	// Порядок членов нормативен: приёмник собирает папку в порядке записей.
	want := []string{"n-1", "n-2", "n-3"}
	if len(got) != len(want) {
		t.Fatalf("в файл поехало %d членов папки, ожидалось %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("член %d = %q, ожидался %q (порядок нормативен)", i, got[i], want[i])
		}
	}
	// Выключенный узел едет выключенным: пользователь его настроил.
	for _, srv := range b.Servers {
		if srv.NodeTag == "n-2" && (srv.Enabled == nil || *srv.Enabled) {
			t.Errorf("выключенный член приехал включённым: %+v", srv)
		}
	}
}

// Пустая папка данных не несёт: её имя живёт только на записях членов, а
// членов нет. Предупреждать не о чем — терять нечего.
func TestExportEmptyFolderIsSilent(t *testing.T) {
	_, warns, err := Export(folderLossState(), ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	for _, w := range warns {
		if w.Detail == "Пустая" || strings.HasPrefix(w.Detail, "Пустая:") {
			t.Errorf("пустая папка без настроек объявлена потерей: %v", w)
		}
	}
}

// Настройки САМОЙ папки в схему не входят: одно предупреждение на папку с
// перечнем, а не строка на каждый ключ.
func TestExportNamesFolderOwnSettings(t *testing.T) {
	s := folderLossState()
	s.Sources[1].TagPolicy = &state.TagPolicy{Prefix: "w-"}
	s.Sources[1].Replace = &state.FolderReplace{Mode: state.FolderReplaceManual, Tag: "work"}

	_, warns, err := Export(s, ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var detail string
	var count int
	for _, w := range warns {
		if w.Code == WarnBackupLocalOnlyDropped && strings.HasPrefix(w.Detail, "Работа:") {
			detail = w.Detail
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ожидался один warning на папку, получено %d: %v", count, warns)
	}
	for _, want := range []string{"tag_policy", "replace"} {
		if !strings.Contains(detail, want) {
			t.Errorf("не названа осевшая настройка %q: %q", want, detail)
		}
	}
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
