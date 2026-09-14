package backup

// Вход формата 1.0: файл → записи состояния (SPEC 127 §6.2, W2.4).
//
// Работы здесь почти нет — и это ровно то, ради чего затевался контракт 1.0:
// записи файла УЖЕ типы состояния (state.Rule, state.DNSOptions, поля
// state.Source), и «перевод» сводится к копированию плюс к трём полям, у
// которых в контракте своё имя (fold, disabled, directions).
//
// Копии, а не общие указатели: разобранный файл переживает импорт (UI
// показывает сводку до применения), и общий с состоянием срез сделал бы
// правку состояния правкой уже прочитанного файла.
//
// Ссылки на папки. `detour.folder_id` и `hops[].folder_id` несут ULID папки
// МАШИНЫ-ЭКСПОРТЁРА. На приёмнике совпавшая по имени папка держит СВОЙ id,
// поэтому ссылки переписываются по карте «id из файла → локальный id» — карту
// строит слияние (оно и решает, совпала папка или заведена новая), а здесь
// только запоминается, какой id у папки был в файле.

import (
	"sort"
	"strings"

	"singbox-launcher/core/state"
)

// decode10 переводит файл 1.0 в записи состояния.
func decode10(b *Backup10, opts ImportOptions) (*decodedFile, error) {
	out := &decodedFile{}

	for _, in := range b.Directions {
		if in.Tag == "" {
			continue
		}
		out.Directions = append(out.Directions, importDirection(in))
	}

	// subIndex — номер ПОДПИСКИ среди источников-подписок файла. Нужен ровно
	// для одного: запасного тега замены свёрнутой подписки. Имя группы 1.0
	// везёт явно (`fold_tag`), но файл чужой стороны может нести только
	// объект `fold` формы контракта 0.11, где тега нет (позиционный
	// дериватив, D-081). Тогда импорт обязан воспроизвести ровно ту же
	// формулу, что у входа 0.x, иначе правила ЭТОГО ЖЕ файла уехали бы в
	// никуда (foldTag10).
	subIndex := 0
	for _, src := range b.Sources {
		item, warns, ok := decode10Source(src, subIndex)
		out.Warnings = append(out.Warnings, warns...)
		if src.Kind == state.SourceKindSubscription {
			subIndex++
		}
		if !ok {
			continue
		}
		out.Sources = append(out.Sources, item)
	}

	// Известные цели: то, что знает принимающая сторона, плюс теги, которые
	// приезжают ЭТИМ ЖЕ файлом (Направления, цепочки и группы свёртки).
	// Без последних правило, метящее в группу свёрнутой подписки того же
	// файла, приезжало бы выключенным «цель не существует» — при том что
	// цель приехала строкой выше.
	knownTags := append([]string(nil), opts.KnownOutbounds...)
	for _, d := range out.Directions {
		knownTags = append(knownTags, d.Tag)
	}
	for _, s := range out.Sources {
		switch s.Kind {
		case decodedChain:
			knownTags = append(knownTags, s.Src.Tag)
		case decodedSubscription, decodedFolder:
			if s.Src.Replace != nil && s.Src.Replace.Tag != "" {
				knownTags = append(knownTags, s.Src.Replace.Tag)
				if s.Src.Replace.Mode == state.FolderReplaceBoth {
					knownTags = append(knownTags, s.Src.Replace.Tag+"-auto")
				}
			}
		}
	}
	known := newTagSet(knownTags)
	presets := newTagSet(opts.KnownPresets)

	for _, r := range b.Rules {
		rule, warns := decode10Rule(r, known, presets)
		out.Warnings = append(out.Warnings, warns...)
		out.Rules = append(out.Rules, rule)
	}

	out.DNS = decode10DNS(b.DNS)
	out.Vars = b.Vars
	if b.Route != nil {
		out.RouteFinal = b.Route.Final
	}
	out.Warp = b.Warp
	return out, nil
}

// decode10Source — запись sources[] 1.0 в элемент слияния.
//
// Третий возврат — едет ли запись дальше: вид, которого union не знает,
// пропускается (это чужая сторона, ушедшая вперёд по схеме).
func decode10Source(in Source10, subIndex int) (decodedSource, []Warning, bool) {
	var warns []Warning
	node := state.Node{
		Kind:    in.Kind,
		Tag:     in.Tag,
		Enabled: in.Enabled,
		Origin:  in.Origin,
		Body:    in.Body,
		Detour:  in.Detour,
		Hops:    in.Hops,
		Group:   in.Group,
		Service: in.Service,
		Reason:  in.Reason,
	}
	identity, iw := importIdentity10(in)
	warns = append(warns, iw...)
	src := state.Source{
		Node:               cloneNode(node),
		ID:                 ensureSourceID(in.ID),
		Name:               in.Name,
		TagPolicy:          cloneTagPolicy(in.TagPolicy),
		URL:                in.URL,
		Identity:           identity,
		RelaysInDirections: in.RelaysInDirections,
		Skip:               cloneSkip(in.Skip),
		MaxNodes:           in.MaxNodes,
		Update:             cloneUpdateSpec(in.Update),
		// Свёртка едет формой контракта (fold) — а в ней тега нет, и
		// материализуется он тем же позиционным деривативом, что у 0.x.
		Replace: importFold(in.Fold, foldTag10(in, subIndex)),
	}
	// Секции узла: форма одна с состоянием, поэтому разбор — копия плюс
	// отсев чужих видов записей (§6.2 W2.5) и — у узла, которому секции не
	// положены вовсе — снятие поля целиком с тем же кодом.
	sections, sw := normalizeImportedSections(in.Sections.Clone(), in.Tag)
	warns = append(warns, sw...)
	sections, kw := dropSectionsForForeignNode(in.Kind, sections, in.Tag)
	warns = append(warns, kw...)
	src.Node.Sections = sections
	src.Node.NormalizeNodeSections()

	switch in.Kind {
	case state.SourceKindSubscription:
		// Отметки выключенных узлов: у только что импортированной подписки
		// nodes[] пусты (кэш в файл не едет), поэтому отметки ждут первого
		// достоверного fetch в PendingDisabled — тот же вердикт O2, что у
		// 0.x. Слияние доливает их к своим (§9 п. 1).
		src.PendingDisabled = disabledTags10(in.Disabled)
		return decodedSource{Kind: decodedSubscription, Src: src, FullSettings: true}, warns, true
	case state.SourceKindFolder:
		var memberSections []bool
		for _, n := range in.Nodes {
			member := cloneNode(n)
			memberSections = append(memberSections, n.Sections != nil)
			ms, mw := normalizeImportedSections(member.Sections, n.Tag)
			warns = append(warns, mw...)
			ms, mk := dropSectionsForForeignNode(n.Kind, ms, n.Tag)
			warns = append(warns, mk...)
			member.Sections = ms
			member.NormalizeNodeSections()
			src.Nodes = append(src.Nodes, member)
		}
		return decodedSource{
			Kind: decodedFolder, Src: src, FileFolderID: in.ID,
			MemberSections: memberSections, FullSettings: true,
		}, warns, true
	case state.SourceKindServer:
		return decodedSource{Kind: decodedServer, Src: src, Sections: in.Sections != nil, FullSettings: true}, warns, true
	case state.SourceKindChain:
		return decodedSource{Kind: decodedChain, Src: src, FullSettings: true}, warns, true
	default:
		// Вид, которого union не знает: применить его нечем, но и ронять
		// файл из-за одной записи нельзя (П3 — непонятое отбрасывается с
		// предупреждением).
		return decodedSource{}, append(warns, Warning{
			Code:   WarnBackupSourceKindUnsupported,
			Detail: sourceExportName(src),
			Kind:   string(in.Kind),
			Nodes:  len(in.Nodes),
		}), false
	}
}

// importIdentity10 — слепок identity записи 1.0: применяемая ЧЕТВЁРКА плюс
// перечень ключей, которые лаунчер не применяет.
//
// Та же дисциплина, что у входа 0.x (importSourceIdentity): объект состояния
// собирается из применённых ключей ЗАНОВО, а не копируется целиком. Форма
// файла у 1.0 другая, а вопрос «что эта сторона умеет» — тот же, и ответ на
// него не может зависеть от формата: иначе одна и та же подписка, записанная
// двумя писателями, давала бы на приёмнике два разных состояния и два разных
// разговора с пользователем.
//
// Mobile-only тройка device_os/ver_os/device_model объявлена в контракте ради
// LxBox-стороны; лаунчер её не применяет (per-source их у него нет). Сложить
// её в состояние значило бы завести состояние-призрак, которое ничего не
// делает, но переопубликовывается каждым следующим экспортом (П1/П3), —
// ровно тот тайный груз, ради сноса которого убран механизм extensions.
func importIdentity10(in Source10) (*state.SubscriptionIdentity, []Warning) {
	id := in.Identity
	if id == nil {
		return nil, nil
	}
	var out state.Source
	ua, hwid := "", ""
	if id.UserAgent != nil {
		ua = *id.UserAgent
	}
	if id.HWID != nil {
		hwid = *id.HWID
	}
	out.SetIdentity(ua, hwid, id.SendHWID, id.HashDeviceModel)

	dropped := id.UnappliedKeys()
	if len(dropped) == 0 {
		return out.Identity, nil
	}
	return out.Identity, []Warning{{
		Code:   WarnBackupSourceIdentityDropped,
		Detail: source10Label(in) + ": " + strings.Join(dropped, ", "),
	}}
}

// source10Label — как назвать запись 1.0 пользователю: адрес подписки, а при
// его отсутствии — имя либо тег.
func source10Label(in Source10) string {
	if s := strings.TrimSpace(in.URL); s != "" {
		return s
	}
	if s := strings.TrimSpace(in.Name); s != "" {
		return s
	}
	return strings.TrimSpace(in.Tag)
}

// foldTag10 — тег замены свёрнутого источника формата 1.0.
//
// Имя группы формат 1.0 везёт ЯВНО (`fold_tag`), и оно здесь главное: в
// модели v8 тег замены — пользовательская настройка, правится руками, и на
// это имя метят правила ТОГО ЖЕ файла. Выводить его формулой значило бы
// подменять «DE-group» на «1:select» молча.
//
// Дериватив остался запасным ходом — для файла чужой стороны, которая пишет
// только объект `fold` формы контракта 0.11 (там поля тега нет вовсе, имя
// было позиционным: «префикс тегов источника, а если он пуст — `<номер>:`»
// плюс `select`, D-081). Такой файл читается ровно как читался бы 0.x.
//
// index — номер записи среди ПОДПИСОК файла, ровно как у 0.x. Для папки
// дериватив смысла не имеет (формула определена для секции subscriptions[]),
// но и вреда не делает: у папки без явного тега имени всё равно неоткуда
// взяться, а свёртка без имени — это свёртка, которую соберёт сборка.
func foldTag10(in Source10, index int) string {
	if in.Fold == nil {
		return ""
	}
	if tag := strings.TrimSpace(in.FoldTag); tag != "" {
		return tag
	}
	prefix := ""
	if in.TagPolicy != nil {
		prefix = in.TagPolicy.Prefix
	}
	return legacyFoldPrefix(prefix, index) + "select"
}

// disabledTags10 — ключи карты disabled{} отсортированным списком.
//
// Значение (unix seconds) у лаунчера смысла не несёт — TTL-очистки здесь нет,
// и карта времён умерла вместе с ней (см. exportDisabledMap). Сортировка
// нужна ради воспроизводимости: обход карты в Go случаен, а PendingDisabled
// уезжает на диск.
func disabledTags10(in map[string]int64) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for tag := range in {
		if strings.TrimSpace(tag) != "" {
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

// decode10Rule — запись правила 1.0: копия плюс проверка целей.
//
// Форма записи не трогается вовсе (в этом и смысл 1.0), но семантика импорта
// та же, что у 0.x: правило с целью, которой нет, приезжает ВЫКЛЮЧЕННЫМ, а
// не роняет конфиг ядра; preset вне шаблона — тоже выключенным.
func decode10Rule(r state.Rule, known, presets tagSet) (state.Rule, []Warning) {
	var warns []Warning
	out := state.CloneRule(r)

	switch r.Kind {
	case state.RuleKindPreset:
		if !presets.empty() && !presets.has(r.Ref) {
			out.Enabled = false
			warns = append(warns, Warning{Code: WarnBackupUnknownPreset, Detail: r.Ref})
		}
	case state.RuleKindInline, state.RuleKindSrs:
		if target := ruleTarget10(r); target != "" && !known.empty() && !known.has(target) {
			out.Enabled = false
			warns = append(warns, Warning{Code: WarnBackupUnknownOutbound, Detail: rule10Label(r) + " → " + target})
		}
	}
	return out, warns
}

// ruleTarget10 — символическая цель правила, какой её видит проверка.
//
// Читается ВИДОМ записи (DecodeBody), а не сырым телом: вид уже знает, что
// `action: reject` — это цель `reject`, а не отсутствие цели, и второй
// разбор тела здесь завёл бы вторую правду о том, куда правило метит.
// Запись, которую вид разобрать не смог, целей не имеет: ронять из-за неё
// импорт нельзя, а сборка скажет о ней сама.
//
// Ветви — по УКАЗАТЕЛЯМ: DecodeBody возвращает один из {*PresetBody,
// *InlineBody, *SrsBody} (rule_types.go:383). Разбор по значениям молча
// проваливался в default и отдавал «цели нет» ДЛЯ ЛЮБОГО правила, то есть
// проверка §9 п. 7 в пути 1.0 не работала вовсе: правило с несуществующей
// целью приезжало включённым и роняло config.json целиком.
func ruleTarget10(r state.Rule) string {
	body, err := r.DecodeBody()
	if err != nil {
		return ""
	}
	switch v := body.(type) {
	case *state.InlineBody:
		return v.Outbound
	case *state.SrsBody:
		return v.Outbound
	}
	return ""
}

func rule10Label(r state.Rule) string {
	if r.Name != "" {
		return r.Name
	}
	if r.Ref != "" {
		return r.Ref
	}
	return string(r.Kind)
}

// decode10DNS — секция dns 1.0: записи состояния копиями.
func decode10DNS(in *state.DNSOptions) *decodedDNS {
	if in == nil {
		return nil
	}
	out := &decodedDNS{
		Final:                 in.Final,
		Strategy:              in.Strategy,
		DefaultDomainResolver: in.DefaultDomainResolver,
	}
	for _, srv := range in.Servers {
		out.Servers = append(out.Servers, state.CloneDNSServer(srv))
	}
	for _, r := range in.Rules {
		out.Rules = append(out.Rules, state.CloneDNSRule(r))
	}
	return out
}
