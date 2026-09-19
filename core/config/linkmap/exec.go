package linkmap

// Исполнитель таблицы маппера (SPEC 133, W1).
//
// Один движок на все схемы и все виды источника. В пакете НЕТ ни одного имени
// схемы, ни одного `if scheme == …`: всё, чем он оперирует, приезжает из
// реестра. Греп-страж — engine_no_scheme_names_test.go.
//
// Нормативный порядок исполнения — MAPPER_ENGINE.md §7: проход A (селекторы),
// проход B (остальные), внутри прохода — порядок объявления.
//
// go1.20-совместимо (Win7-джоба собирает весь модуль тулчейном go1.20): без
// slices/maps/min/max/clear.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// Result — итог исполнения секции над одним элементом входа.
type Result struct {
	// Body — тело узла (карта sing-box), без служебных ключей tag/type.
	Body map[string]interface{}
	// Label — метка узла до фолбэка; пустая, если во входе метки не было.
	Label string
	// LabelFallback — шаблон фолбэка метки, если метки не нашлось.
	LabelFallback string
	// BodySource — как объявила секция (uri/xray/singbox/wgconf/amnezia).
	BodySource string
	// Kind — РОД узла, объявленный секцией через `kind_when` по условию на
	// ВХОД (у wireguard: "awg"/"awg3"). Пустая строка = род не объявлен.
	//
	// В тело не пишется: род нужен ПРАВИЛАМ ЗНАЧЕНИЯ санитайзера, которые
	// читают его оператором `when.source_kind`. Вызывающий обязан донести
	// его до Sanitize и сохранить рядом с телом там, где источник не
	// хранится (contract/docs/MAPPER_ENGINE.md).
	Kind string
	// FormID — форма, которую выбрал разбор. Нужна вызывающему там, где
	// свойство узла зависит от ФОРМЫ, а не от тела: у vmess userinfo есть
	// только у cleartext-формы, а у контейнера v2rayN его нет вовсе.
	FormID string
	// HadUserInfo — нёс ли вход userinfo. Отличает «userinfo пуст» от
	// «userinfo в этой форме не предусмотрен».
	HadUserInfo bool
	// Query — параметры РАСПАКОВАННОГО входа, как справка вызывающему
	// (skip-фильтры, UI). Не вход разбора: его движок читает из
	// пространства сам.
	Query url.Values
	// Notes — коды, поставленные движком (uri_param_unknown и
	// объявленные записями on_*). Порядок = порядок появления.
	Notes []Note
	// Trace — трасса, если она была включена.
	Trace *Trace
}

// resolveKind — РОД узла по условию на ВХОД (`kind_when` секции).
//
// Судит ИСКЛЮЧИТЕЛЬНО пространство источников, а не тело: к моменту, когда
// записи снимут негодные значения, в теле может не остаться ни одного
// признака рода — ссылка `awg://` с битыми awg-параметрами оставляет тело,
// неотличимое от обычного WireGuard. Ровно ради этого случая примитив и
// заведён (contract/docs/MAPPER_ENGINE.md).
//
// Имена родов перебираются в АЛФАВИТНОМ порядке, побеждает первый
// подошедший: порядок ключей карты в Go не определён, а род входит в
// поведение правил, и оставлять его на волю обхода нельзя.
//
// Поддержаны две формы условия:
//
//	{"any_set": ["query.jc", …]}          — задан любой из источников;
//	{"query.keepalive": {"matches": "-"}} — источник содержит подстроку.
func resolveKind(spec map[string]map[string]interface{}, space *Space) string {
	if len(spec) == 0 || space == nil {
		return ""
	}
	names := make([]string, 0, len(spec))
	for name := range spec {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if kindCondHolds(spec[name], space) {
			return name
		}
	}
	return ""
}

// kindCondHolds — одно условие рода. Служебные ключи с ведущим `$` (`$impl`)
// пропускаются: это заметки автора секции, а не предикаты.
func kindCondHolds(cond map[string]interface{}, space *Space) bool {
	matched := false
	for key, want := range cond {
		if strings.HasPrefix(key, "$") {
			continue
		}
		if key == "any_set" {
			list, _ := want.([]interface{})
			hit := false
			for _, item := range list {
				name, _ := item.(string)
				if name == "" {
					continue
				}
				// Считается НАЛИЧИЕ источника, а не непустота значения:
				// `jc=0` — законное «мусор выключен» у настоящего
				// AmneziaWG-узла, и прочесть его как «поля нет» значило бы
				// снять с узла потолок MTU.
				if _, ok := space.Lookup(name); ok {
					hit = true
					break
				}
			}
			if !hit {
				return false
			}
			matched = true
			continue
		}
		v, ok := space.Lookup(key)
		if !ok {
			return false
		}
		spec, _ := want.(map[string]interface{})
		sub, _ := spec["matches"].(string)
		if sub == "" || !strings.Contains(v, sub) {
			return false
		}
		matched = true
	}
	return matched
}

// Note — код с параметрами; в узел их перекладывает вызывающий, потому что
// формат warning'а принадлежит подписке, а не движку.
type Note struct {
	Code string
	// Path — ЧЕГО именно касается код, если код касается одного имени
	// источника (параметра ссылки, ключа `.conf`, поля JSON-элемента).
	//
	// Нужен не для красоты: деградации узла дедуплицируются парой
	// (код, path) — `ParsedNode.addWarning`. Пока путь был пуст, ссылка с
	// двумя незнакомыми параметрами получала ОДИН `uri_param_unknown`, и
	// человек узнавал про первый из них, а второй исчезал молча — ровно
	// та потеря без слов, против которой код и заведён.
	Path   string
	Params map[string]string
}

// execState — рабочее состояние одного исполнения.
type execState struct {
	plan  *Plan
	space *Space
	form  registry.Form
	res   *Result
	trace *Trace

	// writtenBy — какая запись заняла путь тела: нужно разрешению конфликта
	// (priority, затем порядок объявления) и трассе.
	writtenBy map[string]*writeMark
	// bodyType — тип тела (для maps_to по типу и when.$type).
	bodyType string
	// mapperName — "<схема>.<вид>[.<форма>]" для трассы.
	mapperName string
	// schemeVals — служебные ключи `scheme_sets`, начинающиеся с `$`: они НЕ
	// пишутся в тело, а питают записи таблицы (сегодня — `$default_port`,
	// PRIMITIVES §0.10). Дефолт порта бывает свойством НАПИСАНИЯ, а не схемы:
	// у одной схемы `http` написание `proxy-http` даёт 80, `proxy-https` — 443.
	schemeVals map[string]interface{}
}

// schemeValDefaultPort — имя служебного ключа `scheme_sets`, задающего порт по
// умолчанию для записи с `materialize_default`.
const schemeValDefaultPort = "$default_port"

type writeMark struct {
	entry    string
	priority int
	decl     int
}

// Exec исполняет план над распакованным пространством источников.
//
// bodyType — тип тела узла (singbox_type схемы): движок сам его не выводит,
// потому что «как называется тип» — свойство реестра, а не таблицы.
func Exec(plan *Plan, space *Space, form registry.Form, bodyType string, trace *Trace) (*Result, error) {
	if plan == nil || plan.Mapper == nil {
		return nil, fmt.Errorf("linkmap: план не задан")
	}
	st := &execState{
		plan:       plan,
		space:      space,
		form:       form,
		trace:      trace,
		writtenBy:  map[string]*writeMark{},
		schemeVals: map[string]interface{}{},
		bodyType:   bodyType,
		res: &Result{
			Body:        map[string]interface{}{},
			BodySource:  plan.Mapper.BodySource,
			FormID:      form.ID,
			HadUserInfo: space != nil && space.UserInfo != "",
			Query:       space.QueryValues(),
			Trace:       trace,
		},
	}
	st.mapperName = plan.Mapper.Scheme() + "." + plan.Mapper.Kind()
	if form.ID != "" {
		st.mapperName += "." + form.ID
	}

	// Род узла судится по ВХОДУ и потому раньше всякой таблицы: к моменту,
	// когда записи начнут снимать негодные значения, судить уже не по чему —
	// в этом весь смысл примитива.
	st.res.Kind = resolveKind(plan.Mapper.KindWhen, space)

	// scheme_sets — написание схемы задаёт присваивания (socks4/socks4a).
	st.applySchemeSets()

	// userinfo — до таблицы: её записи могут ссылаться на userinfo.* как на
	// источник, а метка из userinfo не берётся вовсе.
	st.applyUserInfo()

	// Проход A: селекторы строят тело, по которому дальше проверяется when.
	for i := range plan.Selectors {
		st.applyEntry(&plan.Selectors[i])
	}
	// Проход B: остальные записи.
	for i := range plan.Rest {
		st.applyEntry(&plan.Rest[i])
	}

	// defaults секции — то, чего не написал никто.
	st.applyDefaults()

	st.applyLabel()
	st.noteUnknownParams()
	st.noteINIDropped()

	// required — ПОСЛЕ обоих проходов и defaults: запись объявлена
	// обязательной в теле, а не во входе, и значение туда законно приходит
	// от materialize_default или defaults секции, а не только из источника.
	if err := st.checkRequired(); err != nil {
		return nil, err
	}

	if trace != nil {
		trace.ResultEvent(st.mapperName, st.res.Body, st.res.Label, st.res.BodySource)
	}
	return st.res, nil
}

// checkRequired отвергает узел, у которого пуст путь обязательной записи.
//
// Отказ разбора, а не код деградации: ссылка без пароля у anytls или без
// адреса — не узел, и отдавать её дальше значит отдать санитайзеру заведомый
// мусор. Прежний путь отвергал такие ссылки перед разбором, поимённо
// перечисляя схемы (node_parser_core.go); здесь это свойство ЗАПИСИ.
//
// ТЕКСТ отказа берётся из `desc_en` записи, когда она его объявила.
// Причина едет ЧЕЛОВЕКУ, и «обязательное поле "uuid" пусто» отвечает не на
// его вопрос: у пустого id в Xray-подписке смысл вполне определённый —
// сервер отдал заглушку, подписка скорее всего протухла, — и прежний
// конвертер это говорил (`xrayReasonEmptyUserID`). Держать такую фразу в
// коде движка нельзя (она про диалект), выдумывать под неё новый атрибут
// незачем: `desc_en` уже есть у каждой записи и означает ровно «что это
// поле значит человеку». Нет `desc_en` — остаётся прежний текст с путём.
func (st *execState) checkRequired() error {
	// userinfo целиком: поля, которые он наполняет, приходят ПОЗИЦИЯМИ into,
	// и записи под ними у части схем нет вовсе (uuid у vless объявлен null).
	// Проверять приходится сам userinfo, а не путь тела.
	if ui := st.plan.Mapper.UserInfo; ui != nil && ui.Required && st.space.UserInfo == "" {
		return fmt.Errorf("linkmap: ссылка без userinfo")
	}
	for _, list := range [][]Entry{st.plan.Selectors, st.plan.Rest} {
		for i := range list {
			e := &list[i]
			if e.Param == nil || !e.Param.Required {
				continue
			}
			// Запись без своего maps_to всё равно бывает обязательной:
			// `split_into` раскладывает ОДИН источник по нескольким путям
			// тела, и «обязателен» значит «хоть один из них заполнен». У
			// masque это `address` → ip/ipv6: ссылка без единого годного
			// адреса не узел, и корпус ждёт parse_error
			// (missing_address_rejected).
			if st.pathOf(e.Param) == "" && len(e.Param.SplitInto) > 0 {
				filled := false
				for p := range e.Param.SplitInto {
					if v, ok := getPath(st.res.Body, p); ok && toString(v) != "" {
						filled = true
						break
					}
				}
				if !filled {
					return fmt.Errorf("linkmap: обязательная запись %q не заполнила ни одного пути", e.Name)
				}
				continue
			}
			// То же и у записи, раскладывающей значение ВЫЕМКОЙ: своего
			// `maps_to` у неё нет, пути называет `extract.into`, и
			// «обязательна» значит «хоть один из них заполнен».
			//
			// Без этой ветки `required` у такой записи не проверялся ВООБЩЕ:
			// `pathOf` отдавал пустую строку, и проверка молча пропускалась.
			// Ровно так блок `.conf` с `[Peer]` без `Endpoint` становился
			// «узлом» с пиром без адреса и порта — узлом, который никуда не
			// соединяется (регрессия поймана TestWGConfBrokenBlockBecomes-
			// UnsupportedRecord на переводе боевого пути, SPEC 133).
			if st.pathOf(e.Param) == "" && e.Param.Extract != nil {
				filled := false
				for _, spec := range e.Param.Extract.Into {
					p := ""
					switch t := spec.(type) {
					case string:
						p = t
					case map[string]interface{}:
						p, _ = t["path"].(string)
					}
					if p == "" || strings.HasPrefix(p, "$") {
						continue
					}
					if v, ok := getPath(st.res.Body, p); ok && !isEmptyValue(v) {
						filled = true
						break
					}
				}
				if !filled {
					if reason := strings.TrimSpace(e.Param.DescEN); reason != "" {
						return fmt.Errorf("%s", reason)
					}
					return fmt.Errorf("linkmap: обязательная запись %q не заполнила ни одного пути", e.Name)
				}
				continue
			}
			path := st.pathOf(e.Param)
			if path == "" {
				continue
			}
			v, ok := getPath(st.res.Body, path)
			if !ok || isEmptyValue(v) {
				if reason := strings.TrimSpace(e.Param.DescEN); reason != "" {
					return fmt.Errorf("%s", reason)
				}
				return fmt.Errorf("linkmap: обязательное поле %q пусто", path)
			}
		}
	}
	return nil
}

// applySchemeSets — присваивания по написанию схемы.
func (st *execState) applySchemeSets() {
	sets := st.plan.Mapper.SchemeSets
	if len(sets) == 0 {
		return
	}
	assigns, ok := sets[st.space.Scheme]
	if !ok {
		// "*" — присваивания, общие для ВСЕХ написаний схемы. Нужны там, где
		// свойство принадлежит протоколу, а не написанию: у hysteria2 TLS
		// включён всегда (транспорт QUIC, выключить его нельзя), и параметра
		// `security` в ссылке нет вовсе. Выразить это записью нечем —
		// источника у неё не будет, — а `defaults` секции пишет ТЕЛО узла
		// поверх всего и потому не годится для структурного признака.
		assigns, ok = sets["*"]
		if !ok {
			return
		}
	}
	// Служебные ключи (`$…`) отделяются ДО присваивания: в тело они не едут,
	// их читают записи таблицы. Иначе `$default_port` уезжал литеральным
	// ключом тела, и узел терял `server_port` (Q133-44).
	//
	// Карта присваиваний принадлежит РЕЕСТРУ и общая для всех узлов — правится
	// не она, а её копия.
	body := make(map[string]interface{}, len(assigns))
	for k, v := range assigns {
		if strings.HasPrefix(k, "$") {
			st.schemeVals[k] = v
			continue
		}
		body[k] = v
	}
	st.applyAssigns("$scheme_sets", body, 0, 0, "", "")
}

// applyUserInfo раскладывает userinfo по объявленным полям.
//
// Резка по разделителю — только когда секция её объявила: у trojan пароль это
// ВЕСЬ userinfo (D133-8), и резка по ':' молча теряла пароли с двоеточием.
func (st *execState) applyUserInfo() {
	ui := st.plan.Mapper.UserInfo
	if ui == nil {
		return
	}
	raw := st.space.UserInfo
	if raw == "" {
		return
	}
	// Конвейер декодеров САМОГО userinfo — область применения уже ATOM
	// грамматики (`userinfo.decode`), и форме ничего добавлять не нужно:
	// SIP002 кодирует в base64 именно userinfo, оставляя адрес и метку
	// открытыми. Порядок объявлен секцией: percent ДО base64, потому что
	// панели экранируют '='-паддинг как %3D.
	for i, dec := range ui.Decode {
		if i >= maxDecodeDepth {
			break
		}
		next, err := decodeNamed(dec, raw)
		if err != nil {
			// Объявленный декодер не сработал — userinfo остаётся как есть.
			// Отказ разбора здесь был бы неверен: `base64?` у ss означает
			// «попробовать», и открытый SS2022 `method:key@host` законен.
			break
		}
		raw = next
	}
	st.space.UserInfo = raw
	if i := strings.Index(raw, ":"); i >= 0 {
		st.space.UserName, st.space.UserPass = raw[:i], raw[i+1:]
	} else {
		st.space.UserName, st.space.UserPass = raw, ""
	}

	parts := []string{raw}
	if ui.Split != nil && ui.Split.Sep != "" {
		limit := ui.Split.Limit
		if limit <= 0 {
			limit = -1
		}
		parts = strings.SplitN(raw, ui.Split.Sep, limit)
	}
	targets := ui.Into
	if len(targets) == 0 && ui.SingleInto != "" {
		targets = []string{ui.SingleInto}
	}
	// Одиночный userinfo без разделителя при объявленном single_into едет
	// туда, а не в первый into (конвенция naive/hysteria2).
	//
	// Перенаправляется САМО ПРОСТРАНСТВО, а не только цель записи: лексер
	// кладёт беспарный userinfo в `userinfo.user`, и запись `username`, читая
	// свой источник напрямую, получала бы тот же текст — узел выходил бы и с
	// username, и с password (Q133-43). Источник обязан быть одной истиной
	// для всех записей.
	if ui.SingleInto != "" && len(parts) == 1 {
		targets = []string{ui.SingleInto}
		// Какой компонент пространства несёт значение, решает ПОЗИЦИЯ цели в
		// `into`, а не её имя: имена полей принадлежат схеме, а userinfo.0 /
		// userinfo.1 — пространству. Цель вне `into` пространство не трогает.
		for i, name := range ui.Into {
			if name != ui.SingleInto {
				continue
			}
			st.space.UserName, st.space.UserPass = "", ""
			if i == 0 {
				st.space.UserName = raw
			} else {
				st.space.UserPass = raw
			}
			break
		}
	}
	for i, target := range targets {
		if i >= len(parts) || target == "" {
			continue
		}
		v := parts[i]
		if v == "" {
			continue
		}
		src := "userinfo"
		if len(targets) > 1 {
			src = "userinfo." + strconv.Itoa(i)
		}
		st.write("$userinfo", src, v, v, target, 0, 0, "")
	}
}

// applyDefaults пишет объявленные секцией значения по умолчанию туда, где
// путь остался пустым.
func (st *execState) applyDefaults() {
	if len(st.plan.Mapper.Defaults) == 0 {
		return
	}
	keys := make([]string, 0, len(st.plan.Mapper.Defaults))
	for k := range st.plan.Mapper.Defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, path := range keys {
		if isReservedAssignKey(path) {
			continue
		}
		if _, taken := st.writtenBy[path]; taken {
			continue
		}
		if _, exists := getPath(st.res.Body, path); exists {
			continue
		}
		v := st.plan.Mapper.Defaults[path]
		setPath(st.res.Body, path, v)
		st.writtenBy[path] = &writeMark{entry: "$defaults", priority: 1 << 30, decl: 1 << 30}
		st.trace.Add(Event{
			Stage: StageDefault, Mapper: st.mapperName, Entry: "$defaults",
			Src: "-", Raw: nil, Val: v, Path: path, Act: ActWrite, Why: WhyDefault,
		})
	}
}

// applyEntry исполняет одну запись таблицы.
func (st *execState) applyEntry(e *Entry) {
	p := e.Param
	if p == nil {
		return
	}

	// when по телу / по источнику / по $type / по $form.
	if !st.whenHolds(p.When) {
		// Код за ПОДАВЛЕНИЕ значения условием: значение во входе было, но
		// структурное правило не дало ему доехать, и молчать об этом нельзя.
		// Так `uplink_data_placement=header` при несовместимом режиме
		// снимается с кодом xhttp_param_reset — иначе поле исчезало бы тихо.
		//
		// Код ставится только когда источник И ВПРАВДУ что-то дал: запись,
		// чьё условие ложно на узле без этого параметра, ни о чём не говорит.
		if code := codeOf(p.OnWhenFalse); code != "" {
			if _, _, found := st.lookupSource(p); found {
				st.note(code, paramsOf(p.OnWhenFalse))
			}
		}
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
			Src: "-", Raw: nil, Val: nil, Path: nil, Act: ActSkip, Why: WhyWhenFalse,
		})
		return
	}

	rawVal, src, found := st.lookupSource(p)
	if !found {
		st.applyMissing(e)
		return
	}

	// on_present — код за САМО наличие значения, независимо от того, едет оно
	// куда-нибудь или нет. Нужен записям с `maps_to: null`: значение осознанно
	// никуда не переводится, и без кода оно исчезало бы молча.
	if code := codeOf(p.OnPresent); code != "" {
		// `value` подставляется САМИМ значением, если объявление не задало
		// его дословно: текст такого кода про значение и говорит («в секции
		// [Interface] указан DNS {value}»), а объявить его в таблице нельзя
		// — оно приходит со входом. Прочие параметры берутся из объявления
		// как есть.
		params := paramsOf(p.OnPresent)
		if _, has := params["value"]; !has {
			if params == nil {
				params = map[string]string{}
			}
			params["value"] = rawVal
		}
		st.note(code, params)
	}

	// Декодирование поверх декодера формы, потом форм-семантика `+`.
	val := st.decodeValue(p, rawVal)

	// list + extract = СПИСОК ПАР в объект тела (PRIMITIVES §0.10).
	// Перехватывается до convert: иначе list резал значение в срез, а extract
	// применялся к его печати — регулярка ловила первую пару и клала её не
	// туда, а остальные заголовки пропадали (Q133-42).
	if p.List != nil && p.Extract != nil {
		st.applyPairList(e, src, rawVal, val)
		st.applySets(e, val)
		st.applyImplies(e)
		return
	}

	// split_into — ОДИН список входа раскладывается по нескольким путям тела
	// по условию на элемент.
	//
	// Нужен там, где ядро держит раздельно то, что ссылка пишет вместе:
	// локальные адреса туннеля у masque приезжают одним `address=`, а в теле
	// это два поля по семейству — `ip` и `ipv6`. Выразить это парой записей
	// нельзя: обе читали бы один источник и обе писали бы весь список.
	if p.List != nil && len(p.SplitInto) > 0 {
		st.applySplitInto(e, src, rawVal, val)
		st.applySets(e, val)
		st.applyImplies(e)
		return
	}

	// normalize / type / value_map — перевод диалекта, не суждение.
	typed, drop := st.convert(p, val)
	if drop {
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
			Src: src, Raw: rawVal, Val: nil, Path: nil, Act: ActSkip, Why: WhyEmpty,
		})
		// value_map → null означает «ключа нет»; но sets/implies по этому
		// значению всё равно могли быть объявлены.
		st.applySets(e, val)
		return
	}

	// extract раскладывает ОДНО значение по нескольким путям.
	if p.Extract != nil {
		st.applyExtract(e, src, rawVal, toString(typed))
		st.applySets(e, val)
		st.applyImplies(e)
		return
	}

	// split_into / list / coerce / flatten / lift / sort_keys.
	if p.Flatten != nil || (p.Coerce != nil && p.Coerce.ObjectToScalar != "") {
		st.applyFlatten(e, src)
	}

	path := st.pathOf(p)
	if path == "" {
		// Запись без maps_to (служебная или осознанно никуда): sets/implies
		// остаются её единственной работой.
		st.applySets(e, val)
		st.applyImplies(e)
		if p.MapsTo != nil && p.MapsTo.ExplicitNull {
			st.trace.Add(Event{
				Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
				Src: src, Raw: rawVal, Val: typed, Path: nil, Act: ActSkip, Why: WhyNone,
			})
		}
		return
	}

	// omit_default — значение, равное дефолту ЯДРА, в тело не пишется.
	//
	// Это не суждение о годности (маппер значений не судит), а отказ от
	// лишнего ключа: `aid=0` у vmess ядро понимает так же, как отсутствие
	// alter_id, но записанный ноль попадает в тело — то есть в identity
	// узла, — и тела двух проектов на одной ссылке разъезжаются. Правило
	// объявляет запись, потому что дефолт — свойство ПОЛЯ, а не схемы.
	//
	// sets/implies при этом остаются: узел значение НЕСЁТ, просто в теле оно
	// выражается отсутствием ключа.
	if matchesOmitDefault(p.OmitDefault, typed) {
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
			Src: src, Raw: rawVal, Val: typed, Path: nil, Act: ActSkip, Why: WhyOmitDefault,
		})
		st.applySets(e, val)
		st.applyImplies(e)
		return
	}

	st.writeMerge(e.Name, src, rawVal, typed, path, p.Priority, e.Decl, "", p.Merge)
	st.applySets(e, val)
	st.applyImplies(e)
}

// matchesOmitDefault — совпадает ли значение с объявленным дефолтом.
//
// Сравнение идёт по КАНОНУ JSON, а не по Go-типу: реестр разобран
// encoding/json и отдаёт 0 как float64, а движок уже привёл значение к int
// (Q133-48). Печать обоих через json.Marshal снимает эту разницу, заодно
// давая верное сравнение строк, булевых и чисел одним правилом.
func matchesOmitDefault(raw json.RawMessage, typed interface{}) bool {
	if len(raw) == 0 {
		return false
	}
	var want interface{}
	if err := json.Unmarshal(raw, &want); err != nil {
		return false
	}
	// Массив дефолтов — «любой из перечисленных». Форма emit.omit_default
	// (список ИМЁН полей) сюда не попадает: там другое место и другой тип.
	if list, ok := want.([]interface{}); ok {
		for _, w := range list {
			if sameJSON(w, typed) {
				return true
			}
		}
		return false
	}
	return sameJSON(want, typed)
}

func sameJSON(a, b interface{}) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ab) == string(bb)
}

// applyMissing — источник промолчал: materialize_default, default_from, sets по
// пустому значению.
func (st *execState) applyMissing(e *Entry) {
	p := e.Param

	// default_from — источник значения по умолчанию (эвристика SNI у trojan
	// выражается именно им, а не веткой кода).
	if len(p.DefaultFrom) > 0 {
		if name := rawString(p.DefaultFrom); name != "" {
			v, ok := st.space.Lookup(name)
			if !ok || v == "" {
				// Имя может называть не ИСТОЧНИК, а ПУТЬ ТЕЛА: SNI по
				// умолчанию равен адресу сервера, а адрес у разных форм
				// приезжает из разных источников (`host` у ссылки,
				// `json.add` у контейнера v2rayN). Написать «host» значило
				// бы назвать источник ОДНОЙ формы, и у другой дефолт молча
				// не срабатывал — так терялся tls.server_name у vmess.
				// Путь тела свободен от этого: к моменту чтения его уже
				// заполнила запись `server`, чей источник объявлен по формам.
				if bv, hit := getPath(st.res.Body, name); hit {
					if s := toString(bv); s != "" {
						v, ok = s, true
					}
				}
			}
			if ok && v != "" {
				if path := st.pathOf(p); path != "" {
					st.write(e.Name, name, v, v, path, p.Priority, e.Decl, WhyDefault)
					st.applyImplies(e)
					return
				}
			}
		}
	}

	// default_when — дефолт с УСЛОВИЕМ. Сегодня условие одно: `absent:
	// true`, то есть «источник молчал», а мы как раз здесь. Значение берётся
	// из самого объявления, потому что это НЕ дефолт ядра, а конвенция обеих
	// сторон: у masque `profile: cloudflare`, `vhttp: h3` и `mtu: 1280`
	// входят в тела живых узлов и в их identity, и не написать их значило бы
	// переписать каждый такой узел.
	if len(p.DefaultWhen) > 0 && p.MaterializeDefault {
		if absent, _ := p.DefaultWhen["absent"].(bool); absent {
			if val, has := p.DefaultWhen["value"]; has {
				if path := st.pathOf(p); path != "" {
					// Дефолт ЗАПОЛНЯЕТ ПУСТОТУ, а не спорит за путь: схема
					// «маппер записывает дефолт, даже когда источник молчал»
					// про молчащий источник и говорит. Путь, который уже
					// занят НАСТОЯЩИМ значением, он не трогает — иначе
					// порядок объявления двух записей решал бы, доедет ли до
					// тела прочитанное значение. Так и было: у формы
					// `conf_b64` порт пира приезжает выемкой из `Endpoint`
					// («91.247.235.94:51821»), а объявленный выше `peer_port`
					// перебивал его своим 51820 — узел молча уходил на
					// дефолтный порт.
					if _, taken := getPath(st.res.Body, path); taken {
						by := "-"
						if mark := st.writtenBy[path]; mark != nil {
							by = mark.entry
						}
						st.trace.Add(Event{
							Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
							Src: "-", Val: val, Path: path, Act: ActSkip,
							Why: WhyLowerPriority(by),
						})
						return
					}
					out := val
					switch {
					case p.List != nil:
						// Дефолт записи со СПИСКОМ разбирается тем же
						// разделителем, что и значение источника: иначе
						// `allowed_ips` приезжал бы в тело одной строкой
						// "0.0.0.0/0,::/0", а ядро ждёт массив CIDR.
						out = st.buildList(p, toString(val))
					case p.Type != "":
						conv, drop := convertType(p.Type, toString(val))
						if drop {
							return
						}
						out = conv
					}
					st.write(e.Name, "-", "", out, path, p.Priority, e.Decl, WhyDefault)
					st.applyImplies(e)
					return
				}
			}
		}
	}

	if p.MaterializeDefault {
		// Значение дефолта берётся из sets[""] секции либо из value_map[""].
		if assigns, ok := p.Sets[""]; ok {
			st.applyAssigns(e.Name, assigns, p.Priority, e.Decl, WhyMaterializeDefault, p.Merge)
			return
		}
		// value_map[""] — перевод ПУСТОГО значения. Записи, у которой нет
		// sets, он и служит дефолтом: у vmess `security` объявлен
		// `value_map: {"": "auto", …}`, то есть «нет значения — auto», и
		// ключ обязан появиться в теле (у ядра поле без omitempty, а
		// санитайзер за его отсутствие валит узел кодом field_missing).
		// Прежде ветки не было, и материализация молча не происходила:
		// корпус видел field_missing на восьми кейсах формы v2rayN, где
		// панель ключ scy вовсе не пишет.
		if mapped, hit, isNull := applyValueMapCase(p.ValueMap, "", p.ValueMapCase != "sensitive"); hit && !isNull {
			if path := st.pathOf(p); path != "" {
				typed := interface{}(mapped)
				if p.Type != "" {
					conv, drop := convertType(p.Type, mapped)
					if drop {
						return
					}
					typed = conv
				}
				st.write(e.Name, "-", "", typed, path,
					p.Priority, e.Decl, WhyMaterializeDefault)
				st.applyImplies(e)
				return
			}
		}
		// Дефолт написания схемы (`$default_port` из scheme_sets) — там, где
		// одна схема несёт два написания с разными портами (Q133-44).
		if v, ok := st.schemeVals[schemeValDefaultPort]; ok {
			if path := st.pathOf(p); path != "" {
				typed := v
				if p.Type != "" {
					conv, drop := convertType(p.Type, toString(v))
					if drop {
						return
					}
					typed = conv
				}
				st.write(e.Name, schemeValDefaultPort, v, typed, path,
					p.Priority, e.Decl, WhyMaterializeDefault)
				st.applyImplies(e)
				return
			}
		}
	}

	// sets[""] — присваивания «источник молчал» (так security="" даёт
	// tls.enabled у trojan).
	if assigns, ok := p.Sets[""]; ok {
		st.applyAssigns(e.Name, assigns, p.Priority, e.Decl, "", p.Merge)
		return
	}

	st.trace.Add(Event{
		Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
		Src: "-", Raw: nil, Val: nil, Path: nil, Act: ActSkip, Why: WhyEmpty,
	})
}

// applySets — присваивания по ЗНАЧЕНИЮ.
func (st *execState) applySets(e *Entry, val string) {
	if len(e.Param.Sets) == 0 {
		return
	}
	assigns, ok := e.Param.Sets[val]
	if !ok {
		// Значение читается fold-case: `security=NONE` в живых списках.
		low := strings.ToLower(val)
		for k, v := range e.Param.Sets {
			if strings.ToLower(k) == low {
				assigns, ok = v, true
				break
			}
		}
	}
	if !ok {
		return
	}
	st.applyAssigns(e.Name, assigns, e.Param.Priority, e.Decl, "", e.Param.Merge)
}

// applyImplies — присваивания по НАЛИЧИЮ.
func (st *execState) applyImplies(e *Entry) {
	if len(e.Param.Implies) == 0 {
		return
	}
	code := codeOf(e.Param.OnImpliesWritten)
	if code == "" {
		st.applyAssigns(e.Name, e.Param.Implies, e.Param.Priority, e.Decl, "", e.Param.Merge)
		return
	}
	// Код ставится за ФАКТ дописывания, а не за наличие implies: при занятом
	// пути присваивание проигрывает владельцу, и сообщать не о чем. Так
	// xhttp_mode_forced_packet_up появляется только там, где режим сочинили
	// мы, а не назвала ссылка.
	before := st.snapshotPaths(e.Param.Implies)
	st.applyAssigns(e.Name, e.Param.Implies, e.Param.Priority, e.Decl, "", e.Param.Merge)
	if st.changedAny(e.Param.Implies, before) {
		st.note(code, paramsOf(e.Param.OnImpliesWritten))
	}
}

// snapshotPaths запоминает значения путей присваивания до записи.
func (st *execState) snapshotPaths(assigns map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(assigns))
	for path := range assigns {
		if v, ok := getPath(st.res.Body, path); ok {
			out[path] = v
		}
	}
	return out
}

// changedAny — хоть один путь присваивания изменился.
func (st *execState) changedAny(assigns map[string]interface{}, before map[string]interface{}) bool {
	for path := range assigns {
		now, ok := getPath(st.res.Body, path)
		was, had := before[path]
		if ok != had || toString(now) != toString(was) {
			return true
		}
	}
	return false
}

// applyAssigns кладёт набор присваиваний. null СНИМАЕТ путь — это отличается
// от «не писать» (MAPPER_ENGINE.md §7, G2).
// isReservedAssignKey — имя, которое присваивание НЕ пишет в тело.
//
// Два префикса, и оба означают «это не путь тела»: `$` — служебное значение
// для записей таблицы, `_` — прозаическая сноска автора реестра. Проверка
// одна на все приёмы присваивания, потому что вопрос про ИМЯ, а не про то,
// каким приёмом оно попало в карту.
func isReservedAssignKey(path string) bool {
	return strings.HasPrefix(path, "$") || strings.HasPrefix(path, "_")
}

func (st *execState) applyAssigns(entry string, assigns map[string]interface{}, priority, decl int, why, merge string) {
	paths := make([]string, 0, len(assigns))
	for k := range assigns {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	for _, path := range paths {
		// СЛУЖЕБНОЕ ИМЯ В ТЕЛО НЕ ЕДЕТ — на любом уровне и у любого приёма
		// присваивания (`sets`, `implies`, `scheme_sets`, `defaults`).
		//
		// `$…` — значение для записей таблицы, а не путь тела (`$default_port`
		// у http); `_…` — прозаическая сноска рядом с присваиваниями, которую
		// сторона вправе положить где угодно (находка LxBox 21.09.2026).
		// Отсечение стояло ТОЛЬКО у scheme_sets, и ровно там оно однажды
		// понадобилось (Q133-44): ключ уезжал в тело литералом, и узел терял
		// server_port. Остальные приёмы шли той же дорогой и ждали своей
		// очереди — теперь правило общее для всех, потому что оно про ИМЯ, а
		// не про приём.
		if isReservedAssignKey(path) {
			continue
		}
		v := assigns[path]
		if v == nil {
			delPath(st.res.Body, path)
			delete(st.writtenBy, path)
			st.trace.Add(Event{
				Stage: StageSets, Mapper: st.mapperName, Entry: entry,
				Src: "-", Raw: nil, Val: nil, Path: path, Act: ActRemove, Why: why,
			})
			continue
		}
		// Вложенное объявление {value, implicit} — значение подставлено
		// конвенцией; на эмите оно не пишется.
		if obj, ok := v.(map[string]interface{}); ok {
			if inner, has := obj["value"]; has {
				v = inner
			}
		}
		// Ссылка на источник вместо литерала: "$host" означает «значение
		// источника host», а не строку из пяти символов. Нужна там, где
		// присваивание подставляет не константу, а часть самого входа
		// (SNI = адрес сервера у naive).
		if s, ok := v.(string); ok && strings.HasPrefix(s, "$") {
			if got, found := st.space.Lookup(strings.TrimPrefix(s, "$")); found {
				v = got
			} else {
				// Источник промолчал — писать литерал "$host" в тело нельзя:
				// это мусор, который уедет в конфиг.
				st.trace.Add(Event{
					Stage: StageSets, Mapper: st.mapperName, Entry: entry,
					Src: s, Raw: nil, Val: nil, Path: path, Act: ActSkip, Why: WhyEmpty,
				})
				continue
			}
		}
		st.writeAssign(entry, v, path, priority, decl, why, merge)
	}
}

// applyPairList собирает объект тела из СПИСКА ПАР одного параметра ссылки
// (PRIMITIVES §0.10): `list` режет значение на элементы, `extract` разбирает
// каждый, группы `$key` / `$value` строят пару.
//
// Годность пары решает сама РЕГУЛЯРКА записи, а не код движка: алфавит имени
// заголовка и запрет CR/LF/NUL в значении — свойства формата, и место им в
// объявлении (Q133-41). Не совпавший элемент пропускается, остальные живут;
// `on_item_invalid` ставит код ОДИН раз на узел, о первом отброшенном.
func (st *execState) applyPairList(e *Entry, src string, raw interface{}, val string) {
	p := e.Param
	path := st.pathOf(p)
	if path == "" {
		return
	}
	re, err := compileShared(p.Extract.Re)
	if err != nil {
		return
	}
	keyGroup, valGroup := "", ""
	for name, spec := range p.Extract.Into {
		switch spec {
		case "$key":
			keyGroup = name
		case "$value":
			valGroup = name
		}
	}
	if keyGroup == "" {
		return
	}

	sep := p.List.Sep
	if sep == "" {
		sep = ","
	}
	out := map[string]interface{}{}
	dropped := false
	for _, item := range strings.Split(val, sep) {
		if strings.TrimSpace(item) == "" {
			continue
		}
		m := re.FindStringSubmatch(item)
		if m == nil {
			dropped = true
			continue
		}
		key, value := "", ""
		for gi, name := range re.SubexpNames() {
			switch name {
			case keyGroup:
				key = strings.TrimSpace(m[gi])
			case valGroup:
				value = strings.TrimSpace(m[gi])
			}
		}
		if key == "" {
			dropped = true
			continue
		}
		out[key] = value
	}
	if dropped {
		if code := codeOf(p.OnItemInvalid); code != "" {
			st.note(code, paramsOf(p.OnItemInvalid))
		}
	}
	if len(out) == 0 {
		// Ни одной годной пары — ключа в теле нет вовсе (так вёл себя и
		// прежний путь: parseNaiveExtraHeaders возвращал nil).
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: e.Name,
			Src: src, Raw: raw, Val: nil, Path: path, Act: ActSkip, Why: WhyEmpty,
		})
		return
	}
	// `sort_keys` здесь не исполняется отдельным шагом: канон сериализации
	// (WriteCanonicalJSON) и так печатает ключи карты по порядку, и объект
	// пар доезжает до тела картой. Атрибут остаётся объявлением НОРМЫ для
	// эмита и для LxBox, где карта порядок несёт.
	st.write(e.Name, src, raw, out, path, p.Priority, e.Decl, "")
}

// applyExtract раскладывает значение регуляркой с именованными группами.
func (st *execState) applyExtract(e *Entry, src, raw, val string) {
	re, err := compileShared(e.Param.Extract.Re)
	if err != nil {
		return
	}
	m := re.FindStringSubmatch(val)
	if m == nil {
		// `take_all` — значение не разложилось, но выбрасывать его нельзя:
		// оно осмысленно ЦЕЛИКОМ. Так в wg-quick читается ГОЛЫЙ IPv6 в
		// Endpoint: несколько ':' без скобок, и отличить адрес от пары
		// «хост:порт» нечем, поэтому вся строка идёт адресом, а порт берётся
		// из `defaults` записи.
		if actionOf(e.Param.OnNoMatch) == "take_all" {
			st.applyExtractTakeAll(e, src, raw, val)
			return
		}
		if code := codeOf(e.Param.OnNoMatch); code != "" {
			st.note(code, nil)
		}
		return
	}
	names := re.SubexpNames()
	// Значения групп по ИМЕНИ: их читает prepend_group.
	byName := map[string]string{}
	for gi, name := range names {
		if gi > 0 && name != "" {
			byName[name] = m[gi]
		}
	}
	// Порядок групп = порядок в регулярке: он объявлен автором записи и
	// потому нормативен.
	for gi, name := range names {
		if gi == 0 || name == "" {
			continue
		}
		spec, ok := e.Param.Extract.Into[name]
		if !ok {
			continue
		}
		g := m[gi]
		if g == "" {
			continue
		}
		st.applyExtractGroup(e, src, raw, name, g, spec, byName)
	}
}

// applyExtractTakeAll кладёт НЕРАЗЛОЖИВШЕЕСЯ значение целиком.
//
// Случай один и он настоящий: голый IPv6 в `Endpoint` файла wg-quick.
// Несколько ':' без скобок, и отличить адрес от пары «хост:порт» нечем —
// значит вся строка есть адрес, а порт берётся из `defaults` записи.
// Выбросить её было бы хуже всего: узел с рабочим адресом просто исчез бы.
//
// `defaults` пишутся с приоритетом записи и потому проигрывают тому, что
// уже заняло путь: это ДОПОЛНЕНИЕ недостающего, а не переназначение.
func (st *execState) applyExtractTakeAll(e *Entry, src, raw, val string) {
	spec := e.Param.OnNoMatch
	path, _ := spec["into"].(string)
	if path == "" {
		return
	}
	st.write(e.Name, src, raw, val, path, e.Param.Priority, e.Decl, "")
	defs, _ := spec["defaults"].(map[string]interface{})
	for p, dv := range defs {
		st.write(e.Name+"."+p, src, raw, dv, p, e.Param.Priority, e.Decl, "")
	}
	if code := codeOf(spec); code != "" {
		st.note(code, nil)
	}
}

// applyExtractGroup кладёт одну группу: либо строкой-путём, либо объектом
// {path, type, implies}.
func (st *execState) applyExtractGroup(e *Entry, src, raw, group, val string, spec interface{}, groups map[string]string) {
	switch t := spec.(type) {
	case string:
		st.write(e.Name+"."+group, src, raw, val, t, e.Param.Priority, e.Decl, "")
	case map[string]interface{}:
		path, _ := t["path"].(string)
		if path == "" {
			return
		}
		// prepend_group — значение ДРУГОЙ группы приписывается спереди.
		//
		// Нужно там, где одна величина разрезана регуляркой на две части,
		// и вторая часть без первой бессмысленна: multi-port authority
		// `host:443,20000-30000` даёт `first=443` (он же `server_port`) и
		// `spec=,20000-30000`. Список портов узла — ОБЕ части вместе, то
		// есть «443,20000-30000». Без этого в тело уезжал бы хвост
		// `-50000` вместо пары `20000:50000` (корпус
		// authority_port_range).
		//
		// Склейка делается ДО нормализации и резки списка: приписывается
		// кусок ТЕКСТА, а не готовый элемент, и разделитель уже стоит в
		// самом хвосте (`,` или `-`), потому что его захватила регулярка.
		if from, _ := t["prepend_group"].(string); from != "" {
			if head, ok := groups[from]; ok && head != "" {
				val = head + val
			}
		}
		var out interface{} = val
		// normalize у группы — та же форма значения, что и у записи.
		if n, _ := t["normalize"].(string); n != "" {
			if ls := listSpecOf(t); ls != nil {
				out = st.buildListWith(ls, n, val)
			} else {
				val = normalizeValue(n, val)
				out = val
			}
		} else if ls := listSpecOf(t); ls != nil {
			out = st.buildListWith(ls, "", val)
		}
		if typ, _ := t["type"].(string); typ != "" {
			conv, drop := convertType(typ, val)
			if drop {
				if code := codeOf(mapOf(t["on_invalid"])); code != "" {
					st.note(code, nil)
				}
				return
			}
			out = conv
		}
		st.write(e.Name+"."+group, src, raw, out, path, e.Param.Priority, e.Decl, "")
		// Код за САМО срабатывание группы: узел описан иначе, чем во входе,
		// и человеку надо об этом сказать. Так `?ed=N` в пути ws становится
		// max_early_data + early_data_header_name (ws_early_data_converted).
		if code, _ := t["code"].(string); code != "" {
			st.note(code, nil)
		}
		if impl, ok := t["implies"].(map[string]interface{}); ok {
			st.applyAssigns(e.Name+"."+group, impl, e.Param.Priority, e.Decl, "", e.Param.Merge)
		}
	}
}

// applyFlatten разворачивает члены вложенного объекта в плоский слой
// пространства: дальше их читают обычные записи по своим source.
func (st *execState) applyFlatten(e *Entry, src string) {
	rawVal, ok := st.space.LookupRaw(src)
	if !ok {
		return
	}
	obj, ok := rawVal.(map[string]interface{})
	if !ok {
		return
	}
	for _, key := range e.Param.Flatten {
		inner, ok := obj[key].(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range inner {
			if _, exists := obj[k]; exists {
				continue
			}
			obj[k] = v
		}
	}
}

// lookupSource — значение по цепочке источников записи.
//
// Цепочка «первый НЕПУСТОЙ»: так читает и сегодняшний queryParam, и корпус на
// этом стоит (sni → peer → host). Возвращает (сырое значение, имя
// сработавшего источника, найдено ли).
func (st *execState) lookupSource(p *registry.Param) (string, string, bool) {
	names := p.Source.ForForm(st.form.ID)
	for _, name := range names {
		name = st.substituteBase(name)
		v, ok := st.space.Lookup(name)
		if !ok {
			continue
		}
		// «Пусто = отсутствует», опт-аут `empty: "significant"`.
		if strings.TrimSpace(v) == "" && p.Empty != "significant" {
			continue
		}
		return v, name, true
	}
	// Алиасы записи — те же источники под другими именами параметра.
	if len(p.Aliases) > 0 {
		for _, alias := range p.Aliases {
			for _, name := range names {
				if !strings.HasPrefix(name, "query.") {
					continue
				}
				v, ok := st.space.Lookup("query." + alias)
				if !ok {
					continue
				}
				if strings.TrimSpace(v) == "" && p.Empty != "significant" {
					continue
				}
				return v, "query." + alias, true
			}
		}
	}
	return "", "", false
}

// substituteBase подставляет якорь формы вместо $base.
func (st *execState) substituteBase(name string) string {
	if st.form.Base == "" || !strings.Contains(name, "$base") {
		return name
	}
	return strings.Replace(name, "$base", st.form.Base, 1)
}

// decodeValue применяет decode_extra и форм-семантику `+`.
func (st *execState) decodeValue(p *registry.Param, raw string) string {
	val := raw
	if p.DecodeExtra != nil {
		n, untilStable := p.DecodeExtra.PassCount()
		val = decodePasses(val, p.DecodeExtra.Mode, n, untilStable, p.DecodeExtra.Max)
	}
	// Формат `pem` — РАЗДЕЛЬНАЯ политика `+` (PRIMITIVES §0.4a): в
	// заголовочных строках это пробел, в теле ключа — символ алфавита
	// base64. Булев plus_literal тут не работает ни в одном положении:
	// true ломает заголовок, false ломает тело (QUIRKS Q133-40).
	if p.Format == "pem" {
		return plusInPEM(val)
	}
	if st.formEncoded() && !st.plusLiteral(p) {
		val = PlusToSpace(val)
	}
	return val
}

// formEncoded — есть ли у пространства формы ФОРМ-КОДИРОВАНИЕ, то есть
// семантика `+` = пробел.
//
// Свойство ПРОСТРАНСТВА, а не поля. `+` = пробел придумано для
// application/x-www-form-urlencoded, и живёт оно ровно там, где значения
// приезжают query-строкой. В ini-документе `.conf` такого кодирования нет
// вовсе: `+` там всегда литерал — и в ключе base64, и в любом другом
// значении.
//
// Без этой проверки ключ Proton `0GCSi+xv9…` превращался в `0GCSi xv9…`,
// переставал быть 32 байтами и ронял узел на санитайзере. Поле-то объявило
// формат в ТЕЛЕ (`body.fields.private_key.normalize: base64_std`), а запись
// маппера ни `type`, ни `format` не несёт — и правило по формату поля,
// верное для ссылок, здесь просто не за что зацепиться. Чинить перечислением
// форматов у каждой записи значило бы лечить следствие: вопрос не в том,
// какое это поле, а в том, что документ не является формой.
func (st *execState) formEncoded() bool {
	switch st.form.Space {
	case "ini", "json":
		return false
	}
	return true
}

// plusInPEM применяет политику `+` к PEM-блоку построчно.
//
// Заголовок (-----BEGIN …----- / -----END …-----) читает `+` как пробел,
// всё остальное — как литерал. Строка, не похожая ни на то, ни на другое,
// считается телом: испортить ключ хуже, чем оставить лишний плюс в тексте.
func plusInPEM(val string) string {
	if !strings.Contains(val, "+") {
		return val
	}
	lines := strings.Split(val, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "-----") {
			lines[i] = PlusToSpace(line)
		}
	}
	return strings.Join(lines, "\n")
}

// plusLiteral — читается ли `+` буквально.
//
// По умолчанию выводится из формата поля (base64*), явное указание
// перекрывает. Одно правило вместо заплат у каждого ключа (D133-7).
func (st *execState) plusLiteral(p *registry.Param) bool {
	if p.DecodeExtra != nil && p.DecodeExtra.PlusLiteral != nil {
		return *p.DecodeExtra.PlusLiteral
	}
	if p.DecodeExtra != nil && p.DecodeExtra.Mode == "path" {
		// В пути `+` литерален: `/ws+v2` не должен стать `/ws v2` (D133-14).
		return true
	}
	// Формат ПОЛЯ и тип записи оба называют base64 одним словом, и правило
	// одно на обоих (D133-7: «политика `+` — свойство ПОЛЯ»). Читался же
	// только `type`, и запись, объявившая `format: base64_32` (ключ защиты
	// заголовка AWG3), получала query-семантику: `+` внутри ключа
	// превращался в пробел, ключ переставал быть 32 байтами и санитайзер
	// ронял узел кодом awg3_header_key_invalid — ровно тот класс «знаем, но
	// читаем не так», ради которого затеяна кампания.
	return strings.HasPrefix(p.Type, "base64") || strings.HasPrefix(p.Format, "base64")
}

// convert применяет normalize, type и value_map.
//
// Второе возвращаемое — «значения нет» (value_map в null, либо тип не сошёлся
// и запись объявила on_invalid с drop).
func (st *execState) convert(p *registry.Param, val string) (interface{}, bool) {
	v := val
	// У СПИСКА нормализация применяется к ЭЛЕМЕНТУ, а не к строке целиком:
	// она описывает форму одного значения. `port_range_spec` над
	// «41000,42000-43000» увидел бы одну строку с дефисом и оставил
	// первый элемент голым — а ядру нужны обе пары (корпус
	// hysteria2/mport_comma_list). Элементы нормализует buildList.
	if p.Normalize != "" && p.List == nil {
		v = normalizeValue(p.Normalize, v)
	}

	if len(p.ValueMap) > 0 {
		mapped, hit, isNull := applyValueMapCase(p.ValueMap, v, p.ValueMapCase != "sensitive")
		if isNull {
			return nil, true
		}
		if hit {
			v = mapped
		}
	}

	if p.List != nil {
		return st.buildList(p, v), false
	}

	if p.Type != "" {
		conv, drop := convertType(p.Type, v)
		if drop {
			// Тип не сошёлся — решает запись своим `on_invalid`, а не движок.
			//
			// `keep` означает «вези КАК ПРИШЛО»: годность значения судит
			// реестр на стадии санитайзера (body.fields), и привести его
			// здесь к типу значило бы вынести суждение раньше и молча. Так
			// объявлен min_idle_session у anytls: `abc` обязан доехать до
			// тела строкой и получить код anytls_min_idle_invalid от правила
			// min:0, а не исчезнуть на маппере.
			if actionOf(p.OnInvalid) == "keep" {
				if code := codeOf(p.OnInvalid); code != "" {
					st.note(code, paramsOf(p.OnInvalid))
				}
				return v, false
			}
			return nil, true
		}
		return conv, false
	}
	return v, false
}

// listSpecOf читает объявление списка у ГРУППЫ extract (карта из JSON).
func listSpecOf(t map[string]interface{}) *registry.ListSpec {
	raw, ok := t["list"].(map[string]interface{})
	if !ok {
		return nil
	}
	ls := &registry.ListSpec{}
	if s, ok := raw["sep"].(string); ok {
		ls.Sep = s
	}
	if s, ok := raw["item"].(string); ok {
		ls.Item = s
	}
	return ls
}

// applySplitInto раскладывает список по путям тела: каждый путь объявляет
// условие на ЭЛЕМЕНТ и то, какой из подошедших берётся.
//
// Условие — по элементу (`when.item`), а не по телу: тела на этот момент
// ещё нет, и вопрос стоит о самом значении. `take: "first"` означает
// «первый подошедший»: у ядра поле одно на семейство, и второй адрес того
// же вида в тело не поместится.
func (st *execState) applySplitInto(e *Entry, src, raw, val string) {
	items := st.buildListWith(e.Param.List, e.Param.Normalize, val)
	// Пути перебираются в стабильном порядке имён. Порядок объявления здесь
	// НЕ нормативен, в отличие от порядка записей таблицы: пути split_into
	// независимы — каждый отбирает свои элементы по своему условию и пишет
	// в СВОЙ ключ тела, — и переставить их местами значит получить то же
	// тело. Сортировка нужна только чтобы трасса не плясала между запусками.
	paths := make([]string, 0, len(e.Param.SplitInto))
	for k := range e.Param.SplitInto {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	for _, path := range paths {
		spec, _ := e.Param.SplitInto[path].(map[string]interface{})
		if spec == nil {
			continue
		}
		cond, _ := mapOf(spec["when"])["item"].(map[string]interface{})
		for _, it := range items {
			item := toString(it)
			if item == "" || !itemMatches(cond, item) {
				continue
			}
			st.write(e.Name+"."+path, src, raw, item, path, e.Param.Priority, e.Decl, "")
			if take, _ := spec["take"].(string); take == "" || take == "first" {
				break
			}
		}
	}
}

// itemMatches проверяет условие на элементе: matches / not_matches.
func itemMatches(cond map[string]interface{}, item string) bool {
	if len(cond) == 0 {
		return true
	}
	if re, _ := cond["matches"].(string); re != "" {
		rx, err := compileShared(re)
		if err != nil || !rx.MatchString(item) {
			return false
		}
	}
	if re, _ := cond["not_matches"].(string); re != "" {
		rx, err := compileShared(re)
		if err != nil || rx.MatchString(item) {
			return false
		}
	}
	return true
}

// buildList режет значение по разделителю, обрезая края элементов.
func (st *execState) buildList(p *registry.Param, v string) []interface{} {
	return st.buildListWith(p.List, p.Normalize, v)
}

// buildListWith — то же для объявления, пришедшего не от записи, а от
// группы extract.
func (st *execState) buildListWith(ls *registry.ListSpec, normalize, v string) []interface{} {
	p := &registry.Param{List: ls, Normalize: normalize}
	return st.buildListInner(p, v)
}

func (st *execState) buildListInner(p *registry.Param, v string) []interface{} {
	sep := p.List.Sep
	if sep == "" {
		sep = ","
	}
	parts := strings.Split(v, sep)
	out := make([]interface{}, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if p.Normalize != "" {
			item = normalizeValue(p.Normalize, item)
		}
		if p.List.Item != "" {
			conv, drop := convertType(p.List.Item, item)
			if drop {
				continue
			}
			out = append(out, conv)
			continue
		}
		out = append(out, item)
	}
	return out
}

// pathOf — целевой путь записи с учётом карты по типу тела.
func (st *execState) pathOf(p *registry.Param) string {
	if p.MapsTo == nil {
		return ""
	}
	if p.MapsTo.Path != "" {
		return p.MapsTo.Path
	}
	if len(p.MapsTo.ByType) > 0 {
		return p.MapsTo.ByType[st.bodyType]
	}
	return ""
}

// write кладёт значение в тело, разрешая конфликт двух записей в один путь.
func (st *execState) write(entry, src string, raw, val interface{}, path string, priority, decl int, why string) {
	st.writeMerge(entry, src, raw, val, path, priority, decl, why, "")
}

func (st *execState) writeMerge(entry, src string, raw, val interface{}, path string, priority, decl int, why, merge string) {
	if !st.claim(entry, path, priority, decl, merge) {
		st.trace.Add(Event{
			Stage: StageField, Mapper: st.mapperName, Entry: entry,
			Src: src, Raw: raw, Val: val, Path: path, Act: ActSkip,
			Why: WhyLowerPriority(st.writtenBy[path].entry),
		})
		return
	}
	act := ActWrite
	if _, exists := getPath(st.res.Body, path); exists {
		act = ActOverride
	}
	setPath(st.res.Body, path, val)
	st.trace.Add(Event{
		Stage: StageField, Mapper: st.mapperName, Entry: entry,
		Src: src, Raw: raw, Val: val, Path: path, Act: act, Why: orDash(why),
	})
}

// writeAssign — то же для присваивания из sets/implies (стадия трассы иная).
func (st *execState) writeAssign(entry string, val interface{}, path string, priority, decl int, why, merge string) {
	if !st.claim(entry, path, priority, decl, merge) {
		st.trace.Add(Event{
			Stage: StageSets, Mapper: st.mapperName, Entry: entry,
			Src: "-", Raw: nil, Val: val, Path: path, Act: ActSkip,
			Why: WhyLowerPriority(st.writtenBy[path].entry),
		})
		return
	}
	act := ActWrite
	if _, exists := getPath(st.res.Body, path); exists {
		act = ActOverride
	}
	setPath(st.res.Body, path, val)
	st.trace.Add(Event{
		Stage: StageSets, Mapper: st.mapperName, Entry: entry,
		Src: "-", Raw: nil, Val: val, Path: path, Act: act, Why: orDash(why),
	})
}

// claim решает, кто владеет путём.
//
// Норма MAPPER_ENGINE.md §7: `priority` (МЕНЬШЕ = раньше), при равном
// приоритете — порядок объявления, а что делать с ЗАНЯТЫМ путём, решает
// `merge` (`keep_first` по умолчанию). То есть первый писавший держит путь,
// пока пришедший позже явно не объявил `overwrite`.
//
// Правило «sets побеждает» неверно: побеждает не источник значения, а
// объявленный порядок.
func (st *execState) claim(entry, path string, priority, decl int, merge string) bool {
	prev, taken := st.writtenBy[path]
	if !taken {
		st.writtenBy[path] = &writeMark{entry: entry, priority: priority, decl: decl}
		return true
	}
	// Запись, которая по порядку ДОЛЖНА была идти раньше занявшей путь,
	// всё равно её перебивает: порядок исполнения и порядок объявления в
	// пределах одного прохода совпадают, поэтому сюда попадает только
	// селектор против записи прохода B — и он объявлен раньше.
	earlier := priority < prev.priority || (priority == prev.priority && decl < prev.decl)
	if earlier {
		st.writtenBy[path] = &writeMark{entry: entry, priority: priority, decl: decl}
		return true
	}
	switch merge {
	case "overwrite", "prepend", "append":
		st.writtenBy[path] = &writeMark{entry: entry, priority: priority, decl: decl}
		return true
	}
	return false
}

// whenHolds проверяет условие записи.
//
// Ключ-путь тела читается по УЖЕ ПОСТРОЕННОМУ телу, ключ-источник — по входу;
// `$type` — тип тела, `$form` — id сработавшей формы. Условие по источнику
// необходимо там, где род узла объявляет ВХОД, а не тело.
func (st *execState) whenHolds(when map[string]interface{}) bool {
	if len(when) == 0 {
		return true
	}
	keys := make([]string, 0, len(when))
	for k := range when {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !st.oneWhen(key, when[key]) {
			return false
		}
	}
	return true
}

func (st *execState) oneWhen(key string, want interface{}) bool {
	var got interface{}
	var present bool
	switch {
	case key == "$type":
		got, present = st.bodyType, st.bodyType != ""
	case key == "$form":
		got, present = st.form.ID, st.form.ID != ""
	case isSourceName(key):
		key = st.substituteBase(key)
		v, ok := st.space.Lookup(key)
		got, present = v, ok
	default:
		got, present = getPath(st.res.Body, key)
	}

	switch w := want.(type) {
	case nil:
		return !present
	case string:
		if !present {
			return w == ""
		}
		return strings.EqualFold(toString(got), w)
	case bool:
		if !present {
			return !w
		}
		b, ok := got.(bool)
		if !ok {
			b = flagTrue(toString(got))
		}
		return b == w
	case float64:
		if !present {
			return false
		}
		return toString(got) == formatNumber(w)
	case map[string]interface{}:
		return st.whenOperator(got, present, w)
	}
	return false
}

// whenOperator — операторы условия: in / not_in / present / absent / lt / gt.
func (st *execState) whenOperator(got interface{}, present bool, spec map[string]interface{}) bool {
	if list, ok := spec["in"].([]interface{}); ok {
		if !present {
			return false
		}
		return valueInList(got, list)
	}
	if list, ok := spec["not_in"].([]interface{}); ok {
		if !present {
			return true
		}
		return !valueInList(got, list)
	}
	if v, ok := spec["present"].(bool); ok {
		return present == v
	}
	if v, ok := spec["absent"].(bool); ok {
		return present != v
	}
	// lt / gt — ЧИСЛОВОЕ сравнение (GRAMMAR_SYNC §1 №7). Заведены ради знака
	// у Xray-sockopt: tcpKeepAliveIdle 0 = «не задано», отрицательное =
	// «выключить keep-alive» (disable_tcp_keep_alive: true). Без них знак
	// выражался бы веткой в коде.
	//
	// Это НЕ min/max реестра: те судят ЗНАЧЕНИЕ и ставят код, lt/gt выбирают,
	// исполнять ли запись. Нечисловое значение и отсутствие источника дают
	// false у обоих операторов: «меньше нуля» неверно для того, чего нет, и
	// неверно для строки — молчаливое приведение прятало бы мусор во входе.
	if v, ok := numericBound(spec["lt"]); ok {
		n, okNum := numericBound(got)
		return present && okNum && n < v
	}
	if v, ok := numericBound(spec["gt"]); ok {
		n, okNum := numericBound(got)
		return present && okNum && n > v
	}
	return false
}

// numericBound приводит значение к числу для lt/gt: JSON несёт float64, а
// пространство источников — строку (query-параметр, поле ini).
func numericBound(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

func valueInList(got interface{}, list []interface{}) bool {
	s := toString(got)
	for _, item := range list {
		switch t := item.(type) {
		case string:
			if strings.EqualFold(s, t) {
				return true
			}
		case float64:
			if s == formatNumber(t) {
				return true
			}
		case bool:
			if s == strconv.FormatBool(t) {
				return true
			}
		}
	}
	return false
}

// isSourceName — ключ адресует ВХОД, а не тело.
func isSourceName(key string) bool {
	for _, p := range []string{"query.", "json.", "ini.", "userinfo"} {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	switch key {
	case "scheme", "authority", "host", "port", "port_raw", "path", "fragment", "hint":
		return true
	}
	return false
}

// applyLabel собирает метку по цепочке источников.
func (st *execState) applyLabel() {
	spec := st.plan.Mapper.Label
	if spec == nil {
		return
	}
	// Звено цепочки считается ОТВЕТИВШИМ только после нормализации.
	//
	// Прежде нормализация шла ПОСЛЕ выбора, и звено занимало место сырым:
	// у tuic цепочка объявлена ["fragment", "path"], а ссылка
	// `tuic://…@host/?reduce_rtt=1` несёт путь «/» и не несёт фрагмента —
	// узел получал имя «/». То же и с пробельной подсказкой: она глушила
	// следующие звенья, ничего не дав взамен. Пусто после нормализации —
	// значит звено не ответило, и слово переходит дальше.
	for _, name := range spec.Source.ForForm(st.form.ID) {
		// Имя-НЕ-источник читается из УЖЕ ПОСТРОЕННОГО тела — тем же
		// правилом, по которому их различает `when` (isSourceName).
		//
		// Нужно там, где звено метки это не сырое значение входа, а его
		// РАЗОБРАННАЯ часть: у формы `.conf` имя безымянного узла берётся с
		// хоста Endpoint, а во входе `Endpoint` лежит целиком,
		// «91.247.235.94:51821». Сырое звено дало бы узлу имя с портом;
		// хост из него уже выделен выемкой в peers[].address, и читать надо
		// его, а не резать значение второй раз своим правилом.
		var v string
		var ok bool
		if isSourceName(name) {
			v, ok = st.space.Lookup(st.substituteBase(name))
		} else if raw, has := getPath(st.res.Body, name); has {
			v, ok = toString(raw), true
		}
		if !ok || v == "" {
			continue
		}
		cand := v
		for _, n := range spec.Normalize {
			cand = normalizeValue(n, cand)
		}
		// Путь «/» — не имя: это остаток синтаксиса ссылки. Отдельного
		// правила для него не нужно — его снимает та же чистка, что и
		// пробелы, если объявить её частью выбора.
		if strings.Trim(cand, "/") == "" {
			continue
		}
		st.res.Label = cand
		st.trace.Add(Event{
			Stage: StageLabel, Mapper: st.mapperName, Entry: "$label",
			Src: name, Raw: v, Val: cand, Path: nil, Act: ActWrite, Why: WhyNone,
		})
		break
	}
	if len(spec.ValueMap) > 0 {
		for from, to := range spec.ValueMap {
			if s, ok := to.(string); ok {
				st.res.Label = strings.ReplaceAll(st.res.Label, from, s)
			}
		}
	}
	if spec.Fallback != nil {
		st.res.LabelFallback = spec.Fallback.Template
	}
}

// noteUnknownParams ставит код на каждый НЕобъявленный параметр ссылки.
//
// Объявленным считается имя из params секции, из любого include-блока
// (включая не применившиеся по when) и из aliases любого из них
// (MAPPER_ENGINE.md §8).
func (st *execState) noteUnknownParams() {
	uk := st.plan.Mapper.UnknownKey
	if uk == nil || uk.Code == "" {
		return
	}
	var ignored map[string]bool
	if len(uk.Ignore) > 0 {
		ignored = make(map[string]bool, len(uk.Ignore))
		for _, n := range uk.Ignore {
			ignored[strings.ToLower(strings.TrimSpace(n))] = true
		}
	}
	for _, name := range st.space.QueryNames() {
		if st.plan.Declared[strings.ToLower(name)] {
			continue
		}
		if ignored[strings.ToLower(name)] {
			continue
		}
		st.notePath(uk.Code, name, map[string]string{"query_name": name})
		st.trace.Add(Event{
			Stage: StageUnknown, Mapper: st.mapperName, Entry: "$unknown",
			Src: "query." + name, Raw: nil, Val: nil, Path: nil,
			Act: ActKeep, Why: WhyNotDeclared,
		})
	}

	st.noteUnknownJSONKeys(uk, ignored)

	// Ключи ДОКУМЕНТА ini — тот же вопрос, другой предмет.
	//
	// Незнакомый ключ `.conf` до тела не доходит по построению (тело строят
	// только объявленные записи), и потому он не «ломает узел» — он
	// исчезает МОЛЧА (Q133-58). Человек не узнаёт, что часть его файла не
	// прочитана: ни кода, ни строки в логе. Имя ключа несёт `query_name` —
	// параметр у кода один и означает «имя того, что не прочитано»,
	// заводить второй ради пространства незачем.
	for _, name := range st.space.INIKeys() {
		if st.plan.DeclaredINI[name] {
			continue
		}
		// Игнор-список сверяется и с полным «секция.ключ», и с голым
		// ключом: не-узловые ключи wg-quick (PostUp, Table) осмысленны
		// независимо от секции, а перечислять их дважды — лишний повод
		// разойтись.
		short := name
		if i := strings.Index(name, "."); i >= 0 {
			short = name[i+1:]
		}
		if ignored[name] || ignored[short] {
			continue
		}
		st.notePath(uk.Code, name, map[string]string{"query_name": name})
		st.trace.Add(Event{
			Stage: StageUnknown, Mapper: st.mapperName, Entry: "$unknown",
			Src: "ini." + name, Raw: nil, Val: nil, Path: nil,
			Act: ActKeep, Why: WhyNotDeclared,
		})
	}
}

// noteUnknownJSONKeys ставит код на каждый необъявленный ключ ВЕРХНЕГО
// УРОВНЯ JSON-элемента (MAPPER_ENGINE.md §8, парный код `json_field_unknown`).
//
// Почему только верхний уровень. Объявленность параметра считается по
// `source`, а вложенный путь (`json.a.b`) плоским слоем не выражается и
// параметром не является — то же правило, что в `plan.go` при сборке
// `Declared`. Спускаться глубже значило бы звать неизвестным лист
// контейнера, который секция читает своей записью: списки `ignore` у
// xray-секций для того и перечисляют `settings`/`streamSettings`.
//
// Пространство ссылки с КОНТЕЙНЕРОМ (v2rayN у vmess) сюда не попадает
// дважды: его ключи видны плоским слоем и уже сосчитаны как `query`.
func (st *execState) noteUnknownJSONKeys(uk *registry.UnknownKey, ignored map[string]bool) {
	if st.space == nil || len(st.space.QueryNames()) > 0 {
		return
	}
	for _, name := range st.space.JSONKeys() {
		if st.plan.Declared[strings.ToLower(name)] {
			continue
		}
		if ignored[strings.ToLower(name)] {
			continue
		}
		st.notePath(uk.Code, name, map[string]string{"query_name": name})
		st.trace.Add(Event{
			Stage: StageUnknown, Mapper: st.mapperName, Entry: "$unknown",
			Src: "json." + name, Raw: nil, Val: nil, Path: nil,
			Act: ActKeep, Why: WhyNotDeclared,
		})
	}
}

// noteINIDropped ставит код на секции ini, чьи ПОВТОРЫ снял диалект.
//
// Решение «какая секция единственная» принимает разборщик (`repeat`), а
// КОД объявляет запись `on_extra` того же диалекта: потеря перестаёт быть
// молчаливой ровно тогда, когда автор секции назвал ей код. Без `on_extra`
// повтор снимается по-прежнему тихо — это тоже объявленный выбор.
//
// `count` — ОБЩЕЕ число секций этого имени, как их написал человек
// (две `[Peer]` → count=2), а не число отброшенных: текст кода говорит
// «в конфигурации {count} секций, узлом стала первая».
func (st *execState) noteINIDropped() {
	d := st.plan.Mapper.IniDialect
	if d == nil || st.space == nil {
		return
	}
	for _, drop := range st.space.iniDropped {
		rule := d.Section(drop.Name)
		if rule == nil || rule.OnExtra == nil || rule.OnExtra.Code == "" {
			continue
		}
		st.note(rule.OnExtra.Code, map[string]string{"count": strconv.Itoa(drop.Count)})
	}
}

func (st *execState) note(code string, params map[string]string) {
	st.res.Notes = append(st.res.Notes, Note{Code: code, Params: params})
}

// notePath — код, который называет ОДНО имя источника.
//
// Отдельно от note, потому что путь тут не украшение: по паре (код, path)
// идёт дедуп деградаций узла, и без него второй незнакомый ключ той же
// ссылки терялся бы молча.
func (st *execState) notePath(code, path string, params map[string]string) {
	st.res.Notes = append(st.res.Notes, Note{Code: code, Path: path, Params: params})
}

// --- общие преобразования значений ---

// applyValueMap переводит значение. Возвращает (новое значение, было ли
// попадание, означает ли попадание «ключа нет»).
//
// Поддерживает и точную карту, и форму {prefix, strip} (диалект uTLS).
func applyValueMap(vm map[string]interface{}, v string) (string, bool, bool) {
	return applyValueMapCase(vm, v, true)
}

// applyValueMapCase — то же с явным указанием, значим ли регистр.
//
// Регистронезависимое попадание — общее правило (живые списки шлют
// `security=NONE`), но там, где ядро сравнивает литерал точно, оно молча
// проглатывает негодное значение: `encryption=None` у vless обязано доехать
// до тела и быть отвергнутым, а не стать «слоя нет».
func applyValueMapCase(vm map[string]interface{}, v string, fold bool) (string, bool, bool) {
	// $ref — именованная таблица общих блоков, разрешённая при сборке плана.
	if ref, ok := vm["$ref"].(string); ok {
		resolved := lookupNamedValueMap(ref)
		if resolved == nil {
			return v, false, false
		}
		vm = resolved
	}
	if raw, ok := vm["prefix"].(map[string]interface{}); ok {
		key := strings.ToLower(v)
		if strip, ok := vm["strip"].([]interface{}); ok {
			for _, s := range strip {
				if ss, ok := s.(string); ok {
					key = strings.ReplaceAll(key, ss, "")
				}
			}
		}
		prefixes := make([]string, 0, len(raw))
		for k := range raw {
			prefixes = append(prefixes, k)
		}
		// Длинный префикс раньше короткого: "hellorandomized" не должен
		// проиграть "hellorandom".
		sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
		for _, p := range prefixes {
			if strings.HasPrefix(key, p) {
				s, _ := raw[p].(string)
				return s, true, false
			}
		}
		return v, false, false
	}

	if mapped, ok := vm[v]; ok {
		if mapped == nil {
			return "", true, true
		}
		return toString(mapped), true, false
	}
	if !fold {
		return v, false, false
	}
	// Регистронезависимое попадание.
	low := strings.ToLower(v)
	for k, mapped := range vm {
		if strings.ToLower(k) == low {
			if mapped == nil {
				return "", true, true
			}
			return toString(mapped), true, false
		}
	}
	return v, false, false
}

// convertType приводит значение к объявленному типу. Второе возвращаемое —
// «значения нет» (негодная форма).
func convertType(typ, v string) (interface{}, bool) {
	switch typ {
	case "", "string":
		return v, false
	case "int":
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, true
		}
		return n, false
	case "bool", "bool_spelled":
		if !flagTrue(v) {
			return nil, true
		}
		return true, false
	case "duration":
		// Форма записи — работа маппера; годность судит реестр. Поэтому
		// значение едет строкой как есть: приведение к числу здесь означало
		// бы суждение о том, что ядро считает валидной длительностью.
		return v, false
	default:
		return v, false
	}
}

// flagTrue — одно правило истинности на все булевы параметры всех схем.
func flagTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// normalizeValue — объявленные нормализации формы значения.
func normalizeValue(kind, v string) string {
	switch kind {
	case "trim":
		return strings.TrimSpace(v)
	case "trim_lower":
		return strings.ToLower(strings.TrimSpace(v))
	case "strip_control":
		return stripControl(v)
	case "cidr_prefix":
		// Голый адрес получает префикс «весь хост»: ядро ждёт CIDR, а
		// подписки пишут и так, и так. /32 для IPv4, /128 для IPv6 —
		// семейство видно по двоеточию.
		t := strings.TrimSpace(v)
		if t == "" || strings.Contains(t, "/") {
			return v
		}
		if strings.Contains(t, ":") {
			return t + "/128"
		}
		return t + "/32"
	case "port_range_spec":
		// Диапазон портов ссылки → форма ядра. Ссылка пишет дефисом
		// (`20000-30000`, конвенция hysteria2), ядро ждёт ДВОЕТОЧИЕ и на
		// дефисе валит весь конфиг («bad port range»). Одиночный порт
		// становится парой N:N — для ядра это и есть «ровно этот порт».
		//
		// Перевод диалекта, а не суждение: мусор уезжает как есть и его
		// судит санитайзер.
		t := strings.TrimSpace(v)
		if t == "" {
			return v
		}
		t = strings.ReplaceAll(t, "-", ":")
		if !strings.Contains(t, ":") {
			t = t + ":" + t
		}
		return t
	case "duration_bare_seconds":
		// Голое число — это СЕКУНДЫ: живая конвенция панелей
		// (`idle_session_timeout=30`), а ядро ждёт единицу измерения и на
		// голом числе валит весь конфиг. Значение с уже написанной единицей
		// не трогаем: это перевод диалекта, а не нормализация величины.
		t := strings.TrimSpace(v)
		if t == "" {
			return v
		}
		for i := 0; i < len(t); i++ {
			if t[i] < '0' || t[i] > '9' {
				return v
			}
		}
		return t + "s"
	}
	return v
}

// stripControl снимает C0-управляющие и DEL, оставляя tab/CR/LF.
func stripControl(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			b.WriteRune(r)
			continue
		}
		if r <= 0x1F || r == 0x7F {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// decodePasses выполняет дополнительные проходы percent-декода.
func decodePasses(v, mode string, n int, untilStable bool, max int) string {
	if untilStable {
		if max <= 0 {
			max = 16
		}
		for i := 0; i < max; i++ {
			next := decodeOnce(v, mode)
			if next == v {
				return v
			}
			v = next
		}
		return v
	}
	for i := 0; i < n; i++ {
		v = decodeOnce(v, mode)
	}
	return v
}

// decodeOnce — один проход percent-декода в объявленном режиме.
//
// Режимы различаются НАМЕРЕННО: в пути `+` литерален, и query-семантика
// превратила бы `/ws+v2%2Fdata` в `/ws v2/data` — сервер отвечает 404.
func decodeOnce(v, mode string) string {
	dec, err := percentUnescape(v)
	if err != nil {
		return v
	}
	if mode == "query" {
		return dec
	}
	return dec
}

// percentUnescape — percent-декод, НЕ трогающий `+` (путь и query решают о нём
// сами). Битая последовательность оставляется как есть: она законна в живых
// подписках, и ронять из-за неё узел нельзя.
func percentUnescape(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		hi, ok1 := unhex(s[i+1])
		lo, ok2 := unhex(s[i+2])
		if !ok1 || !ok2 {
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String(), nil
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// --- пути тела ---

// normalizeNumber приводит ЦЕЛОЕ число из JSON к int.
//
// Значения `defaults`, `sets` и `scheme_sets` приезжают из реестра через
// encoding/json, а он делает из любого числа float64. В теле узла это мина:
// `json.Marshal` печатает float64 так же, как int, поэтому канон сверки и
// корпус разницы не видят, — а код, читающий тело ассертом `.(int)`, молча
// получает ноль. Ровно так у hysteria v1 терялись up_mbps и server_ports, и
// ядро не стартовало (память проекта json-map-type-assert-trap).
//
// Дробное число остаётся float64: превращать 1.5 в 1 значило бы судить
// значение, а маппер значений не судит.
func normalizeNumber(v interface{}) interface{} {
	f, ok := v.(float64)
	if !ok {
		return v
	}
	if f != float64(int64(f)) {
		return v
	}
	// Диапазон int на 32-битных сборках уже int64 — значение, не влезающее в
	// int, оставляем как есть: порча тихим переполнением хуже float64.
	n := int64(f)
	if int64(int(n)) != n {
		return v
	}
	return int(n)
}

// arrayStep — уровень пути, записанный как массив: "peers[]".
//
// Второе возвращаемое — было ли "[]"; первое — имя без него.
//
// Массив ОДНОЭЛЕМЕНТНЫЙ по построению: ссылка и .conf несут ровно один peer
// (форма `key@host:port` другого и не выражает), а многопировый sing-box-вход
// приезжает готовым телом и маппера не касается. Поэтому элемент не
// индексируется: "peers[].address" читается как «поле address единственного
// элемента peers».
func arrayStep(part string) (string, bool) {
	if strings.HasSuffix(part, "[]") {
		return strings.TrimSuffix(part, "[]"), true
	}
	return part, false
}

// descendPath спускается по пути до ПРЕДПОСЛЕДНЕГО уровня, создавая
// недостающие. create=false — только чтение (nil, если уровня нет).
//
// Уровень с "[]" материализуется как []interface{} из одной карты: тело
// wireguard держит peers массивом, и без этого шага путь "peers[].address"
// оседал бы в теле буквальным ключом "peers[]" — ядро такого поля не знает, а
// санитайзер не находил обязательный peers[].address и ронял узел с
// field_missing.
func descendPath(root map[string]interface{}, parts []string, create bool) map[string]interface{} {
	cur := root
	for _, part := range parts {
		name, isArray := arrayStep(part)
		if !isArray {
			next, ok := cur[name].(map[string]interface{})
			if !ok {
				if !create {
					return nil
				}
				next = map[string]interface{}{}
				cur[name] = next
			}
			cur = next
			continue
		}
		switch existing := cur[name].(type) {
		case []interface{}:
			if len(existing) > 0 {
				if m, ok := existing[0].(map[string]interface{}); ok {
					cur = m
					continue
				}
			}
			if !create {
				return nil
			}
			m := map[string]interface{}{}
			cur[name] = []interface{}{m}
			cur = m
		default:
			if !create {
				return nil
			}
			m := map[string]interface{}{}
			cur[name] = []interface{}{m}
			cur = m
		}
	}
	return cur
}

// setPath кладёт значение по точечному пути, создавая недостающие уровни.
func setPath(root map[string]interface{}, path string, v interface{}) {
	v = normalizeNumber(v)
	parts := strings.Split(path, ".")
	cur := descendPath(root, parts[:len(parts)-1], true)
	last, isArray := arrayStep(parts[len(parts)-1])
	if isArray {
		// Последний уровень сам массив ("peers[]" целиком): значение —
		// единственный элемент.
		cur[last] = []interface{}{v}
		return
	}
	cur[last] = v
}

// getPath читает значение по точечному пути.
func getPath(root map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	cur := descendPath(root, parts[:len(parts)-1], false)
	if cur == nil {
		return nil, false
	}
	last, isArray := arrayStep(parts[len(parts)-1])
	v, ok := cur[last]
	if !ok {
		return nil, false
	}
	if isArray {
		if arr, isSlice := v.([]interface{}); isSlice && len(arr) > 0 {
			return arr[0], true
		}
	}
	return v, true
}

// delPath снимает путь; опустевшие родительские уровни убираются вместе с ним,
// иначе в теле остались бы пустые объекты, которых никто не писал.
func delPath(root map[string]interface{}, path string) {
	parts := strings.Split(path, ".")
	maps := []map[string]interface{}{root}
	cur := root
	for i := 0; i < len(parts)-1; i++ {
		next := descendPath(cur, parts[i:i+1], false)
		if next == nil {
			return
		}
		maps = append(maps, next)
		cur = next
	}
	last, _ := arrayStep(parts[len(parts)-1])
	delete(cur, last)
	// Опустевший уровень снимается вместе с путём. Имя уровня берётся БЕЗ
	// "[]": в теле лежит ключ "peers", массив — его значение.
	for i := len(maps) - 1; i > 0; i-- {
		if len(maps[i]) == 0 {
			name, _ := arrayStep(parts[i-1])
			delete(maps[i-1], name)
		}
	}
}

// --- мелочи ---

// isEmptyValue — «поле пусто» для проверки `required`.
//
// Отдельно от toString потому, что toString отвечает на другой вопрос: он
// даёт ПЕЧАТНУЮ форму скаляра и у составного значения честно возвращает "".
// Проверке обязательности это давало ложное «пусто» у записи со `list`:
// `address=10.0.0.2/32` разбирался в непустой []string, и узел отказывался
// разбираться с текстом «обязательное поле "address" пусто». До wireguard
// пара `required` + `list` встречалась лишь у masque, где она идёт веткой
// `split_into` выше и до этой строки не доходит.
func isEmptyValue(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case []string:
		return len(t) == 0
	case []interface{}:
		return len(t) == 0
	case []int:
		return len(t) == 0
	case map[string]interface{}:
		return len(t) == 0
	}
	return toString(v) == ""
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case float64:
		return formatNumber(t)
	}
	return ""
}

func orDash(why string) string {
	if why == "" {
		return WhyNone
	}
	return why
}

func codeOf(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	s, _ := m["code"].(string)
	return s
}

// actionOf читает объявленное действие (`keep` / `drop` / `skip`).
func actionOf(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	s, _ := m["action"].(string)
	return s
}

// paramsOf читает объявленные параметры кода ({"query_name": "ech"}).
func paramsOf(m map[string]interface{}) map[string]string {
	raw, ok := m["params"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		out[k] = toString(v)
	}
	return out
}

func mapOf(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// rawString читает json.RawMessage как строку (default_from объявлен строкой).
func rawString(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		return s[1 : len(s)-1]
	}
	return ""
}

// сохраняем ссылку на regexp, чтобы импорт не выпал при правках выше.
var _ = regexp.MustCompile

// BodyString / BodyInt — чтение готового тела по тому же точечному пути, что
// понимают записи секции, включая уровень-массив ("peers[].address").
//
// Нужны вызывающему за пределами движка: ParsedNode держит адрес и порт
// отдельными полями, а ГДЕ они лежат в теле, знает секция, не код.
func BodyString(body map[string]interface{}, path string) (string, bool) {
	v, ok := getPath(body, path)
	if !ok {
		return "", false
	}
	s, isStr := v.(string)
	return s, isStr
}

func BodyInt(body map[string]interface{}, path string) (int, bool) {
	v, ok := getPath(body, path)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}
