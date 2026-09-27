package tabs

// Конформанс-раннер корпуса ПРАВКИ JSON узла (контракт 1.1.88,
// PARSING_PRINCIPLES §11 п.1; LxBox §576): contract/corpus/node_edit/
// <case>.edit.json — контейнер, прежний источник и ввод вкладки JSON;
// <case>.expected.json — источник после правки, авторское ли тело, назван ли
// несохранённый остаток ввода и (если объявлены) коды узла.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const nodeEditCorpusDir = "../../../contract/corpus/node_edit"

type nodeEditCase struct {
	Container string `json:"container"`
	Origin    struct {
		Kind string `json:"kind"`
		Raw  string `json:"raw"`
	} `json:"origin"`
	Input string `json:"input"`
}

type nodeEditExpected struct {
	Origin struct {
		Kind string          `json:"kind"`
		Raw  json.RawMessage `json:"raw"`
	} `json:"origin"`
	Authored    bool `json:"authored"`
	RestNotKept bool `json:"rest_not_kept"`
	Warnings    []struct {
		Code    string `json:"code"`
		Path    string `json:"path"`
		Applied *bool  `json:"applied"`
	} `json:"warnings"`
}

func TestContractCorpusNodeEdit(t *testing.T) {
	cases, _ := filepath.Glob(filepath.Join(nodeEditCorpusDir, "*.edit.json"))
	sort.Strings(cases)
	if len(cases) == 0 {
		t.Fatalf("корпус правки узла пуст: %s", nodeEditCorpusDir)
	}
	for _, casePath := range cases {
		name := strings.TrimSuffix(filepath.Base(casePath), ".edit.json")
		t.Run(name, func(t *testing.T) {
			var c nodeEditCase
			readJSONFile(t, casePath, &c)
			var want nodeEditExpected
			readJSONFile(t, filepath.Join(nodeEditCorpusDir, name+".expected.json"), &want)

			own := c.Container == "own" || c.Container == "folder"
			src := serverWithBody(c.Origin.Kind, c.Origin.Raw)
			if err := applyServerBodyJSON(&src.Node, c.Input, own); err != nil {
				t.Fatalf("правка отвергнута: %v", err)
			}
			if src.Origin == nil || src.Origin.Kind != want.Origin.Kind {
				t.Fatalf("вид источника = %+v, ожидали %q", src.Origin, want.Origin.Kind)
			}
			if !sameOriginRaw(src.Origin.Raw, want.Origin.Raw) {
				t.Errorf("источник = %s, ожидали %s", src.Origin.Raw, want.Origin.Raw)
			}
			if got := own && src.Node.Authored(); got != want.Authored {
				t.Errorf("authored = %t, ожидали %t", got, want.Authored)
			}
			if got := jsonInputDropsRest(c.Input); got != want.RestNotKept {
				t.Errorf("rest_not_kept = %t, ожидали %t", got, want.RestNotKept)
			}
			if want.Warnings == nil {
				return
			}
			type w struct {
				Code, Path string
				Applied    bool
			}
			var got, exp []w
			for _, x := range src.Node.Warnings {
				got = append(got, w{x.Code, x.Path, x.Applied == nil || *x.Applied})
			}
			for _, x := range want.Warnings {
				exp = append(exp, w{x.Code, x.Path, x.Applied == nil || *x.Applied})
			}
			if !reflect.DeepEqual(got, exp) {
				t.Errorf("коды = %+v, ожидали %+v", got, exp)
			}
		})
	}
}

func readJSONFile(t *testing.T, path string, v interface{}) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}
}

// sameOriginRaw — источник совпадает с ожиданием: JSON-источник по значению
// (ожидание пишется объектом), прочий — строкой.
func sameOriginRaw(got string, want json.RawMessage) bool {
	var ws string
	if json.Unmarshal(want, &ws) == nil {
		return got == ws
	}
	var gv, wv interface{}
	if json.Unmarshal([]byte(got), &gv) != nil || json.Unmarshal(want, &wv) != nil {
		return false
	}
	return reflect.DeepEqual(gv, wv)
}
