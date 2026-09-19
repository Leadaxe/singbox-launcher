// Package nodeflow — ядро конвейера узла (SPEC 131 §3): санитайзер по
// реестру, тупой эмиттер и гейт сборки по версии ядра и платформе.
//
// Ни одного `if scheme == "..."`: все схемные различия живут в реестре
// контракта (contract/registry), код лишь исполняет его правила. Из этого
// следует и порядок warnings — он равен порядку обхода body.order, а значит
// детерминирован и сравним в корпусе (CANON §6).
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package nodeflow

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/registry"
)

// Warning — запись о снятом или приведённом поле (CANON §6).
type Warning = configtypes.Warning

// Result — исход санитайзера.
type Result struct {
	// Clean — карта с каноническими ключами и приведёнными типами. Эмиттер
	// работает только с ней, поэтому ассертов типов в нём быть не должно.
	Clean map[string]interface{}
	// Warnings — коды снятого и приведённого, в порядке обхода body.order.
	Warnings []Warning
	// Drop — узел не собирается (required / on_invalid drop_node). Остальные
	// поля Result при этом заполнены, но телу узла ходу нет.
	Drop *Warning
}

// maskedValue — то, что пишется в Warning.Value вместо секрета.
const maskedValue = "***"

// buildManagedKeys — ключи корня outbound'а, которые пишет СБОРКА конфига, а
// не тело узла: tag берётся из идентичности узла (SPEC 112), type — из его
// схемы. В body-секциях реестра их поэтому нет, и снимать их надо молча:
// unknown_key на них означал бы, что пользователю показывают ⚠ за работу
// самого лаунчера. detour управляется тем же правилом, но у него есть запись
// в реестре с managed:true, и его снимает общий обход.
var buildManagedKeys = map[string]bool{"tag": true, "type": true}

// Входы узла — имена из секции `sources` схемы реестра. Ими правила значений
// отличают тело, написанное в форме ядра (`singbox`), от тела, собранного
// маппером из ссылки или .conf. Словарь один на пакеты — он объявлен в
// configtypes, здесь только удобные имена.
const (
	SourceURI     = configtypes.NodeSourceURI
	SourceSingbox = configtypes.NodeSourceSingbox
	SourceXray    = configtypes.NodeSourceXray
	SourceWGConf  = configtypes.NodeSourceWGConf
	SourceAmnezia = configtypes.NodeSourceAmnezia
)

// sanitizer — состояние одного прохода. Хранит схему и накопители, чтобы не
// таскать их шестым аргументом через рекурсию.
type sanitizer struct {
	reg    *registry.Registry
	scheme string
	// source — вход, которым тело приехало (константы Source*). Пустая
	// строка = вход неизвестен; правила с except_sources тогда работают в
	// своей ОСНОВНОЙ форме (замена), потому что неизвестный вход — это не
	// «пользователь написал сам».
	source string
	// kind — РОД узла, объявленный входом (`kind_when` маппера: "awg",
	// "awg3", …). Пустая строка = род неизвестен, и правила опираются на
	// парный `any_set` по телу. Отдельно от source: тот отвечает «кто сочинил
	// тело», этот — «какой протокол вход просил».
	kind string
	res  Result
	seen   map[string]bool // дедуп по (code, path)
	// srcRoot — исходная карта тела целиком, cleanRoot — уже собранная
	// чистая. Пути в conflicts/requires реестра пишутся ОТ КОРНЯ тела
	// ("tls.reality.public_key"), поэтому связи считаются по корню, а не по
	// соседям: поиск по последнему сегменту схлопнул бы tls.ech.enabled и
	// tls.reality.enabled в один «enabled» и снимал бы reality на ровном месте.
	srcRoot   map[string]interface{}
	cleanRoot map[string]interface{}
	// removed — пути, снятые запретом по схеме (allowed_for/forbidden_for).
	//
	// Связи (conflicts/requires) обязаны считать такое поле
	// ОТСУТСТВУЮЩИМ: оно уже снято, и конфликтовать с ним не с чем. Иначе у
	// naive снятый запретом certificate_public_key_sha256 продолжал бы
	// «конфликтовать» с certificate_path — и узел терял бы СВОЙ сертификат
	// из-за поля, которого в теле не будет (ровно LxBox #140, только с
	// другой стороны).
	removed map[string]bool
	// absent — пути объектов, снятых правилом `absent_when` реестра.
	//
	// Отдельный набор, а не общий с `removed`: смысл разный и его стоит
	// различать при чтении. `removed` — «поле схеме запрещено, о нём сообщили
	// кодом», `absent` — «настройки нет вовсе, и сказать тут нечего». Для
	// связей и условий оба означают отсутствие, поэтому проверяются парой.
	absent map[string]bool
	// missingRequired — обязательные поля, не пережившие обход текущего
	// объекта. Накопитель, а не флаг: один объект может недосчитаться
	// нескольких полей, и сообщить надо про каждое.
	missingRequired []missingRequired
}

// Sanitize приводит карту тела узла к правилам реестра для схемы scheme.
//
// Вход при этом считается НЕизвестным — см. SanitizeFrom. Звать так можно
// там, где тело действительно ниоткуда не приехало (форма редактора правит
// уже готовый узел), и нельзя на входах импорта: правило с `except_sources`
// сработает основной формой.
func Sanitize(scheme string, m map[string]interface{}) Result {
	return SanitizeFrom(scheme, "", m)
}

// SanitizeFrom — то же с явным ВХОДОМ (константы Source*).
//
// Вход — часть условия у правил, которые различают, кто сочинил значение:
// тело в форме ядра (`singbox`) писал человек или подписка напрямую, и
// переписывать его молча лаунчер не вправе — он предупреждает. Значение из
// ссылки или .conf собрал генератор провайдера, и там правило работает
// заменой (решение владельца 18.09.2026, wireguard.mtu у AmneziaWG).
//
// Неизвестная схема — не повод молча пропустить мусор: тело возвращается
// пустым с кодом уровня узла protocol_unsupported.
func SanitizeFrom(scheme, source string, m map[string]interface{}) Result {
	return SanitizeFromKind(scheme, source, "", m)
}

// SanitizeFromKind — то же с явным РОДОМ узла (`kind_when` маппера).
//
// Род отвечает на вопрос «какой протокол просил ВХОД», а не «что уцелело в
// теле», и нужен правилам, чей `any_set` по телу бессилен по построению:
// ссылка `awg://` с негодными awg-значениями оставляет тело, неотличимое от
// обычного WireGuard, — а потолок MTU есть свойство запрошенного протокола
// (contract/docs/MAPPER_ENGINE.md, «Контекст санитайзера: body_source + kind»).
//
// Пустой род — законное «неизвестен»: у входа `singbox` рода от входа нет
// вовсе, у старого состояния он не сохранён. Правило тогда работает своим
// парным `any_set`, и поведение остаётся прежним.
func SanitizeFromKind(scheme, source, kind string, m map[string]interface{}) Result {
	reg, err := registry.Get()
	if err != nil {
		return Result{
			Clean: map[string]interface{}{},
			Drop:  &Warning{Code: "parse_error", Params: map[string]string{"error": err.Error()}},
		}
	}
	body, ok := reg.Body(scheme)
	if !ok {
		return Result{
			Clean: map[string]interface{}{},
			Drop: &Warning{
				Code:   "protocol_unsupported",
				Params: map[string]string{"scheme": scheme},
			},
		}
	}
	s := &sanitizer{reg: reg, scheme: scheme, source: source, kind: kind, seen: map[string]bool{}, srcRoot: m, removed: map[string]bool{}, absent: map[string]bool{}}
	s.cleanRoot = map[string]interface{}{}
	s.res.Clean = s.cleanRoot
	// Запреты по схеме размечаются ДО обхода, а не по ходу: связи
	// (conflicts/requires) читают исходную карту, а поле, запрещённое схеме,
	// может стоять в body.order ПОЗЖЕ того, с кем оно конфликтует. У naive
	// так и вышло: certificate_path (order 11) конфликтовал с
	// certificate_public_key_sha256 (order 12), который сам запрещён naive и
	// до тела не доедет, — и узел терял собственный сертификат из-за поля,
	// которого в теле не будет.
	s.markSchemeForbidden("", body.Order, body.Fields, m)
	// «Объект не задан» размечается тем же предварительным проходом и по той
	// же причине: tls:{enabled:false} для ядра значит «TLS нет», и объекта,
	// которого нет, не должны видеть НИ правила его полей, НИ связи соседей
	// (CANON §6.1). Проход идёт после запретов по схеме: поле, запрещённое
	// схеме, снято раньше и внутрь него заглядывать незачем.
	s.markAbsentObjects("", body.Order, body.Fields, m)
	s.object("", body.Order, body.Fields, m)
	// Связи МЕЖДУ НЕСКОЛЬКИМИ полями считаются после обхода: они смотрят на
	// готовое тело целиком, а не на соседей одного поля, и до конца обхода
	// половина участников ещё не приведена к типу.
	s.relations(body.Relations)
	// Обязательное поле КОРНЯ, не пережившее обход, — это отсутствующий узел:
	// ядро отвергнет такое тело фаталом на весь конфиг. Вложенные объекты
	// свои отметки уже разобрали сами (objectField), поэтому здесь остаются
	// только корневые.
	if len(s.missingRequired) > 0 && s.res.Drop == nil {
		first := s.missingRequired[0]
		// Код отказа — тот, которым о поле УЖЕ сообщили: если значение снял
		// ss_method_invalid, отказ по узлу зовут так же. Иначе пользователь
		// прочёл бы в списке один код, а в причине отбраковки другой.
		if w := s.warningFor(first.Path); w != nil {
			d := *w
			s.res.Drop = &d
		} else {
			s.res.Drop = &Warning{
				Code:   first.Code,
				Path:   first.Path,
				Params: s.declaredParams(first.Code, map[string]string{"field": first.Path, "path": first.Path}),
			}
		}
	}
	return s.res
}

// markSchemeForbidden помечает снятыми все пути, запрещённые текущей схеме,
// чтобы связи полей их не видели. Коды при этом НЕ ставятся — их поставит
// обход, в своём порядке.
func (s *sanitizer) markSchemeForbidden(prefix string, order []string, fields map[string]*registry.Field, src map[string]interface{}) {
	if src == nil {
		return
	}
	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		raw, present := src[name]
		if !present {
			continue
		}
		path := joinPath(prefix, name)
		if !s.allowedForScheme(f) {
			s.removed[path] = true
			continue
		}
		inner, ok := asObject(raw)
		if !ok {
			continue
		}
		subOrder, subFields := emitShape(f, inner)
		if subOrder == nil {
			continue
		}
		s.markSchemeForbidden(path, subOrder, subFields, inner)
	}
}

// markAbsentObjects помечает объекты, которые реестр велит считать НЕ
// ЗАДАННЫМИ (`absent_when`), и делает это ДО обхода.
//
// Смысл правила — у ядра: `tls: {enabled: false}` значит «TLS не задан»
// (конструктор возвращает (nil, nil)), а не «TLS с выключенным флагом»; тем же
// атрибутом описаны вложенные utls / reality / ech со своим `enabled: false`.
// Объект снимается ЦЕЛИКОМ и ТИХО: это запись «настройки нет», а не деградация,
// и сообщать человеку нечего.
//
// Почему проход отдельный и предварительный (норма порядка, CANON §6.1):
// объекта, которого нет, не должны видеть ни правила его собственных полей, ни
// связи соседей. Иначе `tls: {enabled: false, reality: {…}}` дал бы коды на
// поля несуществующего блока, а сосед потерял бы своё значение из-за конфликта
// с блоком, которого в теле не будет.
//
// Проход рекурсивный и общий: имён протоколов и секций он не знает — обходит
// то, что описал реестр, и снимает то, на что реестр поставил атрибут.
func (s *sanitizer) markAbsentObjects(prefix string, order []string, fields map[string]*registry.Field, src map[string]interface{}) {
	if src == nil {
		return
	}
	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		raw, present := src[name]
		if !present {
			continue
		}
		path := joinPath(prefix, name)
		if s.removed[path] {
			// Поле уже снято запретом по схеме — заглядывать внутрь незачем.
			continue
		}
		inner, ok := asObject(raw)
		if !ok {
			continue
		}
		if objectIsAbsent(f, inner) {
			s.absent[path] = true
			// Внутрь снятого объекта не спускаемся: его вложенные блоки для
			// тела узла уже не существуют, и отмечать их по отдельности
			// нечего — проверки наличия остановятся на самом объекте.
			continue
		}
		subOrder, subFields := emitShape(f, inner)
		if subOrder == nil {
			continue
		}
		s.markAbsentObjects(path, subOrder, subFields, inner)
	}
}

// objectIsAbsent — совпал ли объект с условием `absent_when` своего поля.
//
// Совпасть обязаны ВСЕ перечисленные ключи: условие описывает форму записи
// «настройки нет», а не набор подозрительных признаков.
//
// Сравнение — по печатной форме скаляра (как у `values` и `advisory`): тело
// приезжает и разбором JSON, и от маппера, где булев флаг бывает строкой, и
// `false` с `"false"` здесь одно и то же. Приведение типов тут ещё не
// делалось — до него дело не дойдёт вовсе, объект снимается раньше.
func objectIsAbsent(f *registry.Field, m map[string]interface{}) bool {
	if len(f.AbsentWhen) == 0 {
		return false
	}
	for key, want := range f.AbsentWhen {
		got, ok := m[key]
		if !ok || !sameValue(want, got) {
			return false
		}
	}
	return true
}

// warn кладёт код в накопитель, снимая дубли по (code, path).
func (s *sanitizer) warn(code, path string, value interface{}, secret bool, params map[string]string) {
	if code == "" {
		return
	}
	key := code + "\x00" + path
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	w := Warning{Code: code, Path: path, Params: s.declaredParams(code, params)}
	if secret {
		w.Value = maskedValue
	} else if value != nil {
		w.Value = configtypes.TruncateWarningValue(displayValue(value))
	}
	s.res.Warnings = append(s.res.Warnings, w)
}

// declaredParams оставляет только те подстановки, которые код объявил в
// реестре (`params` в warnings.json).
//
// Параметры существуют ради шаблона текста (`{path}`, `{value}`, `{with}`) —
// всё сверх него шум, а этот шум пишется в state.json и в бэкап каждого узла.
// Живой пример: field_missing объявляет один `field`, а звался с парой
// field+path, где path дословно повторял поле рядом.
//
// Код, которого в реестре нет (его ещё не завели), параметры сохраняет: терять
// данные из-за неполноты словаря нельзя.
func (s *sanitizer) declaredParams(code string, params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	entry, ok := s.reg.Warning(code)
	if !ok || entry == nil || len(entry.Params) == 0 {
		return params
	}
	out := make(map[string]string, len(entry.Params))
	for _, name := range entry.Params {
		if v, has := params[name]; has {
			out[name] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// dropNode фиксирует отказ по узлу. Первый отказ побеждает: дальнейший обход
// нужен только чтобы собрать остальные коды для диагностики.
func (s *sanitizer) dropNode(code, path string, value interface{}, secret bool, params map[string]string) {
	s.warn(code, path, value, secret, params)
	if s.res.Drop != nil {
		return
	}
	d := Warning{Code: code, Path: path, Params: s.declaredParams(code, params)}
	if secret {
		d.Value = maskedValue
	} else if value != nil {
		d.Value = configtypes.TruncateWarningValue(displayValue(value))
	}
	s.res.Drop = &d
}

// requiredFailed фиксирует, что обязательное поле по пути path не собралось.
//
// Кого при этом хоронить, решает НЕ это место, а уровень объекта, которому
// поле принадлежит:
//
//   - поле корня тела (server, uuid, method) — узла нет, ядро отвергнет его
//     фаталом; отказ по узлу;
//   - поле внутри НЕОБЯЗАТЕЛЬНОГО объекта (obfs.type, obfs.password) — «если
//     obfs задан, в нём обязан быть тип». Узел без обфускации работает, и
//     хоронить его за неё нельзя: коды obfs_unknown и obfs_password_missing
//     объявлены в реестре с severity `warning`, а warning на отброшенном узле
//     стоять не может (CANON §4, ловушка Л11). Такой объект снимается целиком,
//     узел живёт.
//
// Поэтому здесь только отметка, а разбирает её objectField (для вложенного
// объекта) либо Sanitize (для корня).
func (s *sanitizer) requiredFailed(path, code string) {
	s.warn(code, path, nil, false, map[string]string{"field": path, "path": path})
	s.missingRequired = append(s.missingRequired, missingRequired{Path: path, Code: code})
}

// warningFor — уже поставленный код по этому пути, если он есть.
func (s *sanitizer) warningFor(path string) *Warning {
	for i := range s.res.Warnings {
		if s.res.Warnings[i].Path == path {
			return &s.res.Warnings[i]
		}
	}
	return nil
}

// requiredFailedQuiet — то же для поля, которое УЖЕ получило код от правила
// значения: отметка ставится, второй код — нет.
func (s *sanitizer) requiredFailedQuiet(path, code string) {
	s.missingRequired = append(s.missingRequired, missingRequired{Path: path, Code: code})
}

// missingRequired — незаполненное обязательное поле и код, которым о нём
// сообщать.
type missingRequired struct {
	Path string
	Code string
}

// object обходит один уровень карты по order реестра и возвращает чистую
// карту этого уровня.
func (s *sanitizer) object(prefix string, order []string, fields map[string]*registry.Field, src map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	if prefix == "" && s.cleanRoot != nil {
		out = s.cleanRoot
	}
	if src == nil {
		src = map[string]interface{}{}
	}
	known := make(map[string]bool, len(fields))
	for name := range fields {
		known[name] = true
	}

	for _, name := range order {
		f := fields[name]
		if f == nil {
			continue
		}
		path := joinPath(prefix, name)
		raw, present := src[name]

		// Поля, которые пишет сборка конфига, в теле узла не живут: detour
		// вычисляется из Направлений и цепочек. Снимаем молча — это не
		// ошибка источника (SPEC 131 §3.2).
		if f.Managed {
			continue
		}
		// Объект, помеченный `absent_when`: реестр велел считать его НЕ
		// ЗАДАННЫМ. Снимаем молча и до всего остального — это запись «настройки
		// нет», а не ошибка источника. Разметку сделал предварительный проход
		// (markAbsentObjects), чтобы связи соседей тоже не видели объекта.
		if present && s.absent[path] {
			continue
		}
		if !s.allowedForScheme(f) {
			if present {
				s.warn(s.forbiddenCode(f), path, raw, f.Secret, map[string]string{"path": path})
				s.removed[path] = true
			}
			continue
		}
		if !present {
			// Дефолт, который реестр велит МАТЕРИАЛИЗОВАТЬ (default_when).
			// Обычный `default` сюда не попадает: дефолты ядра в тело не
			// пишутся (CANON §2.4). Сюда попадают только поля, без которых
			// ядро не собирает outbound вовсе — у hysteria v1 отсутствующий
			// up_mbps даёт «missing upload speed» фаталом на весь конфиг.
			if dw := f.DefaultWhen; dw != nil && dw.Absent && dw.Value != nil && s.conditionHolds(dw.When) {
				if v, ok := coerce(f, dw.Value); ok {
					out[name] = v
					if dw.Code != "" {
						s.warn(dw.Code, path, nil, false, map[string]string{"path": path})
					}
					continue
				}
			}
			if f.Required {
				s.requiredFailed(path, codeOr(f.Code, "field_missing"))
			}
			continue
		}
		v, ok := s.value(path, prefix, f, raw)
		if ok {
			if s.omitAsUnset(f, v) {
				// Пустая строка у обычного (не tristate) поля — это «не
				// задано», а не значение: ядро пишет такие поля с omitempty
				// и трактует пустое как отсутствующее (tls.server_name →
				// адрес сервера). Ключ с "" и отсутствие ключа для ядра
				// одно и то же, а в теле узла они дают РАЗНЫЕ байты — и
				// разошлись бы с эталонами корпуса на ровном месте.
				// У tristate-полей (packet_encoding, strip_evasion) пустое
				// значимо, и там ключ остаётся (SPEC 131 §4).
				continue
			}
			out[name] = v
			continue
		}
		// Поле было, но не пережило проверки. Обязательное поле без
		// значения означает, что объект, которому оно принадлежит, собрать
		// нельзя; КАКОЙ объект при этом гибнет — тело узла или необязательная
		// секция внутри него, — решает вызывающий уровень (requiredFailed).
		//
		// Второго кода на том же пути не ставим: причину снятия уже назвало
		// правило значения (ss_method_invalid, obfs_unknown), и «а ещё поля
		// нет» добавило бы к ней шум, а не смысл.
		if f.Required {
			s.requiredFailedQuiet(path, codeOr(f.Code, "field_missing"))
		}
	}

	// Поле, не прошедшее условный минимум ОТСУТСТВИЕМ: ядро читает его как 0
	// (см. minWhenAbsent). Проход отдельный и ПОСЛЕ основного — условие
	// правила читает соседей, а они к этому моменту уже приведены.
	s.minWhenAbsent(order, fields, src)

	// Всё, чего нет в fields, — вне схемы: ядро отвергнет ключ и не запустит
	// весь конфиг, поэтому снимаем с кодом.
	for _, name := range sortedKeys(src) {
		if known[name] || (prefix == "" && buildManagedKeys[name]) {
			continue
		}
		path := joinPath(prefix, name)
		s.warn("unknown_key", path, src[name], false, map[string]string{"path": path})
	}
	return out
}

// omitAsUnset — пишем ли ключ с этим значением в тело.
//
// Пустая строка = «не задано» для всех полей, КРОМЕ двух видов:
//   - tristate: там пустое значение значимо (vless.packet_encoding "" —
//     «без инкапсуляции», а отсутствие ключа — дефолт xudp);
//   - required: ядро пишет такое поле всегда (server без omitempty), и
//     молчаливое исчезновение ключа сменило бы форму тела.
func (s *sanitizer) omitAsUnset(f *registry.Field, v interface{}) bool {
	if f.Tristate || f.Required {
		return false
	}
	str, ok := v.(string)
	return ok && str == ""
}

// forbiddenCode — код, которым сообщать о поле, снятом запретом по схеме.
//
// Один запрет, но разный ИСХОД для пользователя, и код обязан это разделять:
// у naive снятое TLS-поле — настройка, которой узел лишился (warning), а на
// QUIC-протоколах uTLS/REALITY не применились бы в принципе (ядро зовёт
// STDConfig(), а у uTLS- и REALITY-конфигов он возвращает ошибку) — снята
// бессмыслица, узел не пострадал, и код там info. Поэтому у поля кроме общего
// `code` есть `forbidden_codes` — словарь «схема → код»; схема без записи
// берёт общий.
func (s *sanitizer) forbiddenCode(f *registry.Field) string {
	if c, ok := f.ForbiddenCodes[s.scheme]; ok && c != "" {
		return c
	}
	return codeOr(f.Code, "unknown_key")
}

// allowedForScheme — разрешено ли поле текущей схеме (allowed_for/forbidden_for).
func (s *sanitizer) allowedForScheme(f *registry.Field) bool {
	for _, sc := range f.ForbiddenFor {
		if sc == s.scheme {
			return false
		}
	}
	if len(f.AllowedFor) > 0 {
		for _, sc := range f.AllowedFor {
			if sc == s.scheme {
				return true
			}
		}
		return false
	}
	return true
}

// value обрабатывает одно поле: связи с соседями, приведение типа, проверки
// значения. Второй результат false — поле снято.
//
// siblings — исходная карта уровня (для conflicts/requires по
// относительным путям), clean — уже собранная чистая карта того же уровня.
func (s *sanitizer) value(path, prefix string, f *registry.Field, raw interface{}) (interface{}, bool) {
	// Связи полей считаются до приведения типа: снятое поле не должно
	// тратить коды на собственный формат.
	if !s.relationsOK(path, prefix, f) {
		return nil, false
	}

	switch f.Type {
	case "object":
		return s.objectField(path, f, raw)
	case "array":
		return s.arrayField(path, f, raw)
	}

	v, ok := coerce(f, raw)
	if !ok {
		return s.onInvalid(path, f, raw)
	}
	// Литерал, означающий «этого нет»: проверяется ПОСЛЕ normalize и ДО
	// ограничений — иначе правило значения судило бы слово-выключатель как
	// значение и хоронило узел за выключенную настройку (vless.encryption:
	// `none` — собственный литерал ядра, выключающий слой, а не строка
	// грамматики). Ключ при этом не пишется вовсе, кода нет: сказать тут
	// нечего, «нет слоя» — это не деградация.
	if isAbsentValue(f, v) {
		return nil, false
	}
	if !s.constraintsOK(f, v) {
		return s.onInvalid(path, f, raw)
	}
	if !s.applyMinWhen(path, f, v) {
		return nil, false
	}
	v = s.applyMaxWhen(path, f, v)
	s.noteNormalized(path, f, raw, v)
	s.advisory(path, prefix, f, v)
	return v, true
}

// isAbsentValue — значение является литералом-выключателем (Field.AbsentValues).
//
// Сравнение ТОЧНОЕ и только для строк: смысл атрибута в том, чтобы повторить
// литерал ядра буква в букву. Регистронезависимое сравнение здесь было бы
// прямой ошибкой — ядро сличает свой `none` с учётом регистра, и `None` для
// него настоящее значение, на котором падает весь конфиг; спрятать его под
// видом «нет слоя» значит пропустить негодный узел в ядро.
func isAbsentValue(f *registry.Field, v interface{}) bool {
	if len(f.AbsentValues) == 0 {
		return false
	}
	str, ok := v.(string)
	if !ok {
		return false
	}
	for _, a := range f.AbsentValues {
		if as, ok := a.(string); ok && as == str {
			return true
		}
	}
	return false
}

// applyMaxWhen исполняет условный потолок значения (Field.MaxWhen).
//
// Потолок применяется ПОСЛЕ обычных проверок: значение, которое ядро не
// примет в принципе (mtu 9000 при max 1500), снимает on_invalid, и клампить
// там нечего.
//
// Два исхода, и разделяет их не техника, а владение телом:
//
//   - вход из `except_sources` (sing-box JSON) — тело написано в форме ядра
//     самим человеком или подпиской; значение остаётся как есть, узел
//     получает информационный код. Молча переписывать чужое осознанное
//     решение лаунчер не вправе;
//   - все прочие входы (ссылка, .conf, форма) — значение сочинил генератор
//     провайдера; оно заменяется потолком с кодом-предупреждением.
//
// Решение владельца 18.09.2026: парность входов здесь нарушена НАМЕРЕННО
// (DRIFT §7.23).
func (s *sanitizer) applyMaxWhen(path string, f *registry.Field, v interface{}) interface{} {
	mw := f.MaxWhen
	if mw == nil || !s.conditionHolds(mw.When) {
		return v
	}
	n, ok := numericValue(v)
	if !ok || n <= mw.Max {
		return v
	}
	params := map[string]string{"path": path, "value": displayValue(v)}
	if s.sourceExcepted(mw.ExceptSources) {
		// Значение остаётся; человеку сообщают, чем это грозит.
		s.warn(mw.NoteCode, path, v, f.Secret, params)
		return v
	}
	s.warn(mw.Code, path, v, f.Secret, params)
	capped, ok := coerce(f, mw.Max)
	if !ok {
		return v
	}
	return capped
}

// applyMinWhen исполняет условный минимум значения (Field.MinWhen).
//
// Зеркало applyMaxWhen, но без исключения по входу и БЕЗ замены значения:
// порог здесь стоит не ради качества связи, а ради того, примет ли ядро
// конфиг вообще. Подставить минимум вместо написанного значило бы выдумать
// за провайдера размер паддинга, от которого зависит рукопожатие.
//
// Второй результат false — значение порог не прошло и поле снято (либо узел
// похоронен, если правило так велит).
func (s *sanitizer) applyMinWhen(path string, f *registry.Field, v interface{}) bool {
	mw := f.MinWhen
	if mw == nil || !s.conditionHolds(mw.When) {
		return true
	}
	n, ok := numericValue(v)
	if !ok || n >= mw.Min {
		return true
	}
	params := map[string]string{
		"path":  path,
		"field": path,
		"min":   displayValue(mw.Min),
	}
	if !f.Secret {
		params["value"] = displayValue(v)
	}
	if mw.Action == "drop_node" {
		s.dropNode(mw.Code, path, v, f.Secret, params)
	} else {
		s.warn(mw.Code, path, v, f.Secret, params)
	}
	return false
}

// minWhenAbsent разбирает поля, которые порога не прошли ОТСУТСТВИЕМ.
//
// Ядро читает незаданный s2 как 0, поэтому пара «ключ защиты заголовков
// задан + паддинга нет» так же фатальна, как «ключ + паддинг 5»: конфиг не
// загрузится целиком. Обычный обход такое поле не видит вовсе (нет ключа —
// нет и проверок), и без отдельного прохода правило молчало бы ровно на том
// случае, который в живых подписках встречается чаще битого значения.
func (s *sanitizer) minWhenAbsent(order []string, fields map[string]*registry.Field, src map[string]interface{}) {
	for _, name := range order {
		f := fields[name]
		if f == nil || f.MinWhen == nil || !f.MinWhen.AbsentIsZero {
			continue
		}
		if _, present := src[name]; present {
			continue
		}
		if f.MinWhen.Min <= 0 || !s.conditionHolds(f.MinWhen.When) {
			continue
		}
		params := map[string]string{
			"path":  name,
			"field": name,
			"min":   displayValue(f.MinWhen.Min),
			"value": "0",
		}
		if f.MinWhen.Action == "drop_node" {
			s.dropNode(f.MinWhen.Code, name, nil, false, params)
		} else {
			s.warn(f.MinWhen.Code, name, nil, false, params)
		}
	}
}

// relations исполняет связи МЕЖДУ НЕСКОЛЬКИМИ полями тела (Relation2).
//
// Неизвестный вид связи пропускается молча: реестр вправе уехать вперёд
// кода, и правило, которого исполнитель ещё не знает, не должно ронять узлы.
func (s *sanitizer) relations(rels []registry.Relation2) {
	for i := range rels {
		switch rels[i].Kind {
		case "ranges_disjoint":
			s.rangesDisjoint(&rels[i])
		case "cooccurrence":
			s.cooccurrence(&rels[i])
		}
	}
}

// cooccurrence — сочетание полей, о котором человеку стоит знать.
//
// Отличие от rangesDisjoint: тот судит набор по ОДНОМУ свойству (попарная
// непересекаемость), этот — по разнородному условию `when` над разными
// полями. У AmneziaWG 3.x это `random_trailers` вместе с широким диапазоном
// magic-заголовков: ни одно из полей не битое, менять нечего, узел живёт —
// сообщается лишь цена сочетания (сервер референсной реализации принимает
// часть исходящих пакетов за хендшейки и отбрасывает их).
//
// Условие читается по ЧИСТОЙ карте: поле, снятое санитайзером за негодное
// значение, в тело не поедет, и судить по нему сочетание значило бы винить
// настройку, которой не будет (тот же довод, что у rangesDisjoint).
func (s *sanitizer) cooccurrence(rel *registry.Relation2) {
	if len(rel.When) == 0 {
		// Связь без условия сработала бы на каждом узле, где поля просто
		// есть. Это заведомо не то, что имел в виду реестр, — молчим.
		return
	}
	for key, want := range rel.When {
		if !s.relationWhenHolds(key, want) {
			return
		}
	}
	if rel.Action == "drop_node" {
		s.dropNode(rel.Code, rel.Paths[0], nil, false, nil)
		return
	}
	s.warn(rel.Code, rel.Paths[0], nil, false, nil)
}

// relationWhenHolds — одно звено условия связи.
//
// Служебный оператор отличается ведущим `$` и потому с путём тела не
// сталкивается. Неизвестный оператор считается НЕвыполненным: реестр вправе
// уехать вперёд кода, и правило, которого исполнитель не знает, не должно
// ставить код наугад.
func (s *sanitizer) relationWhenHolds(key string, want interface{}) bool {
	if strings.HasPrefix(key, "$") {
		if key == "$range_width" {
			return s.rangeWidthHolds(want)
		}
		return false
	}
	v, ok := lookupPath(s.cleanRoot, strings.Split(key, "."))
	if !ok {
		return false
	}
	// Сравнение по печатной форме скаляра, как в absent_when: тело приезжает
	// и из JSON, и от маппера, где булев флаг бывает строкой.
	return fmt.Sprintf("%v", v) == fmt.Sprintf("%v", want)
}

// rangeWidthHolds — `{paths: [...], gt: N}`: ХОТЯ БЫ у одного из полей
// значение записано диапазоном «lo-hi» и hi-lo > N.
//
// Поле-ЧИСЛО ширины не имеет и условие не выполняет: у AmneziaWG одиночный
// magic-заголовок — это ровно одно значение, и ложных срабатываний
// классификатора он не даёт.
func (s *sanitizer) rangeWidthHolds(want interface{}) bool {
	spec, ok := want.(map[string]interface{})
	if !ok {
		return false
	}
	gt, ok := numericValue(spec["gt"])
	if !ok {
		return false
	}
	paths, ok := spec["paths"].([]interface{})
	if !ok {
		return false
	}
	for _, p := range paths {
		name, ok := p.(string)
		if !ok {
			continue
		}
		v, ok := lookupPath(s.cleanRoot, strings.Split(name, "."))
		if !ok {
			continue
		}
		// Диапазоном считается только запись «lo-hi»: rangeBounds отдаёт
		// lo==hi и для одиночного числа, а у него ширины нет.
		str, isStr := v.(string)
		if !isStr || !strings.Contains(str, "-") {
			continue
		}
		lo, hi, parsed := rangeBounds(v)
		if !parsed || hi < lo {
			continue
		}
		if hi-lo > gt {
			return true
		}
	}
	return false
}

// rangesDisjoint — попарно непересекающиеся диапазоны перечисленных полей.
//
// Смысл у AmneziaWG: h1..h4 — это magic-заголовки, по которым ядро РАЗЛИЧАЕТ
// типы сообщений. Пересечение двух диапазонов оставляет его без способа
// различать, и оно отвергает endpoint на конфигурировании устройства
// («headers must not overlap», submodules/wireguard-go/device/uapi.go) — то
// есть падает ВЕСЬ конфиг, а не один узел.
//
// Незаданное поле участвует своим дефолтом ядра (h1=1 … h4=4): «ключа нет»
// здесь не значит «участника нет», иначе h1=2 рядом с незаданным h2 прошло бы
// проверку и уронило конфиг на старте.
func (s *sanitizer) rangesDisjoint(rel *registry.Relation2) {
	type span struct {
		name   string
		lo, hi float64
		known  bool
	}
	spans := make([]span, 0, len(rel.Paths))
	for i, p := range rel.Paths {
		sp := span{name: p}
		if i < len(rel.Defaults) {
			sp.lo, sp.hi, sp.known = rel.Defaults[i], rel.Defaults[i], true
		}
		// Читаем ТОЛЬКО чистую карту. Заглядывать в исходную тут нельзя:
		// поле, которое санитайзер уже снял за негодное значение, в тело не
		// поедет, и ядро прочтёт вместо него свой дефолт. Пока связь читала
		// исходник, снятый h1=«1-4294967296» продолжал «пересекаться» с
		// соседями и хоронил узел кодом awg_headers_overlap вместо честного
		// awg_header_invalid на самом поле — вина уезжала не на того.
		parts := strings.Split(p, ".")
		if v, ok := lookupPath(s.cleanRoot, parts); ok {
			if lo, hi, parsed := rangeBounds(v); parsed {
				sp.lo, sp.hi, sp.known = lo, hi, true
			}
		}
		if sp.known {
			spans = append(spans, sp)
		}
	}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[i].lo > spans[j].hi || spans[j].lo > spans[i].hi {
				continue
			}
			params := map[string]string{"a": spans[i].name, "b": spans[j].name}
			if rel.Action == "drop_node" {
				s.dropNode(rel.Code, spans[i].name, nil, false, params)
			} else {
				s.warn(rel.Code, spans[i].name, nil, false, params)
			}
			return
		}
	}
}

// rangeBounds — границы значения типа awg_range: число даёт [n,n], строка
// "lo-hi" — свои границы, голое число строкой — [n,n].
func rangeBounds(v interface{}) (float64, float64, bool) {
	if n, ok := numericValue(v); ok {
		return n, n, true
	}
	str, ok := v.(string)
	if !ok {
		return 0, 0, false
	}
	str = strings.TrimSpace(str)
	loStr, hiStr, isRange := strings.Cut(str, "-")
	if !isRange {
		n, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0, 0, false
		}
		return n, n, true
	}
	lo, errLo := strconv.ParseFloat(strings.TrimSpace(loStr), 64)
	hi, errHi := strconv.ParseFloat(strings.TrimSpace(hiStr), 64)
	if errLo != nil || errHi != nil {
		return 0, 0, false
	}
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo, hi, true
}

// sourceExcepted — входит ли текущий вход в список исключений правила.
//
// Пустой вход («неизвестно, откуда тело») исключением НЕ считается: молчать
// про завышенный MTU только потому, что вход не назвали, значит терять
// правило на каждом месте, куда его забыли протянуть.
func (s *sanitizer) sourceExcepted(sources []string) bool {
	if s.source == "" {
		return false
	}
	for _, src := range sources {
		if src == s.source {
			return true
		}
	}
	return false
}

// conditionHolds — выполнено ли условие применимости правила.
//
// nil-условие = правило безусловно (так работает default_when у hysteria
// up_mbps). `any_set` — задан любой из перечисленных КЛЮЧЕЙ; пути читаются от
// КОРНЯ тела, как и у conflicts/requires.
//
// Считается НАЛИЧИЕ ключа, а не непустота значения (в отличие от
// conflicts/requires, где ядро судит по значению). Разница не косметическая:
// `jc: 0` — законная запись «мусорные пакеты выключены» у настоящего
// AmneziaWG-узла (кейс корпуса awg_jc_zero_explicit), и прочитать её как
// «поля нет» значило бы снять с такого узла потолок MTU и вернуть ему ровно
// ту тихую поломку, от которой правило заведено.
func (s *sanitizer) conditionHolds(c *registry.Condition) bool {
	if c == nil || (len(c.AnySet) == 0 && len(c.SourceKind) == 0) {
		return true
	}
	// Род узла объявил ВХОД, и это ИЛИ-ветка условия, а не отдельное правило:
	// ссылка `awg://` с негодными awg-значениями оставляет тело, неотличимое
	// от обычного WireGuard, и any_set по телу здесь бессилен по построению.
	// Пустой род (вход `singbox`, старое состояние) ветку просто не проходит —
	// решает парный any_set ниже.
	if s.kind != "" {
		for _, k := range c.SourceKind {
			if k == s.kind {
				return true
			}
		}
	}
	for _, p := range c.AnySet {
		if s.gone(p) {
			// Поле снято запретом по схеме либо лежит в объекте, который
			// реестр объявил незаданным, — для условий его нет.
			continue
		}
		parts := strings.Split(p, ".")
		if _, ok := lookupPath(s.cleanRoot, parts); ok {
			return true
		}
		if _, ok := lookupPath(s.srcRoot, parts); ok {
			return true
		}
	}
	return false
}

// numericValue — число из приведённого значения, если оно число.
func numericValue(v interface{}) (float64, bool) {
	switch vv := v.(type) {
	case int:
		return float64(vv), true
	case int64:
		return float64(vv), true
	case float64:
		return vv, true
	}
	return 0, false
}

// noteNormalized сообщает, что normalize ВЫБРОСИЛ часть значения.
//
// Обрезка пробелов и смена регистра деградацией не считаются: ядро читает
// такое значение одинаково, и код на каждой второй ноде был бы шумом.
// Сравнение поэтому идёт с уже обрезанной и приведённой формой — код ставится
// только там, где исчезли символы (hex_only на short_id: `0x1a2` → `01a2` —
// ДРУГОЙ идентификатор, и сервер такой узел не узнает).
func (s *sanitizer) noteNormalized(path string, f *registry.Field, raw, v interface{}) {
	if f.NormalizeCode == "" {
		return
	}
	before, ok := raw.(string)
	if !ok {
		return
	}
	after, ok := v.(string)
	if !ok || after == strings.ToLower(strings.TrimSpace(before)) {
		return
	}
	params := map[string]string{"path": path}
	if !f.Secret {
		params["value"] = displayValue(before)
	}
	s.warn(f.NormalizeCode, path, before, f.Secret, params)
}

// objectField обрабатывает вложенный объект: по варианту дискриминатора
// (transport), по описанным полям или как свободную карту (headers).
func (s *sanitizer) objectField(path string, f *registry.Field, raw interface{}) (interface{}, bool) {
	m, ok := asObject(raw)
	if !ok {
		return s.onInvalid(path, f, raw)
	}
	if len(f.Variants) > 0 {
		return s.variantObject(path, f, m)
	}
	if len(f.Fields) == 0 {
		// Свободная карта (headers, extra_headers): значения не описаны,
		// решать нечего — отдаём как есть.
		return m, true
	}
	mark := len(s.missingRequired)
	out := s.object(path, f.Order, f.Fields, m)
	if missing := s.missingRequired[mark:]; len(missing) > 0 {
		// Внутри объекта не собралось обязательное поле. Объект снимается
		// ЦЕЛИКОМ, а узел живёт: «obfs без типа» означает «узел без
		// обфускации», а не «узла нет» (см. requiredFailed). Коды уже
		// поставлены, второй раз о том же не сообщаем.
		s.missingRequired = s.missingRequired[:mark]
		return nil, false
	}
	// Атрибут all_or_nothing санитайзер НЕ отрабатывает: ядро при частично
	// заданной секции оставляет незаданные поля нулями (= без лимита), и
	// дописывать дефолты соседей нельзя — это меняет поведение живого узла
	// (случай transport.xmux, SPEC 131 DRIFT §7). Атрибут остаётся в реестре
	// как документация о поведении ядра.
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// variantObject выбирает вариант по значению дискриминатора и обходит его
// поля. Неизвестное значение = ядро не стартует, поэтому блок снимается
// целиком с кодом на самом дискриминаторе.
func (s *sanitizer) variantObject(path string, f *registry.Field, m map[string]interface{}) (interface{}, bool) {
	discPath := joinPath(path, f.Discriminator)
	rawDisc, present := m[f.Discriminator]
	if !present {
		s.warn("field_missing", discPath, nil, false, map[string]string{"field": discPath})
		return nil, false
	}
	disc, ok := rawDisc.(string)
	if !ok {
		s.warn("type_invalid", discPath, rawDisc, false, map[string]string{"path": discPath})
		return nil, false
	}
	variant := f.Variants[strings.TrimSpace(disc)]
	if variant == nil {
		s.warn("type_invalid", discPath, disc, false, map[string]string{"path": discPath})
		return nil, false
	}
	rest := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == f.Discriminator {
			continue
		}
		rest[k] = v
	}
	out := s.object(path, variant.Order, variant.Fields, rest)
	out[f.Discriminator] = strings.TrimSpace(disc)
	return out, true
}

// arrayField обходит массив объектов (wireguard.peers).
func (s *sanitizer) arrayField(path string, f *registry.Field, raw interface{}) (interface{}, bool) {
	list, ok := asSlice(raw)
	if !ok || f.Items == nil {
		return s.onInvalid(path, f, raw)
	}
	out := make([]interface{}, 0, len(list))
	for i, item := range list {
		itemPath := path + "[" + strconv.Itoa(i) + "]"
		if len(f.Items.Fields) == 0 {
			out = append(out, item)
			continue
		}
		m, ok := asObject(item)
		if !ok {
			s.warn("type_invalid", itemPath, item, false, map[string]string{"path": itemPath})
			continue
		}
		out = append(out, s.object(itemPath, f.Items.Order, f.Items.Fields, m))
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// relationsOK проверяет conflicts / requires.
//
// Конфликт снимает ТЕКУЩЕЕ поле (младшее по order): к моменту проверки
// старший сосед уже прошёл обход и лежит в clean.
func (s *sanitizer) relationsOK(path, prefix string, f *registry.Field) bool {
	for _, c := range f.Conflicts {
		if c.With == "" {
			continue
		}
		if !s.pathPresent(c.With, prefix) {
			continue
		}
		s.warn(codeOr(c.Code, "field_conflict"), path, nil, false,
			map[string]string{"path": path, "with": c.With})
		return false
	}
	for _, rq := range f.Requires {
		if rq.Path == "" {
			continue
		}
		if rq.Equals != nil {
			// Требование по ЗНАЧЕНИЮ соседа: поле осмыслено только при
			// таком-то варианте (границы размера пакета — только у gecko).
			// Ядро лишнее поле молча игнорирует, но в теле узла оно ломает
			// сравнение с тем же узлом, пришедшим другим входом.
			if s.pathEquals(rq.Path, prefix, rq.Equals) {
				continue
			}
		} else if s.pathPresent(rq.Path, prefix) {
			continue
		}
		s.warn(codeOr(rq.Code, "field_requires"), path, nil, false,
			map[string]string{"path": path, "requires": rq.Path})
		return false
	}
	// Ветка forbidden_when снята вместе с атрибутом (контракт 1.1.4): его не
	// несло ни одно поле реестра и не знала схема. Обратная связь («поле
	// запрещено, когда сосед задан») выражается `conflicts` у того же поля.
	return true
}

// pathPresent — есть ли по пути непустое значение.
//
// Путь реестра пишется от корня тела ("tls.reality.public_key"); ищем его в
// исходной карте и в уже собранной чистой (второе важно для requires: к
// моменту проверки старшее поле обхода уже приведено). Запасной вариант —
// путь относительно текущего объекта: так в реестре записаны связи внутри
// вариантов транспорта ("transport.xmux.max_connections" при обходе
// transport.xmux).
func (s *sanitizer) pathPresent(path, prefix string) bool {
	if s.lookupNonEmpty(path) {
		return true
	}
	if prefix != "" {
		// "transport.mode" при обходе внутри transport — тот же ключ.
		if i := strings.Index(path, "."); i >= 0 {
			head := path[:i]
			if head == prefix || strings.HasSuffix(prefix, "."+head) {
				return s.lookupNonEmpty(joinPath(prefix, path[i+1:]))
			}
		}
		return s.lookupNonEmpty(joinPath(prefix, path))
	}
	return false
}

// pathEquals — равно ли значение по пути ожидаемому.
//
// Читается из ЧИСТОЙ карты, если поле уже обошли, иначе из исходной: у
// дискриминатора объекта (obfs.type) обход идёт раньше зависимых полей, и
// его приведённое значение уже готово.
func (s *sanitizer) pathEquals(path, prefix string, want interface{}) bool {
	candidates := []string{path}
	if prefix != "" {
		if i := strings.Index(path, "."); i >= 0 {
			head := path[:i]
			if head == prefix || strings.HasSuffix(prefix, "."+head) {
				candidates = append(candidates, joinPath(prefix, path[i+1:]))
			}
		}
		candidates = append(candidates, joinPath(prefix, path))
	}
	for _, p := range candidates {
		if s.gone(p) {
			continue
		}
		parts := strings.Split(p, ".")
		if v, ok := lookupPath(s.cleanRoot, parts); ok {
			return sameValue(want, v)
		}
		if v, ok := lookupPath(s.srcRoot, parts); ok {
			return sameValue(want, v)
		}
	}
	return false
}

func (s *sanitizer) lookupNonEmpty(path string) bool {
	if s.gone(path) {
		// Поле снято запретом по схеме или объявлено незаданным
		// (`absent_when`) — для связей его нет.
		return false
	}
	parts := strings.Split(path, ".")
	if v, ok := lookupPath(s.cleanRoot, parts); ok {
		return !isEmptyValue(v)
	}
	if v, ok := lookupPath(s.srcRoot, parts); ok {
		return !isEmptyValue(v)
	}
	return false
}

// gone — «для проверок наличия этого пути в теле нет».
//
// Два источника: поле, снятое запретом по схеме (`removed`), и объект,
// объявленный незаданным правилом `absent_when` (`absent`). Второй забирает с
// собой и ВСЁ, что внутри: у выключенного блока `tls` пути
// `tls.reality.enabled` для связей не существует — иначе `tls.ech.enabled`
// продолжал бы конфликтовать с REALITY, которого в теле не будет.
//
// Префикс сверяется по сегменту пути, а не подстрокой: иначе `tls_fragment`
// исчезал бы вместе с `tls`.
func (s *sanitizer) gone(path string) bool {
	if s.removed[path] || s.absent[path] {
		return true
	}
	for p := range s.absent {
		if strings.HasPrefix(path, p+".") {
			return true
		}
	}
	return false
}

func lookupPath(m map[string]interface{}, parts []string) (interface{}, bool) {
	cur := interface{}(m)
	for _, p := range parts {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = obj[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// advisory ставит информационный код на значении, которое ядро принимает, но
// человеку о нём знать стоит (legacy-шифры shadowsocks, отпечаток без
// гибридного шара под REALITY).
//
// Три формы правила, в порядке разбора:
//
//   - `values` — код на перечисленных значениях;
//   - `except` — код на ЛЮБОМ значении, кроме перечисленных (правило вида
//     «все, кто не умеет X»); пустое значение под него не попадает — это
//     «не задано», а не выбор автора ссылки;
//   - `when` — условие по соседнему полю: без него правило сработало бы у
//     узлов, к которым отношения не имеет.
func (s *sanitizer) advisory(path, prefix string, f *registry.Field, v interface{}) {
	for _, a := range f.Advisory {
		if !advisoryValueMatches(a, v) {
			continue
		}
		if a.When != nil && a.When.Path != "" {
			want := true
			if a.When.Present != nil {
				want = *a.When.Present
			}
			if s.pathPresent(a.When.Path, prefix) != want {
				continue
			}
		}
		params := map[string]string{"path": path}
		if !f.Secret {
			params["value"] = displayValue(v)
		}
		s.warn(a.Code, path, v, f.Secret, params)
		return
	}
}

// advisoryValueMatches — подходит ли значение под values/except правила.
func advisoryValueMatches(a registry.Advisory, v interface{}) bool {
	for _, want := range a.Values {
		if sameValue(want, v) {
			return true
		}
	}
	if len(a.Except) == 0 {
		return false
	}
	if str, ok := v.(string); ok && str == "" {
		return false
	}
	for _, skip := range a.Except {
		if sameValue(skip, v) {
			return false
		}
	}
	return true
}

// onInvalid исполняет правило on_invalid: снять, подставить или отбросить
// узел. Без правила — снять с type_invalid: значение, которое ядро отвергает
// фатально, в теле остаться не может (CANON §8).
func (s *sanitizer) onInvalid(path string, f *registry.Field, raw interface{}) (interface{}, bool) {
	oi := f.OnInvalid
	if oi == nil {
		s.warn("type_invalid", path, raw, f.Secret, map[string]string{"path": path})
		return nil, false
	}
	params := map[string]string{"path": path, "field": path}
	if !f.Secret {
		params["value"] = displayValue(raw)
	}
	switch oi.Action {
	case "coerce":
		v, ok := coerce(f, oi.Value)
		if !ok {
			s.warn(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
			return nil, false
		}
		s.warn(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
		return v, true
	case "drop_node":
		s.dropNode(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
		return nil, false
	default: // drop
		s.warn(codeOr(oi.Code, "type_invalid"), path, raw, f.Secret, params)
		return nil, false
	}
}

// constraintsOK — enum, format, min/max, len, len_parity.
func (s *sanitizer) constraintsOK(f *registry.Field, v interface{}) bool {
	// Обязательное строковое поле пустым быть не может: ядро пишет такие
	// поля без omitempty и на пустом значении падает («invalid server
	// address»). У необязательного поля пустое значение — это «не задано»,
	// и его снимает omitAsUnset уже после проверок.
	if f.Required {
		if str, isStr := v.(string); isStr && strings.TrimSpace(str) == "" {
			return false
		}
	}
	if len(f.Values) > 0 {
		switch vv := v.(type) {
		case []string:
			for _, item := range vv {
				if !inValues(f.Values, item) {
					return false
				}
			}
		case string, float64, int, int64, bool:
			if !inValues(f.Values, v) {
				return false
			}
		}
	}
	str, isStr := v.(string)
	if f.Format != "" {
		switch vv := v.(type) {
		case []string:
			for _, item := range vv {
				if !formatOK(f.Format, item) {
					return false
				}
			}
		default:
			if isStr && !formatOK(f.Format, str) {
				return false
			}
			if !isStr && !numericFormatOK(f.Format, vv) {
				return false
			}
		}
	}
	if f.Pattern != "" {
		switch vv := v.(type) {
		case []string:
			for _, item := range vv {
				if !patternOK(f.Pattern, item) {
					return false
				}
			}
		default:
			if isStr && !patternOK(f.Pattern, str) {
				return false
			}
		}
	}
	// min/max: у строк это длина, у чисел — значение (реестр так и
	// использует: short_id max:16 — длина, server_port max:65535 — значение).
	if f.Min != nil || f.Max != nil {
		var measure float64
		switch vv := v.(type) {
		case string:
			measure = float64(len(vv))
		case float64:
			measure = vv
		case int:
			measure = float64(vv)
		case int64:
			measure = float64(vv)
		case []string:
			measure = float64(len(vv))
		default:
			measure = 0
		}
		if f.Min != nil && measure < *f.Min {
			return false
		}
		if f.Max != nil && measure > *f.Max {
			return false
		}
	}
	if f.Len != nil {
		switch vv := v.(type) {
		case string:
			if len(vv) != *f.Len {
				return false
			}
		case []string:
			if len(vv) != *f.Len {
				return false
			}
		case []int:
			if len(vv) != *f.Len {
				return false
			}
		}
	}
	if f.LenParity != "" && isStr {
		odd := len(str)%2 == 1
		if (f.LenParity == "even" && odd) || (f.LenParity == "odd" && !odd) {
			return false
		}
	}
	return true
}

// codeOr — код правила или запасной, если реестр код не назвал.
func codeOr(code, fallback string) string {
	if code != "" {
		return code
	}
	return fallback
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// sortedKeys — детерминированный порядок для ключей вне схемы: карта в Go
// обходится случайно, а список warnings обязан быть сравним в корпусе.
func sortedKeys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// isEmptyValue — «значение не задано» для слоя связей (conflicts / requires).
//
// Ядро судит взаимоисключающие поля по ЗНАЧЕНИЮ, а не по наличию ключа:
// у transport.xmux конфликт max_concurrency↔max_connections срабатывает только
// когда оба > 0 (transport/v2rayxhttp/xmux.go), а "0" в JSON означает «не
// задано». Поэтому нулём считается и число-строка "0"/"0-0": подписка,
// выписавшая все поля секции с нулями, не должна терять заданные соседние
// значения. То же правило нужно certificate↔pins и reality↔ech.
func isEmptyValue(v interface{}) bool {
	switch vv := v.(type) {
	case nil:
		return true
	case string:
		return isZeroNumericString(vv)
	case bool:
		return !vv
	case float64:
		return vv == 0
	case int:
		return vv == 0
	case []interface{}:
		return len(vv) == 0
	case []string:
		return len(vv) == 0
	case map[string]interface{}:
		return len(vv) == 0
	}
	return false
}

// isZeroNumericString — пустая строка или число/диапазон из одних нулей
// ("0", "0-0"). Диапазоны XmuxRange приходят строками, и "0" в них — штатная
// запись «без лимита», то есть «поле не задано».
func isZeroNumericString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	seenDigit := false
	for _, r := range s {
		switch {
		case r == '0':
			seenDigit = true
		case r == '-':
			// разделитель диапазона; знак минуса тут не встречается
		case r >= '1' && r <= '9':
			return false
		default:
			return false // не число и не диапазон — обычная строка
		}
	}
	return seenDigit
}

func displayValue(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		if vv == float64(int64(vv)) {
			return strconv.FormatInt(int64(vv), 10)
		}
		return strconv.FormatFloat(vv, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(vv)
	case []string:
		return strings.Join(vv, ",")
	}
	return fmt.Sprintf("%v", v)
}

func sameValue(want, got interface{}) bool {
	return displayValue(want) == displayValue(got)
}

func inValues(values []interface{}, v interface{}) bool {
	got := displayValue(v)
	for _, want := range values {
		if displayValue(want) == got {
			return true
		}
	}
	return false
}

// patternCache — скомпилированные выражения Field.Pattern.
//
// Реестр неизменен в пределах процесса, а выражений всего несколько, поэтому
// кэш растёт до размера реестра и больше не двигается. Под мьютексом:
// санитайзер зовут из разбора подписок, а тот ходит по узлам параллельно.
var (
	patternCacheMu sync.Mutex
	patternCache   = map[string]*regexp.Regexp{}
)

// patternOK — соответствие значения выражению Field.Pattern.
//
// Невалидное выражение ПРОПУСКАЕТ значение, а не отбраковывает его: реестр
// может уехать вперёд кода, и молчаливый отказ узлов из-за опечатки в
// контракте был бы хуже пропущенной проверки. Сама опечатка ловится раньше —
// линтером реестра, который обязан компилировать каждое выражение.
//
// Якоря живут в самом выражении (см. Field.Pattern): здесь совпадение
// проверяется как есть, без неявного оборачивания в ^…$.
func patternOK(pattern, v string) bool {
	patternCacheMu.Lock()
	re, seen := patternCache[pattern]
	if !seen {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			compiled = nil
		}
		patternCache[pattern] = compiled
		re = compiled
	}
	patternCacheMu.Unlock()
	if re == nil {
		return true
	}
	return re.MatchString(v)
}

// formatOK — проверка строкового формата. Неизвестный формат пропускается:
// реестр может уехать вперёд кода, и молчаливый отказ был бы хуже.
func formatOK(format, v string) bool {
	switch format {
	case "uuid":
		return uuidOK(v)
	case "hex":
		if v == "" {
			return true
		}
		_, err := hex.DecodeString(v)
		return err == nil
	case "base64":
		if v == "" {
			return true
		}
		if _, err := base64.StdEncoding.DecodeString(v); err == nil {
			return true
		}
		if _, err := base64.RawURLEncoding.DecodeString(v); err == nil {
			return true
		}
		_, err := base64.RawStdEncoding.DecodeString(v)
		return err == nil
	case "base64_32":
		// Ключ Curve25519/X25519: 32 байта ПОСЛЕ декода (REALITY public_key,
		// ключи wireguard и masque). Просто «декодируется» здесь мало:
		// `enabled` и `true` — валидный base64 на 5 и 3 байта, и ядро на них
		// отвечает «invalid public_key» отказом ВСЕГО конфига (DRIFT §2(b2)).
		// Длина считается по декоду, а не по строке: один и тот же ключ
		// записывают base64url без padding (43 символа) и base64std с ним (44).
		if v == "" {
			return true
		}
		return decodedKeyLen(v) == 32
	case "host":
		// Пустую строку решает НЕ формат, а обязательность поля, и решает её
		// вызывающий (constraintsOK): у обязательного адреса пустое значение
		// фатально («invalid server address» на весь конфиг), у
		// необязательного tls.server_name — законное «не задано», которое
		// ядро принимает и заменяет адресом сервера. Один и тот же формат,
		// разные исходы — потому что разная обязательность.
		return !strings.ContainsAny(v, " \t\r\n/")
	case "url_path":
		// Путь, который ядро разбирает через url.Parse (ws/httpupgrade/http):
		// битое percent-кодирование даёт «invalid URL escape» и отказ ВСЕГО
		// конфига. Проверяем ровно то, на чём падает ядро, — синтаксис
		// экранирования, а не форму пути.
		return validPercentEscapes(v)
	case "port":
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 65535
	case "ipv4":
		ip := net.ParseIP(v)
		return ip != nil && ip.To4() != nil
	case "cidr":
		if _, _, err := net.ParseCIDR(v); err == nil {
			return true
		}
		return net.ParseIP(v) != nil
	}
	return true
}

func numericFormatOK(format string, v interface{}) bool {
	if format != "port" {
		return true
	}
	switch vv := v.(type) {
	case int:
		return vv >= 1 && vv <= 65535
	case int64:
		return vv >= 1 && vv <= 65535
	case float64:
		return vv >= 1 && vv <= 65535
	}
	return true
}

func uuidOK(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, c := range v {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexDigit(byte(c)) {
				return false
			}
		}
	}
	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// validPercentEscapes — корректно ли экранирование в строке: каждый «%»
// сопровождается двумя hex-цифрами. Ровно этот разбор делает url.Parse в
// ядре, и ровно на нём оно роняет конфиг целиком.
func validPercentEscapes(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] != '%' {
			continue
		}
		if i+2 >= len(v) || !isHexDigit(v[i+1]) || !isHexDigit(v[i+2]) {
			return false
		}
		i += 2
	}
	return true
}

// decodedKeyLen — длина ключа после декода base64, -1 если не декодируется.
//
// Перебираются все четыре написания (url/std × с padding и без): один и тот
// же 32-байтный ключ панели пишут по-разному, и отвергнуть рабочий узел
// из-за формы записи нельзя.
func decodedKeyLen(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(v); err == nil {
			return len(b)
		}
	}
	return -1
}
