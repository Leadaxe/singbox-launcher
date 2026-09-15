package backup

// Legacy-вход бэкапа: файл 0.x → записи состояния (SPEC 127 §6.2, W2.4).
//
// Файлы 0.10–0.12 уже у пользователей на руках, и читать их лаунчер обязан
// всегда (ONE_NAMESPACE §1: «legacy-вход обязателен обеим сторонам; это
// единственное место, где старые имена остаются, и только на чтении»).
// Поэтому здесь живёт весь перевод чужих имён в свои:
//
//	node_tag / label           → Node.Tag
//	config_json / uri          → Node.Body / Origin
//	servers[].folder           → папка, собранная ПО ИМЕНИ
//	rules[]: match + outbound  → Rule.Body (конструкторами состояния)
//	rules[]: kind=json, match  → inline-записи, тело как есть (splitRuleBodies)
//	rules[]: ref / refs        → Rule.Refs
//	dnsRef: name + value       → DNSServer.Tag + Body
//	chains[].chain             → Node.Body + Node.Hops
//	disabled{}                 → PendingDisabled (слияние доливает, §9 п. 1)
//	fold{}                     → FolderReplace
//
// Слияния здесь нет ни строчки: всё, что получилось, уезжает в общий
// applyDecoded (import.go) — тот же, что обслуживает вход 1.0.
//
// ЗАЧЕМ ОТДЕЛЬНЫЙ ФАЙЛ. Писатель 0.12 снят в v1.6.0 (D-110): лаунчер пишет
// только 1.0. Читатель — НЕТ, он живёт, пока живы файлы: у писателя и
// читателя разные сроки жизни, и потому они всегда жили в разных файлах —
// снятие писателя не утянуло за собой чтение того, что уже выпущено.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// errSkipRule — правило пропущено осознанно (чужой kind), а не сломалось.
// Отдельная ошибка, а не bool: вызывающий обязан различать «пропусти это» и
// «импорт невозможен», иначе одно чужое правило уронит весь файл.
var errSkipRule = errors.New("rule skipped")

// decodeLegacy переводит файл 0.x в записи состояния.
//
// Порядок разделов внутри decodedFile повторяет порядок файла: слияние
// опирается на него (§9 п. 8, «новые встают в конец, в порядке файла»).
func decodeLegacy(b *Backup, opts ImportOptions) (*decodedFile, error) {
	out := &decodedFile{Format: FileFormatLegacy}

	// Подписки. Индекс нужен ДЛЯ ТЕГА ЗАМЕНЫ: контракт 0.x его не несёт, и
	// обе стороны обязаны вывести один и тот же позиционный дериватив.
	for i, sub := range b.Subscriptions {
		src, warns := importSubscription(sub, i)
		out.Warnings = append(out.Warnings, warns...)
		out.Sources = append(out.Sources, decodedSource{Kind: decodedSubscription, Src: src})
		// Группы, которые породит свёртка приехавшей подписки (D-081):
		// правила и route.final ТОГО ЖЕ файла ссылаются на дериватив
		// `<N>:select` / `<N>:auto`, а список известных целей принимающей
		// стороны снят ДО импорта и о нём знать не может.
		for tag := range foldDerivedDirectionTags(sub, i) {
			out.KnownTagsFromFile = append(out.KnownTagsFromFile, tag)
		}
	}

	// Серверы: запись с пометкой folder уходит не в корень, а в папку с этим
	// именем. Порядок записей нормативен, поэтому элементы кладутся плоско в
	// порядке файла, а группировкой занимается слияние.
	for _, srv := range b.Servers {
		src, warns := importServer(srv)
		out.Warnings = append(out.Warnings, warns...)
		out.Sources = append(out.Sources, decodedSource{
			Kind:     decodedServer,
			Src:      src,
			Folder:   srv.Folder,
			Sections: srv.Sections != nil,
		})
	}

	for _, in := range b.Directions {
		if in.Tag == "" {
			continue
		}
		out.Directions = append(out.Directions, importDirection(in, opts.BlockTag))
	}

	for _, in := range b.Chains {
		if in.Tag == "" || in.Chain == nil {
			continue
		}
		src, warns := importChain(in)
		out.Warnings = append(out.Warnings, warns...)
		out.Sources = append(out.Sources, decodedSource{Kind: decodedChain, Src: src})
	}

	// Правила: чужой вид пропускается с warning, а не роняет файл. Пресет вне
	// шаблона проверяется ЗДЕСЬ — это перевод записи. Цель правила — после
	// слияния, одним списком известных целей на оба формата
	// (checkImportedRuleTargets в applyDecoded): цель может приехать этим же
	// файлом (Направление, цепочка, свёртка) или быть системным тегом шаблона
	// приёмника, и знает об этом только состояние после слияния.
	presets := newTagSet(opts.KnownPresets)
	for i, r := range b.Rules {
		rules, warns, err := importRule(r, i, presets)
		out.Warnings = append(out.Warnings, warns...)
		if errors.Is(err, errSkipRule) {
			continue // правило не наше — пропущено с warning, импорт живёт
		}
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", ruleLabel(r), err)
		}
		out.Rules = append(out.Rules, rules...)
	}

	out.DNS = decodeLegacyDNS(b.DNS)
	out.Vars = b.Vars
	// SPEC 129 Н8 — тем же правилом, что у 1.0: разбор внутри мажора один.
	moveFileRootDNSVars(out, opts.RecordVars)
	if b.Route != nil {
		out.RouteFinal = b.Route.Final
	}
	out.Warp = b.Warp
	return out, nil
}

// decodeLegacyDNS — секция dns файла 0.x записями состояния.
//
// Здесь и живёт перевод чужих имён: тег записи зовётся `name` (см. dnsRefTag),
// тело лежит в `value`. Тела переносятся только у kind=user: тело
// template/preset принадлежит шаблону принимающей стороны.
func decodeLegacyDNS(dns *DNS) *decodedDNS {
	if dns == nil {
		return nil
	}
	out := &decodedDNS{Final: dns.Final, Strategy: dns.Strategy}
	for _, ref := range dns.Servers {
		srv := state.DNSServer{
			Kind:    state.DNSServerKind(ref.Kind),
			Tag:     dnsRefTag(ref),
			Ref:     ref.Ref,
			Enabled: ref.Enabled == nil || *ref.Enabled,
		}
		if ref.Kind == "user" && len(ref.Value) > 0 {
			var body map[string]interface{}
			if json.Unmarshal(ref.Value, &body) == nil {
				srv.Body = body
			}
		}
		// SPEC 129: `vars` записи шаблонного сервера (так их пишет LxBox и в
		// 0.x) — значения его переменных. До SPEC 129 поле читалось и молча
		// терялось.
		if ref.Kind == string(state.DNSServerKindTemplate) && len(ref.Vars) > 0 {
			srv.Vars = make(map[string]string, len(ref.Vars))
			for k, v := range ref.Vars {
				srv.Vars[k] = v
			}
		}
		out.Servers = append(out.Servers, srv)
	}
	for _, ref := range dns.Rules {
		var body map[string]interface{}
		if ref.Kind == "user" && len(ref.Value) > 0 {
			_ = json.Unmarshal(ref.Value, &body)
		}
		out.Rules = append(out.Rules, state.DNSRule{
			Kind:    state.DNSRuleKind(ref.Kind),
			Ref:     ref.Ref,
			Enabled: ref.Enabled == nil || *ref.Enabled,
			Body:    body,
		})
	}
	return out
}

// dnsRefTag — тег DNS-записи файла 0.x.
//
// Читается `name`, а не `tag`, хотя схема 0.12 объявляет `tag` (ловушка
// CODEMAP §7.7): реальные файлы, выпущенные релизами, несут `name` — его и
// писал экспорт. `dnsRefKeys` разрешает оба имени, поэтому scanUnknown на
// файле со схемным `tag` молчит, но ЗНАЧЕНИЕ берётся оттуда, где оно есть у
// живых файлов. Расхождение кончается вместе с форматом 0.x: в 1.0 тег
// зовётся тегом и живёт в записи состояния.
func dnsRefTag(ref DNSRef) string {
	return ref.Name
}

// importSourceRef восстанавливает ссылку источника на цель дозвона
// (тройня контракта → NodeLink модели, convert_v7.go).
func importSourceRef(src *state.Source, ref SourceRef) {
	src.Detour = importNodeLinkRef(ref)
}

// ensureSourceID — Р3 (SPEC 117): ULID рождается в момент создания Source.
// Обратного синка Save, который раньше доминтовывал пустые id, больше нет —
// бэкап без id (чужой/рукописный файл) обязан получить ULID здесь.
func ensureSourceID(id string) string {
	if id == "" {
		return state.MakeULID()
	}
	return id
}

// importSubscription — подписка контракта 0.11 в источник v7.
//
// index — позиция записи в файле: тег ЗАМЕНЫ (fold → replace) контракт не
// несёт, а в v7 он явный. Материализуем его прежним позиционным деривативом
// (`<N>:select`), тем же, что писала старая свёртка: правила и route.final
// приезжают из того же файла и ссылаются именно на него.
//
// Второй возврат — потери конвертации, которые контракт выразить умеет, а
// модель v7 больше нет: маска тегов и локальные Направления источника. Обе
// приезжают в бэкапах v1.5.x, и обе обязаны быть названы вслух.
func importSubscription(sub Subscription, index int) (state.Source, []Warning) {
	var warns []Warning
	src := state.Source{
		Node:     state.Node{Kind: state.SourceKindSubscription, Enabled: sub.Enabled == nil || *sub.Enabled},
		ID:       ensureSourceID(sub.ID),
		URL:      sub.URL,
		Name:     sub.Label,
		MaxNodes: sub.MaxNodes,
		Skip:     sub.Skip,
	}
	importSourceRef(&src, sub.SourceRef)
	// Чем подписка представляется провайдеру (контракт 0.12). Применяются
	// четыре ключа, которые у модели v7 есть; mobile-only тройка и любое
	// незнакомое — отбрасываются ОДНИМ warning'ом с перечнем.
	if w, ok := importSourceIdentity(&src, sub); ok {
		warns = append(warns, w)
	}
	if sub.Tag != nil {
		if sub.Tag.Prefix != "" || sub.Tag.Postfix != "" {
			src.TagPolicy = &state.TagPolicy{Prefix: sub.Tag.Prefix, Postfix: sub.Tag.Postfix}
		}
	}
	// Маска ПОДПИСКИ — шаблон имени для каждой ноды; prefix/postfix её не
	// заменяют. Потеря названа, тегам нод она не подставляется.
	if mask := importMaskTag(sub.Tag); mask != "" {
		warns = append(warns, Warning{Code: WarnBackupTagMaskDropped, Detail: subscriptionLabel(sub) + ": " + mask})
	}
	src.Replace = importFold(sub.Fold, backupReplaceTag(sub, index))
	if sub.Update != nil {
		src.Update = &state.UpdateSpec{IntervalHours: sub.Update.IntervalHours, AutoRefresh: sub.Update.Auto}
	}
	// Локальные Направления источника: пара, порождённая свёрткой, уже
	// приехала заменой (Replace выше) — второй раз её импортировать нельзя,
	// это дало бы двух владельцев одного тега. Остальные упразднены классом.
	if derived := foldDerivedDirectionTags(sub, index); len(sub.Outbounds) > 0 {
		for _, ob := range sub.Outbounds {
			tag := strings.TrimSpace(ob.Tag)
			if tag == "" || derived[tag] {
				continue
			}
			warns = append(warns, Warning{Code: WarnBackupLocalDirectionDropped, Detail: subscriptionLabel(sub) + " → " + tag})
		}
	}
	// Флаги «убрать из общего списка» / «показывать теги группы»: класс
	// упразднён (SPEC 118), узлы источника остаются в пуле кандидатов.
	// Поля объявлены в типах контракта, поэтому общий scanUnknown их не
	// видит — без явного warning'а они пропали бы молча (П6).
	if sub.ExcludeFromGlobal || sub.ExposeGroupTagsToGlobal {
		warns = append(warns, Warning{Code: WarnBackupSourceFlagDropped, Detail: subscriptionLabel(sub)})
	}
	// Отметки выключения: узлов у только что импортированной подписки нет
	// (nodes[] в контракт не едут), поэтому они ждут первого достоверного
	// fetch в PendingDisabled — вердикт O2.
	for tag := range sub.Disabled {
		if strings.TrimSpace(tag) != "" {
			src.PendingDisabled = append(src.PendingDisabled, tag)
		}
	}
	sort.Strings(src.PendingDisabled)
	return src, warns
}

// subscriptionLabel — как назвать подписку в предупреждении: подпись, а если
// её нет — URL (единственное, что у записи есть всегда).
func subscriptionLabel(sub Subscription) string {
	if l := strings.TrimSpace(sub.Label); l != "" {
		return l
	}
	return sub.URL
}

// importSourceIdentity применяет к источнику приехавший объект identity и
// возвращает предупреждение о том, что применить не удалось.
//
// Применяются ровно те четыре ключа, которым в модели v7 есть куда лечь.
// Остальные (mobile-only device_os/ver_os/device_model и любые незнакомые)
// НЕ применяются и НЕ провозятся дальше: провоз непонятого создаёт
// состояние-призрак, ради сноса которого убран механизм extensions (П1/П3).
//
// Warning ровно один на подписку, с перечнем ключей: потеря у пользователя
// одна, и строка на каждый ключ утопила бы её в списке. Пустой объект
// identity (или объект с одними применёнными ключами) не даёт ничего —
// предупреждают о потере, а не о факте наличия поля.
func importSourceIdentity(src *state.Source, sub Subscription) (Warning, bool) {
	id := sub.Identity
	if id == nil {
		return Warning{}, false
	}
	// Применяется ЧЕТВЁРКА, а не объект целиком: mobile-only ключи
	// (device_os/ver_os/device_model) лаунчер не применяет и не провозит —
	// провоз непонятого создаёт состояние-призрак (П1/П3). Поэтому объект
	// состояния собирается из применённых ключей заново, а не копируется.
	ua, hwid := "", ""
	if id.UserAgent != nil {
		ua = *id.UserAgent
	}
	if id.HWID != nil {
		hwid = *id.HWID
	}
	src.SetIdentity(ua, hwid, id.SendHWID, id.HashDeviceModel)
	dropped := id.UnappliedKeys()
	if len(dropped) == 0 {
		return Warning{}, false
	}
	return Warning{
		Code:   WarnBackupSourceIdentityDropped,
		Detail: subscriptionLabel(sub) + ": " + strings.Join(dropped, ", "),
	}, true
}

// backupReplaceTag — тег замены свёрнутой подписки, приехавшей из бэкапа:
// префикс тегов подписки с позиционным умолчанием «<номер>:» плюс `select`.
// Формула та же, что у старой свёртки, — по этим тегам ссылаются правила
// того же файла.
func backupReplaceTag(sub Subscription, index int) string {
	prefix := ""
	if sub.Tag != nil {
		prefix = sub.Tag.Prefix
	}
	return legacyFoldPrefix(prefix, index) + "select"
}

// importServer — одиночный узел контракта в источник v7.
//
// Тело материализуется не здесь: URI приезжает в origin.raw, и узел
// становится собираемым после первого прохода материализации (Regen from raw
// в окне источника либо сборка). ConfigJSON — уже готовое тело.
func importServer(srv Server) (state.Source, []Warning) {
	var warns []Warning
	src := state.Source{
		Node: state.Node{Kind: state.SourceKindServer, Enabled: srv.Enabled == nil || *srv.Enabled, Tag: srv.NodeTag},
		ID:   ensureSourceID(srv.ID),
	}
	// Label НЕ кладётся в Source.Label: у того поля `json:"-"`, и подпись
	// умирала на первом же Save, а экспорт потом писал пустую строку. У
	// канона v7 у узла одно имя — тег (SPEC 112). Пустой тег label ещё
	// может спасти (иначе узел приехал бы безымянным), но разошедшаяся
	// подпись — потеря, и её называют вслух.
	if src.Tag == "" {
		src.Tag = strings.TrimSpace(srv.Label)
	} else if l := strings.TrimSpace(srv.Label); l != "" && l != src.Tag {
		warns = append(warns, Warning{Code: WarnBackupLabelDropped, Detail: l + " → " + src.Tag})
	}
	if srv.ExcludeFromGlobal {
		warns = append(warns, Warning{Code: WarnBackupSourceFlagDropped, Detail: serverLabel(srv)})
	}
	importSourceRef(&src, srv.SourceRef)
	// SPEC 121: секции узла. В файле 0.12 они едут непрозрачным блоком в
	// форме СОСТОЯНИЯ — там их писал прежний экспорт (ловушка §7.6), и такие
	// файлы уже у пользователей на руках. Пустой набор нормализуется в nil —
	// третьего состояния у поля нет.
	if srv.Sections != nil {
		sections, sw := decodeBackupSections(srv.Sections, src.Tag)
		src.Node.Sections = sections
		warns = append(warns, sw...)
		src.Node.NormalizeNodeSections()
	}
	switch {
	case len(srv.ConfigJSON) > 0:
		src.Body = append(json.RawMessage(nil), srv.ConfigJSON...)
		src.Origin = &state.Origin{Kind: state.OriginKindJSON, Raw: string(srv.ConfigJSON)}
	case strings.TrimSpace(srv.URI) != "":
		// Вид определяется ФОРМОЙ текста: в поле `uri` контракта едет и
		// ссылка, и блок wg-quick (контракт общий с LxBox, третьего ключа в
		// нём нет). Записать блоку kind=uri значило бы потерять вид на первом
		// же Save — узел перестал бы пересобираться из исходника провайдера.
		kind := state.OriginKindURI
		if len(subscription.WGConfBlocksOf(srv.URI)) > 0 {
			kind = state.OriginKindWGIni
		}
		src.Origin = &state.Origin{Kind: kind, Raw: srv.URI}
	}
	return src, warns
}

// serverLabel — как назвать одиночный узел в предупреждении: тег, а если
// его нет — подпись; и то и другое пусто у безымянной записи, тогда URI.
func serverLabel(srv Server) string {
	if t := strings.TrimSpace(srv.NodeTag); t != "" {
		return t
	}
	if l := strings.TrimSpace(srv.Label); l != "" {
		return l
	}
	return srv.URI
}

// importChain переводит каноническую запись chains[] во внутренний источник.
//
// Тег записи едет в NodeTag, отображаемое имя — в Label: обе роли имеют своё
// поле. Раньше тег клался в Label (другого места не было), из-за чего импорт
// чужого label разъехался бы со ссылками правил, route.final и позиций других
// цепочек.
func importChain(in Chain) (state.Source, []Warning) {
	var warns []Warning
	src := state.Source{
		Node: state.Node{
			Kind:    state.SourceKindChain,
			Enabled: in.Enabled == nil || *in.Enabled,
			Tag:     in.Tag,
			Body:    importChainBody(in.Chain),
			Hops:    importHops(in.Chain.HopsOrNil()),
		},
		ID: ensureSourceID(in.ID),
	}
	// Label не применяется и предупреждения НЕ даёт: с контракта 0.12.4
	// (D-094) это объявленное поле LxBox — он подпись цепочки пишет и
	// читает, у лаунчера имя одно, тег (SPEC 112). Чужое объявленное
	// игнорируется молча (BACKUP.md §1), иначе warning шумел бы на каждом
	// импорте файла LxBox.
	if in.ExcludeFromGlobal {
		warns = append(warns, Warning{Code: WarnBackupSourceFlagDropped, Detail: in.Tag})
	}
	return src, warns
}

// importRule — запись rules[] файла 0.x в записи состояния.
//
// Записей может получиться несколько: сырое правило `kind: json` с массивом
// тел раскладывается по норме «одно правило — одно тело» (importJSONRule).
// index — номер записи в файле, им называется безымянная запись в
// предупреждении.
func importRule(r Rule, index int, presets tagSet) ([]state.Rule, []Warning, error) {
	if RuleKind(r.Kind) == RuleJSON {
		rules, warns := importJSONRule(r, index, presets)
		if len(rules) == 0 {
			return nil, warns, errSkipRule
		}
		return rules, warns, nil
	}

	var warns []Warning
	enabled := r.Enabled == nil || *r.Enabled

	// Цель `outbound` уезжает в тело записи конструктором; символическую
	// ссылку в никуда выключает общий проход после слияния
	// (checkImportedRuleTargets, BACKUP.md §3).

	num := legacyRuleNum(r)

	// Тело записи пишут только конструкторы состояния (SPEC 127 §0) — здесь
	// свой json.Marshal(XBody) больше не собирается.
	var out state.Rule
	switch RuleKind(r.Kind) {
	case RulePreset:
		if !presets.empty() && !presets.has(r.Ref) {
			enabled = false
			warns = append(warns, Warning{Code: WarnBackupUnknownPreset, Detail: r.Ref})
		}
		out = state.NewPresetRule(r.Ref, r.Vars)
	case RuleInline:
		var match map[string]interface{}
		if len(r.Match) > 0 {
			if err := json.Unmarshal(r.Match, &match); err != nil {
				return nil, warns, fmt.Errorf("match: %w", err)
			}
		}
		out = state.NewInlineRule(r.Name, match, r.Outbound)
	case RuleSRS:
		// `refs` (все наборы) сильнее `ref` (первый): файл без `refs` — от
		// стороны, которая знает один набор на правило.
		urls := r.Refs
		if len(urls) == 0 {
			urls = []string{r.Ref}
		}
		out = state.NewSrsRule(r.Name, urls, r.Outbound)
	default:
		return nil, append(warns, Warning{Code: WarnBackupUnknownField, Detail: "rules[].kind=" + string(r.Kind)}), errSkipRule
	}

	// Конструкторы задают вид и тело; метаданные записи дописываются поверх.
	out.Enabled = enabled
	out.Num = num

	return []state.Rule{out}, warns, nil
}

// importJSONRule — `kind: json` файла 0.x: сырое правило sing-box в `match`
// (схема 0.12: «kind=inline|json: sing-box rule-фрагмент»).
//
// Раньше такое правило пропускалось целиком с backup_unknown_field — «структура
// чужая». Чужой она не была: это правило sing-box как есть, то есть ровно
// запись 1.0 (BACKUP.md §11: вид json снят, «это был тот же inline»), и
// пропуск стоил пользователю рабочего правила. Теперь запись становится
// inline-записью с телом как есть и проходит норму «одно правило — одно тело»
// ТОЙ ЖЕ функцией, что вход 1.0 (splitRuleBodies): объект — одна запись,
// массив — по записи на элемент, не-объект и битый JSON — отброс с прежним
// кодом backup_unknown_field.
//
// Цель остаётся В ТЕЛЕ: `outbound` | `action` правила sing-box. Плоский
// `outbound` записи к json-правилу не применяется — вторая цель рядом с телом
// была бы второй правдой о том, куда правило ведёт. Проверка цели та же, что у
// записи 1.0 (checkImportedRuleTargets после слияния): цель, которой нет,
// выключает часть с backup_unknown_outbound. Часть без цели ввозится как есть — так же лаунчер
// ввозит inline-правило 0.x без `outbound` и запись 1.0 без цели в теле.
func importJSONRule(r Rule, index int, presets tagSet) ([]state.Rule, []Warning) {
	rec := state.Rule{
		Kind:    state.RuleKindInline,
		Name:    r.Name,
		Enabled: r.Enabled == nil || *r.Enabled,
		Num:     legacyRuleNum(r),
		Body:    append(json.RawMessage(nil), r.Match...),
	}
	parts, warns := splitRuleBodies(rec, "rules["+ruleEntryLabel(r.Name, index)+"].match")
	out := make([]state.Rule, 0, len(parts))
	for _, part := range parts {
		rule, w := decode10Rule(part, presets)
		warns = append(warns, w...)
		out = append(out, rule)
	}
	return out, warns
}

// legacyRuleNum — номер оси записи 0.x: бэкап несёт его как float64 (схема
// 0.12), состояние — как int.
func legacyRuleNum(r Rule) *int {
	if r.Num == nil {
		return nil
	}
	n := int(*r.Num)
	return &n
}

func ruleLabel(r Rule) string {
	if r.Name != "" {
		return r.Name
	}
	if r.Ref != "" {
		return r.Ref
	}
	return string(r.Kind)
}
