package backup

// Граница «модель ↔ формы контракта» (SPEC 118 §4.F.2 и §4.F.3).
//
// Тесты рядом смотрят на файл: purity_test проверяет БАЙТ-тождественность
// экспорт→импорт→экспорт, то есть свойство формата. Здесь предмет другой —
// МОДЕЛЬ: после экспорта и импорта поля модели (enabled узлов, replace, detour
// как NodeLink, хопы как NodeLink) обязаны означать то же самое, что до.
// Байтовая тождественность этого не доказывает: пара «экспорт теряет X —
// импорт выдумывает X» даёт одинаковые файлы и разъехавшуюся модель.
//
// Входов импорта два, и модель обязана выйти одной: файл 1.0, который
// лаунчер пишет сейчас, и файл 0.12, который писали релизы до v1.6.0 (писатель
// 0.12 снят, D-110, — второй вход дан сырым JSON, снятым прежним писателем).

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

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

// legacyV7Model012 — файл 0.12, который прежний писатель снимал с состояния
// TestRoundTripV7ModelEquivalent: свёртка без имени группы, detour-тройня,
// хопы строками, канон цепочки в `chain`, disabled-карта.
const legacyV7Model012 = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "subscriptions": [{
    "id": "01SUB0000000000000000000",
    "url": "https://example-1.com/sub",
    "label": "Main",
    "max_nodes": 200,
    "tag": {"prefix": "[A] "},
    "update": {"interval_hours": 12, "auto": true},
    "disabled": {"NL-2": 0, "node-a": 0, "node-b": 0},
    "skip": [{"contains": "trial", "field": "tag"}],
    "fold": {"mode": "select"},
    "detour_node_source_id": "01SRV0000000000000000000",
    "detour_node_tag": "🔥 WARP",
    "detour_node_label": "🔥 WARP"
  }],
  "servers": [{
    "id": "01SRV0000000000000000000",
    "uri": "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s",
    "node_tag": "🔥 WARP",
    "detour_node_tag": "hop-1",
    "detour_node_label": "hop-1"
  }],
  "directions": [{"tag": "vpn-de", "include_direct": true}],
  "chains": [{
    "id": "01CHN0000000000000000000",
    "tag": "relay",
    "chain": {
      "hops": ["vpn-de", "🔥 WARP"],
      "idle_timeout": "0s",
      "strip_evasion": false,
      "strip": {"tls.utls": false},
      "rewrite": {"vless": {"flow": null}}
    }
  }],
  "rules": [
    {"kind": "preset", "num": 1000, "ref": "traffic-processing", "vars": {"mode": "on"}},
    {"kind": "inline", "name": "Work", "num": 1001, "outbound": "vpn-de",
     "match": {"domain_suffix": ["example.com"]}},
    {"kind": "inline", "name": "Chained", "num": 1002, "outbound": "relay",
     "match": {"domain_suffix": ["example.com"]}}
  ],
  "dns": {
    "servers": [
      {"kind": "template", "name": "google_dot"},
      {"kind": "user", "name": "my_dns", "value": {"server": "10.0.0.1", "type": "udp"}}
    ],
    "rules": [
      {"kind": "user", "enabled": false, "value": {"domain_suffix": "example.com", "server": "my_dns"}}
    ],
    "final": "dns_shield",
    "strategy": "ipv4_only"
  },
  "vars": {"log_level": "debug"},
  "route": {"final": "vpn-de"},
  "warp": [{"client_v4": "172.16.0.2", "client_v6": "", "peer_public": "pub", "private_key": "priv", "type": "wg"}]
}`

// §4.F.2: экспорт→импорт на этой же машине — модель эквивалентна, и та же
// модель выходит из файла 0.12, снятого с неё прежним лаунчером.
//
// Проверяются конвертации, ради которых существует convert_v7.go: enabled ⇄
// disabled-карта и replace ⇄ fold у обоих входов, NodeLink ⇄ тройня и хопы ⇄
// строки у входа 0.12. Задокументированные потери названы прямо в
// утверждениях.
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

	// Тег замены выставлен ровно тем деривативом, который файл 0.12 умеет
	// воспроизвести из префикса: имени группы в свёртке 0.12 нет, и другое
	// явное имя такой файл не переживал (1.0 везёт его ключом fold_tag).
	src.Sources[0].Replace.Tag = "[A]select"

	for _, in := range importBothFormats(t, src, legacyV7Model012, importKnowsEverything()) {
		t.Run(in.format, func(t *testing.T) {
			assertV7ModelEquivalent(t, src, in.state, in.warns)
		})
	}
}

// assertV7ModelEquivalent — утверждения TestRoundTripV7ModelEquivalent на
// один вход импорта.
func assertV7ModelEquivalent(t *testing.T, src, dst *state.State, warns []Warning) {
	t.Helper()
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

	// replace ⇄ fold: режим и тег обязаны совпасть. 1.0 везёт имя группы
	// явно (fold_tag), 0.12 — нет: там импорт материализует его прежним
	// позиционным деривативом, и он обязан совпасть с исходным, иначе
	// правила того же файла указывают в никуда.
	if sub.Replace == nil {
		t.Fatal("replace потерян на roundtrip")
	}
	if sub.Replace.Mode != src.Sources[0].Replace.Mode {
		t.Errorf("replace.mode: %q, было %q", sub.Replace.Mode, src.Sources[0].Replace.Mode)
	}
	if sub.Replace.Tag != src.Sources[0].Replace.Tag {
		t.Errorf("replace.tag: %q, было %q", sub.Replace.Tag, src.Sources[0].Replace.Tag)
	}

	// detour-NodeLink на корневой узел ⇄ объект `{tag}` / тройня 0.12 с id
	// сервера: корневой узел адресуется тегом, и тройня с
	// `detour_node_source_id` сервера обязана приехать корневой формой
	// (NODE_LINK.md §7.4), а не висящим folder_id.
	if sub.Detour == nil {
		t.Fatal("detour подписки потерян")
	}
	if sub.Detour.FolderID != "" || sub.Detour.Tag != "🔥 WARP" {
		t.Errorf("detour подписки: %+v, ожидалось {\"\", 🔥 WARP}", *sub.Detour)
	}

	// TagPolicy ⇄ tag_policy / tag{prefix,postfix}.
	if sub.TagPolicy == nil || sub.TagPolicy.Prefix != "[A] " {
		t.Errorf("tag policy: %+v", sub.TagPolicy)
	}

	// detour корневого пространства (FolderID пуст) ⇄ одиночный тег.
	srv := findSourceByID(t, dst, "01SRV0000000000000000000")
	if srv.Detour == nil || srv.Detour.FolderID != "" || srv.Detour.Tag != "hop-1" {
		t.Errorf("detour узла: %+v, ожидалось {\"\", hop-1}", srv.Detour)
	}

	// hops []NodeLink ⇄ []NodeLink (1.0) / []string (0.12): порядок и состав
	// обязаны совпасть. Адреса папки у этих хопов нет ни в каком формате:
	// «vpn-de» — Направление, «🔥 WARP» — корневой узел.
	chain := findSourceByID(t, dst, "01CHN0000000000000000000")
	if len(chain.Hops) != 2 {
		t.Fatalf("хопы цепочки: %v, ожидалось 2 позиции", chain.Hops)
	}
	if chain.Hops[0].Tag != "vpn-de" || chain.Hops[1].Tag != "🔥 WARP" {
		t.Errorf("порядок хопов разъехался: %v", chain.Hops)
	}
	for _, h := range chain.Hops {
		if h.FolderID != "" {
			t.Errorf("хоп корневого пространства получил адрес папки: %+v", h)
		}
	}

	// Настройки маршрута цепочки живут в теле узла (компенсация W5) и обязаны
	// пережить границу: у 1.0 тело едет как есть, у 0.12 — формой контракта.
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

// §4.F.2, legacy-половина: хоп файла 0.12 — голая СТРОКА, и импорт поднимает
// её до адресной ссылки по живому индексу, но не выдумывает адрес.
//
// Это единственный случай, где импорт обязан ДОБАВИТЬ адрес, которого в файле
// не было: форма 0.12 знает только строку. У файла 1.0 хоп адрес уже несёт
// (`folder_id`), и этим проходом не трогается — поэтому вход здесь один.
//
// Файл — тот, что прежний писатель снимал с состояния «подписка с узлом NL-1
// и цепочка с хопом в этот узел»: адрес папки при записи терялся.
func TestRoundTripV7ResolvesHopIntoContainer(t *testing.T) {
	const legacy = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "subscriptions": [{"id": "01SUB0000000000000000000", "url": "https://example-1.com/sub", "label": "Main"}],
  "chains": [{"id": "01CHN0000000000000000000", "tag": "relay", "chain": {"hops": ["NL-1", "direct"]}}]
}`
	importLegacy := func(t *testing.T, dst *state.State) {
		t.Helper()
		b, _, err := Parse([]byte(legacy))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if _, err := ImportFile(dst, b, ImportOptions{KnownOutbounds: []string{"relay"}}); err != nil {
			t.Fatalf("Import: %v", err)
		}
	}

	// Индекс живого набора строится по узлам ПРИЕХАВШИХ контейнеров, а nodes[]
	// в файл не едут — значит поднять адрес импорту не из чего, и хоп обязан
	// остаться fail-closed-ссылкой корневого пространства. Проверяем именно
	// это: «резолв по живому индексу» не должен выдумывать адрес.
	dst := &state.State{}
	importLegacy(t, dst)
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
	importLegacy(t, live)
	// Импорт СЛИВАЕТ источники (D-095, BACKUP.md §9): папка приёмника, которой
	// в файле нет, остаётся жить — и именно поэтому адрес хопа поднимается.
	// Прежде здесь стоял обратный вердикт: replace сносил папку, живого
	// набора не оставалось и резолвить было не по чему.
	var folder *state.Source
	for i := range live.Sources {
		if live.Sources[i].Kind == state.SourceKindFolder {
			folder = &live.Sources[i]
		}
	}
	if folder == nil {
		t.Fatalf("слияние потеряло папку приёмника, которой в файле не было")
	}
	merged := findSourceByID(t, live, "01CHN0000000000000000000")
	if len(merged.Hops) != 2 {
		t.Fatalf("хопы: %v", merged.Hops)
	}
	if merged.Hops[0].FolderID != folder.ID || merged.Hops[0].Tag != "NL-1" {
		t.Errorf("адрес хопа не поднялся по живому набору: %+v", merged.Hops[0])
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
	res, err := ImportFile(dst, b, ImportOptions{KnownOutbounds: []string{"[P]select"}})
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
	res, err := ImportFile(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Тег доезжает молча; а вот `label` СЕРВЕРА, разошедшийся с тегом, —
	// потеря: у канона v7 имени, кроме тега, нет (SPEC 112), и Source.Label —
	// поле `json:"-"`, которое умерло бы на первом Save беззвучно.
	// Предупреждение РОВНО одно: у цепочки в том же файле label тоже есть, но
	// это объявленное поле LxBox (D-094) — оно игнорируется молча.
	warns := warnCodes(append(parseWarns, res.Warnings...))
	wantWarns := []string{WarnBackupLabelDropped}
	if !equalStrings(warns, wantWarns) {
		t.Errorf("предупреждения: получено %v, ожидалось %v", warns, wantWarns)
	}
	srv := findSourceByID(t, dst, "01SRV0000000000000000000")
	if srv.Tag != "🔥 WARP" {
		t.Errorf("имя узла не стало Node.tag: %q", srv.Tag)
	}
	if srv.Label != "" {
		t.Errorf("импорт заполнил Label (json:\"-\"): %q", srv.Label)
	}
	chain := findSourceByID(t, dst, "01CHN0000000000000000000")
	if chain.Tag != "relay" {
		t.Errorf("тег цепочки не стал Node.tag: %q", chain.Tag)
	}
	if len(chain.Hops) != 2 || chain.Hops[0].Tag != "🔥 WARP" {
		t.Errorf("хопы цепочки: %v", chain.Hops)
	}
}

// П6 на полях, которые схема ЗНАЕТ, а модель v7 больше нет: флаг
// `exclude_from_global` объявлен в типах контракта, поэтому общий scanUnknown
// его не видит — без явного кода он пропадал бы совсем молча, и узлы
// источника молча возвращались бы в общий пул кандидатов.
func TestImportNamesDroppedSourceFlags(t *testing.T) {
	raw := []byte(`{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.2"},
  "exported_at": "2025-03-01T00:00:00Z",
  "subscriptions": [
    {"id": "01SUB0000000000000000000", "url": "https://example.invalid/s",
     "label": "WL", "exclude_from_global": true}
  ],
  "servers": [
    {"id": "01SRV0000000000000000000", "node_tag": "Tokyo",
     "uri": "vless://11111111-1111-1111-1111-111111111111@example-2.com:443?type=tcp#s",
     "exclude_from_global": true}
  ]
}`)
	b, parseWarns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(parseWarns) != 0 {
		t.Fatalf("поля объявлены в схеме, лишних предупреждений разбора быть не должно: %v", parseWarns)
	}
	dst := &state.State{}
	res, err := ImportFile(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	got := 0
	for _, w := range res.Warnings {
		if w.Code == WarnBackupSourceFlagDropped {
			got++
		}
	}
	if got != 2 {
		t.Fatalf("предупреждений о снятом флаге %d, ожидалось 2 (подписка + сервер): %v", got, res.Warnings)
	}
}

// Подпись цепочки: с контракта 0.12.4 (D-094) `label` — объявленное поле
// LxBox. Лаунчер зовёт цепочку тегом (SPEC 112), поэтому приехавшее значение
// он не применяет и МОЛЧА отбрасывает (BACKUP.md §1): это не потеря, о
// которой надо говорить, а чужое объявленное поле, и warning шумел бы на
// каждом импорте файла LxBox.
func TestImportChainLabelIgnoredSilently(t *testing.T) {
	raw := []byte(`{
  "lx_backup": 1,
  "exported_by": {"app": "lxbox", "version": "2.2.0"},
  "exported_at": "2025-03-01T00:00:00Z",
  "chains": [
    {"id": "01CHN0000000000000000000", "tag": "my-chain", "label": "Моя цепочка",
     "chain": {"hops": ["direct"]}}
  ]
}`)
	b, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dst := &state.State{}
	res, err := ImportFile(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	chain := findSourceByID(t, dst, "01CHN0000000000000000000")
	if chain.Tag != "my-chain" {
		t.Errorf("тег цепочки потерян: %q", chain.Tag)
	}
	if chain.Label != "" {
		t.Errorf("импорт заполнил Label (json:\"-\"): %q", chain.Label)
	}
	if hasWarn(res.Warnings, WarnBackupLabelDropped) {
		t.Fatalf("объявленное поле LxBox дало предупреждение: %v", res.Warnings)
	}

	// После Save→Load терять больше нечего: то, что уцелело, уцелело.
	path := filepath.Join(t.TempDir(), "state.json")
	if err := dst.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := state.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := findSourceByID(t, back, "01CHN0000000000000000000")
	if got.Tag != "my-chain" {
		t.Errorf("тег цепочки не пережил Save→Load: %q", got.Tag)
	}
}
