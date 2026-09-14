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
// только запоминается, какой id у папки был в файле. Ссылку на член папки,
// записанную БЕЗ folder_id одним финальным тегом, после слияния поднимает
// normalizeMemberLinks10.

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/state"
)

// decode10 переводит файл 1.0 в записи состояния.
func decode10(b *Backup10, opts ImportOptions) (*decodedFile, error) {
	out := &decodedFile{Format: FileFormat10}

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

	for i, r := range b.Rules {
		// Тело массивом раскладывается на записи (норма «одно правило — одно
		// тело», splitRuleBodies); запись без тела нормой не затронута и
		// едет как раньше.
		parts := []state.Rule{r}
		if ruleBodyPresent(r) {
			var sw []Warning
			parts, sw = splitRuleBodies(r, "rules["+ruleEntryLabel(r.Name, i)+"].body")
			out.Warnings = append(out.Warnings, sw...)
		}
		for _, part := range parts {
			rule, warns := decode10Rule(part, known, presets)
			out.Warnings = append(out.Warnings, warns...)
			out.Rules = append(out.Rules, rule)
		}
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

// splitRuleBodies — норма «одно правило — одно тело» (D-111, BACKUP.md §2).
// ОДНА функция на оба входа: запись 1.0 (decode10) и `kind: json` файла 0.x
// (importJSONRule). Две реализации одной нормы разошлись бы на первой правке,
// и один и тот же массив дал бы у пользователя разный набор правил в
// зависимости от того, каким писателем снят файл.
//
// Запись правила маршрута несёт в `body` РОВНО ОДИН объект правила sing-box.
// Массив формой записи не является, но на входе встречается (сырое
// json-правило держало и массив; рукописная или чужая запись 1.0). Такой вход
// не отбрасывается целиком, а раскладывается на записи так, как если бы
// массив был развёрнут в файле подряд, — поэтому и config.json из них
// байт-в-байт тот же:
//
//   - по записи на элемент-объект, в порядке массива; тело — элемент как
//     есть, цель правила (`outbound` | `action`) остаётся в нём;
//   - имена `name`, `name #2`, `name #3`… — по порядку получившихся записей.
//     Безымянная запись даёт безымянные: к пустому имени суффикс не
//     приклеивается, у лаунчера безымянное правило так и остаётся
//     безымянным (идентичность `unnamed`), а « #2» было бы именем без имени;
//   - `enabled` общий: тумблер у тела был один;
//   - `num` — номер исходной записи у всех частей. Равный номер держит части
//     вместе и в порядке массива (перенумерация импорта сортирует
//     устойчиво), а сплошная перенумерация раздаёт им номера подряд. Номера
//     N+1, N+2… здесь столкнулись бы с номером СЛЕДУЮЩЕЙ записи файла и
//     перемешали бы части с ней;
//   - `id` (метаданные другой стороны) — только у первой части: две записи с
//     одним id дали бы ему двух владельцев;
//   - `refs`/`vars` — у каждой части свои копии.
//
// Элемент, который не объект, отбрасывается кодом непонятого правила
// (backup_unknown_field), соседи живут. Тело не объект и не массив (строка,
// число, пусто, битый JSON) и пустой массив — запись отбрасывается тем же
// кодом: применять нечего, а терять запись молча нельзя (П6). Новых кодов
// норма не заводит.
//
// where — путь тела в предупреждении: `rules[<имя>].body` у 1.0,
// `rules[<имя>].match` у json-правила 0.x.
func splitRuleBodies(r state.Rule, where string) ([]state.Rule, []Warning) {
	raw := bytes.TrimSpace(r.Body)
	if !json.Valid(raw) {
		return nil, []Warning{{Code: WarnBackupUnknownField, Detail: where}}
	}
	switch raw[0] {
	case '{':
		return []state.Rule{r}, nil
	case '[':
		var elems []json.RawMessage
		if err := json.Unmarshal(raw, &elems); err != nil || len(elems) == 0 {
			return nil, []Warning{{Code: WarnBackupUnknownField, Detail: where}}
		}
		var (
			out   []state.Rule
			warns []Warning
		)
		for i, el := range elems {
			if el = bytes.TrimSpace(el); len(el) == 0 || el[0] != '{' {
				warns = append(warns, Warning{
					Code:   WarnBackupUnknownField,
					Detail: where + "[#" + strconv.Itoa(i+1) + "]",
				})
				continue
			}
			part := state.CloneRule(r)
			part.Body = append(json.RawMessage(nil), el...)
			if len(out) > 0 {
				part.ID = ""
				if r.Name != "" {
					part.Name = r.Name + " #" + strconv.Itoa(len(out)+1)
				}
			}
			out = append(out, part)
		}
		return out, warns
	default:
		return nil, []Warning{{Code: WarnBackupUnknownField, Detail: where}}
	}
}

// ruleBodyPresent — у записи 1.0 есть тело, которое норма splitRuleBodies
// обязана посмотреть.
//
// Только у видов с телом (inline, srs): у preset тела нет по форме записи.
// Отсутствующее тело и `null` нормой не затронуты — это не «массив вместо
// объекта», а пустое тело, и читатель состояния (DecodeBody) понимает его как
// пустой объект; такая запись едет как ехала.
func ruleBodyPresent(r state.Rule) bool {
	if r.Kind != state.RuleKindInline && r.Kind != state.RuleKindSrs {
		return false
	}
	raw := bytes.TrimSpace(r.Body)
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}

// ruleEntryLabel — как назвать запись rules[] в пути предупреждения: имя, а
// у безымянной — номер записи в файле (та же форма `#N`, что у общего обхода
// неизвестных ключей, entryLabel).
func ruleEntryLabel(name string, index int) string {
	if name != "" {
		return name
	}
	return "#" + strconv.Itoa(index+1)
}

// normalizeMemberLinks10 — терпимость читателя 1.0 к ссылке без folder_id на
// член папки (BACKUP.md §4, §6).
//
// Форма ссылки — NodeLink `{folder_id, tag}`: у ссылки на член папки
// `folder_id` обязателен, `tag` — СЫРОЙ тег узла внутри неё; у корневого узла
// `folder_id` пуст. Старые и чужие файлы пишут ссылку на член папки одним
// финальным тегом конфига, без folder_id. Сборка ищет такую ссылку только в
// корневом пространстве (config.NodeLinkTargets.Resolve), и узел,
// дозванивающийся через член папки, и цепочка с таким хопом уходили
// fail-closed на каждой сборке. Это терпимость ЧИТАТЕЛЯ, а не форма записи:
// писатель лаунчера folder_id у такой ссылки ставит всегда.
//
// Переписывается ссылка, у которой:
//
//   - folder_id пуст;
//   - тег не занят корневым пространством результата — корневой узел,
//     Направление, тег замены, известная цель приёмника, зарезервированный
//     литерал (importKnownTags, reservedTargetLiteral). Корень сильнее члена
//     папки — ровно как у Resolve, который туда смотрит первым;
//   - тег совпал с финальным тегом (TagPolicy.FinalTag) РОВНО ОДНОГО члена
//     папки или подписки,
//
// — в `{folder_id: <id контейнера здесь>, tag: <сырой тег члена здесь>}`.
//
// Финальный тег сверяется в два яруса. Сперва — пространство ФАЙЛА: члены
// папок файла под теми именами, которые они носили в файле
// (mergedInfo.landed.fileFinals), и узлы подписок, приехавших файлом (их
// узлы в файл не едут, но сырые теги у сторон одни и те же, а политика тегов
// после слияния — файловая). Ссылка файла называет узел файла, и член,
// которого слияние уникализировало или узнало по телу под другим тегом,
// находится по прежнему имени, а не уводит ссылку на здешнего тёзку
// (NODE_LINK.md §7.2). Не нашлось в файле — члены результата (член может
// лежать в локальной папке, которой в файле нет), кроме добавленных файлом под
// другим тегом: их здешнее имя файлу не принадлежит.
//
// Совпало несколько — ссылка остаётся как есть: выбирать за пользователя
// нечем, а своего кода висячей ссылки у импорта нет — недостижимая цель
// вопрос сборки, где она уходит fail-closed с названной причиной (§4, §6).
// Трогаются только ссылки, приехавшие ЭТИМ файлом (linked).
//
// Проход идёт после слияния и переписи адресов: сопоставлять надо с составом
// РЕЗУЛЬТАТА — член может лежать в локальной папке, которой в файле нет, или
// в папке файла, объявленной ниже ссылки.
func normalizeMemberLinks10(s *state.State, merged *mergedInfo, rootNames []string) {
	byFinal := map[string][]state.NodeLink{}
	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Kind != state.SourceKindFolder && src.Kind != state.SourceKindSubscription {
			continue
		}
		if src.ID == "" {
			continue // адресовать контейнер без id нечем
		}
		for j := range src.Nodes {
			n := &src.Nodes[j]
			raw := strings.TrimSpace(n.Tag)
			// Неразобранная запись в сборку не едет вовсе и целью ссылки
			// быть не может (convert_v7.go, resolveImportedHops — то же).
			if raw == "" || n.IsUnsupported() {
				continue
			}
			here := state.NodeLink{FolderID: src.ID, Tag: n.Tag}
			if merged.landed.renamed[here] {
				continue
			}
			final := strings.TrimSpace(src.TagPolicy.FinalTag(raw))
			byFinal[final] = append(byFinal[final], here)
		}
	}
	fileTier := make(map[string][]state.NodeLink, len(merged.landed.fileFinals))
	for final, hits := range merged.landed.fileFinals {
		fileTier[final] = append([]state.NodeLink(nil), hits...)
	}
	for _, sub := range linkedSubscriptions(s, merged) {
		for j := range sub.Nodes {
			n := &sub.Nodes[j]
			raw := strings.TrimSpace(n.Tag)
			if raw == "" || n.IsUnsupported() {
				continue
			}
			final := strings.TrimSpace(sub.TagPolicy.FinalTag(raw))
			fileTier[final] = append(fileTier[final], state.NodeLink{FolderID: sub.ID, Tag: n.Tag})
		}
	}
	if len(byFinal) == 0 && len(fileTier) == 0 {
		return
	}
	root := make(map[string]bool, len(rootNames))
	for _, t := range rootNames {
		if t = strings.TrimSpace(t); t != "" {
			root[t] = true
		}
	}
	fix := func(link *state.NodeLink) {
		if link == nil || link.FolderID != "" {
			return
		}
		tag := strings.TrimSpace(link.Tag)
		if tag == "" || root[tag] || reservedTargetLiteral(tag) {
			return
		}
		hits, inFile := fileTier[tag]
		if !inFile {
			hits = byFinal[tag]
		}
		if len(hits) == 1 {
			*link = hits[0]
		}
	}
	for _, ln := range merged.linked {
		n := ln.at.resolve(s)
		if n == nil {
			continue
		}
		fix(n.Detour)
		for i := range n.Hops {
			fix(&n.Hops[i])
		}
	}
}

// linkedSubscriptions — подписки результата, приехавшие этим файлом (новые и
// узнанные по URL).
func linkedSubscriptions(s *state.State, merged *mergedInfo) []*state.Source {
	var out []*state.Source
	for _, ln := range merged.linked {
		if ln.at.node >= 0 || ln.at.src < 0 || ln.at.src >= len(s.Sources) {
			continue
		}
		if src := &s.Sources[ln.at.src]; src.Kind == state.SourceKindSubscription && src.ID != "" {
			out = append(out, src)
		}
	}
	return out
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
