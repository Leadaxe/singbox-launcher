package backup

// Слияние при импорте (D-095, BACKUP.md §9).
//
// Кейсы data-критичные: каждый ловит способ ТИХО потерять или задвоить то,
// что пользователь настроил руками. Формат строк и вёрстку здесь не
// проверяют — только то, что после импорта лежит в состоянии.

import (
	"encoding/json"
	"testing"

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
	local.UserAgent = "local-ua"
	local.PendingDisabled = []string{"local-mark"}
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
	if got.UserAgent != "" {
		t.Errorf("UA %q — объекта identity в файле нет, значит «как в системе»", got.UserAgent)
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
	s.Directions = append(s.Directions, importDirection(Direction{Tag: "Work"}))

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
	s.Directions = append(s.Directions, importDirection(Direction{Tag: "Work"}))

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
