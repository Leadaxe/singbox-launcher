package nodeflow

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// CoreInfo — ядро, под которое собирается config.json.
type CoreInfo struct {
	// Version — версия ядра форка, "1.14.1-lx.4"; пустая = гейт по версии
	// не применяется (не деградируем по догадке — та же политика, что у
	// проб возможностей ядра).
	Version string
	// GOOS — целевая ОС сборки; пустая = runtime.GOOS текущего процесса.
	GOOS string
}

// GateForCore снимает из готового тела ключи, которых это ядро не знает
// (min_core выше его версии) или которые на этой ОС не работают (platform).
//
// Тело узла в state заморожено и от запущенного ядра не зависит — гейт живёт
// только на сборке (SPEC 131 §3.4). ⚠ на узле от него НЕ ставится: тело
// верное, ограничен рантайм; снятые пути возвращаются для WARN-строки в лог
// сборки.
func GateForCore(scheme string, body []byte, core CoreInfo) (out []byte, dropped []string, err error) {
	reg, err := registry.Get()
	if err != nil {
		return nil, nil, err
	}
	schema, ok := reg.Body(scheme)
	if !ok {
		return body, nil, nil
	}
	var m map[string]interface{}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&m); err != nil {
		return nil, nil, err
	}
	if core.GOOS == "" {
		core.GOOS = runtime.GOOS
	}
	g := &gate{core: core}
	g.object("", schema.Order, schema.Fields, m)
	if len(g.dropped) == 0 {
		return body, nil, nil
	}
	var buf bytes.Buffer
	if err := emitObject(&buf, schema.Order, schema.Fields, m); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), g.dropped, nil
}

type gate struct {
	core    CoreInfo
	dropped []string
}

func (g *gate) object(prefix string, order []string, fields map[string]*registry.Field, m map[string]interface{}) {
	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		if _, ok := m[name]; !ok {
			continue
		}
		path := joinPath(prefix, name)
		if !g.supported(f) {
			delete(m, name)
			g.dropped = append(g.dropped, path)
			continue
		}
		inner, ok := m[name].(map[string]interface{})
		if !ok {
			continue
		}
		subOrder, subFields := emitShape(f, inner)
		if subOrder == nil {
			continue
		}
		g.object(path, subOrder, subFields, inner)
	}
}

// supported — умеет ли целевое ядро это поле.
func (g *gate) supported(f *registry.Field) bool {
	if f.Platform != "" && g.core.GOOS != "" && f.Platform != g.core.GOOS {
		return false
	}
	if f.MinCore != "" && g.core.Version != "" {
		if compareCoreVersions(g.core.Version, f.MinCore) < 0 {
			return false
		}
	}
	return true
}

// compareCoreVersions сравнивает версии ядра форка вида MAJOR.MINOR.PATCH-lx.N.
//
// Своя функция, а не core.CompareVersions: тот живёт в пакете core, который
// импортирует core/config, — обратная ссылка дала бы цикл. И правило тут
// другое: суффикс -lx.N значим (lx.4 > lx.3 на одной базе), а не просто
// «есть суффикс = новее». Ядро без -lx (апстрим) на равной базе считается
// младше форковой сборки: lx-поля в нём появиться не могли.
func compareCoreVersions(have, want string) int {
	haveBase, haveLx := splitCoreVersion(have)
	wantBase, wantLx := splitCoreVersion(want)
	if c := compareNumericParts(haveBase, wantBase); c != 0 {
		return c
	}
	if haveLx == wantLx {
		return 0
	}
	if haveLx < wantLx {
		return -1
	}
	return 1
}

// splitCoreVersion разбирает "1.14.1-lx.4-rc.2" на базу и номер релиза форка.
// Номер -1 означает «форкового суффикса нет».
func splitCoreVersion(v string) (base []int, lx int) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	lx = -1
	rest := v
	if i := strings.Index(v, "-"); i >= 0 {
		rest = v[:i]
		if j := strings.Index(v[i:], "lx."); j >= 0 {
			tail := v[i+j+len("lx."):]
			end := 0
			for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
				end++
			}
			if end > 0 {
				if n, err := strconv.Atoi(tail[:end]); err == nil {
					lx = n
				}
			}
		}
	}
	for _, p := range strings.Split(rest, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		base = append(base, n)
	}
	return base, lx
}

func compareNumericParts(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
