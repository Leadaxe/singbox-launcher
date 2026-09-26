package backup

// Слияние при импорте (D-095, BACKUP.md §9).
//
// Кейсы data-критичные: каждый ловит способ ТИХО потерять или задвоить то,
// что пользователь настроил руками. Формат строк и вёрстку здесь не
// проверяют — только то, что после импорта лежит в состоянии.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// subSourceWithNodes — локальная подписка с составом и историей.
func subSourceWithNodes(id, url, name string, tags ...string) state.Source {
	src := state.Source{
		Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
		ID:   id, URL: url, Name: name,
		UpdateStatus: &state.SubUpdateStatus{LastStatus: "ok", NodesCountFetched: len(tags)},
	}
	for _, tag := range tags {
		src.Nodes = append(src.Nodes, state.Node{Kind: state.SourceKindServer, Tag: tag, Enabled: true})
	}
	return src
}

func uriServer(tag, raw string) state.Source {
	return state.Source{
		Node: state.Node{
			Kind: state.SourceKindServer, Tag: tag, Enabled: true,
			Origin: &state.Origin{Kind: state.OriginKindURI, Raw: raw},
		},
		ID: "01LOCAL" + tag,
	}
}

// TestMergeSubscriptionKeepsLocalIdentityAndHistory — совпавшая по URL
// подписка держит свою идентичность и состав, а настройки берёт из файла.
//
// Это главный кейс решения: перезапись здесь стирала бы узлы, отметки и
// историю обновлений, а пользователь узнал бы об этом только по пустому
// списку узлов и слетевшему выбору.
func TestMergeSubscriptionKeepsLocalIdentityAndHistory(t *testing.T) {
	local := subSourceWithNodes("01LOCALSUB", "https://example-1.com/sub", "Local name", "NL-1", "DE-2")
	local.TagPolicy = &state.TagPolicy{Prefix: "loc:"}
	local.SetIdentityUserAgent("local-ua")
	local.PendingDisabled = []string{"local-mark"}
	local.RelaysInDirections = true
	s := &state.State{Sources: []state.Source{local}}

	b := &Backup{
		LxBackup: FormatVersion,
		Subscriptions: []Subscription{{
			ID:       "01FILESUB",
			URL:      "https://example-1.com/sub",
			Label:    "File name",
			Tag:      &TagPolicy{Prefix: "file:"},
			MaxNodes: 42,
			Disabled: map[string]int64{"DE-2": 1, "ghost": 2},
		}},
	}
	res, err := Import(s, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(s.Sources) != 1 {
		t.Fatalf("источников %d, ожидался 1 — подписка задвоена", len(s.Sources))
	}
	got := s.Sources[0]
	if got.ID != "01LOCALSUB" {
		t.Errorf("id %q — совпавшая запись обязана держать локальный", got.ID)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("состав %d узлов — слияние потеряло узлы локальной подписки", len(got.Nodes))
	}
	if got.UpdateStatus == nil || got.UpdateStatus.LastStatus != "ok" {
		t.Error("история обновлений локальной подписки потеряна")
	}
	// Настройки — из файла.
	if got.Name != "File name" {
		t.Errorf("имя %q, ожидалось из файла", got.Name)
	}
	if got.TagPolicy == nil || got.TagPolicy.Prefix != "file:" {
		t.Errorf("политика тегов %+v, ожидалась из файла", got.TagPolicy)
	}
	if got.MaxNodes != 42 {
		t.Errorf("max_nodes %d, ожидалось 42", got.MaxNodes)
	}
	// identity в файле нет → сброс на «как в системе».
	if got.IdentityUserAgent() != "" {
		t.Errorf("UA %q — объекта identity в файле нет, значит «как в системе»", got.IdentityUserAgent())
	}
	// disabled — ОБЪЕДИНЕНИЕ: своя отметка жива, приехавшая доехала.
	if !hasString(got.PendingDisabled, "local-mark") {
		t.Error("своя отметка выключения затёрта приехавшими")
	}
	if !hasString(got.PendingDisabled, "ghost") {
		t.Error("приехавшая отметка неизвестного узла не доехала")
	}
	// Тег, который есть среди узлов, применяется сразу.
	if got.Nodes[1].Tag != "DE-2" || got.Nodes[1].Enabled {
		t.Errorf("отметка на живой узел не применена: %+v", got.Nodes[1])
	}
	if res.UpdatedSubscriptions != 1 || res.AddedSubscriptions != 0 {
		t.Errorf("счётчики: обновлено %d, добавлено %d", res.UpdatedSubscriptions, res.AddedSubscriptions)
	}
	// relays_in_directions входом 0.12 НЕ трогается: в той схеме поля нет
	// вовсе, и применить его «ноль» значило бы снять галку пользователя
	// импортом старого файла (BACKUP.md §9, local-only).
	if !got.RelaysInDirections {
		t.Error("вход 0.12 снял relays_in_directions — в схеме 0.12 поля нет, значит файл про него молчит")
	}
}

// TestMergeSubscriptionTakesFullSettingsFromFormat10 — у совпавшей подписки
// вход 1.0 применяет и то, чего в схеме 0.12 не было.
//
// Разница между ВХОДАМИ, а не между записями: relays_in_directions получил
// дом в форме 1.0 (§6.0), и на совпавшей записи значение файла обязано
// замещать локальное — иначе перенос на машину, где подписка уже есть (самый
// частый случай), молча оставляет чужую настройку. Вход 0.12 того же поля не
// несёт, и там оно не трогается — это проверено в тесте выше.
func TestMergeSubscriptionTakesFullSettingsFromFormat10(t *testing.T) {
	for _, tc := range []struct {
		name              string
		local, file, want bool
	}{
		{"файл включает", false, true, true},
		{"файл выключает", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := subSourceWithNodes("01LOCALSUB", "https://example-1.com/sub", "Local", "NL-1")
			local.RelaysInDirections = tc.local
			s := &state.State{Sources: []state.Source{local}}

			b := &Backup10{
				LxBackup: FormatVersion10,
				Sources: []Source10{{
					Kind: state.SourceKindSubscription, Enabled: true,
					ID: "01FILESUB", Name: "File", URL: "https://example-1.com/sub",
					RelaysInDirections: tc.file,
				}},
			}
			if _, err := Import10(s, b, ImportOptions{}); err != nil {
				t.Fatalf("Import10: %v", err)
			}
			if len(s.Sources) != 1 {
				t.Fatalf("источников %d — подписка задвоена", len(s.Sources))
			}
			if got := s.Sources[0].RelaysInDirections; got != tc.want {
				t.Errorf("relays_in_directions %v, ожидалось из файла %v", got, tc.want)
			}
			if s.Sources[0].ID != "01LOCALSUB" {
				t.Errorf("id %q — совпавшая запись обязана держать локальный", s.Sources[0].ID)
			}
		})
	}
}

// TestMergeFolderSettingsComeFromFormat10File — у совпавшей ПО ИМЕНИ папки
// собственные настройки берутся из файла 1.0, а идентичность и состав
// остаются локальными.
//
// Ровно та возможность, ради которой §6.0 завёл папке дом в схеме 1.0
// («настройки папки едут»). Пока настройки применялись только к НОВОЙ папке,
// главный сценарий переноса — на приёмнике папка с таким именем уже есть —
// терял их молча: ни применения, ни предупреждения, а обратный экспорт давал
// другой файл.
func TestMergeFolderSettingsComeFromFormat10File(t *testing.T) {
	local := state.Source{
		Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
		ID:   "01LOCALFLD", Name: "DE",
		TagPolicy: &state.TagPolicy{Prefix: "old:"},
		Nodes: []state.Node{{
			Kind: state.SourceKindServer, Tag: "keep-me", Enabled: true,
			Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://local#keep-me"},
		}},
	}
	s := &state.State{Sources: []state.Source{local}}

	b := &Backup10{
		LxBackup: FormatVersion10,
		Sources: []Source10{{
			Kind: state.SourceKindFolder, Enabled: true,
			ID: "01FILEFLD", Name: "DE",
			TagPolicy: &state.TagPolicy{Prefix: "[D] "},
			Detour:    &state.NodeLink{Tag: "WARP"},
			Replace:   &state.FolderReplace{Mode: state.FolderReplaceAuto, Tag: "DE-group"},
			Nodes: []state.Node{{
				Kind: state.SourceKindServer, Tag: "from-file", Enabled: true,
				Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://file#from-file"},
			}},
		}},
	}
	res, err := Import10(s, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import10: %v", err)
	}
	if len(s.Sources) != 1 {
		t.Fatalf("источников %d, ожидался 1 — папка задвоена", len(s.Sources))
	}
	got := s.Sources[0]

	// Идентичность и состав — локальные.
	if got.ID != "01LOCALFLD" {
		t.Errorf("id папки %q — совпавшая держит локальный (на него ссылаются detour/hops)", got.ID)
	}
	if len(got.Nodes) != 2 || got.Nodes[0].Tag != "keep-me" {
		t.Fatalf("состав папки %d узлов, первый %q — локальный член обязан остаться на месте",
			len(got.Nodes), got.Nodes[0].Tag)
	}

	// Настройки — из файла, все три.
	if got.TagPolicy == nil || got.TagPolicy.Prefix != "[D] " {
		t.Errorf("политика тегов папки %+v, ожидалась из файла", got.TagPolicy)
	}
	if got.Detour == nil || got.Detour.Tag != "WARP" {
		t.Errorf("общий detour папки %+v, ожидался из файла", got.Detour)
	}
	if got.Replace == nil || got.Replace.Tag != "DE-group" || got.Replace.Mode != state.FolderReplaceAuto {
		t.Errorf("свёртка папки %+v, ожидалась из файла с ЯВНЫМ тегом DE-group", got.Replace)
	}
	if res.UpdatedFolders != 1 || res.AddedFolders != 0 {
		t.Errorf("счётчики папок: обновлено %d, добавлено %d", res.UpdatedFolders, res.AddedFolders)
	}
}

// TestMergeKeepsLocalSourcesAbsentFromFile — локальное, чего в файле нет,
// остаётся; новое дописывается в КОНЕЦ, совпавшее держит свою позицию.
func TestMergeKeepsLocalSourcesAbsentFromFile(t *testing.T) {
	s := &state.State{Sources: []state.Source{
		subSourceWithNodes("01A", "https://example-1.com/a", "A"),
		subSourceWithNodes("01B", "https://example-2.com/b", "B"),
	}}
	b := &Backup{LxBackup: FormatVersion, Subscriptions: []Subscription{
		{URL: "https://example-2.com/b", Label: "B from file"},
		{ID: "01C", URL: "https://example-3.com/c", Label: "C"},
	}}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	var order []string
	for _, src := range s.Sources {
		order = append(order, src.Name)
	}
	want := []string{"A", "B from file", "C"}
	if !equalStrings(order, want) {
		t.Errorf("порядок %v, ожидался %v: совпавшая держит позицию, новая в конец", order, want)
	}
}

// TestMergeSubscriptionEmptyLabelKeepsLocalName — пустой label не затирает
// локальное имя: отсутствие поля значит «имени не носит», а не «сотри своё».
func TestMergeSubscriptionEmptyLabelKeepsLocalName(t *testing.T) {
	s := &state.State{Sources: []state.Source{
		subSourceWithNodes("01A", "https://example-1.com/a", "My name"),
	}}
	b := &Backup{LxBackup: FormatVersion, Subscriptions: []Subscription{
		{URL: "https://example-1.com/a"},
	}}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if s.Sources[0].Name != "My name" {
		t.Errorf("имя %q — пустой label затёр локальное", s.Sources[0].Name)
	}
}

// TestMergeSubscriptionURLIsByteExact — ключ URL сравнивается байт в байт:
// адрес, отличающийся слэшем или регистром хоста, — ДРУГАЯ подписка.
//
// Нормализация была бы соблазнительна, но обязана совпасть у двух реализаций
// посимвольно; расхождение дало бы разный итог из одного файла.
func TestMergeSubscriptionURLIsByteExact(t *testing.T) {
	s := &state.State{Sources: []state.Source{
		subSourceWithNodes("01A", "https://example-1.com/sub", "A"),
	}}
	b := &Backup{LxBackup: FormatVersion, Subscriptions: []Subscription{
		{URL: "https://example-1.com/sub/", Label: "trailing slash"},
		{URL: "https://EXAMPLE-1.com/sub", Label: "upper host"},
	}}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(s.Sources) != 3 {
		t.Fatalf("подписок %d, ожидалось 3 — адреса сравнены не байт в байт", len(s.Sources))
	}
}

// TestMergeServersDedupByBody — дедуп одиночных серверов идёт по ТЕЛУ:
// переименованный на другой машине узел узнаётся и НЕ плодит копию, а
// фрагмент `#имя` и порядок ключей config_json на сравнение не влияют.
func TestMergeServersDedupByBody(t *testing.T) {
	const uri = "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp#Home"
	body := json.RawMessage(`{"type":"trojan","server":"example-2.com","server_port":443,"password":"testpass123"}`)

	s := &state.State{Sources: []state.Source{
		uriServer("Home", uri),
		{
			Node: state.Node{
				Kind: state.SourceKindServer, Tag: "Json", Enabled: true,
				Body:   body,
				Origin: &state.Origin{Kind: state.OriginKindJSON, Raw: string(body)},
			},
			ID: "01JSON",
		},
	}}

	b := &Backup{LxBackup: FormatVersion, Servers: []Server{
		// То же тело, другое имя и другой фрагмент.
		{NodeTag: "Renamed", URI: "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp#Totally-Other"},
		// То же тело, ключи переставлены, свои tag/detour внутри JSON.
		{NodeTag: "JsonReordered", ConfigJSON: json.RawMessage(
			`{"tag":"JsonReordered","detour":"relay","password":"testpass123","server_port":443,"server":"example-2.com","type":"trojan"}`)},
		// Настоящий новый узел.
		{NodeTag: "Fresh", URI: "vless://11111111-1111-1111-1111-111111111111@example-3.com:443?type=tcp#Fresh"},
	}}

	res, err := Import(s, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var tags []string
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindServer {
			tags = append(tags, src.Tag)
		}
	}
	want := []string{"Home", "Json", "Fresh"}
	if !equalStrings(tags, want) {
		t.Errorf("корневые серверы %v, ожидались %v", tags, want)
	}
	if res.SkippedServers != 2 {
		t.Errorf("пропущено %d, ожидалось 2", res.SkippedServers)
	}
	// Пропуск дубля — не потеря, и warning'а не даёт.
	for _, w := range res.Warnings {
		t.Errorf("дедуп по телу дал предупреждение %s: %s", w.Code, w.Detail)
	}
}

// TestMergeServerTagUniquifiedAgainstRootSpace — новый узел с занятым именем
// получает суффикс `-2`, причём занятыми считаются и теги Направлений.
func TestMergeServerTagUniquifiedAgainstRootSpace(t *testing.T) {
	s := &state.State{
		Sources: []state.Source{
			uriServer("DE", "vless://11111111-1111-1111-1111-111111111111@example-1.com:443#DE"),
		},
	}
	s.Directions = append(s.Directions, importDirection(Direction{Tag: "Work"}, ""))

	b := &Backup{LxBackup: FormatVersion, Servers: []Server{
		{NodeTag: "DE", URI: "vless://11111111-1111-1111-1111-111111111111@example-9.com:443#DE"},
		{NodeTag: "Work", URI: "vless://11111111-1111-1111-1111-111111111111@example-8.com:443#Work"},
	}}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	var tags []string
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindServer {
			tags = append(tags, src.Tag)
		}
	}
	want := []string{"DE", "DE-2", "Work-2"}
	if !equalStrings(tags, want) {
		t.Errorf("теги %v, ожидались %v — имя Направления тоже занято", tags, want)
	}
}

// TestMergeFolderByNameCaseSensitive — папка ищется по имени КАК ЕСТЬ:
// «Proton» дополняется, «proton» заводит свою.
func TestMergeFolderByNameCaseSensitive(t *testing.T) {
	s := &state.State{Sources: []state.Source{{
		Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
		ID:   "01FLD", Name: "Proton",
		Nodes: []state.Node{{
			Kind: state.SourceKindServer, Tag: "P1", Enabled: true,
			Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "vless://u@example-1.com:443#P1"},
		}},
	}}}

	b := &Backup{LxBackup: FormatVersion, Servers: []Server{
		// То же тело в той же папке — дубль.
		{NodeTag: "P1-elsewhere", Folder: "Proton", URI: "vless://u@example-1.com:443#Other"},
		{NodeTag: "P2", Folder: "Proton", URI: "vless://u@example-2.com:443#P2"},
		{NodeTag: "P3", Folder: "proton", URI: "vless://u@example-3.com:443#P3"},
	}}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	folders := map[string][]string{}
	for _, src := range s.Sources {
		if src.Kind != state.SourceKindFolder {
			continue
		}
		var tags []string
		for i := range src.Nodes {
			tags = append(tags, src.Nodes[i].Tag)
		}
		folders[src.Name] = tags
	}
	if !equalStrings(folders["Proton"], []string{"P1", "P2"}) {
		t.Errorf("Proton = %v, ожидалось [P1 P2]", folders["Proton"])
	}
	if !equalStrings(folders["proton"], []string{"P3"}) {
		t.Errorf("proton = %v, ожидалось [P3] — имя папки регистрозависимо", folders["proton"])
	}
}

// TestMergeChainAndDirectionTagConflicts — занятый тег ссылочной сущности не
// перезаписывается, а совпадение точное: «Relay» и «relay» — разные теги.
func TestMergeChainAndDirectionTagConflicts(t *testing.T) {
	s := &state.State{
		Sources: []state.Source{{
			Node: state.Node{Kind: state.SourceKindChain, Enabled: true, Tag: "Relay",
				Body: json.RawMessage(`{"type":"chain"}`)},
			ID: "01LOCALCHAIN",
		}},
	}
	s.Directions = append(s.Directions, importDirection(Direction{Tag: "Work"}, ""))

	b := &Backup{LxBackup: FormatVersion,
		Chains: []Chain{
			{Tag: "Relay", Chain: &configtypes.SourceChain{Hops: []string{"a", "b"}}},
			{Tag: "relay", Chain: &configtypes.SourceChain{Hops: []string{"a", "b"}}},
		},
		Directions: []Direction{{Tag: "Work"}, {Tag: "work"}},
	}
	res, err := Import(s, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var chains []string
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindChain {
			chains = append(chains, src.Tag)
		}
	}
	if !equalStrings(chains, []string{"Relay", "relay"}) {
		t.Errorf("цепочки %v: занятый тег не перезаписывается, а «relay» — другой тег", chains)
	}
	// Локальная цепочка не тронута.
	if s.Sources[0].ID != "01LOCALCHAIN" {
		t.Error("локальная цепочка перезаписана приехавшей тёзкой")
	}
	var dirs []string
	for _, d := range s.Directions {
		dirs = append(dirs, d.Tag)
	}
	if !equalStrings(dirs, []string{"Work", "work"}) {
		t.Errorf("Направления %v", dirs)
	}
	codes := warnCodes(res.Warnings)
	if !hasString(codes, WarnBackupChainExists) || !hasString(codes, WarnBackupDirectionExists) {
		t.Errorf("конфликты тегов обязаны быть названы: %v", codes)
	}
}

// TestMergeRulesReplacedDNSMerged — rules[] замещаются целиком, а DNS
// сливается «своё сильнее»: совпавший резолвер остаётся локальным, новый
// дописывается в конец.
func TestMergeRulesReplacedDNSMerged(t *testing.T) {
	s := &state.State{
		Rules: []state.Rule{{Kind: state.RuleKindInline, Enabled: true,
			Body: json.RawMessage(`{"name":"local rule"}`)}},
	}
	s.DNS.Servers = []state.DNSServer{{
		Kind: "user", Tag: "home", Enabled: true,
		Body: map[string]interface{}{"server": "192.0.2.1"},
	}}
	s.DNS.Final = "home"

	b := &Backup{LxBackup: FormatVersion,
		Rules: []Rule{{Kind: RuleInline, Name: "from file", Match: json.RawMessage(`{}`)}},
		DNS: &DNS{
			Servers: []DNSRef{
				// Тот же kind+tag: своё сильнее, тело НЕ переписывается.
				{Kind: "user", Name: "home", Value: json.RawMessage(`{"server":"198.51.100.9"}`)},
				{Kind: "user", Name: "work", Value: json.RawMessage(`{"server":"198.51.100.1"}`)},
			},
			Final: "work",
		},
	}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(s.Rules) != 1 {
		t.Fatalf("правил %d — секция обязана быть замещена целиком", len(s.Rules))
	}
	if name := ruleName(s.Rules[0]); name != "from file" {
		t.Errorf("правило %q, ожидалось из файла", name)
	}
	if len(s.DNS.Servers) != 2 {
		t.Fatalf("DNS-серверов %d, ожидалось 2 (своё + новое)", len(s.DNS.Servers))
	}
	if s.DNS.Servers[0].Tag != "home" || s.DNS.Servers[0].Body["server"] != "192.0.2.1" {
		t.Errorf("совпавший резолвер переписан файлом: %+v", s.DNS.Servers[0])
	}
	if s.DNS.Servers[1].Tag != "work" {
		t.Errorf("новый резолвер не дописан: %+v", s.DNS.Servers[1])
	}
	// Одиночные значения замещаются: слить два ответа нечем.
	if s.DNS.Final != "work" {
		t.Errorf("dns.final %q, ожидался из файла", s.DNS.Final)
	}
}

// TestMergeWarpKeepsLocalAccount — локальная регистрация WARP переживает
// импорт: затереть её значило бы осиротить узлы, которые на ней стоят.
func TestMergeWarpKeepsLocalAccount(t *testing.T) {
	s := &state.State{WarpAccounts: &state.WarpAccountsSection{
		WG: &state.WarpWGAccount{PrivateKey: "local-key", DeviceID: "local-device"},
	}}
	b := &Backup{LxBackup: FormatVersion, Warp: []json.RawMessage{
		json.RawMessage(`{"type":"wg","private_key":"file-key","device_id":"file-device"}`),
		json.RawMessage(`{"type":"masque","private_key_der":"file-masque"}`),
	}}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if s.WarpAccounts.WG.PrivateKey != "local-key" {
		t.Errorf("локальный wg-аккаунт затёрт приехавшим: %+v", s.WarpAccounts.WG)
	}
	// Пустой слот занимается приехавшим — это и есть «добавление».
	if s.WarpAccounts.Masque == nil || s.WarpAccounts.Masque.PrivateKeyDER != "file-masque" {
		t.Error("свободный masque-слот не занят приехавшим аккаунтом")
	}
}

// TestMergeNewSourceIDCollisionGetsFreshULID — id из файла держится, пока
// свободен; при коллизии минтится свежий, иначе у одной адресации оказалось
// бы два владельца.
func TestMergeNewSourceIDCollisionGetsFreshULID(t *testing.T) {
	s := &state.State{Sources: []state.Source{
		subSourceWithNodes("01COLLIDE", "https://example-1.com/a", "A"),
	}}
	b := &Backup{LxBackup: FormatVersion, Subscriptions: []Subscription{
		{ID: "01COLLIDE", URL: "https://example-2.com/b", Label: "B"},
		{ID: "01FREE", URL: "https://example-3.com/c", Label: "C"},
	}}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	byName := map[string]string{}
	for _, src := range s.Sources {
		byName[src.Name] = src.ID
	}
	if byName["A"] != "01COLLIDE" {
		t.Errorf("локальный id перебит приехавшим: %q", byName["A"])
	}
	if byName["B"] == "01COLLIDE" || byName["B"] == "" {
		t.Errorf("коллизия id не разрешена свежим ULID: %q", byName["B"])
	}
	if byName["C"] != "01FREE" {
		t.Errorf("свободный id из файла не сохранён: %q", byName["C"])
	}
}

func hasString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestMergeDedupMaterializedLocalNode — дедуп работает против ЖИВОГО узла,
// у которого уже есть тело.
//
// Регрессия на порядок разбора в nodeBodyKey: локальный сервер после сборки
// или fetch материализован (Body заполнен), а приехавший из файла uri-узел
// тела ещё не имеет. Пока ключ строился сначала от Body, эти двое не
// совпадали, и каждый повторный импорт одного и того же файла удваивал
// сервер. В корпусе не ловилось: там обе стороны не материализованы.
func TestMergeDedupMaterializedLocalNode(t *testing.T) {
	const uri = "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp#Home"
	const jsonRaw = `{"type":"trojan","server":"example-2.com","server_port":443,"password":"testpass123"}`

	// Оба локальных узла МАТЕРИАЛИЗОВАНЫ: Body заполнен, origin на месте.
	local := state.Source{
		Node: state.Node{
			Kind: state.SourceKindServer, Tag: "Home", Enabled: true,
			Origin: &state.Origin{Kind: state.OriginKindURI, Raw: uri},
			Body:   json.RawMessage(`{"type":"vless","server":"example-1.com","server_port":443,"uuid":"11111111-1111-1111-1111-111111111111"}`),
		},
		ID: "01URI",
	}
	localJSON := state.Source{
		Node: state.Node{
			Kind: state.SourceKindServer, Tag: "Json", Enabled: true,
			Origin: &state.Origin{Kind: state.OriginKindJSON, Raw: jsonRaw},
			Body:   json.RawMessage(jsonRaw),
		},
		ID: "01JSON",
	}
	s := &state.State{Sources: []state.Source{local, localJSON}}

	b := &Backup{LxBackup: FormatVersion, Servers: []Server{
		// Тот же uri, другой #фрагмент — тела у приехавшего нет.
		{NodeTag: "Renamed", URI: "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?type=tcp#Other-Name"},
		// Тот же config_json, ключи переставлены, свои tag/detour.
		{NodeTag: "JsonRenamed", ConfigJSON: json.RawMessage(
			`{"tag":"JsonRenamed","detour":"relay","password":"testpass123","server_port":443,"server":"example-2.com","type":"trojan"}`)},
	}}

	res, err := Import(s, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var tags []string
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindServer {
			tags = append(tags, src.Tag)
		}
	}
	if !equalStrings(tags, []string{"Home", "Json"}) {
		t.Errorf("серверы %v, ожидались [Home Json]: материализованный локальный узел обязан узнаваться", tags)
	}
	if res.SkippedServers != 2 {
		t.Errorf("пропущено %d, ожидалось 2", res.SkippedServers)
	}

	// Повторный импорт того же файла ничего не добавляет — иначе каждый
	// импорт удваивал бы состав.
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("повторный Import: %v", err)
	}
	count := 0
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindServer {
			count++
		}
	}
	if count != 2 {
		t.Errorf("после повторного импорта серверов %d, ожидалось 2 — импорт не идемпотентен", count)
	}
}

// TestMergeFolderMembersIdempotentForRefKinds — повторный импорт файла 1.0 не
// удваивает ССЫЛОЧНЫХ членов папки (цепочку и провайдерскую группу).
//
// У этих видов тела нет вовсе: состав цепочки живёт в hops, состав группы — в
// group, а адресуются они ТЕГОМ, и в корне слияние их так и ключует. Пока
// внутри папки ключом служило только тело, он выходил пустым, «сравнивать
// нечем» означало «не дубль», и каждый следующий импорт одного файла дописывал
// ещё одну копию (auto-eu, auto-eu-2, auto-eu-3…): лишняя urltest-группа в
// конфиге и безграничный рост состояния. Форма 1.0 повезла таких членов
// впервые (§6.0), в 0.12 их вместо этого называли потерей.
func TestMergeFolderMembersIdempotentForRefKinds(t *testing.T) {
	b := &Backup10{
		LxBackup: FormatVersion10,
		Sources: []Source10{{
			Kind: state.SourceKindFolder, Enabled: true,
			ID: "01FLD", Name: "Proton",
			Nodes: []state.Node{
				{
					Kind: state.SourceKindServer, Tag: "ams", Enabled: true,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://ams#ams"},
				},
				{
					Kind: state.SourceKindAuto, Tag: "auto-eu", Enabled: true,
					Group: &state.AutoGroup{
						GroupType: state.AutoGroupURLTest,
						Members:   []state.NodeLink{{Tag: "ams"}},
					},
				},
				{
					Kind: state.SourceKindChain, Tag: "via-ams", Enabled: true,
					Hops: []state.NodeLink{{Tag: "ams"}},
				},
			},
		}},
	}

	s := state.New()
	for pass := 1; pass <= 3; pass++ {
		if _, err := Import10(s, b, ImportOptions{}); err != nil {
			t.Fatalf("импорт %d: %v", pass, err)
		}
	}
	if len(s.Sources) != 1 {
		t.Fatalf("источников %d, ожидалась одна папка", len(s.Sources))
	}
	var tags []string
	for _, n := range s.Sources[0].Nodes {
		tags = append(tags, string(n.Kind)+":"+n.Tag)
	}
	want := []string{"server:ams", "auto:auto-eu", "chain:via-ams"}
	if !equalStrings(tags, want) {
		t.Errorf("состав папки после трёх импортов %v, ожидался %v — ссылочные члены задвоены", tags, want)
	}
}

// TestMergeDNSPresetServersSurviveByRef — три preset-сервера ОДНОГО пресета
// переживают импорт в пустое состояние и повторный импорт без дублей.
//
// У kind=preset тега нет вовсе: идентичность записи — `ref` вида
// "<preset_id>:<local_tag>". Пока ключ слияния был kind+tag, все они давали
// один ключ "preset\x00", и после первого остальные молча отбрасывались как
// «своё сильнее». На живом состоянии владельца из 17 DNS-серверов после
// импорта в пустое оставалось 15 — пропадали russian:yandex_doh и
// russian:yandex_dot, без единого предупреждения.
func TestMergeDNSPresetServersSurviveByRef(t *testing.T) {
	refs := []string{"russian:yandex_udp", "russian:yandex_doh", "russian:yandex_dot"}

	b := &Backup10{LxBackup: FormatVersion10, DNS: &state.DNSOptions{Strategy: "prefer_ipv4"}}
	for _, ref := range refs {
		b.DNS.Servers = append(b.DNS.Servers, state.DNSServer{
			Kind: "preset", Ref: ref, Enabled: true,
		})
	}
	// Тёзки по tag у других видов не должны пострадать от общего ключа.
	b.DNS.Servers = append(b.DNS.Servers,
		state.DNSServer{Kind: "template", Tag: "local", Enabled: true},
		state.DNSServer{Kind: "user", Tag: "home", Enabled: true,
			Body: map[string]interface{}{"server": "192.0.2.1"}},
	)

	s := state.New()
	for pass := 1; pass <= 2; pass++ {
		if _, err := Import10(s, b, ImportOptions{}); err != nil {
			t.Fatalf("импорт %d: %v", pass, err)
		}
	}

	var got []string
	for _, srv := range s.DNS.Servers {
		got = append(got, string(srv.Kind)+":"+srv.Tag+srv.Ref)
	}
	want := []string{
		"preset:russian:yandex_udp",
		"preset:russian:yandex_doh",
		"preset:russian:yandex_dot",
		"template:local",
		"user:home",
	}
	if !equalStrings(got, want) {
		t.Fatalf("DNS-серверы после двух импортов %v, ожидалось %v", got, want)
	}
}

// TestMergeDNSPresetServerNotOverwrittenByFile — совпавший по ref preset
// остаётся ЛОКАЛЬНЫМ (§9 п. 5, «своё сильнее»): новый ключ не должен
// превратить слияние в добавление.
func TestMergeDNSPresetServerNotOverwrittenByFile(t *testing.T) {
	s := state.New()
	s.DNS.Servers = []state.DNSServer{
		{Kind: "preset", Ref: "russian:yandex_udp", Enabled: false},
	}

	b := &Backup10{LxBackup: FormatVersion10, DNS: &state.DNSOptions{
		Servers: []state.DNSServer{
			{Kind: "preset", Ref: "russian:yandex_udp", Enabled: true},
			{Kind: "preset", Ref: "russian:yandex_doh", Enabled: true},
		},
	}}
	if _, err := Import10(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import10: %v", err)
	}
	if len(s.DNS.Servers) != 2 {
		t.Fatalf("DNS-серверов %d, ожидалось 2 (свой + новый)", len(s.DNS.Servers))
	}
	if s.DNS.Servers[0].Ref != "russian:yandex_udp" || s.DNS.Servers[0].Enabled {
		t.Errorf("совпавший preset переписан файлом: %+v", s.DNS.Servers[0])
	}
	if s.DNS.Servers[1].Ref != "russian:yandex_doh" {
		t.Errorf("новый preset не дописан: %+v", s.DNS.Servers[1])
	}
}

// twinFolderState — состояние с ДВУМЯ папками-тёзками (разные id, разный
// состав). UI такую раскладку допускает, и она встретилась на живом
// состоянии владельца.
func twinFolderState() *state.State {
	s := state.New()
	s.Sources = []state.Source{
		{
			Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			ID:   "01FOLDERA", Name: "Folder 1",
			Nodes: []state.Node{
				{Kind: state.SourceKindServer, Tag: "a1", Enabled: true,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://a1#a1"}},
				{Kind: state.SourceKindServer, Tag: "a2", Enabled: true,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://a2#a2"}},
			},
		},
		{
			Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			ID:   "01FOLDERB", Name: "Folder 1",
			Nodes: []state.Node{
				{Kind: state.SourceKindServer, Tag: "b1", Enabled: true,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://b1#b1"}},
				{Kind: state.SourceKindServer, Tag: "b2", Enabled: true,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://b2#b2"}},
				{Kind: state.SourceKindServer, Tag: "b3", Enabled: true,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://b3#b3"}},
			},
		},
	}
	return s
}

// folderLayout — «id → состав» по тегам, в порядке источников.
func folderLayout(s *state.State) []string {
	var out []string
	for _, src := range s.Sources {
		if src.Kind != state.SourceKindFolder {
			continue
		}
		line := src.ID + "/" + src.Name + "="
		for i := range src.Nodes {
			if i > 0 {
				line += ","
			}
			line += src.Nodes[i].Tag
		}
		out = append(out, line)
	}
	return out
}

// TestImport10TwinFoldersMatchByID — свой же экспорт 1.0, ввезённый обратно в
// то же состояние, НИЧЕГО не меняет, даже когда папок-тёзок две.
//
// Пока папка матчилась только по имени, карта «имя → папка» видела из двух
// тёзок первую, и состав второй папки файла дописывался в первую локальную
// (дедуп по телу не спасал: тела разные). Состояние росло на каждом импорте
// собственного бэкапа.
func TestImport10TwinFoldersMatchByID(t *testing.T) {
	s := twinFolderState()
	before := folderLayout(s)

	b, warns, err := Export10(s, ExportOptions{})
	if err != nil {
		t.Fatalf("Export10: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("экспорт дал предупреждения: %v", warnCodes(warns))
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f, pw, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pw) != 0 {
		t.Fatalf("Parse дал предупреждения: %v", warnCodes(pw))
	}

	for pass := 1; pass <= 2; pass++ {
		if _, err := ImportFile(s, f, ImportOptions{}); err != nil {
			t.Fatalf("импорт %d: %v", pass, err)
		}
		after := folderLayout(s)
		if !equalStrings(after, before) {
			t.Fatalf("после импорта %d раскладка папок %v, ожидалась %v", pass, after, before)
		}
	}

	// Тот же файл в ПУСТОЕ состояние: обе папки обязаны доехать обеими, со
	// своими id и составом. Пока папка, заведённая этим же импортом,
	// находилась по ИМЕНИ, вторая запись файла (id B) попадала в только что
	// заведённую A: 18 источников превращались в 17, состав первой папки — обе.
	// «Одно имя = одна папка» — правило про состояние приёмника, а не про
	// содержимое файла: в файле папок две.
	fresh := state.New()
	for pass := 1; pass <= 2; pass++ {
		if _, err := ImportFile(fresh, f, ImportOptions{}); err != nil {
			t.Fatalf("импорт в пустое, проход %d: %v", pass, err)
		}
		after := folderLayout(fresh)
		if !equalStrings(after, before) {
			t.Fatalf("импорт в пустое, проход %d: раскладка %v, ожидалась %v", pass, after, before)
		}
	}
}

// TestImport10FolderFromOtherMachineMatchesByName — кросс-машинный случай:
// id из файла здесь неизвестен, и папка находится ПО ИМЕНИ (норма §9 п. 3).
// Совпавшая держит СВОЙ id: на него смотрят ссылки этой машины.
func TestImport10FolderFromOtherMachineMatchesByName(t *testing.T) {
	s := state.New()
	s.Sources = []state.Source{{
		Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
		ID:   "01LOCALB", Name: "X",
		Nodes: []state.Node{
			{Kind: state.SourceKindServer, Tag: "local", Enabled: true,
				Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://local#local"}},
		},
	}}

	b := &Backup10{LxBackup: FormatVersion10, Sources: []Source10{{
		Kind: state.SourceKindFolder, Enabled: true,
		ID: "01FILEA", Name: "X",
		Nodes: []state.Node{
			{Kind: state.SourceKindServer, Tag: "fromfile", Enabled: true,
				Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://fromfile#fromfile"}},
		},
	}}}
	if _, err := Import10(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import10: %v", err)
	}

	got := folderLayout(s)
	want := []string{"01LOCALB/X=local,fromfile"}
	if !equalStrings(got, want) {
		t.Fatalf("раскладка %v, ожидалась %v — папка обязана слиться по имени в локальную, id локальный", got, want)
	}
}

// TestImportLinksFollowMergeAddresses — ссылки файла идут туда, где слияние
// положило их цели (NODE_LINK.md §7.2, §7.4), и сборка результата разрешает
// их все: ни одна не висит и ни одна не ушла на здешнего тёзку.
//
// Слияние меняет адрес цели четырьмя способами, и каждый здесь есть:
// подписка, узнанная по URL, держит свой id; член папки уникализирован
// (`de-1` → `de-1-2`) или узнан по телу под другим тегом (`fr-1` →
// `fr-local`); корневой узел уникализирован (`tokyo` → `tokyo-2`) или узнан по
// телу (`osaka` → `osaka-local`). Ссылки — всех видов: detour члена и корневого
// узла, позиции цепочки, члены и умолчание группы, ссылка одним финальным
// тегом (терпимость §7.3). Legacy 0.12 — id сервера как источник-цель.
func TestImportLinksFollowMergeAddresses(t *testing.T) {
	body := func(server, password string) json.RawMessage {
		return json.RawMessage(`{"type":"trojan","server":"` + server + `","server_port":443,"password":"` + password + `"}`)
	}
	server := func(tag string, b json.RawMessage) state.Node {
		return state.Node{Kind: state.SourceKindServer, Tag: tag, Enabled: true, Body: b}
	}
	emitted := func(t *testing.T, s *state.State) map[string]map[string]interface{} {
		t.Helper()
		pc := &config.ParserConfig{}
		pc.ParserConfig.Version = config.ParserConfigVersion
		for i := range s.Sources {
			pc.ParserConfig.Proxies = append(pc.ParserConfig.Proxies, s.Sources[i].ToProxySourceV4())
		}
		res, err := config.GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, config.DirectionBuildOptions{})
		if err != nil {
			t.Fatalf("сборка: %v", err)
		}
		if len(res.EmissionWarnings) > 0 || len(res.BrokenChains) > 0 {
			t.Errorf("ссылки не разрешились на сборке: %s / %+v",
				strings.Join(config.EmissionWarningTexts(res.EmissionWarnings), " | "), res.BrokenChains)
		}
		out := map[string]map[string]interface{}{}
		for _, line := range res.OutboundsJSON {
			if at := strings.Index(line, "{"); at >= 0 {
				var m map[string]interface{}
				if json.Unmarshal([]byte(strings.TrimRight(strings.TrimSpace(line[at:]), ",")), &m) == nil {
					tag, _ := m["tag"].(string)
					out[tag] = m
				}
			}
		}
		return out
	}
	find := func(t *testing.T, s *state.State, name string) *state.Source {
		t.Helper()
		for i := range s.Sources {
			if s.Sources[i].Tag == name || s.Sources[i].Name == name {
				return &s.Sources[i]
			}
		}
		t.Fatalf("после импорта нет %q", name)
		return nil
	}
	member := func(t *testing.T, src *state.Source, tag string) *state.Node {
		t.Helper()
		for i := range src.Nodes {
			if src.Nodes[i].Tag == tag {
				return &src.Nodes[i]
			}
		}
		t.Fatalf("в %q нет члена %q", src.Name, tag)
		return nil
	}
	importJSON := func(t *testing.T, s *state.State, raw string) {
		t.Helper()
		f, _, err := Parse([]byte(raw))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if _, err := ImportFile(s, f, ImportOptions{}); err != nil {
			t.Fatalf("Import: %v", err)
		}
	}

	t.Run("1.0", func(t *testing.T) {
		s := state.New()
		s.Sources = []state.Source{
			{Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true}, ID: "01SUBLOCAL", URL: "https://example-1.com/sub",
				Nodes: []state.Node{server("US-1", body("example-10.com", "us"))}},
			{Node: state.Node{Kind: state.SourceKindFolder, Enabled: true}, ID: "01FLDLOCAL", Name: "Work",
				Nodes: []state.Node{server("de-1", body("example-9.com", "local")), server("fr-local", body("example-8.com", "fr"))}},
			{Node: server("tokyo", body("example-7.com", "tokyo-local")), ID: "01TOKYOLOCAL"},
			{Node: server("osaka-local", body("example-6.com", "osaka")), ID: "01OSAKALOCAL"},
		}
		importJSON(t, s, `{
  "lx_backup": 2, "exported_by": {"app": "launcher", "version": "t", "platform": "t"}, "exported_at": "2026-09-15T00:00:00Z",
  "sources": [
    {"kind": "subscription", "id": "01SUBFILE", "name": "P", "enabled": true, "url": "https://example-1.com/sub"},
    {"kind": "folder", "id": "01FLDFILE", "name": "Work", "enabled": true, "tag_policy": {"prefix": "[W] "}, "nodes": [
      {"kind": "server", "tag": "de-1", "enabled": true, "body": {"type": "trojan", "server": "example-2.com", "server_port": 443, "password": "file"}},
      {"kind": "server", "tag": "fr-1", "enabled": true, "body": {"type": "trojan", "server": "example-8.com", "server_port": 443, "password": "fr"}},
      {"kind": "auto", "tag": "grp", "enabled": true, "group": {"group_type": "selector", "default": "de-1", "members": [
        {"folder_id": "01FLDFILE", "tag": "de-1"}, {"folder_id": "01FLDFILE", "tag": "fr-1"}]}},
      {"kind": "server", "tag": "de-2", "enabled": true, "body": {"type": "trojan", "server": "example-3.com", "server_port": 443, "password": "de2"},
       "detour": {"folder_id": "01FLDFILE", "tag": "de-1"}}
    ]},
    {"kind": "server", "tag": "tokyo", "enabled": true, "body": {"type": "trojan", "server": "example-4.com", "server_port": 443, "password": "tokyo-file"}},
    {"kind": "server", "tag": "osaka", "enabled": true, "body": {"type": "trojan", "server": "example-6.com", "server_port": 443, "password": "osaka"}},
    {"kind": "server", "tag": "via-final", "enabled": true, "body": {"type": "trojan", "server": "example-5.com", "server_port": 443, "password": "vf"},
     "detour": {"tag": "[W] de-1"}},
    {"kind": "chain", "tag": "route", "enabled": true, "body": {"type": "chain"}, "hops": [
      {"folder_id": "01SUBFILE", "tag": "US-1"}, {"folder_id": "01FLDFILE", "tag": "fr-1"}, {"tag": "tokyo"}, {"tag": "osaka"}]}
  ]
}`)

		work := find(t, s, "Work")
		wantDe1 := state.NodeLink{FolderID: "01FLDLOCAL", Tag: "de-1-2"}
		wantFr := state.NodeLink{FolderID: "01FLDLOCAL", Tag: "fr-local"}
		if d := member(t, work, "de-2").Detour; d == nil || *d != wantDe1 {
			t.Errorf("detour члена на уникализированный член: %+v, ожидалось %+v", d, wantDe1)
		}
		if d := find(t, s, "via-final").Detour; d == nil || *d != wantDe1 {
			t.Errorf("ссылка финальным тегом на уникализированный член: %+v, ожидалось %+v", d, wantDe1)
		}
		g := member(t, work, "grp").Group
		if !reflect.DeepEqual(g.Members, []state.NodeLink{wantDe1, wantFr}) || g.Default == nil || *g.Default != wantDe1 {
			t.Errorf("группа: члены %+v, умолчание %+v", g.Members, g.Default)
		}
		wantHops := []state.NodeLink{{FolderID: "01SUBLOCAL", Tag: "US-1"}, wantFr, {Tag: "tokyo-2"}, {Tag: "osaka-local"}}
		if hops := find(t, s, "route").Hops; !reflect.DeepEqual(hops, wantHops) {
			t.Errorf("позиции цепочки %+v, ожидалось %+v", hops, wantHops)
		}

		out := emitted(t, s)
		if got := out["[W] de-2"]; got == nil || got["detour"] != "[W] de-1-2" {
			t.Errorf("сборка: detour члена %v", got)
		}
		if got := out["via-final"]; got == nil || got["detour"] != "[W] de-1-2" {
			t.Errorf("сборка: detour корневого узла %v", got)
		}
		if got := out["route"]; got == nil || !reflect.DeepEqual(got["outbounds"], []interface{}{"US-1", "[W] fr-local", "tokyo-2", "osaka-local"}) {
			t.Errorf("сборка: цепочка %v", got)
		}
		if got := out["[W] grp"]; got == nil || got["default"] != "[W] de-1-2" {
			t.Errorf("сборка: группа %v", got)
		}
	})

	t.Run("0.12", func(t *testing.T) {
		s := state.New()
		s.Sources = []state.Source{
			{Node: server("hop", body("example-9.com", "hop-local")), ID: "01HOPLOCAL"},
			{Node: state.Node{Kind: state.SourceKindFolder, Enabled: true}, ID: "01BOXLOCAL", Name: "Box",
				Nodes: []state.Node{server("member", body("example-8.com", "member-local"))}},
		}
		importJSON(t, s, `{
  "lx_backup": 1, "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"}, "exported_at": "2025-06-15T15:06:40Z",
  "servers": [
    {"id": "01SRVHOP", "config_json": {"type": "trojan", "server": "example-2.com", "server_port": 443, "password": "hop-file"}, "node_tag": "hop"},
    {"id": "01SRVDEP", "config_json": {"type": "trojan", "server": "example-3.com", "server_port": 443, "password": "dep"}, "node_tag": "dep",
     "detour_node_source_id": "01SRVHOP", "detour_node_tag": "hop", "detour_node_label": "hop"},
    {"id": "01SRVMEM", "config_json": {"type": "trojan", "server": "example-4.com", "server_port": 443, "password": "member-file"}, "node_tag": "member", "folder": "Box"},
    {"id": "01SRVDEP2", "config_json": {"type": "trojan", "server": "example-5.com", "server_port": 443, "password": "dep2"}, "node_tag": "dep2",
     "detour_node_source_id": "01SRVMEM", "detour_node_tag": "member", "detour_node_label": "member"}
  ],
  "chains": [{"id": "01CHN", "tag": "legacy-route", "chain": {"hops": ["member", "hop"]}}]
}`)

		wantHop := state.NodeLink{Tag: "hop-2"}
		wantMember := state.NodeLink{FolderID: "01BOXLOCAL", Tag: "member-2"}
		if d := find(t, s, "dep").Detour; d == nil || *d != wantHop {
			t.Errorf("detour по id корневого сервера: %+v, ожидалось %+v", d, wantHop)
		}
		if d := find(t, s, "dep2").Detour; d == nil || *d != wantMember {
			t.Errorf("detour по id сервера в папке: %+v, ожидалось %+v", d, wantMember)
		}
		if hops := find(t, s, "legacy-route").Hops; !reflect.DeepEqual(hops, []state.NodeLink{wantMember, wantHop}) {
			t.Errorf("строковые позиции: %+v, ожидалось %+v", hops, []state.NodeLink{wantMember, wantHop})
		}

		out := emitted(t, s)
		if got := out["dep"]; got == nil || got["detour"] != "hop-2" {
			t.Errorf("сборка: detour по id корневого сервера %v", got)
		}
		if got := out["dep2"]; got == nil || got["detour"] != "member-2" {
			t.Errorf("сборка: detour по id сервера в папке %v", got)
		}
		if got := out["legacy-route"]; got == nil || !reflect.DeepEqual(got["outbounds"], []interface{}{"member-2", "hop-2"}) {
			t.Errorf("сборка: цепочка %v", got)
		}
	})
}
