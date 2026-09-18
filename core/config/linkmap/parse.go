package linkmap

// Точка входа движка: ТЕКСТ ссылки → пространство источников → исполнение
// плана.
//
// Движок принимает текст, а не разобранный URL-объект (MAPPER_ENGINE.md §1):
// платформенный парсер URL отвергает то, что движок обязан принять
// (`host:443,20000-30000` у multi-port).
//
// Декодеры оболочки объявляются формой (`forms[].decode`) и применяются
// конвейером с ограничением глубины: иначе подписка base64 внутри base64
// раскрывалась бы неограниченно.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// maxDecodeDepth — предел конвейера декодеров одной формы.
//
// Ограничение объявлено движком, а не формой: форма называет декодеры, а
// сколько их можно применить подряд — свойство защиты, общее для всех форм.
const maxDecodeDepth = 8

// ParseInput — текст одного элемента входа вместе с тем, что о нём уже знает
// уровень документа.
type ParseInput struct {
	// Text — сырой текст элемента (одна ссылка, одна строка списка).
	Text string
	// Scheme — написание схемы, как оно стоит в ссылке; пустое, если элемент
	// пришёл не ссылкой.
	Scheme string
	// InArray — имя массива документа, из которого пришёл элемент.
	InArray string
}

// ParseURI разбирает ссылку планом схемы.
//
// Схему выбирает ВЫЗЫВАЮЩИЙ (по detect секций либо по написанию), потому что
// выбор плана — уровень документа, а не таблицы. Движок только исполняет.
func ParseURI(plan *Plan, text, bodyType string, trace *Trace) (*Result, error) {
	space, form, err := UnwrapURI(plan, text)
	if err != nil {
		return nil, err
	}
	return Exec(plan, space, form, bodyType, trace)
}

// UnwrapURI выбирает форму и распаковывает текст в пространство источников.
func UnwrapURI(plan *Plan, text string) (*Space, registry.Form, error) {
	if plan == nil || plan.Mapper == nil {
		return nil, registry.Form{}, fmt.Errorf("linkmap: план не задан")
	}
	form, sel := SelectForm(plan.Mapper, NewContent(text))
	if sel.Index < 0 {
		return nil, registry.Form{}, fmt.Errorf("linkmap: форма не распознана")
	}

	body := text
	for i, raw := range form.Decode {
		if i >= maxDecodeDepth {
			return nil, form, fmt.Errorf("linkmap: превышена глубина декодирования")
		}
		next, err := applyDecoder(raw, body)
		if err != nil {
			return nil, form, err
		}
		body = next
	}

	space, err := lexURI(body)
	if err != nil {
		return nil, form, err
	}
	buildOverlays(plan.Mapper.Overlays, space)
	return space, form, nil
}

// buildOverlays распаковывает наложенные пространства, объявленные секцией.
//
// Битый слой — НЕ отказ разбора: подписки шлют полуобрезанный JSON, и терять
// из-за него весь узел нельзя (плоский слой остаётся рабочим). Слой просто не
// появляется, и записи читают то, что нашли в основном пространстве.
func buildOverlays(specs []registry.Overlay, space *Space) {
	for i := range specs {
		spec := &specs[i]
		if spec.Name == "" {
			continue
		}
		raw := ""
		for _, name := range spec.Source.All() {
			if v, ok := space.Lookup(name); ok && strings.TrimSpace(v) != "" {
				raw = strings.TrimSpace(v)
				break
			}
		}
		if raw == "" {
			continue
		}
		for _, dec := range spec.Decode {
			switch dec {
			case "base64?":
				if s, err := decodeBase64Any(raw); err == nil {
					raw = s
				}
			case "percent":
				// Значение уже percent-декодировано лексером один раз;
				// второй проход нужен панелям, кодирующим слой дважды.
				if !strings.HasPrefix(raw, "{") {
					if s, err := percentUnescape(raw); err == nil {
						raw = s
					}
				}
			}
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &obj); err != nil {
			continue
		}
		flat := map[string]string{}
		for k, v := range obj {
			if nested, ok := v.(map[string]interface{}); ok {
				if containsFold(spec.Flatten, k) {
					for nk, nv := range nested {
						if s := overlayScalar(nv); s != "" {
							flat[nk] = s
						}
					}
				}
				continue
			}
			if s := overlayScalar(v); s != "" {
				flat[k] = s
			}
		}
		space.SetOverlay(spec.Name, flat)
	}
}

// overlayScalar приводит значение слоя к строке.
//
// Числа печатаются БЕЗ экспоненты и без хвоста `.0`: слой приезжает JSON'ом, и
// `30.0` там означает то же, что `30`, а `1e+06` в теле — мусор.
func overlayScalar(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return formatNumber(t)
	}
	return ""
}

// applyDecoder применяет один декодер конвейера формы.
//
// Имена декодеров закрытые и общие для всех схем: url, percent, base64,
// base64url, base64?, json, ini. "base64?" — «попробовать, а не вышло —
// оставить как есть» (живые подписки шлют обе формы под одной схемой).
func applyDecoder(raw json.RawMessage, body string) (string, error) {
	name := rawString(raw)
	if name == "" {
		// Объектная форма {"reparse": "url"} — смена пространства, а не
		// декодирование текста; на этом шаге она ничего не меняет.
		return body, nil
	}
	switch name {
	case "url", "percent", "json", "ini":
		// url — не декодер текста, а объявление пространства: разбор делает
		// лексер ниже. json/ini ставятся формой у не-ссылочных видов входа и
		// сюда не приходят.
		return body, nil
	case "base64", "base64url":
		dec, err := decodeBase64Any(body)
		if err != nil {
			return "", fmt.Errorf("linkmap: base64: %w", err)
		}
		return dec, nil
	case "base64?":
		if dec, err := decodeBase64Any(body); err == nil {
			return dec, nil
		}
		return body, nil
	}
	return body, nil
}

// decodeBase64Any пробует четыре варианта base64 (std/url × с padding и без).
func decodeBase64Any(s string) (string, error) {
	s = strings.TrimSpace(s)
	encs := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	for _, enc := range encs {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("не base64")
}

// lexURI — СВОЙ лексер ссылки (MAPPER_ENGINE.md §3).
//
// net/url здесь не годится: он отвергает multi-port authority целиком, а
// `+` в query превращает в пробел ДО того, как станет известно, какое это
// поле. Разбор идёт руками, значения percent-декодируются один раз, `+`
// доезжает до записи сырым.
func lexURI(text string) (*Space, error) {
	s := &Space{}
	rest := strings.TrimSpace(text)

	// scheme:// — обязателен: без него это не ссылка.
	idx := strings.Index(rest, "://")
	if idx < 0 {
		return nil, fmt.Errorf("linkmap: в ссылке нет схемы")
	}
	s.Scheme = strings.ToLower(rest[:idx])
	rest = rest[idx+len("://"):]

	// fragment — по ПЕРВОМУ '#' после authority: '#' внутри метки уже не
	// разделитель.
	if i := strings.Index(rest, "#"); i >= 0 {
		frag := rest[i+1:]
		rest = rest[:i]
		// Метку percent-декодируем path-семантикой: '+' в имени узла
		// литерален ("A+B" — живое имя, не "A B").
		if dec, err := percentUnescape(frag); err == nil {
			frag = dec
		}
		s.Fragment = frag
	}

	// query — по первому '?'.
	if i := strings.Index(rest, "?"); i >= 0 {
		s.SetQuery(ParseQueryOrdered(rest[i+1:]))
		rest = rest[:i]
	}

	// path — по первому '/' ПОСЛЕ authority.
	authority := rest
	if i := strings.Index(rest, "/"); i >= 0 {
		authority = rest[:i]
		p := rest[i:]
		if dec, err := percentUnescape(p); err == nil {
			p = dec
		}
		s.Path = p
	}

	s.Authority = authority
	userinfo, host, portRaw := SplitAuthority(authority)
	if dec, err := percentUnescape(userinfo); err == nil {
		userinfo = dec
	}
	s.UserInfo = userinfo
	if i := strings.Index(userinfo, ":"); i >= 0 {
		s.UserName = userinfo[:i]
		s.UserPass = userinfo[i+1:]
	} else {
		s.UserName = userinfo
	}
	if dec, err := percentUnescape(host); err == nil {
		host = dec
	}
	// IPv6 в скобках: тело адреса без скобок — так его ждёт ядро.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	s.Host = host
	s.PortRaw = portRaw
	s.Port = PortOf(portRaw)
	return s, nil
}
