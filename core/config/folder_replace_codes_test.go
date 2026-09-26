package config

import (
	"encoding/json"
	"strconv"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// folderReplaceTestSource — папка-источник из узлов socks с каноном и
// свёрткой; enabled задаёт включённость всех её узлов.
func folderReplaceTestSource(id string, rep *configtypes.FolderReplace, enabled bool, tags ...string) ProxySource {
	nodes := make([]configtypes.CanonicalNode, 0, len(tags))
	for i, tag := range tags {
		body := `{"type":"socks","server":"10.0.0.1","server_port":` + strconv.Itoa(1080+i) + `}`
		nodes = append(nodes, configtypes.CanonicalNode{Kind: "server", Tag: tag, Enabled: enabled, Body: json.RawMessage(body)})
	}
	return ProxySource{
		ID:        id,
		Label:     id,
		Canonical: &configtypes.CanonicalSource{FolderID: id, IsContainer: true, Nodes: nodes, Replace: rep},
	}
}

// Контракт 1.1.80: тег свёртки — объявленное корневое имя. Узел-тёзка не
// конфликт: он уникализируется суффиксом, а группа свёртки собирается под
// своим тегом. Свёрнутый источник без включённых узлов — код
// replace_group_empty {tag, mode} в отчёте сборки вместо строки лога.
func TestFolderReplaceNamesakeNodeAndEmptyGroupCodes(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{
		folderReplaceTestSource("F1", &configtypes.FolderReplace{Mode: configtypes.FolderReplaceManual, Tag: "AL:tokyo"}, true, "AL:tokyo", "AL:osaka"),
		folderReplaceTestSource("F2", &configtypes.FolderReplace{Mode: configtypes.FolderReplaceBoth, Tag: "EMPTY"}, false, "BR:one"),
	}
	opts := DirectionBuildOptions{BlockTag: "block-out", DirectTag: "direct-out"}
	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, opts)
	if err != nil {
		t.Fatalf("генерация: %v", err)
	}

	byTag := map[string]map[string]interface{}{}
	for _, entry := range res.OutboundsJSON {
		var m map[string]interface{}
		if json.Unmarshal([]byte(decodeCorpusEntry(entry)), &m) == nil {
			if tag, _ := m["tag"].(string); tag != "" {
				byTag[tag] = m
			}
		}
	}
	sel := byTag["AL:tokyo"]
	if sel == nil || sel["type"] != "selector" {
		t.Fatalf("тег свёртки должен остаться за группой: %v", sel)
	}
	if n := byTag["AL:tokyo-2"]; n == nil || n["type"] != "socks" {
		t.Fatalf("узел-тёзка должен получить суффикс -2: %v", byTag)
	}
	if _, has := byTag["EMPTY"]; has {
		t.Errorf("пустая группа свёртки не должна попасть в конфиг")
	}

	var empty, conflicts int
	for _, w := range res.EmissionWarnings {
		switch w.Code {
		case codeReplaceGroupEmpty:
			empty++
			if w.Params["tag"] != "EMPTY" || w.Params["mode"] != configtypes.FolderReplaceBoth || w.SourceID != "F2" {
				t.Errorf("replace_group_empty: params=%v source=%q", w.Params, w.SourceID)
			}
		case codeReplaceTagConflict:
			conflicts++
		case "":
			t.Errorf("предупреждение без кода: %s", w.Text)
		}
	}
	if empty != 1 {
		t.Errorf("replace_group_empty: %d записей, ждали одну на свёртку", empty)
	}
	if conflicts != 0 {
		t.Errorf("узел-тёзка — не конфликт объявленных имён, а replace_tag_conflict стоит %d раз", conflicts)
	}
}
