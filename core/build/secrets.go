package build

import (
	"strings"

	"singbox-launcher/core/template"
)

// MaterializeSecretsInVars один раз кладёт сгенерированные значения для всех
// объявленных в шаблоне type:"secret" переменных в `vars`, если соответствующий
// ключ в `vars` отсутствует либо лежит unresolved-форма (пустая строка /
// `@<name>` / плейсхолдер `CHANGE_THIS_*` — см. template.SecretUnresolved).
//
// Обобщение прежнего clash_secret-специфичного поведения (SPEC 067 follow-up):
// генерируется не только Clash API secret, но и любой secret-var (например
// proxy_in_password). Секрет материализуется всегда — даже если шаблон его не
// вставит (proxy_in_password не попадёт в config без непустого
// proxy_in_username; это решает #if в шаблоне, а не отсутствие пароля).
//
// Цель: предотвратить генерацию нового секрета при каждом
// `template.GetEffectiveConfig` — иначе preview каждый раз показывал бы свежий
// секрет, и любая запись в config.json меняла бы реально активное значение.
//
// Mutates: входной map `vars` может быть расширен ключами secret-переменных.
// Если td/vars nil — no-op. Источник энтропии — `crypto/rand` внутри
// `template.MaybeGenerateSecrets`.
func MaterializeSecretsInVars(td *template.TemplateData, vars map[string]string) {
	if td == nil || vars == nil {
		return
	}

	// Собираем secret-переменные, которые ещё нужно материализовать.
	var pending []string
	for _, v := range td.Vars {
		if v.Separator || !strings.EqualFold(strings.TrimSpace(v.Type), "secret") {
			continue
		}
		if s, ok := vars[v.Name]; !ok || template.SecretUnresolved(s) {
			pending = append(pending, v.Name)
		}
	}
	if len(pending) == 0 {
		return
	}

	resolved := template.ResolveTemplateVars(td.Vars, vars, td.RawTemplate)
	template.MaybeGenerateSecrets(td.Vars, resolved)
	for _, name := range pending {
		if rv, ok := resolved[name]; ok {
			if s := strings.TrimSpace(rv.Scalar); s != "" {
				vars[name] = s
			}
		}
	}
}
