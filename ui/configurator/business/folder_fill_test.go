package business

import (
	"errors"
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 116 этап 3, W6 — наполнение папки (цель 2, сценарий С1).
//
// Проверяется то, что данные, а не то, как оно нарисовано (правило
// no-ui-format-tests): узел лёг в `Source.Nodes` папки, а не в корень; тело
// материализовано; сырой тег уникален В ПРЕДЕЛАХ ПАПКИ; безымянный узел
// получил имя файла, а именованный — своё собственное; подписка в папку не
// попадает ни при каких условиях.

const fillTestURI = "vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?security=tls&sni=example.com&type=tcp"

func fillTestModel(nodes ...corestate.Node) *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{Sources: []corestate.Source{
		moveTestFolder("01FOLDER", "Proton", nodes...),
	}}
}

// Узел из вставленной ссылки едет В ПАПКУ (а не отдельным Source в корень) и
// приезжает с материализованным телом: канона нет = собирать не из чего.
func TestAppendNodesToFolder_PutsMaterializedNodeIntoFolder(t *testing.T) {
	m := fillTestModel()

	res, err := AppendNodesToFolder(m, "01FOLDER", fillTestURI+"#NL-1", "")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if res.Added != 1 {
		t.Fatalf("добавлено %d узлов, ожидался 1", res.Added)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("в корне появились лишние источники: %d", len(m.Sources))
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 1 {
		t.Fatalf("в папке %d узлов, ожидался 1", len(folder.Nodes))
	}
	n := folder.Nodes[0]
	if n.Tag != "NL-1" {
		t.Fatalf("тег узла %q, ожидался NL-1 (из #fragment)", n.Tag)
	}
	if len(n.Body) == 0 {
		t.Fatal("тело узла пусто — материализация не отработала")
	}
	if !n.Enabled {
		t.Fatal("новый узел обязан быть включён")
	}
	if n.Origin == nil || n.Origin.Raw == "" {
		t.Fatal("origin.raw пуст — узел не помнит, из чего собран")
	}
	if n.Origin.SubURL != "" {
		t.Fatalf("узел, добавленный руками, не принадлежит подписке, а subUrl = %q", n.Origin.SubURL)
	}
}

// Уникализация сырого тега считается В ПРЕДЕЛАХ ПАПКИ: сырой тег —
// идентичность узла внутри контейнера, двух одинаковых там быть не может.
func TestAppendNodesToFolder_UniquifiesRawTagWithinFolder(t *testing.T) {
	m := fillTestModel(moveTestNode("NL-1", true, ""))

	if _, err := AppendNodesToFolder(m, "01FOLDER", fillTestURI+"#NL-1", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 2 {
		t.Fatalf("в папке %d узлов, ожидалось 2", len(folder.Nodes))
	}
	if folder.Nodes[0].Tag != "NL-1" {
		t.Fatalf("прежний узел переименован в %q — уникализируется приезжающий, а не сидящий", folder.Nodes[0].Tag)
	}
	if folder.Nodes[1].Tag != "NL-1-2" {
		t.Fatalf("тег приехавшего %q, ожидался NL-1-2", folder.Nodes[1].Tag)
	}
}

// Узел приезжает В ХВОСТ: порядок узлов папки принадлежит пользователю, и
// приехавший не вправе раздвигать чужие позиции.
func TestAppendNodesToFolder_AppendsToTail(t *testing.T) {
	m := fillTestModel(moveTestNode("first", true, ""), moveTestNode("second", true, ""))

	if _, err := AppendNodesToFolder(m, "01FOLDER", fillTestURI+"#third", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	folder := findFolder(t, m, "01FOLDER")
	got := make([]string, len(folder.Nodes))
	for i := range folder.Nodes {
		got[i] = folder.Nodes[i].Tag
	}
	if strings.Join(got, ",") != "first,second,third" {
		t.Fatalf("порядок узлов %v — приехавший обязан быть в хвосте", got)
	}
}

// Имя файла достаётся ТОЛЬКО безымянному узлу: собственный #fragment написал
// пользователь, и подменять его именем файла значило бы терять его выбор.
func TestAppendNodesToFolder_FileNameOnlyForUnnamed(t *testing.T) {
	m := fillTestModel()

	// Без #fragment — имя берётся из файла.
	if _, err := AppendNodesToFolder(m, "01FOLDER", fillTestURI, "Netherlands"); err != nil {
		t.Fatalf("add unnamed: %v", err)
	}
	// С #fragment — своё имя сильнее имени файла.
	if _, err := AppendNodesToFolder(m, "01FOLDER", fillTestURI+"#own-name", "Netherlands"); err != nil {
		t.Fatalf("add named: %v", err)
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 2 {
		t.Fatalf("в папке %d узлов, ожидалось 2", len(folder.Nodes))
	}
	if folder.Nodes[0].Tag != "Netherlands" {
		t.Fatalf("безымянный узел получил тег %q, ожидался Netherlands (имя файла)", folder.Nodes[0].Tag)
	}
	if folder.Nodes[1].Tag != "own-name" {
		t.Fatalf("именованный узел переименован в %q — #fragment обязан пережить импорт", folder.Nodes[1].Tag)
	}
}

// Несколько безымянных узлов из ОДНОГО файла: имя файла одно, а узлов много —
// расходятся штатной уникализацией папки, а не теряют друг друга.
func TestAppendNodesToFolder_ManyUnnamedFromOneFile(t *testing.T) {
	m := fillTestModel()

	input := fillTestURI + "\n" + fillTestURI
	res, err := AppendNodesToFolder(m, "01FOLDER", input, "Netherlands")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if res.Added != 2 {
		t.Fatalf("добавлено %d узлов, ожидалось 2", res.Added)
	}
	folder := findFolder(t, m, "01FOLDER")
	if folder.Nodes[0].Tag != "Netherlands" || folder.Nodes[1].Tag != "Netherlands-2" {
		t.Fatalf("теги %q/%q, ожидались Netherlands / Netherlands-2",
			folder.Nodes[0].Tag, folder.Nodes[1].Tag)
	}
}

// Подписка в папку не кладётся: вложенных контейнеров нет. Ошибка — сентинел,
// чтобы UI подменил её переведённым текстом, не сравнивая подстроки.
func TestAppendNodesToFolder_RejectsSubscriptionURL(t *testing.T) {
	m := fillTestModel()

	_, err := AppendNodesToFolder(m, "01FOLDER", "https://example.com/subscription", "")
	if !errors.Is(err, ErrSubscriptionInFolder) {
		t.Fatalf("ошибка %v, ожидался ErrSubscriptionInFolder", err)
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 0 {
		t.Fatalf("в папку попало %d узлов от подписочной ссылки", len(folder.Nodes))
	}
	if len(m.Sources) != 1 {
		t.Fatal("подписка ушла в корень мимо папки — этого пути здесь нет")
	}
}

// Смешанный вход: ссылка-узел проходит, подписочная строка отбрасывается, и
// операция не проваливается целиком — узлы, которые можно взять, взяты.
func TestAppendNodesToFolder_MixedInputTakesNodes(t *testing.T) {
	m := fillTestModel()

	input := "https://example.com/subscription\n" + fillTestURI + "#NL-1"
	res, err := AppendNodesToFolder(m, "01FOLDER", input, "")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if res.Added != 1 {
		t.Fatalf("добавлено %d узлов, ожидался 1", res.Added)
	}
	if res.SkippedSubscriptions != 1 {
		t.Fatalf("отброшенных подписок %d, ожидалась 1 — строка не должна теряться молча", res.SkippedSubscriptions)
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 1 || folder.Nodes[0].Tag != "NL-1" {
		t.Fatalf("состав папки не тот: %+v", folder.Nodes)
	}
}

// Целью может быть только папка: подписка чужая, у корня свой путь (Add).
func TestAppendNodesToFolder_RejectsNonFolderTarget(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{{
		Node: corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
		ID:   "01SUB",
		URL:  "https://example.com/sub",
	}}}

	if _, err := AppendNodesToFolder(m, "01SUB", fillTestURI+"#NL-1", ""); err == nil {
		t.Fatal("подписка принята целью наполнения — её состав принадлежит провайдеру")
	}
}

// TagFromFileName: расширение и путь отбрасываются — тег с `/` внутри сломал
// бы фильтры Направлений, а `.conf` в списке outbound'ов выглядит как чужая
// внутренность.
func TestTagFromFileName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/home/user/configs/Netherlands.conf", "Netherlands"},
		{`C:\configs\Germany.vpn`, "Germany"},
		{"plain", "plain"},
		{"two.dots.here.json", "two.dots.here"},
		{".hidden", ".hidden"}, // ведущая точка — часть имени, а не расширение
		{"  spaced.txt  ", "spaced"},
		{"", ""},
	}
	for _, c := range cases {
		if got := TagFromFileName(c.in); got != c.want {
			t.Errorf("TagFromFileName(%q) = %q, ожидалось %q", c.in, got, c.want)
		}
	}
}

// Ядро разбора одно на корень и на папку: вставленный текст даёт ОДИН И ТОТ ЖЕ
// набор узлов независимо от адреса назначения. Это и есть страховка от
// «второго разбора» (Д6, ловушка «эмиттер и парсер ходят парой»).
func TestParseSourceInput_SameNodesForRootAndFolder(t *testing.T) {
	input := fillTestURI + "#NL-1"

	parsed, err := parseSourceInput(input, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Nodes) != 1 {
		t.Fatalf("разбор дал %d узлов, ожидался 1", len(parsed.Nodes))
	}

	// Корневой путь.
	rootModel := &wizardmodels.WizardModel{}
	if _, err := AppendURLsToSources(stubStaleUIUpdater{model: rootModel}, input); err != nil {
		t.Fatalf("root add: %v", err)
	}
	if len(rootModel.Sources) != 1 {
		t.Fatalf("в корень легло %d источников, ожидался 1", len(rootModel.Sources))
	}

	// Папочный путь.
	folderModel := fillTestModel()
	if _, err := AppendNodesToFolder(folderModel, "01FOLDER", input, ""); err != nil {
		t.Fatalf("folder add: %v", err)
	}
	folder := findFolder(t, folderModel, "01FOLDER")
	if len(folder.Nodes) != 1 {
		t.Fatalf("в папку легло %d узлов, ожидался 1", len(folder.Nodes))
	}

	rootNode := rootModel.Sources[0].Node
	folderNode := folder.Nodes[0]
	if rootNode.Tag != folderNode.Tag {
		t.Fatalf("теги разошлись: корень %q, папка %q", rootNode.Tag, folderNode.Tag)
	}
	if string(rootNode.Body) != string(folderNode.Body) {
		t.Fatalf("тела разошлись — значит разборов два:\nкорень: %s\nпапка:  %s",
			rootNode.Body, folderNode.Body)
	}
	if rootNode.Origin == nil || folderNode.Origin == nil ||
		rootNode.Origin.Kind != folderNode.Origin.Kind ||
		rootNode.Origin.Raw != folderNode.Origin.Raw {
		t.Fatalf("origin разошёлся: корень %+v, папка %+v", rootNode.Origin, folderNode.Origin)
	}
}
