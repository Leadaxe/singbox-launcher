package backup

// Объект identity подписки (контракт 0.12.0, в 1.0 форма та же) и legacy-вход
// label.
//
// Проверяется ровно то, что data-критично: настройка, которой подписка
// представляется провайдеру, обязана пережить круг экспорт→импорт (иначе на
// новой машине провайдер отдаст ДРУГОЙ набор узлов), а всё, что применить не
// удалось, обязано быть названо (П6).

import (
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// Круг «экспорт → импорт»: четыре применяемых ключа возвращаются дословно — и
// те же четыре приезжают из файла 0.12, снятого с этой подписки прежним
// лаунчером (legacy-вход).
//
// Именно круг, а не проверка одной стороны: потеря на любой из границ даёт
// одинаковый симптом — подписка спрашивает провайдера не тем, чем спрашивала.
func TestIdentityRoundTrip(t *testing.T) {
	send := false
	hashModel := true
	src := state.Source{
		ID:   "01SUB0000000000000000000",
		Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
		URL:  "https://example.invalid/s",
		Name: "Liberty",
	}
	src.SetIdentity("Happ/1.0", "7c9e6679-7425-40de-944b-e07fc1f90ae7", &send, &hashModel)

	// Файл 0.12, который прежний писатель снимал с этой подписки.
	const legacy = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "subscriptions": [{
    "id": "01SUB0000000000000000000",
    "url": "https://example.invalid/s",
    "label": "Liberty",
    "identity": {
      "user_agent": "Happ/1.0",
      "send_hwid": false,
      "hwid": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "hash_device_model": true
    }
  }]
}`

	// Через сырой JSON, а не по структуре: между сторонами едет файл, и
	// проверять надо то, что в нём действительно лежит.
	for _, in := range importBothFormats(t, &state.State{Sources: []state.Source{src}}, legacy, ImportOptions{}) {
		if hasWarn(in.warns, WarnBackupSourceIdentityDropped) {
			t.Errorf("%s: свои же ключи объявлены потерей: %v", in.format, in.warns)
		}
		if len(in.state.Sources) != 1 {
			t.Fatalf("%s: источников после импорта: %d", in.format, len(in.state.Sources))
		}
		got := in.state.Sources[0]
		if got.IdentityUserAgent() != src.IdentityUserAgent() {
			t.Errorf("%s: user_agent = %q, ожидалось %q", in.format, got.IdentityUserAgent(), src.IdentityUserAgent())
		}
		if got.IdentityHWID() != src.IdentityHWID() {
			t.Errorf("%s: hwid = %q, ожидалось %q", in.format, got.IdentityHWID(), src.IdentityHWID())
		}
		// Указатели, а не bool: «явно false» обязано отличаться от «не
		// задано», иначе выключенная отправка HWID молча включается на
		// приёмнике.
		if v := got.IdentitySendHWID(); v == nil || *v {
			t.Errorf("%s: send_hwid = %v, ожидалось явное false", in.format, v)
		}
		if v := got.IdentityHashDeviceModel(); v == nil || !*v {
			t.Errorf("%s: hash_device_model = %v, ожидалось явное true", in.format, v)
		}
	}
}

// Ни одна настройка не задана — объекта identity в файле нет вовсе.
//
// Экспорт — чистая функция состояния (П1): пустышка в каждом файле была бы
// шумом, а на приёмнике «ключ есть, значение пустое» неотличимо от «выключи».
func TestIdentityAbsentWhenUnset(t *testing.T) {
	raw := fixedExport10(t, &state.State{Sources: []state.Source{{
		ID:   "01SUB0000000000000000000",
		Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
		URL:  "https://example.invalid/s",
	}}})
	if strings.Contains(string(raw), "\"identity\"") {
		t.Errorf("пустой identity уехал в файл: %s", raw)
	}
}

// Ключи, которых эта сторона не применяет: ОДИН warning на подписку с
// перечнем. Применяемое рядом с ними применяется — потеря частичная.
func TestIdentityUnappliedKeysWarnOnce(t *testing.T) {
	raw := []byte(`{
	  "lx_backup": 1,
	  "exported_by": {"app": "lxbox", "version": "1.0"},
	  "exported_at": "2026-09-02T00:00:00Z",
	  "subscriptions": [{
	    "url": "https://example.invalid/s",
	    "label": "Liberty",
	    "identity": {
	      "user_agent": "Happ/1.0",
	      "device_os": "android",
	      "ver_os": "14"
	    }
	  }]
	}`)
	b, parseWarns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Внутрь identity общий обход неизвестных ключей не спускается: иначе
	// одна потеря давала бы два предупреждения разными кодами.
	for _, w := range parseWarns {
		if w.Code == WarnBackupUnknownField && strings.Contains(w.Detail, "identity") {
			t.Errorf("ключ identity продублирован как неизвестное поле: %v", w)
		}
	}

	dst := &state.State{}
	res, err := ImportFile(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var details []string
	for _, w := range res.Warnings {
		if w.Code == WarnBackupSourceIdentityDropped {
			details = append(details, w.Detail)
		}
	}
	if len(details) != 1 {
		t.Fatalf("ожидался ровно один warning на подписку, получено %d: %v", len(details), details)
	}
	d := details[0]
	if !strings.Contains(d, "Liberty") {
		t.Errorf("предупреждение не называет подписку: %q", d)
	}
	for _, want := range []string{"device_os", "ver_os"} {
		if !strings.Contains(d, want) {
			t.Errorf("не назван неприменённый ключ %q: %q", want, d)
		}
	}
	if strings.Contains(d, "user_agent") {
		t.Errorf("применённый ключ назван потерей: %q", d)
	}
	if len(dst.Sources) != 1 || dst.Sources[0].IdentityUserAgent() != "Happ/1.0" {
		t.Errorf("применяемый ключ не применён: %+v", dst.Sources)
	}
	assertMobileOnlyIdentityNotStored(t, dst.Sources[0])
}

// TestIdentityUnappliedKeysWarnOnceFormat10 — тот же identity входом 1.0 даёт
// то же состояние и то же предупреждение.
//
// Ответ на вопрос «что эта сторона умеет применить» не может зависеть от
// формата: иначе одна и та же подписка, записанная двумя писателями, давала бы
// на приёмнике два разных состояния и два разных разговора с пользователем —
// при том что оба входа ведут в ОДИН код слияния. Mobile-only тройку лаунчер
// не применяет (per-source её у него нет), и сложить её в состояние значило бы
// завести состояние-призрак, который ничего не делает, но переопубликовывается
// каждым следующим экспортом (П1/П3).
func TestIdentityUnappliedKeysWarnOnceFormat10(t *testing.T) {
	raw := []byte(`{
	  "lx_backup": 2,
	  "exported_by": {"app": "lxbox", "version": "2.0"},
	  "exported_at": "2026-09-14T00:00:00Z",
	  "sources": [{
	    "kind": "subscription",
	    "enabled": true,
	    "url": "https://example.invalid/s",
	    "name": "Liberty",
	    "identity": {
	      "user_agent": "Happ/1.0",
	      "device_os": "android",
	      "ver_os": "14"
	    }
	  }]
	}`)
	b, parseWarns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, w := range parseWarns {
		if w.Code == WarnBackupUnknownField && strings.Contains(w.Detail, "identity") {
			t.Errorf("ключ identity продублирован как неизвестное поле: %v", w)
		}
	}

	dst := &state.State{}
	res, err := ImportFile(dst, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var details []string
	for _, w := range res.Warnings {
		if w.Code == WarnBackupSourceIdentityDropped {
			details = append(details, w.Detail)
		}
	}
	if len(details) != 1 {
		t.Fatalf("ожидался ровно один warning на подписку, получено %d: %v", len(details), details)
	}
	for _, want := range []string{"device_os", "ver_os"} {
		if !strings.Contains(details[0], want) {
			t.Errorf("не назван неприменённый ключ %q: %q", want, details[0])
		}
	}
	if len(dst.Sources) != 1 || dst.Sources[0].IdentityUserAgent() != "Happ/1.0" {
		t.Errorf("применяемый ключ не применён: %+v", dst.Sources)
	}
	assertMobileOnlyIdentityNotStored(t, dst.Sources[0])
}

// assertMobileOnlyIdentityNotStored — mobile-only ключи не осели в состоянии.
//
// Проверяется СОСТОЯНИЕ, а не только warning: предупредить о потере и тут же
// сохранить потерянное — это и есть состояние-призрак, которое следующий
// экспорт переопубликует как настоящую настройку.
func assertMobileOnlyIdentityNotStored(t *testing.T, src state.Source) {
	t.Helper()
	id := src.Identity
	if id == nil {
		return
	}
	if id.DeviceOS != nil || id.VerOS != nil || id.DeviceModel != nil {
		t.Errorf("mobile-only ключи осели в состоянии: device_os=%v ver_os=%v device_model=%v",
			id.DeviceOS, id.VerOS, id.DeviceModel)
	}
}

// Пустой объект identity — не потеря: предупреждают о том, что не доехало, а
// не о факте наличия поля.
func TestIdentityEmptyObjectIsSilent(t *testing.T) {
	raw := []byte(`{
	  "lx_backup": 1,
	  "exported_by": {"app": "lxbox", "version": "1.0"},
	  "exported_at": "2026-09-02T00:00:00Z",
	  "subscriptions": [{"url": "https://example.invalid/s", "identity": {}}]
	}`)
	b, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res, err := ImportFile(&state.State{}, b, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if hasWarn(res.Warnings, WarnBackupSourceIdentityDropped) {
		t.Errorf("пустой identity объявлен потерей: %v", res.Warnings)
	}
}

// Legacy-вход label (файлы 0.11 и раньше): у сервера БЕЗ node_tag подпись
// становится тегом — иначе узел приехал бы безымянным. Потери нет, значит и
// предупреждения нет.
func TestLegacyServerLabelBecomesTag(t *testing.T) {
	raw := []byte(`{
	  "lx_backup": 1,
	  "exported_by": {"app": "lxbox", "version": "1.0"},
	  "exported_at": "2026-09-02T00:00:00Z",
	  "servers": [{"uri": "ss://YWVzLTI1Ni1nY206cHdk@1.2.3.4:8388#node", "label": "Netherlands"}]
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
	if len(dst.Sources) != 1 {
		t.Fatalf("источников после импорта: %d", len(dst.Sources))
	}
	if got := dst.Sources[0].Tag; got != "Netherlands" {
		t.Errorf("тег = %q, ожидалось %q (подпись спасает безымянный узел)", got, "Netherlands")
	}
	if hasWarn(res.Warnings, WarnBackupLabelDropped) {
		t.Errorf("подпись стала тегом, потери нет — предупреждать не о чем: %v", res.Warnings)
	}
}

// Тот же legacy-вход, но тег уже есть и подпись с ним разошлась: применить её
// некуда (у канона v7 имя одно), и потеря обязана быть названа.
func TestLegacyServerLabelDivergedWarns(t *testing.T) {
	raw := []byte(`{
	  "lx_backup": 1,
	  "exported_by": {"app": "lxbox", "version": "1.0"},
	  "exported_at": "2026-09-02T00:00:00Z",
	  "servers": [{
	    "uri": "ss://YWVzLTI1Ni1nY206cHdk@1.2.3.4:8388#node",
	    "node_tag": "nl-01",
	    "label": "Netherlands"
	  }]
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
	if got := dst.Sources[0].Tag; got != "nl-01" {
		t.Errorf("подпись увела тег из-под ссылок: %q", got)
	}
	if !hasWarn(res.Warnings, WarnBackupLabelDropped) {
		t.Errorf("разошедшаяся подпись отброшена молча: %v", res.Warnings)
	}
}

// Круг папки: экспорт → импорт → та же папка с тем же составом и порядком.
//
// Входов два, и форма папки у них разная: у 1.0 папка — своя запись с
// составом в nodes[] и собственными настройками, у файла 0.12 её нет вовсе —
// обе стороны собирают её ПО ИМЕНИ из записей servers[] с полем folder. Круг —
// единственная честная проверка: потеря пометки folder у одной записи молча
// растащила бы состав по корню списка.
func TestFolderRoundTrip(t *testing.T) {
	s := &state.State{Sources: []state.Source{
		{
			ID:   "01FLD0000000000000000000",
			Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Name: "Proton",
			// Настройка САМОЙ папки: в 1.0 у неё есть дом, в 0.12 — нет.
			TagPolicy: &state.TagPolicy{Prefix: "p-"},
			Nodes: []state.Node{
				{
					Kind: state.SourceKindServer, Tag: "nl-01", Enabled: true,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://YWVzLTI1Ni1nY206cHdk@1.2.3.4:8388#nl"},
				},
				{
					Kind: state.SourceKindServer, Tag: "de-02", Enabled: false,
					Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://YWVzLTI1Ni1nY206cHdk@5.6.7.8:8388#de"},
				},
			},
		},
		{
			ID: "01SRV0000000000000000000",
			Node: state.Node{Kind: state.SourceKindServer, Tag: "root-01", Enabled: true,
				Origin: &state.Origin{Kind: state.OriginKindURI, Raw: "ss://YWVzLTI1Ni1nY206cHdk@9.9.9.9:8388#root"}},
		},
	}}

	// Файл 0.12, который прежний писатель снимал с этого состояния: политику
	// тегов папки он терял (называя потерю на экспорте), членов писал
	// записями servers[] с именем папки.
	const legacy = `{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.9", "platform": "darwin"},
  "exported_at": "2025-06-15T15:06:40Z",
  "servers": [
    {"uri": "ss://YWVzLTI1Ni1nY206cHdk@1.2.3.4:8388#nl", "node_tag": "nl-01", "folder": "Proton"},
    {"uri": "ss://YWVzLTI1Ni1nY206cHdk@5.6.7.8:8388#de", "node_tag": "de-02", "folder": "Proton", "enabled": false},
    {"id": "01SRV0000000000000000000", "uri": "ss://YWVzLTI1Ni1nY206cHdk@9.9.9.9:8388#root", "node_tag": "root-01"}
  ]
}`

	for _, in := range importBothFormats(t, s, legacy, ImportOptions{}) {
		for _, w := range in.warns {
			if w.Code == WarnBackupUnknownField && strings.Contains(w.Detail, "folder") {
				t.Errorf("%s: объявленное поле папки прочитано как неизвестное: %v", in.format, w)
			}
		}
		dst := in.state
		if len(dst.Sources) != 2 {
			t.Fatalf("%s: источников после импорта: %d, ожидалось 2 (папка + корневой узел)", in.format, len(dst.Sources))
		}
		folder := dst.Sources[0]
		if folder.Kind != state.SourceKindFolder || folder.Name != "Proton" {
			t.Fatalf("%s: первым источником ожидалась папка Proton: kind=%s name=%q", in.format, folder.Kind, folder.Name)
		}
		if len(folder.Nodes) != 2 {
			t.Fatalf("%s: в папке %d узлов, ожидалось 2", in.format, len(folder.Nodes))
		}
		// Порядок членов нормативен.
		if folder.Nodes[0].Tag != "nl-01" || folder.Nodes[1].Tag != "de-02" {
			t.Errorf("%s: порядок членов не сохранён: %q, %q", in.format, folder.Nodes[0].Tag, folder.Nodes[1].Tag)
		}
		if folder.Nodes[1].Enabled {
			t.Errorf("%s: выключенный член приехал включённым", in.format)
		}
		// Запись без пометки folder осталась в корне, а не всосалась в папку.
		if dst.Sources[1].Kind != state.SourceKindServer || dst.Sources[1].Tag != "root-01" {
			t.Errorf("%s: корневой узел не остался в корне: %+v", in.format, dst.Sources[1])
		}
		// Настройка самой папки: у 1.0 дом есть, и она обязана доехать.
		if in.format == "1.0" && (folder.TagPolicy == nil || folder.TagPolicy.Prefix != "p-") {
			t.Errorf("1.0: политика тегов папки не доехала: %+v", folder.TagPolicy)
		}
	}
}
