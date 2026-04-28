package template

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// isIntCastVar: вары, которые в sing-box JSON ожидаются как числа, а не строки.
// Плейсхолдер "@name" в шаблоне подменяется целым числом (strconv.Atoi).
func isIntCastVar(name string) bool {
	switch name {
	case "tun_mtu", "mixed_listen_port", "proxy_in_listen_port", "urltest_tolerance":
		return true
	}
	return false
}

// SubstituteVarsInJSON заменяет литералы "@name" в дереве JSON на разрешённые значения.
func SubstituteVarsInJSON(data []byte, vars []TemplateVar, resolved map[string]ResolvedVar) ([]byte, error) {
	varTypes := make(map[string]string, len(vars))
	for _, v := range vars {
		if v.Separator {
			continue
		}
		varTypes[v.Name] = v.Type
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root interface{}
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	substituteWalk(&root, varTypes, resolved)
	return json.Marshal(root)
}

func substituteWalk(v *interface{}, varTypes map[string]string, resolved map[string]ResolvedVar) {
	switch x := (*v).(type) {
	case map[string]interface{}:
		for k, val := range x {
			substituteWalk(&val, varTypes, resolved)
			x[k] = val
		}
	case []interface{}:
		if len(x) == 1 {
			if s, ok := x[0].(string); ok && strings.HasPrefix(s, "@") {
				name := s[1:]
				if name != "" && !strings.Contains(name, "@") {
					if rep := replacementForPlaceholder(name, varTypes, resolved); rep != nil {
						*v = rep
						return
					}
				}
			}
		}
		for i := range x {
			substituteWalk(&x[i], varTypes, resolved)
		}
	case string:
		if strings.HasPrefix(x, "@") {
			name := x[1:]
			if name != "" && !strings.Contains(name, "@") {
				if rep := replacementForPlaceholder(name, varTypes, resolved); rep != nil {
					*v = rep
				}
			}
		}
	}
}

func replacementForPlaceholder(name string, varTypes map[string]string, resolved map[string]ResolvedVar) interface{} {
	r, ok := resolved[name]
	if !ok {
		debuglog.WarnLog("substitute: unresolved @%s", name)
		return ""
	}
	typ := varTypes[name]
	if typ == "text_list" {
		if len(r.List) == 0 {
			debuglog.WarnLog("substitute: empty text_list @%s", name)
			return []interface{}{}
		}
		out := make([]interface{}, len(r.List))
		for i, s := range r.List {
			out[i] = s
		}
		return out
	}
	s := strings.TrimSpace(r.Scalar)
	if typ == "bool" {
		if s == "" {
			return false
		}
		return strings.EqualFold(s, "true")
	}
	if s == "" {
		debuglog.WarnLog("substitute: empty scalar @%s", name)
		if isIntCastVar(name) {
			return 0
		}
		return ""
	}
	if isIntCastVar(name) {
		n, err := strconv.Atoi(s)
		if err != nil {
			debuglog.WarnLog("substitute: invalid int @%s: %v", name, err)
			return 0
		}
		return n
	}
	return s
}
