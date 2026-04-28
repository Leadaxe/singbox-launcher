package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// validateIfIfOrRefs проверяет взаимоисключение if/if_or и ссылки только на bool vars (для params и vars).
func validateIfIfOrRefs(ctx string, ifNames, ifOrNames []string, varByName map[string]TemplateVar) error {
	if len(ifNames) > 0 && len(ifOrNames) > 0 {
		return fmt.Errorf("%s: if and if_or cannot both be set", ctx)
	}
	for _, iname := range ifNames {
		vd, ok := varByName[iname]
		if !ok {
			return fmt.Errorf("%s.if: unknown var %q", ctx, iname)
		}
		if vd.Type != "bool" {
			return fmt.Errorf("%s.if: var %q must be type bool, got %q", ctx, iname, vd.Type)
		}
	}
	for _, iname := range ifOrNames {
		vd, ok := varByName[iname]
		if !ok {
			return fmt.Errorf("%s.if_or: unknown var %q", ctx, iname)
		}
		if vd.Type != "bool" {
			return fmt.Errorf("%s.if_or: var %q must be type bool, got %q", ctx, iname, vd.Type)
		}
	}
	return nil
}

// validWizardVarNameRE — лексика vars[].name (SPEC 032).
var validWizardVarNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateWizardTemplate проверяет vars и ссылки @name в config и params (PLAN / TASKS 032).
// Секция vars в сыром JSON на плейсхолдеры @ не сканируется — только config и params.
func ValidateWizardTemplate(vars []TemplateVar, params []TemplateParam, config json.RawMessage) error {
	names := make(map[string]struct{})
	varByName := make(map[string]TemplateVar, len(vars))
	for i, v := range vars {
		if v.Separator {
			if err := validateVarsSeparator(i, v); err != nil {
				return err
			}
			continue
		}
		nm := strings.TrimSpace(v.Name)
		if nm == "" {
			return fmt.Errorf("vars[%d]: empty name", i)
		}
		if !validWizardVarNameRE.MatchString(nm) {
			return fmt.Errorf("vars[%d]: invalid name %q (expected [A-Za-z_][A-Za-z0-9_]*)", i, nm)
		}
		if _, dup := names[nm]; dup {
			return fmt.Errorf("vars: duplicate name %q", nm)
		}
		names[nm] = struct{}{}
		varByName[nm] = v
	}

	for i, v := range vars {
		ctx := fmt.Sprintf("vars[%d]", i)
		if err := validateIfIfOrRefs(ctx, v.If, v.IfOr, varByName); err != nil {
			return err
		}
	}

	for i, p := range params {
		ctx := fmt.Sprintf("params[%d]", i)
		if err := validateIfIfOrRefs(ctx, p.If, p.IfOr, varByName); err != nil {
			return err
		}
		refs, err := collectPlaceholderNamesFromJSON(p.Value)
		if err != nil {
			return fmt.Errorf("params[%d].value: %w", i, err)
		}
		for _, ref := range refs {
			if _, ok := names[ref]; !ok {
				return fmt.Errorf("params[%d]: @%q is not declared in vars", i, ref)
			}
		}
	}

	refs, err := collectPlaceholderNamesFromJSON(config)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	for _, ref := range refs {
		if _, ok := names[ref]; !ok {
			return fmt.Errorf("config: @%q is not declared in vars", ref)
		}
	}
	return nil
}

// validateVarsSeparator: {"separator": true} — только оформление Settings; без name и плейсхолдеров.
func validateVarsSeparator(i int, v TemplateVar) error {
	ctx := fmt.Sprintf("vars[%d]", i)
	if strings.TrimSpace(v.Name) != "" {
		return fmt.Errorf("%s: separator must not set name", ctx)
	}
	if strings.TrimSpace(v.Type) != "" {
		return fmt.Errorf("%s: separator must not set type", ctx)
	}
	if !v.DefaultValue.IsEmpty() || strings.TrimSpace(v.DefaultNode) != "" {
		return fmt.Errorf("%s: separator must not set default_value or default_node", ctx)
	}
	if len(v.Options) > 0 {
		return fmt.Errorf("%s: separator must not set options", ctx)
	}
	if strings.TrimSpace(v.Title) != "" || strings.TrimSpace(v.Tooltip) != "" {
		return fmt.Errorf("%s: separator must not set title or tooltip", ctx)
	}
	if len(v.If) > 0 || len(v.IfOr) > 0 {
		return fmt.Errorf("%s: separator must not set if or if_or", ctx)
	}
	wu := strings.ToLower(strings.TrimSpace(v.WizardUI))
	if wu != "" && wu != "hidden" {
		return fmt.Errorf("%s: separator wizard_ui must be empty or \"hidden\"", ctx)
	}
	return nil
}

func collectPlaceholderNamesFromJSON(raw json.RawMessage) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var v interface{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var out []string
	walkJSONPlaceholders(v, &out)
	return out, nil
}

func walkJSONPlaceholders(v interface{}, out *[]string) {
	switch x := v.(type) {
	case map[string]interface{}:
		for _, val := range x {
			walkJSONPlaceholders(val, out)
		}
	case []interface{}:
		if len(x) == 1 {
			if s, ok := x[0].(string); ok {
				if name := parseAtVarName(s); name != "" {
					*out = append(*out, name)
					return
				}
			}
		}
		for _, el := range x {
			walkJSONPlaceholders(el, out)
		}
	case string:
		if name := parseAtVarName(x); name != "" {
			*out = append(*out, name)
		}
	case json.Number, bool, nil:
		// skip
	default:
		// float64 from JSON — не плейсхолдеры
	}
}

func parseAtVarName(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "@") {
		return ""
	}
	name := strings.TrimSpace(s[1:])
	if name == "" || strings.Contains(name, "@") {
		return ""
	}
	return name
}
