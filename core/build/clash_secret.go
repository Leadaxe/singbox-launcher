package build

import (
	"strings"

	"singbox-launcher/core/template"
)

// MaterializeClashSecretInVars один раз кладёт сгенерированный clash_secret
// в `vars`, если соответствующая template-переменная объявлена в td.Vars и
// в `vars` либо отсутствует ключ "clash_secret", либо лежит unresolved-форма
// (`@clash_secret` / пустая строка / т.п. — см. template.ClashSecretUnresolved).
//
// Цель: предотвратить генерацию нового секрета при каждом
// `template.GetEffectiveConfig`. Без этой функции preview бы каждый раз
// показывал свежий секрет, и любая запись в config.json меняла бы реально
// активный bearer для Clash API.
//
// Mutates: входной map `vars` может быть расширен ключом "clash_secret".
// Если td/vars nil — no-op.
//
// Pure (модулём): сторонних эффектов нет; источник энтропии — `crypto/rand`
// внутри `template.MaybeGenerateClashSecret`.
//
// Ссылка на legacy: ui/wizard/business/create_config.go::MaterializeClashSecretIfNeeded
func MaterializeClashSecretInVars(td *template.TemplateData, vars map[string]string) {
	if td == nil || vars == nil {
		return
	}

	// Декларация var должна существовать в шаблоне; иначе материализация не нужна.
	hasVar := false
	for _, v := range td.Vars {
		if v.Name == "clash_secret" {
			hasVar = true
			break
		}
	}
	if !hasVar {
		return
	}

	// Уже резолвится в осмысленное значение → ничего не делаем.
	if s, ok := vars["clash_secret"]; ok && !template.ClashSecretUnresolved(s) {
		return
	}

	resolved := template.ResolveTemplateVars(td.Vars, vars, td.RawTemplate)
	template.MaybeGenerateClashSecret(resolved)
	if rv, ok := resolved["clash_secret"]; ok {
		if s := strings.TrimSpace(rv.Scalar); s != "" {
			vars["clash_secret"] = s
		}
	}
}
