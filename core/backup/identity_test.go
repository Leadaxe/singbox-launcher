package backup

// Объект identity подписки (контракт 0.12.0) и legacy-вход label.
//
// Проверяется ровно то, что data-критично: настройка, которой подписка
// представляется провайдеру, обязана пережить круг экспорт→импорт (иначе на
// новой машине провайдер отдаст ДРУГОЙ набор узлов), а всё, что применить не
// удалось, обязано быть названо (П6).

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// Круг «экспорт → импорт»: четыре применяемых ключа возвращаются дословно.
//
// Именно круг, а не проверка одной стороны: потеря на любой из границ даёт
// одинаковый симптом — подписка спрашивает провайдера не тем, чем спрашивала.
func TestIdentityRoundTrip(t *testing.T) {
	send := false
	hashModel := true
	src := state.Source{
		ID:              "01SUB0000000000000000000",
		Node:            state.Node{Kind: state.SourceKindSubscription, Enabled: true},
		URL:             "https://example.invalid/s",
		Name:            "Liberty",
		UserAgent:       "Happ/1.0",
		HWID:            "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		SendHWID:        &send,
		HashDeviceModel: &hashModel,
	}
	b, _, err := Export(&state.State{Sources: []state.Source{src}}, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Через сырой JSON, а не по структуре: между сторонами едет файл, и
	// проверять надо то, что в нём действительно лежит.
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	dst := &state.State{}
	res, err := Import(dst, parsed, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if hasWarn(res.Warnings, WarnBackupSourceIdentityDropped) {
		t.Errorf("свои же ключи объявлены потерей: %v", res.Warnings)
	}
	if len(dst.Sources) != 1 {
		t.Fatalf("источников после импорта: %d", len(dst.Sources))
	}
	got := dst.Sources[0]
	if got.UserAgent != src.UserAgent {
		t.Errorf("user_agent = %q, ожидалось %q", got.UserAgent, src.UserAgent)
	}
	if got.HWID != src.HWID {
		t.Errorf("hwid = %q, ожидалось %q", got.HWID, src.HWID)
	}
	// Указатели, а не bool: «явно false» обязано отличаться от «не задано»,
	// иначе выключенная отправка HWID молча включается на приёмнике.
	if got.SendHWID == nil || *got.SendHWID {
		t.Errorf("send_hwid = %v, ожидалось явное false", got.SendHWID)
	}
	if got.HashDeviceModel == nil || !*got.HashDeviceModel {
		t.Errorf("hash_device_model = %v, ожидалось явное true", got.HashDeviceModel)
	}
}

// Ни одна настройка не задана — объекта identity в файле нет вовсе.
//
// Экспорт — чистая функция состояния (П1): пустышка в каждом файле была бы
// шумом, а на приёмнике «ключ есть, значение пустое» неотличимо от «выключи».
func TestIdentityAbsentWhenUnset(t *testing.T) {
	b, _, err := Export(&state.State{Sources: []state.Source{{
		ID:   "01SUB0000000000000000000",
		Node: state.Node{Kind: state.SourceKindSubscription, Enabled: true},
		URL:  "https://example.invalid/s",
	}}}, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
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
	res, err := Import(dst, b, ImportOptions{})
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
	if len(dst.Sources) != 1 || dst.Sources[0].UserAgent != "Happ/1.0" {
		t.Errorf("применяемый ключ не применён: %+v", dst.Sources)
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
	res, err := Import(&state.State{}, b, ImportOptions{})
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
	res, err := Import(dst, b, ImportOptions{})
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
	res, err := Import(dst, b, ImportOptions{})
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
// Папка не имеет секции в файле и собирается на приёмнике ПО ИМЕНИ, поэтому
// круг — единственная честная проверка: потеря пометки folder у одной записи
// молча растащила бы состав по корню списка.
func TestFolderRoundTrip(t *testing.T) {
	s := &state.State{Sources: []state.Source{
		{
			ID:   "01FLD0000000000000000000",
			Node: state.Node{Kind: state.SourceKindFolder, Enabled: true},
			Name: "Proton",
			// Настройка САМОЙ папки: в схему не входит, обязана быть названа.
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

	b, warns, err := Export(s, ExportOptions{AppVersion: "test"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	// Настройка папки осела здесь — молчать нельзя (П6).
	var named bool
	for _, w := range warns {
		if w.Code == WarnBackupLocalOnlyDropped && strings.Contains(w.Detail, "tag_policy") {
			named = true
		}
	}
	if !named {
		t.Errorf("политика тегов папки осела молча: %v", warns)
	}

	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, parseWarns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, w := range parseWarns {
		if w.Code == WarnBackupUnknownField && strings.Contains(w.Detail, "folder") {
			t.Errorf("объявленное поле folder прочитано как неизвестное: %v", w)
		}
	}

	dst := &state.State{}
	if _, err := Import(dst, parsed, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(dst.Sources) != 2 {
		t.Fatalf("источников после импорта: %d, ожидалось 2 (папка + корневой узел)", len(dst.Sources))
	}
	folder := dst.Sources[0]
	if folder.Kind != state.SourceKindFolder || folder.Name != "Proton" {
		t.Fatalf("первым источником ожидалась папка Proton: kind=%s name=%q", folder.Kind, folder.Name)
	}
	if len(folder.Nodes) != 2 {
		t.Fatalf("в папке %d узлов, ожидалось 2", len(folder.Nodes))
	}
	// Порядок членов нормативен.
	if folder.Nodes[0].Tag != "nl-01" || folder.Nodes[1].Tag != "de-02" {
		t.Errorf("порядок членов не сохранён: %q, %q", folder.Nodes[0].Tag, folder.Nodes[1].Tag)
	}
	if folder.Nodes[1].Enabled {
		t.Errorf("выключенный член приехал включённым")
	}
	// Запись без пометки folder осталась в корне, а не всосалась в папку.
	if dst.Sources[1].Kind != state.SourceKindServer || dst.Sources[1].Tag != "root-01" {
		t.Errorf("корневой узел не остался в корне: %+v", dst.Sources[1])
	}
}
