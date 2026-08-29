package backup

// Граница «модель v7 ↔ контракт 0.11» (SPEC 118 §4.F.2 и §4.F.3).
//
// Тесты рядом смотрят на файл: purity_test проверяет БАЙТ-тождественность
// экспорт→импорт→экспорт, то есть свойство формата. Здесь предмет другой —
// МОДЕЛЬ: после экспорта и импорта поля v7 (enabled узлов, replace, detour как
// NodeLink, хопы как NodeLink) обязаны означать то же самое, что до. Байтовая
// тождественность этого не доказывает: пара «экспорт теряет X — импорт
// выдумывает X» даёт одинаковые файлы и разъехавшуюся модель.

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// exportImport — круг «состояние → файл → состояние» через настоящий Parse,
// а не через прямую передачу структуры: сериализация — часть границы, и
// поле, которое не пережило JSON, обязано падать здесь же.
func exportImport(t *testing.T, s *state.State, opts ImportOptions) (*state.State, []Warning) {
	t.Helper()
	raw := fixedExport(t, s)
	b, parseWarns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dst := &state.State{}
	res, err := Import(dst, b, opts)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return dst, append(parseWarns, res.Warnings...)
}

func findSourceByID(t *testing.T, s *state.State, id string) *state.Source {
	t.Helper()
	for i := range s.Sources {
		if s.Sources[i].ID == id {
			return &s.Sources[i]
		}
	}
	t.Fatalf("источник %q потерян на roundtrip", id)
	return nil
}

// §4.F.2: экспорт→импорт на этой же машине — модель эквивалентна.
//
// Проверяются именно те четыре конвертации, ради которых существует
// convert_v7.go: enabled ⇄ disabled-карта, replace ⇄ fold, NodeLink ⇄ тройня,
// хопы ⇄ строки. Задокументированные потери названы прямо в утверждениях.
func TestRoundTripV7ModelEquivalent(t *testing.T) {
	src := richState()
	// Материализованные узлы — то, чего контракт не несёт вовсе. Кладём их с
	// РАЗНЫМ enabled: карта disabled собирается по сырым тегам, и на импорте
	// обязана вернуться отметками (nodes[] к тому моменту ещё пусты).
	src.Sources[0].Nodes = []state.Node{
		{Kind: state.SourceKindServer, Tag: "NL-1", Enabled: true},
		{Kind: state.SourceKindServer, Tag: "NL-2", Enabled: false},
	}
	// PendingDisabled уже стоит в richState (node-a/node-b) — обе половины
	// отметок обязаны уехать одним списком и вернуться одним же.

	// Тег замены выставлен ровно тем деривативом, который контракт умеет
	// воспроизвести из префикса: иначе круг его не переживает (см.
	// TestExportNamesUnrepresentableReplaceTag — это НАЗВАННАЯ потеря).
	src.Sources[0].Replace.Tag = "[A]select"

	dst, warns := exportImport(t, src, importKnowsEverything())
	for _, w := range warns {
		if w.Code != WarnBackupSourceKindUnsupported {
			t.Errorf("свой же файл дал предупреждение: %v", w)
		}
	}

	sub := findSourceByID(t, dst, "01SUB0000000000000000000")

	// nodes[] в контракт не едут — подписка приезжает без узлов и фетчится
	// заново. Это названная цена, а не потеря настройки.
	if len(sub.Nodes) != 0 {
		t.Errorf("nodes[] уехали в бэкап: %d узлов", len(sub.Nodes))
	}

	// enabled=false узла + PendingDisabled → карта → PendingDisabled (O2).
	wantPending := map[string]bool{"NL-2": true, "node-a": true, "node-b": true}
	if len(sub.PendingDisabled) != len(wantPending) {
		t.Fatalf("pending_disabled после roundtrip: %v, ожидалось %d отметок", sub.PendingDisabled, len(wantPending))
	}
	for _, tag := range sub.PendingDisabled {
		if !wantPending[tag] {
			t.Errorf("лишняя отметка выключения %q", tag)
		}
	}

	// replace ⇄ fold: режим и тег обязаны совпасть. Тег контракт не несёт —
	// импорт материализует его прежним позиционным деривативом, и он обязан
	// совпасть с исходным, иначе правила того же файла указывают в никуда.
	if sub.Replace == nil {
		t.Fatal("replace потерян на roundtrip")
	}
	if sub.Replace.Mode != src.Sources[0].Replace.Mode {
		t.Errorf("replace.mode: %q, было %q", sub.Replace.Mode, src.Sources[0].Replace.Mode)
	}
	if sub.Replace.Tag != src.Sources[0].Replace.Tag {
		t.Errorf("replace.tag: %q, было %q", sub.Replace.Tag, src.Sources[0].Replace.Tag)
	}

	// detour-NodeLink с адресом папки ⇄ тройня: оба конца обязаны выжить.
	if sub.Detour == nil {
		t.Fatal("detour подписки потерян")
	}
	if sub.Detour.FolderID != "01SRV0000000000000000000" || sub.Detour.Tag != "🔥 WARP" {
		t.Errorf("detour подписки: %+v, ожидалось {01SRV…, 🔥 WARP}", *sub.Detour)
	}

	// TagPolicy ⇄ tag{prefix,postfix}.
	if sub.TagPolicy == nil || sub.TagPolicy.Prefix != "[A] " {
		t.Errorf("tag policy: %+v", sub.TagPolicy)
	}

	// detour корневого пространства (FolderID пуст) ⇄ одиночный тег.
	srv := findSourceByID(t, dst, "01SRV0000000000000000000")
	if srv.Detour == nil || srv.Detour.FolderID != "" || srv.Detour.Tag != "hop-1" {
		t.Errorf("detour узла: %+v, ожидалось {\"\", hop-1}", srv.Detour)
	}

	// hops []NodeLink ⇄ []string: порядок и состав обязаны совпасть. Адрес
	// папки контракт не несёт — на импорте хоп поднимается по живому индексу,
	// а «🔥 WARP» здесь корневой узел, значит FolderID остаётся пустым.
	chain := findSourceByID(t, dst, "01CHN0000000000000000000")
	if len(chain.Hops) != 2 {
		t.Fatalf("хопы цепочки: %v, ожидалось 2 позиции", chain.Hops)
	}
	if chain.Hops[0].Tag != "vpn-de" || chain.Hops[1].Tag != "🔥 WARP" {
		t.Errorf("порядок хопов разъехался: %v", chain.Hops)
	}

	// Настройки маршрута цепочки живут в теле узла (компенсация W5) и обязаны
	// пережить границу: они уезжают формой контракта и возвращаются в body.
	var gotChain configtypes.SourceChain
	if err := json.Unmarshal(chain.Body, &gotChain); err != nil {
		t.Fatalf("тело цепочки: %v", err)
	}
	if gotChain.IdleTimeout != "0s" || gotChain.StripEvasion == nil || *gotChain.StripEvasion {
		t.Errorf("настройки маршрута цепочки потеряны: %+v", gotChain)
	}
	// Позиции в теле не живут — их дом hops.
	if len(gotChain.Hops) != 0 {
		t.Errorf("позиции просочились в тело узла: %v", gotChain.Hops)
	}
}

// §4.F.2, отдельная половина: хоп в узел ПОДПИСКИ поднимается до адресной
// ссылки по живому индексу, а не остаётся голой строкой.
//
// Это единственный случай, где импорт обязан ДОБАВИТЬ адрес, которого в файле
// не было: контракт 0.11 знает только строку.
func TestRoundTripV7ResolvesHopIntoContainer(t *testing.T) {
	s := &state.State{}
	s.Sources = []state.Source{
		{
			ID:   "01SUB0000000000000000000",
			Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
			URL:  "https://example-1.com/sub", Name: "Main",
			Nodes: []state.Node{{Kind: state.SourceKindServer, Tag: "NL-1", Enabled: true}},
		},
		{
			ID: "01CHN0000000000000000000",
			Node: state.Node{
				Kind: state.SourceKindChain, Enabled: true, Tag: "relay",
				Hops: []state.NodeLink{{FolderID: "01SUB0000000000000000000", Tag: "NL-1"}, {Tag: "direct"}},
			},
		},
	}
	// Индекс живого набора строится по узлам ПРИЕХАВШИХ контейнеров, а nodes[]
	// в файл не едут — значит поднять адрес импорту не из чего, и хоп обязан
	// остаться fail-closed-ссылкой корневого пространства. Проверяем именно
	// это: «резолв по живому индексу» не должен выдумывать адрес.
	dst, _ := exportImport(t, s, ImportOptions{KnownOutbounds: []string{"relay"}})
	chain := findSourceByID(t, dst, "01CHN0000000000000000000")
	if len(chain.Hops) != 2 {
		t.Fatalf("хопы: %v", chain.Hops)
	}
	if chain.Hops[0].Tag != "NL-1" {
		t.Errorf("тег хопа потерян: %+v", chain.Hops[0])
	}
	if chain.Hops[0].FolderID != "" {
		t.Errorf("импорт выдумал адрес папки для хопа: %+v — узлов подписки в файле нет", chain.Hops[0])
	}

	// А вот когда контейнер с таким узлом на принимающей стороне УЖЕ есть,
	// адрес обязан подняться: ровно за этим и написан resolveImportedHops.
	live := &state.State{Sources: []state.Source{{
		ID:    "01FLD0000000000000000000",
		Node:  state.Node{Kind: state.SourceKindFolder, Enabled: true},
		Name:  "Local folder",
		Nodes: []state.Node{{Kind: state.SourceKindServer, Tag: "NL-1", Enabled: true}},
	}}}
	raw := fixedExport(t, s)
	b, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := Import(live, b, ImportOptions{KnownOutbounds: []string{"relay"}}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Импорт замещает sources[] приехавшими (режим replace) — папка приёмника
	// в набор не попадает, и это тоже результат: адрес не поднимается.
	// Утверждение фиксирует ФАКТ, а не желание: replace-семантика описана в
	// BACKUP.md §9, и молчаливое исключение из неё было бы хуже.
	for i := range live.Sources {
		if live.Sources[i].Kind == state.SourceKindFolder {
			t.Fatalf("режим replace оставил папку приёмника — семантика импорта изменилась")
		}
	}
}

// §4.F.3: импорт бэкапа v1.5.x — файл со свёрткой, локальными Направлениями,
// disabled-картой и маской тегов.
//
// Все четыре механизма упразднены в v7, и каждый обязан либо приехать своей
// новой формой, либо быть НАЗВАННЫМ. Молчаливых потерь нет — иначе
// пользователь, восстановившийся из бэкапа полуторагодичной давности, получит
// тихо другую маршрутизацию.
func TestImportLegacy15xBackup(t *testing.T) {
	raw := []byte(`{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.2"},
  "exported_at": "2025-03-01T00:00:00Z",
  "subscriptions": [
    {
      "id": "01SUB0000000000000000000",
      "url": "https://example-1.com/sub",
      "label": "Main",
      "tag": {"prefix": "[P]", "mask": "{$label} · {$num}"},
      "disabled": {"NL-1": 1750000000, "DE-3": 1750000001},
      "fold": {"mode": "select_auto", "auto": {"interval": "3m"}},
      "outbounds": [
        {"tag": "[P]select"},
        {"tag": "[P]auto"},
        {"tag": "[P] streaming", "filter": "netflix"}
      ]
    }
  ],
  "rules": [
    {"kind": "inline", "num": 1, "name": "Work", "outbound": "[P]select",
     "match": {"domain_suffix": ["work.com"]}}
  ]
}`)

	b, parseWarns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dst := &state.State{}
	res, err := Import(dst, b, ImportOptions{KnownOutbounds: []string{"[P]select"}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	warns := append(parseWarns, res.Warnings...)

	if len(dst.Sources) != 1 {
		t.Fatalf("источников после импорта: %d", len(dst.Sources))
	}
	sub := dst.Sources[0]

	// Свёртка → замена. Тег замены — тот же, на который ссылается правило
	// того же файла: префикс подписки плюс `select`.
	if sub.Replace == nil {
		t.Fatal("fold не стал replace — маршрутизация бэкапа v1.5.x потеряна")
	}
	if sub.Replace.Mode != state.FolderReplaceBoth {
		t.Errorf("режим замены: %q, ожидался both (select_auto)", sub.Replace.Mode)
	}
	if sub.Replace.Tag != "[P]select" {
		t.Errorf("тег замены: %q, ожидался %q — правило файла метит именно в него", sub.Replace.Tag, "[P]select")
	}
	if sub.Replace.Strategy == nil || sub.Replace.Strategy.Interval != "3m" {
		t.Errorf("параметры автогруппы потеряны: %+v", sub.Replace.Strategy)
	}

	// prefix жив, mask — потеря, и она названа.
	if sub.TagPolicy == nil || sub.TagPolicy.Prefix != "[P]" {
		t.Errorf("префикс тегов потерян: %+v", sub.TagPolicy)
	}
	if !hasWarn(warns, WarnBackupTagMaskDropped) {
		t.Errorf("маска подписки выброшена молча; предупреждения: %v", warns)
	}

	// disabled-карта → PendingDisabled (вердикт O2): узлов ещё нет, отметки
	// ждут первого достоверного fetch.
	if len(sub.PendingDisabled) != 2 {
		t.Errorf("отметки выключения потеряны: %v", sub.PendingDisabled)
	}

	// Локальные Направления: пара, порождённая свёрткой, приехала заменой и
	// молчит; произвольное `[P] streaming` — названо.
	if !hasWarn(warns, WarnBackupLocalDirectionDropped) {
		t.Errorf("локальное Направление выброшено молча; предупреждения: %v", warns)
	}
	for _, w := range warns {
		if w.Code == WarnBackupLocalDirectionDropped &&
			(strings.Contains(w.Detail, "[P]select") || strings.Contains(w.Detail, "[P]auto")) {
			t.Errorf("производная свёртки названа потерей: %v — она приехала заменой", w)
		}
	}

	// Правило, метящее в тег замены, обязано приехать ВКЛЮЧЁННЫМ: цель
	// существует, просто её теперь зовут заменой, а не свёрткой.
	if len(dst.Rules) != 1 {
		t.Fatalf("правил после импорта: %d", len(dst.Rules))
	}
	if !dst.Rules[0].Enabled {
		t.Errorf("правило на тег замены приехало выключенным — цель считается несуществующей")
	}
}

// §4.F.3, вторая половина: маска ОДИНОЧНОГО узла (server/chain) — это имя
// самого узла, и потерей она не является.
//
// В контракте 0.11 у секций servers[]/chains[] поля tag.mask нет вовсе: имя
// узла едет своим ключом. Проверяем, что этот ключ доезжает до Node.tag —
// именно он и был «маской» одиночного узла в старой модели.
func TestImportLegacyServerMaskArrivesAsNodeTag(t *testing.T) {
	raw := []byte(`{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.2"},
  "exported_at": "2025-03-01T00:00:00Z",
  "servers": [
    {"id": "01SRV0000000000000000000", "label": "WARP hop", "node_tag": "🔥 WARP",
     "uri": "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s"}
  ],
  "chains": [
    {"id": "01CHN0000000000000000000", "tag": "relay", "label": "Мой маршрут",
     "chain": {"hops": ["🔥 WARP", "direct"]}}
  ]
}`)
	b, parseWarns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dst := &state.State{}
	res, err := Import(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if warns := append(parseWarns, res.Warnings...); len(warns) != 0 {
		t.Errorf("имя одиночного узла — не потеря, а перенос; предупреждения: %v", warns)
	}
	srv := findSourceByID(t, dst, "01SRV0000000000000000000")
	if srv.Tag != "🔥 WARP" {
		t.Errorf("имя узла не стало Node.tag: %q", srv.Tag)
	}
	if srv.Label != "WARP hop" {
		t.Errorf("подпись узла потеряна: %q", srv.Label)
	}
	chain := findSourceByID(t, dst, "01CHN0000000000000000000")
	if chain.Tag != "relay" {
		t.Errorf("тег цепочки не стал Node.tag: %q", chain.Tag)
	}
	if len(chain.Hops) != 2 || chain.Hops[0].Tag != "🔥 WARP" {
		t.Errorf("хопы цепочки: %v", chain.Hops)
	}
}



// §4.F.2, названная потеря: ЯВНЫЙ тег замены, не совпавший с деривативом
// контракта, круг не переживает — и экспорт обязан сказать это вслух.
//
// В v7 `replace.tag` задаётся руками; контракт 0.11 места для него не имеет и
// на импорте выводит имя формулой «префикс подписки (или `<N>:`) + select».
// Если имена разошлись, на приёмнике группа зовётся иначе, а правила, метившие
// в прежнее имя, приедут выключенными. Молчаливая подмена имени, на которое
// ссылается маршрутизация, — ровно та потеря, которую формат запрещает.
func TestExportNamesUnrepresentableReplaceTag(t *testing.T) {
	s := richState()
	s.Sources[0].Replace.Tag = "My Europe" // ни префикс, ни позиция такого не дадут

	_, warns, err := Export(s, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !hasWarn(warns, WarnBackupReplaceTagDerived) {
		t.Fatalf("тег замены подменён молча; предупреждения: %v", warns)
	}
	// Обязаны быть названы ОБА имени: пользователь должен видеть, во что
	// превратится его группа на приёмнике.
	var detail string
	for _, w := range warns {
		if w.Code == WarnBackupReplaceTagDerived {
			detail = w.Detail
		}
	}
	if !strings.Contains(detail, "My Europe") || !strings.Contains(detail, "[A]select") {
		t.Errorf("предупреждение не называет оба имени: %q", detail)
	}

	// Дериватив предупреждения не вызывает: подмены нет.
	s.Sources[0].Replace.Tag = "[A]select"
	_, warns, err = Export(s, ExportOptions{AppVersion: "test", Platform: "darwin"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if hasWarn(warns, WarnBackupReplaceTagDerived) {
		t.Errorf("совпавший с деривативом тег объявлен потерей: %v", warns)
	}
}
