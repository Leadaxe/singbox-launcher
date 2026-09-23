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
	return ParseURIHint(plan, text, bodyType, "", trace)
}

// ParseURIHint — то же с ИМЕНЕМ ОТ ВЫЗЫВАЮЩЕГО (источник `hint`).
//
// Имя, которого во входе нет, но которое знает вызывающий: описание
// профиля `vpn://`, имя контейнера, имя файла. Куда его поставить в
// цепочке метки, решает секция своим `label.source`, а не вызывающий.
func ParseURIHint(plan *Plan, text, bodyType, hint string, trace *Trace) (*Result, error) {
	space, form, err := UnwrapURI(plan, text)
	if err != nil {
		return nil, err
	}
	space.Hint = hint
	return Exec(plan, space, form, bodyType, trace)
}

// UnwrapURI выбирает форму и распаковывает текст в пространство источников.
//
// Сначала конвейер `decode` формы, потом её `detect` — и предикат
// проверяется по распакованному телу, а если не сошёлся, то по исходному
// тексту (подробнее — у самой проверки ниже).
//
// По ОДНОМУ ЛИШЬ исходному тексту решать нельзя там, где формы различает
// пейлоад: у vmess обе формы — «base64 на authority», и что под ним лежит —
// объект JSON v2rayN или cleartext `method:uuid@host:port` — видно только
// после декодирования. Ровно так решает и прежний путь: декодирует, пробует
// json.Unmarshal и откатывается в cleartext (node_parser_vmess.go:78-82).
//
// Порядок объявления форм НОРМАТИВЕН: первая совпавшая и берётся, ветка
// `default` — последняя. Поэтому «JSON пробуется первым, cleartext —
// откат» выражается порядком, а не приоритетом.
func UnwrapURI(plan *Plan, text string) (*Space, registry.Form, error) {
	if plan == nil || plan.Mapper == nil {
		return nil, registry.Form{}, fmt.Errorf("linkmap: план не задан")
	}
	forms := plan.Mapper.Forms
	if len(forms) == 0 {
		return nil, registry.Form{}, rejectUnrecognized(fmt.Errorf("linkmap: форма не распознана"))
	}

	fallback := -1
	var firstErr error
	for i := range forms {
		form := forms[i]
		if form.Detect != nil && form.Detect.Default {
			if fallback < 0 {
				fallback = i
			}
			continue
		}
		body, err := unwrapBody(form, text)
		if err != nil {
			// Форма не разворачивается — она просто не эта форма; ошибку
			// придержим на случай, если не подойдёт ни одна.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// Предикат проверяется по РАСПАКОВАННОМУ телу, а если не сошёлся —
		// по НЕРАСПАКОВАННОМУ куску, над которым работал декодер.
		//
		// Два вида предикатов описывают разные вещи, и оба законны:
		//
		//   - `json`/`ini` говорят о ПЕЙЛОАДЕ («под оболочкой лежит
		//     объект») — проверить их можно только ПОСЛЕ decode (формы
		//     vmess);
		//   - `regex`/`text` обычно описывают саму ОБОЛОЧКУ («тело — один
		//     base64-блоб, в нём нет @») — такой предикат по построению
		//     ложен на распакованном тексте, где `@` и `/` уже появились
		//     (форма `wrapped` у hysteria2).
		//
		// Второй текст — именно кусок под областью декодера, а не вся
		// ссылка: `^[A-Za-z0-9+/=_-]+$` описывает АЛФАВИТ КОДИРОВКИ, и
		// приклеенное спереди `hysteria2://` ломает его двоеточием и
		// слэшем. Автор такого предиката пишет про блоб, а не про ссылку
		// целиком.
		//
		// Помечать, к чему относится предикат, автор не обязан: форма, чей
		// предикат сошёлся хоть на одном из двух текстов, — эта форма и
		// есть, потому что распаковка у неё своя.
		if form.Detect != nil &&
			!Matches(form.Detect, NewContent(body)) &&
			!Matches(form.Detect, NewContent(decodeSubject(form, text))) {
			continue
		}
		space, err := lexSpace(form, body, text, plan.Mapper.IniDialect, plan.Mapper.Label.CommentRule())
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		buildOverlays(plan.Mapper.Overlays, space)
		return space, form, nil
	}

	if fallback >= 0 {
		form := forms[fallback]
		body, err := unwrapBody(form, text)
		if err != nil {
			return nil, form, rejectUnrecognized(err)
		}
		space, err := lexSpace(form, body, text, plan.Mapper.IniDialect, plan.Mapper.Label.CommentRule())
		if err != nil {
			return nil, form, rejectUnrecognized(err)
		}
		buildOverlays(plan.Mapper.Overlays, space)
		return space, form, nil
	}
	if firstErr != nil {
		return nil, registry.Form{}, rejectUnrecognized(firstErr)
	}
	return nil, registry.Form{}, rejectUnrecognized(fmt.Errorf("linkmap: форма не распознана"))
}

// unwrapBody прогоняет конвейер декодеров одной формы и отдаёт ПЕЙЛОАД —
// то, что будет разбирать `space`.
//
// Декодеры с областью (`scope`) работают над кусками ССЫЛКИ и потому
// возвращают текст, у которого оболочка на месте: `vmess://{"add":…}`.
// Лексеру ссылки она нужна (он по ней и режет), а разборщику пейлоада —
// нет: JSON-объект с приклеенным спереди `vmess://` не JSON ни для
// json.Unmarshal, ни для предиката detect. Поэтому у формы с чужим
// пространством оболочка снимается здесь, один раз, — и detect, и lexSpace
// видят ровно одно и то же тело.
func unwrapBody(form registry.Form, text string) (string, error) {
	body := text
	// scoped — конвейер формы содержал декодер с ОБЛАСТЬЮ (`scope`).
	// Ровно он и порождает оболочку в результате, и ровно он же её снимает
	// ниже: признак структурный, из самой формы.
	scoped := false
	for i, raw := range form.Decode {
		if i >= maxDecodeDepth {
			return "", fmt.Errorf("linkmap: превышена глубина декодирования")
		}
		name, scope := decodeSpec(raw)
		if name == "" {
			// Объектная форма {"reparse": …} — смена пространства, а не
			// декодирование текста.
			continue
		}
		if scope != scopeAll {
			scoped = true
		}
		next, err := decodeScoped(name, scope, body)
		if err != nil {
			return "", err
		}
		body = next
	}
	// Снимать оболочку `схема://…#метка` нужно лишь тому, у кого она ЕСТЬ.
	//
	// Признак СТРУКТУРНЫЙ: оболочку оставляет за собой декодер с областью
	// (`scope`) — он собирает результат обратно как `head + dec + tail`
	// (см. decodeScoped), то есть возвращает пейлоад СНОВА в обёртке
	// вместе с меткой: и у vmess (`vmess://<base64>#Имя`), и у wireguard
	// (`awg://<base64>#Имя`). Голый документ (`.conf` файлом, элемент
	// JSON) через такой декодер не проходит и обёртки не имеет вовсе.
	//
	// Раньше здесь стояло наличие "://" в ТЕКСТЕ — временный признак,
	// переживший появление уровня вида источника: он судил вход вместо
	// формы и потому срабатывал бы на любом теле, где "://" встретилось
	// случайно (URL в комментарии `.conf`, значение внутри JSON).
	//
	// Резать голый ini по '#' было разрушительно: там это законный
	// синтаксис тела (комментарий, и им же провайдеры пишут имя узла сразу
	// под `[Peer]`). Файл с «# US-FREE#137» терял всё, что стояло ниже
	// первого комментария, и отвергался как «обязательное поле
	// peers[].public_key пусто» (Q133-60).
	if form.Space != "" && form.Space != "url" && scoped {
		body = stripURIWrapper(body)
	}
	return body, nil
}

// decodeSubject — НЕраспакованный кусок текста, над которым работает первый
// декодер формы: он же и есть предмет предиката об оболочке.
func decodeSubject(form registry.Form, text string) string {
	for _, raw := range form.Decode {
		name, scope := decodeSpec(raw)
		if name == "" {
			continue
		}
		switch scope {
		case scopeAuthority:
			_, authority, _ := splitAuthorityPart(text)
			if authority != "" {
				return authority
			}
		case scopeUserInfo:
			_, authority, _ := splitAuthorityPart(text)
			if at := strings.LastIndex(authority, "@"); at >= 0 {
				return authority[:at]
			}
		}
		break
	}
	return text
}

// stripURIWrapper снимает `схема://` спереди и `#метку` сзади, оставляя
// пейлоад. Метка — свойство оболочки: у vmess она стоит СНАРУЖИ base64
// (`vmess://<base64>#Имя`), и внутрь пейлоада ей нельзя.
func stripURIWrapper(text string) string {
	s := strings.TrimSpace(text)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+len("://"):]
	}
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// lexSpace превращает распакованный текст в пространство источников ТЕМ
// разборщиком, который назвала форма (`forms[].space`).
//
// Раньше здесь всегда стоял lexURI, и `space` был объявлением без
// исполнителя: форма v2rayN у vmess несёт под base64 не ссылку, а ОБЪЕКТ
// JSON — `{"add": "...", "port": "443", "id": "..."}`, — и лексер ссылки
// видел в нём текст без "://". Записи секции адресуют его `json.add`,
// `json.port`: имена источников у пространства JSON свои, и Lookup их уже
// знает — не хватало только того, кто наполнит.
//
// Метка узла у формы JSON живёт ВНУТРИ объекта (`ps`), а не во фрагменте
// ссылки, но фрагмент бывает и там: `vmess://<base64>#Имя` — поэтому
// фрагмент исходного текста переносится в пространство. Побеждает тот, кого
// запись `label` назовёт первым (у vmess это `json.ps`, фрагмент — запасной).
func lexSpace(form registry.Form, body, original string, dialect *registry.IniDialect, comment *registry.LabelComment) (*Space, error) {
	switch form.Space {
	case "", "url":
		return lexURI(body)
	case "json":
		var v interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &v); err != nil {
			return nil, fmt.Errorf("linkmap: json: %w", err)
		}
		s := &Space{}
		s.SetJSON(v)
		// Скалярные ключи КОНТЕЙНЕРА видны ещё и как `query.<имя>`.
		//
		// Не удобство, а условие переиспользования ОБЩИХ БЛОКОВ. Блоки
		// transports#uri, tls#uri и dialer#uri адресуют поля `query.path`,
		// `query.host`, `query.sni`: у ссылочных форм эти поля и вправду
		// лежат в query. Контейнер v2rayN называет ТЕ ЖЕ поля теми же
		// именами, только ключами объекта, — и без общего пространства
		// vmess пришлось бы либо переписать блоки на «источник по формам»
		// в каждой записи, либо завести им копии. Оба пути расходятся
		// молча, а это ровно то, ради чего затеян один движок.
		//
		// Так же устроен и LxBox (contract_draft/uri/vmess.json: контейнер
		// «раскладывается плоским слоем имён»), и их отступления написаны
		// в расчёте на это. Собственные записи секции продолжают читать
		// `json.<путь>`: вложенные пути плоский слой не выражает, а имя,
		// объявленное обоими способами, берётся записью по её `source`.
		if obj, ok := v.(map[string]interface{}); ok {
			for k, item := range obj {
				if sv := overlayScalar(item); sv != "" {
					s.AddQuery(k, sv)
				}
			}
		}
		// Схема и фрагмент — свойства ОБОЛОЧКИ, а не пейлоада: под base64
		// их нет, а секции они нужны (detect по схеме уже прошёл, но
		// `label.fallback.scheme_source` и фрагмент читаются позже).
		if idx := strings.Index(original, "://"); idx > 0 {
			s.Scheme = strings.ToLower(original[:idx])
		}
		if i := strings.Index(original, "#"); i >= 0 {
			frag := original[i+1:]
			if dec, err := percentUnescape(frag); err == nil {
				frag = dec
			}
			s.Fragment = frag
		}
		return s, nil
	case "ini":
		// wg-quick/.conf: под текстом лежит не ссылка, а ini-документ.
		// Секции адресуют его `ini.<Секция>.<Ключ>`, и Lookup эти имена уже
		// знает — не хватало только того, кто наполнит пространство.
		sections, dropped, ok := parseINIDialect(body, dialect)
		if !ok {
			return nil, fmt.Errorf("linkmap: ini: секций нет")
		}
		s := &Space{}
		s.SetINI(sections, parseINIComments(body, dialect, comment))
		s.iniDropped = dropped
		// Схема и фрагмент — свойства ОБОЛОЧКИ, как и у формы JSON: под
		// base64 их нет, а `label` и `scheme_source` читают их позже.
		// У формы `.conf` (голый файл, без "://") оболочки нет вовсе, и оба
		// имени остаются пустыми — метку тогда даёт цепочка `ini.$comment`
		// и `hint`.
		// Оболочка есть только у формы СО ссылкой (`awg://<base64>#Имя`).
		// У голого файла её нет, и там первый '#' — комментарий тела, а не
		// метка: взять его хвост фрагментом значило бы объявить меткой узла
		// весь остаток файла. Признак тот же, что и у снятия обёртки выше, —
		// наличие "://"; имя узла у голого `.conf` даёт цепочка
		// `ini.$comment.<Секция>`.
		if idx := strings.Index(original, "://"); idx > 0 {
			s.Scheme = strings.ToLower(original[:idx])
			if i := strings.Index(original, "#"); i >= 0 {
				frag := original[i+1:]
				if dec, err := percentUnescape(frag); err == nil {
					frag = dec
				}
				s.Fragment = frag
			}
		}
		return s, nil
	}
	return nil, fmt.Errorf("linkmap: неизвестное пространство %q", form.Space)
}

// parseINIComments — ПЕРВЫЙ комментарий каждой секции, то есть имя узла,
// которое провайдеры пишут сразу под заголовком (`[Peer]` / `# CH-FREE#11`).
//
// Что именем НЕ считается, объявляет метка секции (`label.comment`,
// PRIMITIVES §0.8): у wg-quick `require_no: "="` — `# Bouncing = 0` это
// отключённая настройка, а не название. Своего умолчания у движка нет: без
// правила годится первый непустой комментарий. Сам '#' внутри значения
// законен («US-FREE#137»), поэтому режется только ведущий маркер. Диалект
// тот же, что у parseINI: комментарий — целая строка, начинающаяся с '#'
// или ';'.
func parseINIComments(text string, d *registry.IniDialect, rule *registry.LabelComment) map[string]string {
	prefixes := d.Prefixes()
	out := map[string]string{}
	section := ""
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		if section == "" {
			continue
		}
		if !hasAnyPrefix(line, prefixes) {
			// Дошли до настоящего поля — имени в этой секции нет. Отметка
			// «искать больше нечего» ставится пустой строкой, иначе
			// комментарий, стоящий НИЖЕ полей, стал бы именем узла.
			if _, seen := out[section]; !seen {
				out[section] = ""
			}
			continue
		}
		if _, seen := out[section]; seen {
			continue
		}
		name := strings.TrimSpace(strings.TrimLeft(line, strings.Join(prefixes, "")))
		if name == "" || !rule.Accepts(name) {
			continue
		}
		out[section] = name
	}
	return out
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
		// Слой приезжает ДВУМЯ формами, и обе живые.
		//
		// ТЕКСТОМ — у ссылки: `extra` есть query-параметр с JSON внутри
		// (vless, форки Xray), и его надо разобрать самим.
		//
		// УЖЕ ОБЪЕКТОМ — у контейнера: vmess-ссылка сама есть JSON, и
		// Marzban кладёт `extra` его ВЛОЖЕННЫМ объектом
		// (`payload["extra"] = extra`, app/subscription/v2ray.py:249).
		// Такое значение до слоя не доезжало вовсе: `Lookup` ведёт объект
		// через `jsonScalar`, а тот на объекте молчит намеренно (§4), и
		// xmux с sc*-полями vmess-узла терялись МОЛЧА — тот же вход у vless
		// разбирался полностью (D-7 аудита панелей).
		var obj map[string]interface{}
		raw := ""
		for _, name := range spec.Source.All() {
			if v, ok := space.LookupRaw(name); ok {
				if m, isObj := v.(map[string]interface{}); isObj {
					obj = m
					break
				}
			}
			if v, ok := space.Lookup(name); ok && strings.TrimSpace(v) != "" {
				raw = strings.TrimSpace(v)
				break
			}
		}
		if obj == nil && raw == "" {
			continue
		}
		// Декодеры объявлены для ТЕКСТОВОЙ формы: объект уже разобран, и
		// применять к нему percent/base64 нечего.
		if obj == nil {
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
			if err := json.Unmarshal([]byte(raw), &obj); err != nil {
				continue
			}
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
// decodeSpec читает один шаг конвейера формы: имя декодера и область его
// применения (PRIMITIVES §0.10).
//
// Две записи: строка ("base64") — область `all`, прежнее поведение; объект
// {"decoder": "base64", "scope": "authority"} — область названа явно. Объект
// {"reparse": …} декодером не является и даёт пустое имя.
func decodeSpec(raw json.RawMessage) (name, scope string) {
	if s := rawString(raw); s != "" {
		return s, scopeAll
	}
	var obj struct {
		Decoder string `json:"decoder"`
		Scope   string `json:"scope"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil || obj.Decoder == "" {
		return "", ""
	}
	if obj.Scope == "" {
		obj.Scope = scopeAll
	}
	return obj.Decoder, obj.Scope
}

// Области применения декодера формы.
const (
	scopeAll       = "all"
	scopeUserInfo  = "userinfo"
	scopeAuthority = "authority"
)

// decodeScoped применяет декодер к НАЗВАННОЙ части ссылки.
//
// Нужно потому, что base64 у ss накрывает разные куски в разных формах:
// SIP002 кодирует только userinfo, legacy — весь authority, а метка `#…` в
// обеих формах остаётся открытым текстом СНАРУЖИ. Декодер «на весь текст»
// ломает обе: в первой он спотыкается об открытый адрес, во второй — о метку.
//
// Части режутся и собираются обратно в ТЕКСТ, а не в пространство: лексер
// остаётся единственным местом, где ссылка превращается в источники.
func decodeScoped(name, scope, text string) (string, error) {
	if name == "url" {
		// url — не декодер текста, а объявление пространства.
		return text, nil
	}
	if scope == scopeAll || scope == "" {
		return decodeNamed(name, text)
	}

	head, authority, tail := splitAuthorityPart(text)
	if authority == "" {
		return text, nil
	}
	target := authority
	prefix := ""
	if scope == scopeUserInfo {
		at := strings.LastIndex(authority, "@")
		if at < 0 {
			// Формы без userinfo декодировать нечего — не отказ разбора.
			return text, nil
		}
		target, prefix = authority[:at], authority[at:]
	}
	dec, err := decodeNamed(name, target)
	if err != nil {
		return "", err
	}
	if scope == scopeUserInfo {
		return head + dec + prefix + tail, nil
	}
	return head + dec + tail, nil
}

// splitAuthorityPart режет текст ссылки на «до authority», authority и
// «после» (путь, query, фрагмент). Фрагмент отрезается ПЕРВЫМ: в legacy-форме
// ss метка стоит снаружи base64, и утащить её под декодер нельзя.
func splitAuthorityPart(text string) (head, authority, tail string) {
	rest := strings.TrimSpace(text)
	idx := strings.Index(rest, "://")
	if idx < 0 {
		return "", "", ""
	}
	head, rest = rest[:idx+len("://")], rest[idx+len("://"):]

	// Режется ТОЛЬКО по '#'.
	//
	// Прежде резалось ещё по '?' и '/' — по синтаксису ссылки, которой
	// здесь ещё нет: под base64 лежит НЕРАЗОБРАННЫЙ блоб, а '/' и '+' —
	// законные символы 63-го и 62-го значений стандартного алфавита
	// (RFC 4648 §4), как и '=' в паддинге. Блоб legacy-формы vmess
	// (`method:uuid@host:port?type=ws&path=%2Fws`) содержит '/' в теле
	// кодировки: разрез отдавал декодеру первую половину, base64 её
	// доедал без ошибки — и query узла терялся МОЛЧА, вместе с
	// транспортом и TLS (корпус legacy_cleartext_userinfo).
	//
	// '#' резать обязательно и безопасно: метка стоит СНАРУЖИ base64
	// открытым текстом в обеих формах, а в алфавит кодировки не входит.
	cut := len(rest)
	if i := strings.Index(rest, "#"); i >= 0 {
		cut = i
	}
	return head, rest[:cut], rest[cut:]
}

// decodeNamed применяет ОДИН именованный декодер к куску текста.
//
// Общий для конвейера формы (весь текст) и конвейера userinfo: имя декодера
// значит одно и то же, на какую бы часть ссылки его ни навели. Область
// применения — свойство объявления (`forms[].decode` против
// `userinfo.decode`), а не самого декодера.
func decodeNamed(name, body string) (string, error) {
	switch name {
	case "url", "json", "ini":
		// Объявления ПРОСТРАНСТВА, а не декодеры текста: разбор ведёт
		// соответствующий разборщик, текст на этом шаге не меняется.
		return body, nil
	case "percent":
		// В отличие от конвейера формы, здесь percent работает: лексер
		// декодирует части ссылки один раз, а панели экранируют '='-паддинг
		// base64 как %3D — второй проход нужен ДО base64.
		if dec, err := percentUnescape(body); err == nil {
			return dec, nil
		}
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

	// path — по первому '/' ПОСЛЕ userinfo, а не после "://".
	//
	// Искать с начала нельзя: в userinfo законно лежит сырой '/'. У masque
	// это base64(SEC1 DER) приватного ключа, и слэш в нём — 63-е значение
	// алфавита; прежний разрез уводил половину ключа в путь, а остаток
	// становился ИМЕНЕМ ХОСТА (корпус masque/*, все восемь кейсов). Чинить
	// это percent-кодированием ДО разбора, как делал рукописный путь
	// (percentEncodeWGUserinfoSlashes), движку незачем: он режет сам и
	// знает, где кончается userinfo.
	//
	// Граница — ПОСЛЕДНИЙ '@': '@' законен и внутри userinfo (пароль с
	// собакой), а вот после authority его уже быть не может — query и
	// fragment отрезаны выше.
	authority := rest
	from := 0
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		from = at + 1
	}
	if i := strings.Index(rest[from:], "/"); i >= 0 {
		cut := from + i
		authority = rest[:cut]
		p := rest[cut:]
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
