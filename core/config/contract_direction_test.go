package config

// Конформанс-раннер корпуса НАПРАВЛЕНИЙ (SPEC 104, фаза 4).
//
// Гоняет contract/corpus/direction/*.direction.json через тот же путь, что
// боевая сборка: канонические Направления → configtypes.Direction →
// GenerateOutboundsFromParserConfig. Проверяется то, что увидит ядро, а не
// промежуточная структура: иначе раннер начал бы жить своей жизнью.
//
// Тот же корпус читает LxBox — расхождение expected означает, что модель
// разъехалась между платформами.
//
// Регенерация: go test ./core/config -run TestContractCorpusDirection -update

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

var updateDirectionCorpus = flag.Bool("update-direction", false,
	"перезаписать expected-файлы корпуса направлений")

// corpusDirection — каноническая форма Направления (contract/schema/direction.schema.json).
type corpusDirection struct {
	Tag                       string           `json:"tag"`
	Enabled                   *bool            `json:"enabled,omitempty"`
	Filter                    string           `json:"filter,omitempty"`
	Invert                    bool             `json:"invert,omitempty"`
	Default                   string           `json:"default,omitempty"`
	IncludeDirect             bool             `json:"include_direct,omitempty"`
	IncludeBlock              bool             `json:"include_block,omitempty"`
	Include                   []string         `json:"include,omitempty"`
	InterruptExistConnections *bool            `json:"interrupt_exist_connections,omitempty"`
	Auto                      *corpusAutoGroup `json:"auto,omitempty"`
}

// corpusChain — каноническая форма источника-цепочки (SPEC 110).
type corpusChain struct {
	Tag          string                 `json:"tag"`
	Hops         []string               `json:"hops"`
	IdleTimeout  string                 `json:"idle_timeout,omitempty"`
	StripEvasion *bool                  `json:"strip_evasion,omitempty"`
	Strip        map[string]bool        `json:"strip,omitempty"`
	Rewrite      map[string]interface{} `json:"rewrite,omitempty"`
}

type corpusAutoGroup struct {
	Mode                      string   `json:"mode,omitempty"`
	URL                       string   `json:"url,omitempty"`
	Interval                  string   `json:"interval,omitempty"`
	Tolerance                 int      `json:"tolerance,omitempty"`
	IdleTimeout               string   `json:"idle_timeout,omitempty"`
	InterruptExistConnections *bool    `json:"interrupt_exist_connections,omitempty"`
	Pool                      int      `json:"pool,omitempty"`
	PoolTolerance             int      `json:"pool_tolerance,omitempty"`
	StickyHash                []string `json:"sticky_hash,omitempty"`
}

type corpusDirectionCase struct {
	Doc        string            `json:"_"`
	Directions []corpusDirection `json:"directions"`
	NodeTags   []string          `json:"node_tags"`
	GroupTags  []string          `json:"group_tags,omitempty"`
	Magic      map[string]string `json:"magic,omitempty"`

	// Fold — свёртка единственной подписки кейса (SPEC 108,
	// schema/source_fold.schema.json). nil — подписка не свёрнута, её узлы
	// идут в Направления по отдельности.
	Fold *corpusSourceFold `json:"fold,omitempty"`

	// TagPrefix — префикс тегов подписки: от него зависят теги её групп.
	TagPrefix string `json:"tag_prefix,omitempty"`

	// Chains — источники-цепочки кейса (SPEC 110), в порядке объявления:
	// цепочка вправе сослаться на объявленную выше, и порядок нормативен.
	Chains []corpusChain `json:"chains,omitempty"`

	// CoreSupportsChain — умеет ли ядро тип `chain` (SPEC 110). Отсутствует
	// = умеет. Кейс с false проверяет деградацию: ядро без with_lx_chain
	// отвергает конфиг ЦЕЛИКОМ, поэтому цепочка обязана не эмититься вовсе.
	// Это единственное свойство ОКРУЖЕНИЯ в корпусе, и оно здесь потому,
	// что от него зависит выход, а не форма модели.
	CoreSupportsChain *bool `json:"core_supports_chain,omitempty"`
}

// corpusSourceFold — каноническая форма свёртки
// (contract/schema/source_fold.schema.json).
type corpusSourceFold struct {
	Mode string           `json:"mode,omitempty"`
	Auto *corpusAutoGroup `json:"auto,omitempty"`
}

func (c corpusAutoGroup) toDirectionAuto() *configtypes.DirectionAuto {
	return &configtypes.DirectionAuto{
		Mode:                      c.Mode,
		URL:                       c.URL,
		Interval:                  c.Interval,
		Tolerance:                 configtypes.NewTemplateInt(c.Tolerance),
		IdleTimeout:               c.IdleTimeout,
		InterruptExistConnections: c.InterruptExistConnections,
		Pool:                      c.Pool,
		PoolTolerance:             configtypes.NewTemplateInt(c.PoolTolerance),
		StickyHash:                c.StickyHash,
	}
}

type corpusDirectionExpected struct {
	Groups   []map[string]interface{} `json:"groups"`
	Warnings []string                 `json:"warnings,omitempty"`
}

// toDirection переводит каноническую форму во внутреннюю.
//
// Это и есть проверяемый шов: если маппинг перестанет быть однозначным,
// корпус развалится раньше, чем расхождение доедет до пользователя.
func (c corpusDirection) toDirection() configtypes.Direction {
	d := configtypes.Direction{
		Tag:      c.Tag,
		Type:     "selector",
		Disabled: c.Enabled != nil && !*c.Enabled,
		Filters:  configtypes.SetDirectionFilterTag(nil, c.Filter, c.Invert),
	}
	d.PreferredDefault = configtypes.SetDirectionFilterTag(nil, c.Default, false)

	// Порядок опций нормативен: сначала другие Направления, потом служебные.
	d.AddOutbounds = append(d.AddOutbounds, c.Include...)
	if c.IncludeDirect {
		d.AddOutbounds = append(d.AddOutbounds, "direct-out")
	}
	if c.IncludeBlock {
		d.AddOutbounds = append(d.AddOutbounds, "block-out")
	}
	if c.InterruptExistConnections != nil {
		d.Options = map[string]interface{}{
			"interrupt_exist_connections": *c.InterruptExistConnections,
		}
	}
	if c.Auto != nil {
		d.Auto = c.Auto.toDirectionAuto()
	}
	return d
}

func TestContractCorpusDirection(t *testing.T) {
	dir := filepath.Join("..", "..", "contract", "corpus", "direction")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("корпус недоступен: %v", err)
	}

	cases := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".direction.json") {
			continue
		}
		caseName := strings.TrimSuffix(name, ".direction.json")
		cases++
		t.Run(caseName, func(t *testing.T) {
			if reason := corpusDivergence[caseName]; reason != "" {
				t.Skip(reason)
			}
			runDirectionCorpusCase(t, dir, caseName)
		})
	}
	if cases == 0 {
		t.Fatal("корпус направлений пуст — раннер молча проходил бы всегда")
	}
}

// corpusDivergence — кейсы корпуса, ожидания которых относятся к УПРАЗДНЁННОЙ
// свёртке (`fold` с позиционным тегом), а не к её наследнику в модели v7
// (`FolderReplace` с ЯВНЫМ тегом).
//
// SPEC 118, расхождение Р2 (задекларировано в etalon/README.md и утверждено
// капитаном): в режиме «селектор + автогруппа» пара тегов перестала быть
// `<PFX>select` + `<PFX>auto` — автогруппа стала ДВОЙНИКОМ селектора и носит
// производный тег `<tag>-auto`, той же формулой, что твины Направлений. На
// совпадении формулы держится узнавание пар, и второй схемы имён быть не
// может.
//
// Корпус — общий контракт с LxBox-стороной и в этой кампании не меняется
// (SPEC 118 §2). Поэтому кейс не «подгоняется» и не удаляется: он назван
// здесь вслух и вернётся, когда контракт догонит модель (этап 4).
var corpusDivergence = map[string]string{
	"fold_select_auto": "SPEC 118 Р2: пара тегов свёртки both стала `<tag>` + `<tag>-auto` " +
		"(двойник), корпус ждёт прежние `<PFX>select` + `<PFX>auto`; контракт догоняет модель на этапе 4",
}

func runDirectionCorpusCase(t *testing.T, dir, caseName string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, caseName+".direction.json"))
	if err != nil {
		t.Fatalf("read case: %v", err)
	}
	var in corpusDirectionCase
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("parse case: %v", err)
	}

	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	src := ProxySource{Source: "https://example.com/sub", TagPrefix: in.TagPrefix}
	var folderReplace *configtypes.FolderReplace
	// SPEC 118 W5: свёртка контракта (fold) переводится в замену канона
	// (FolderReplace) — тем же соответствием, что конвертеры бэкапа. Тег
	// замены материализуем прежним позиционным деривативом: корпус ссылается
	// на `<PFX>select`/`<PFX>auto` из своих Направлений.
	if in.Fold != nil {
		prefix := strings.TrimSpace(in.TagPrefix)
		if prefix == "" {
			prefix = "1:"
		}
		rep := &configtypes.FolderReplace{}
		switch in.Fold.Mode {
		case "auto":
			rep.Mode = configtypes.FolderReplaceAuto
			rep.Tag = prefix + "auto"
		case "select_auto":
			rep.Mode = configtypes.FolderReplaceBoth
			rep.Tag = prefix + "select"
		default:
			rep.Mode = configtypes.FolderReplaceManual
			rep.Tag = prefix + "select"
		}
		if in.Fold.Auto != nil {
			rep.Strategy = in.Fold.Auto.toDirectionAuto()
		}
		folderReplace = rep
	}
	pc.ParserConfig.Proxies = []ProxySource{src}
	// SPEC 110: источники-цепочки идут отдельными записями, в порядке
	// объявления кейса — от него зависит, разрешится ли вложенная цепочка.
	for _, cc := range in.Chains {
		pc.ParserConfig.Proxies = append(pc.ParserConfig.Proxies, ProxySource{
			Label: cc.Tag,
			Canonical: &configtypes.CanonicalSource{
				Nodes: []configtypes.CanonicalNode{{Kind: "chain", Tag: cc.Tag, Enabled: true}},
			},
			Chain: &configtypes.SourceChain{
				Hops:         cc.Hops,
				IdleTimeout:  cc.IdleTimeout,
				StripEvasion: cc.StripEvasion,
				Strip:        cc.Strip,
				Rewrite:      cc.Rewrite,
			},
		})
	}
	for _, cd := range in.Directions {
		pc.ParserConfig.Outbounds = append(pc.ParserConfig.Outbounds, cd.toDirection())
	}

	groupTags := make(map[string]bool, len(in.GroupTags))
	for _, tag := range in.GroupTags {
		groupTags[tag] = true
	}
	nodes := make([]*ParsedNode, 0, len(in.NodeTags))
	for i, tag := range in.NodeTags {
		n := &ParsedNode{Tag: tag, Scheme: "socks", Server: "10.0.0.1", Port: 1080 + i}
		if groupTags[tag] {
			// Группа выбора подписки приходит обычным узлом (SPEC 094 A5);
			// от сервера её отличает схема.
			n.Scheme = SchemeGroup
			n.Outbound = map[string]interface{}{
				"type": "urltest", "outbounds": []interface{}{},
			}
		}
		nodes = append(nodes, n)
	}

	if in.CoreSupportsChain != nil {
		prev := ChainSupportProbe
		supported := *in.CoreSupportsChain
		ChainSupportProbe = func() (bool, string) {
			if supported {
				return true, ""
			}
			return false, "ядро собрано без with_lx_chain"
		}
		defer func() { ChainSupportProbe = prev }()
	}

	opts := DirectionBuildOptions{
		BlockTag:  in.Magic["block"],
		DirectTag: in.Magic["direct"],
	}
	// SPEC 118 Т5: узлы приезжают КАНОНОМ, а не загрузчиком тел — конвейер
	// сборки парсер подписок больше не зовёт вовсе.
	canonNodes := make([]configtypes.CanonicalNode, 0, len(nodes))
	for _, n := range nodes {
		if n.Scheme == SchemeGroup {
			// Группа выбора внутри подписки — узел kind=auto канона: в
			// импортированном теле группа приезжает ровно так (SPEC 094 A5).
			canonNodes = append(canonNodes, configtypes.CanonicalNode{
				Kind: "auto", Tag: n.Tag, Enabled: true,
				Group: &configtypes.CanonicalAutoGroup{GroupType: "urltest"},
			})
			continue
		}
		body := `{"type":"socks","server":"` + n.Server + `","server_port":` + strconv.Itoa(n.Port) + `}`
		canonNodes = append(canonNodes, configtypes.CanonicalNode{
			Kind: "server", Tag: n.Tag, Enabled: true, Body: json.RawMessage(body),
		})
	}
	pc.ParserConfig.Proxies[0].Canonical = &configtypes.CanonicalSource{
		FolderID: "F1", IsContainer: true, Nodes: canonNodes, Replace: folderReplace,
	}

	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, opts)
	if err != nil && len(in.NodeTags) > 0 {
		t.Fatalf("генерация: %v", err)
	}

	got := corpusDirectionExpected{Groups: []map[string]interface{}{}}
	if res != nil {
		nodeTagSet := make(map[string]bool, len(in.NodeTags))
		for _, tag := range in.NodeTags {
			nodeTagSet[tag] = true
		}
		for _, entry := range res.OutboundsJSON {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(decodeCorpusEntry(entry)), &m); err != nil {
				continue
			}
			tag, _ := m["tag"].(string)
			if tag == "" || nodeTagSet[tag] {
				continue // сами узлы в ожидания не входят — проверяем группы
			}
			if t, _ := m["type"].(string); t == configtypes.ChainOutboundType {
				// SPEC 110: цепочка эмитится узлом. В ожиданиях она нужна —
				// это и есть проверяемый результат, — но идёт как есть, без
				// чистки шаблонных полей ниже.
				got.Groups = append(got.Groups, m)
				continue
			}
			delete(m, "interrupt_exist_connections") // шаблонное поле, не про модель
			got.Groups = append(got.Groups, m)
		}
		for _, name := range res.EmptyDirections {
			_ = name
			got.Warnings = append(got.Warnings, "direction_filter_matched_nothing")
		}
		for _, c := range res.BrokenChains {
			// Код различает причину: «ядро не умеет» и «позиция потерялась»
			// требуют от пользователя разных действий — обновить ядро или
			// починить состав, — и один общий код скрыл бы это различие.
			// Имена — из contract/registry/warnings.json.
			code := "chain_invalid"
			switch {
			case strings.Contains(c.Reason, "with_lx_chain"):
				code = "chain_unsupported_by_core"
			case strings.Contains(c.Reason, "not found among nodes"):
				code = "chain_hop_missing"
			}
			got.Warnings = append(got.Warnings, code)
		}
		for range res.ChainCycles {
			got.Warnings = append(got.Warnings, "chain_cycle_through_direction")
		}
	}
	sort.Strings(got.Warnings)

	expPath := filepath.Join(dir, caseName+".expected.json")
	if *updateDirectionCorpus {
		writeCorpusExpected(t, expPath, got)
		return
	}

	expRaw, err := os.ReadFile(expPath)
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}
	var want corpusDirectionExpected
	if err := json.Unmarshal(expRaw, &want); err != nil {
		t.Fatalf("parse expected: %v", err)
	}
	sort.Strings(want.Warnings)

	gotJSON, _ := json.MarshalIndent(got, "", "  ")
	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("%s\n--- получено ---\n%s\n--- ожидалось ---\n%s",
			in.Doc, gotJSON, wantJSON)
	}
}

// decodeCorpusEntry снимает обёртку генератора: строчный комментарий и
// хвостовую запятую элемента массива.
func decodeCorpusEntry(raw string) string {
	body := raw
	if i := strings.LastIndex(body, "\n"); i >= 0 {
		body = body[i+1:]
	}
	return strings.TrimSuffix(strings.TrimSpace(body), ",")
}

func writeCorpusExpected(t *testing.T, path string, v corpusDirectionExpected) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal expected: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write expected: %v", err)
	}
}
