// File uri_params.go — чтение параметра ссылки ПО РЕЕСТРУ (SPEC 131 W2d).
//
// До этой волны набор написаний одного и того же параметра жил в коде и
// отличался от парсера к парсеру: `insecure` у базы знал три имени, у TUIC
// шесть, у MASQUE — своё подчёркнутое `skip_cert_verify` и регистрозависимый
// поиск, а Xray-ветка читала только JSON-bool. Один и тот же узел терял
// `allow_insecure` на одном пути и принимал его на другом (DRIFT §2(a)) —
// расхождение, которое ни один санитайзер закрыть не может: до него значение
// просто не доезжает.
//
// Теперь таблица написаний — в реестре (`uri.query.<param>.aliases`), и
// парсеры читают её отсюда. Канон списка — первое имя; обратный эмиттер
// share-URI обязан писать именно его, иначе пара парсер/эмиттер расходится
// (память emitter-parser-pairing).
package subscription

import (
	"net/url"
	"strings"

	"singbox-launcher/core/config/registry"
)

// queryParam — значение параметра ссылки со всеми его написаниями из реестра.
//
// Регистронезависимость сохранена намеренно: корпус на неё опирается
// (`AllowInsecure=1`, `SNI=`, `Fp=` — живые формы публичных списков), а
// объединение имён и регистронезависимость — независимые свойства: реестр
// даёт первое, queryGetFold — второе.
//
// Схема, которой реестр не знает, и параметр без записи `aliases` читаются
// одним своим именем: отсутствие таблицы не повод потерять значение.
func queryParam(q url.Values, scheme, param string) string {
	for _, name := range queryParamNames(scheme, param) {
		if v := strings.TrimSpace(queryGetFold(q, name)); v != "" {
			return v
		}
	}
	return ""
}

// queryParamRaw — то же без обрезки пробелов по краям: значение, где пробел
// значим (пароль, путь), обрезать нельзя.
func queryParamRaw(q url.Values, scheme, param string) string {
	for _, name := range queryParamNames(scheme, param) {
		if v := queryGetFold(q, name); v != "" {
			return v
		}
	}
	return ""
}

// queryParamNames — написания параметра, канон первым.
func queryParamNames(scheme, param string) []string {
	reg, err := registry.Get()
	if err != nil {
		return []string{param}
	}
	if names, ok := reg.QueryAliases(scheme, param); ok {
		return names
	}
	return []string{param}
}

// queryFlagTrue — булев параметр ссылки по всем его написаниям.
//
// Истина — `1`, `true`, `yes` после trim+lower. Набор один на все схемы и все
// пути: до W2d MASQUE не знал `yes`, hysteria2 не знал его в skip-ветке, а
// Xray-ветка принимала только настоящий JSON-bool.
func queryFlagTrue(q url.Values, scheme, param string) bool {
	return flagValueTrue(queryParam(q, scheme, param))
}

// flagValueTrue — одно правило истинности для всех булевых параметров ссылок.
func flagValueTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}
