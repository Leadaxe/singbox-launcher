package backup

// Импорт LX Backup в состояние лаунчера (контракт 0.12.0).

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// Warning — то, что импортёр не смог применить дословно.
//
// Не ошибка: импорт продолжается. Но и не молчание — пользователь обязан
// узнать, что правило приехало выключенным или что настройка не применилась
// (BACKUP_PRINCIPLES.md П6, «молчаливых потерь нет»).
type Warning struct {
	// Code — машинный код (contract/registry/warnings.json).
	Code string
	// Detail — что именно затронуто: имя правила, тег, URL.
	Detail string
	// Kind — вид записи, о которой warning, когда одного кода мало.
	// `backup_source_kind_unsupported` выдаётся и на папку, и на
	// провайдерскую группу: код у потери один (контракт этого вида не
	// знает), а сказать пользователю их надо разными словами. Раньше вид
	// приклеивался к началу Detail, и UI пришлось бы отрывать его обратно
	// разбором строки, которую сам же и собирал.
	Kind string
	// Nodes — сколько узлов уехало вместе с записью, о которой warning.
	// Ноль означает «неприменимо» (правило, переменная, поле), а не «узлов
	// не было». Поле нужно ровно там, где потеря измеряется не фактом, а
	// объёмом: выпавшая из файла папка стоит своего состава, и назвать
	// пользователю «папка не поехала», умолчав о её десяти узлах, значит
	// пересказать половину потери. Разбирать число из Detail в UI было бы
	// вторым парсером человеческой строки — поле дешевле и не врёт.
	Nodes int
}

func (w Warning) String() string { return w.Code + ": " + w.Detail }

// Коды предупреждений импорта.
const (
	// WarnBackupUnknownOutbound — цель rules[].outbound не существует:
	// правило импортируется ВЫКЛЮЧЕННЫМ. Молча включить его нельзя —
	// правило с несуществующей целью роняет конфиг ядра.
	WarnBackupUnknownOutbound = "backup_unknown_outbound"
	// WarnBackupFinalDropped — route.final указывает в никуда: не
	// применяется (иначе весь трафик уходит в несуществующий outbound).
	WarnBackupFinalDropped = "backup_final_dropped"
	// WarnBackupUnknownPreset — preset id вне шаблона принимающей стороны.
	WarnBackupUnknownPreset = "backup_unknown_preset"
	// WarnBackupVarSkipped — переменная не в списке переносимых.
	WarnBackupVarSkipped = "backup_var_skipped"
	// WarnBackupUnknownField — ключ вне схемы: в состояние не попадает (П3).
	// Detail называет и ключ, и сущность, в которой он встретился.
	WarnBackupUnknownField = "backup_unknown_field"
	// WarnBackupFieldTypeMismatch — ключ модели пришёл ЧУЖОГО ТИПА: поле
	// отбрасывается, разбор файла продолжается. Отдельный код, а не
	// backup_unknown_field: ключ-то знакомый, разошёлся его тип
	// (`subscriptions[].skip` — boolean у LxBox 0.10.x, список фильтров
	// отсева у launcher), и пользователю важно различать «такого поля тут
	// нет» и «поле есть, но значение записано по-другому». Detail называет
	// полный путь: subscriptions[https://…].skip.
	WarnBackupFieldTypeMismatch = "backup_field_type_mismatch"
	// WarnBackupExtensionsDropped — файл схемы 0.10.x с механизмом
	// extensions. Один warning на файл с перечнем затронутых записей: пока
	// extensions существовал, он был не «одним лишним ключом», а карманом с
	// произвольным содержимым, и перечислять его внутренности по одной
	// значило бы утопить пользователя в списке.
	WarnBackupExtensionsDropped = "backup_extensions_dropped"
	// WarnBackupDirectionExists — Направление с таким тегом уже есть:
	// приехавшее НЕ применяется. Перезапись стёрла бы настройки, сделанные
	// на этой машине, а правила и так найдут цель по тегу (SPEC 104).
	WarnBackupDirectionExists = "backup_direction_exists"
	// WarnBackupSourceKindUnsupported — вид источника, которого контракт
	// 0.11 не знает (папка, провайдерская группа): в файл он не поехал.
	// Секция folders[] — отдельный трек с LxBox-стороной (SPEC 118 §2).
	WarnBackupSourceKindUnsupported = "backup_source_kind_unsupported"
	// WarnBackupTagMaskDropped — `tag.mask` ПОДПИСКИ не применён: маска была
	// шаблоном имени для каждой ноды (`{$label}` и подстановки), а в модели
	// v7 у контейнера остались только prefix/postfix. У одиночного узла
	// (server/chain) маска — это имя самого узла, и она молча становится
	// его тегом; предупреждение ставится только там, где ПОТЕРЯ реальна.
	WarnBackupTagMaskDropped = "backup_tag_mask_dropped"
	// WarnBackupLocalDirectionDropped — локальное Направление источника,
	// которое не породила свёртка: класс упразднён (SPEC 118), переносить
	// его некуда. Fold-производная пара (`<PFX>select`/`<PFX>auto`) сюда не
	// попадает — она приезжает заменой (FolderReplace), а не потерей.
	WarnBackupLocalDirectionDropped = "backup_local_direction_dropped"
	// WarnBackupReplaceTagDerived — ЯВНЫЙ тег замены папки/подписки не
	// переживает контракт 0.11: там свёртка несла только режим, а имя группы
	// было позиционным деривативом префикса. На приёмнике группа получит
	// деривативное имя, и правила, метившие в прежнее, приедут выключенными.
	// Предупреждение ставится на ЭКСПОРТЕ — там, где ещё видно оба имени.
	WarnBackupReplaceTagDerived = "backup_replace_tag_derived"
	// WarnBackupChainExists — цепочка с таким тегом уже есть: приехавшая НЕ
	// применяется, своя сильнее. Warning ставится ВСЕГДА, даже когда «своя
	// победила» — молчание скрыло бы случайных тёзок: две несвязанные
	// цепочки, одинаково названные на разных устройствах, склеиваются в
	// одну, и пользователь обязан узнать об этом (BACKUP.md §4).
	WarnBackupChainExists = "backup_chain_exists"
	// WarnBackupSourceIdentityDropped — ключи ОБЪЕКТА identity, которые
	// принимающая сторона не применяет: mobile-only device_os / ver_os /
	// device_model и всё незнакомое. Код один на оба направления и обе
	// стороны — потеря для пользователя одна и та же («подписка спросит
	// провайдера не тем, чем спрашивала»), и разные коды заставили бы UI
	// объяснять два раза одно.
	//
	// Ставится ОДИН на подписку, с перечнем неприменённых ключей: потеря у
	// пользователя одна, а не по строке на ключ. Потеря не косметическая —
	// провайдеры ВЕТВЯТ выдачу по UA, и на новой машине та же ссылка отдаст
	// другой набор узлов.
	//
	// Для настроек, которым в схеме дома нет вовсе, код другой —
	// WarnBackupLocalOnlyDropped: «ключ есть, но здесь не применяется» и
	// «такого поля в общем формате нет» — разные разговоры с пользователем.
	WarnBackupSourceIdentityDropped = "backup_source_identity_dropped"
	// WarnBackupLocalOnlyDropped — настройки СВОЕЙ стороны, которым в общей
	// схеме дома нет: они остаются здесь и в файл не едут.
	//
	// Один код на оба направления и обе стороны. У лаунчера это
	// relays_in_directions подписки (у LxBox такой развилки нет вовсе),
	// настройки самой папки (tag_policy, replace, detour) и её члены,
	// которых секция servers[] выразить не может (цепочки, неразобранные
	// записи); у LxBox — import_rules, свои настройки папки и политика
	// detour источника.
	//
	// Почему не «завести поля в схеме»: односторонний ключ в общем формате —
	// это возвращённый тайный груз, ради сноса которого убран механизм
	// extensions (П1/П3). Честный ход — объявить потерю вслух (П6), а не
	// возить непонятное чужой стороне. Detail: `<сущность>: <поля>`.
	WarnBackupLocalOnlyDropped = "backup_local_only_dropped"
	// WarnBackupSourceFlagDropped — `exclude_from_global` /
	// `expose_group_tags_to_global` приехали из бэкапа v1.5.x: класс флагов
	// упразднён (SPEC 118), узлы источника остаются в общем пуле кандидатов.
	// Поля объявлены в типах контракта, поэтому scanUnknown их не ловит —
	// без этого кода они пропадали бы совсем молча.
	WarnBackupSourceFlagDropped = "backup_source_flag_dropped"
	// WarnBackupLabelDropped — `label` одиночного СЕРВЕРА разошёлся с тегом
	// и применён не будет: у канона v7 имени, кроме тега, нет (SPEC 112,
	// «идентичность узла = тег»). Раньше label клался в Source.Label — поле
	// с `json:"-"`, то есть умирал на первом же Save, а пользователю об этом
	// не говорили. У цепочек и Направлений кода нет: там `label` с контракта
	// 0.12.4 — объявленное поле LxBox, игнорируемое МОЛЧА (D-094).
	WarnBackupLabelDropped = "backup_label_dropped"
)

// errSkipRule — правило пропущено осознанно (чужой kind), а не сломалось.
// Отдельная ошибка, а не bool: вызывающий обязан различать «пропусти это» и
// «импорт невозможен», иначе одно чужое правило уронит весь файл.
var errSkipRule = errors.New("rule skipped")

// ImportOptions — контекст принимающей стороны.
//
// Режима «заменить всё» нет и флага режима нет (BACKUP.md §9, D-095):
// ИСТОЧНИКИ всегда сливаются по идентичности — подписки по URL, одиночные
// серверы по телу, цепочки и Направления по тегу; DNS сливается «своё
// сильнее», warp[] добавляется. Полной замене подлежит ровно одна секция —
// rules[].
//
// Почему у правил слияния нет: у сторон свои абсолютные номера оси порядка,
// и «долить» чужие правила означало бы перенумеровать уже стоявшие, включая
// якорные зоны шаблона (SPEC 113-C, инвариант «файл = ось»). У остальных
// секций такой оси нет — там идентичность есть у каждой записи.
type ImportOptions struct {
	// KnownOutbounds — теги, на которые правилу разрешено ссылаться.
	// Пустой список означает «проверять нечем» — тогда ссылки не режутся:
	// выключить всё подряд хуже, чем импортировать как есть.
	KnownOutbounds []string
	// KnownPresets — id пресетов шаблона принимающей стороны.
	KnownPresets []string
}

// ImportResult — что получилось.
type ImportResult struct {
	Warnings []Warning
	// AppliedRules / AppliedSources / AppliedDirections — сколько записей
	// реально применено.
	AppliedRules      int
	AppliedSources    int
	AppliedDirections int

	// Раскладка применённого по видам события — для отчёта UI (§9 п. 8).
	//
	// Одного числа «применено» после перехода на слияние мало: «добавлено 3»
	// и «обновлено 3» для пользователя — разные новости (в первом случае у
	// него стало больше источников, во втором переписаны настройки уже
	// стоявших), а «пропущено» вообще не потеря и не повод для warning'а —
	// это «такой сервер у тебя уже есть».
	AddedSubscriptions   int
	UpdatedSubscriptions int
	AddedServers         int
	// SkippedServers — записи servers[], чьё тело уже лежит здесь: дубли
	// пропущены МОЛЧА, без warning (§9 п. 2).
	SkippedServers int
	AddedFolders   int
	AddedChains    int
}

// Import применяет бэкап к state.
//
// Состояние после импорта неотличимо от настроенного руками (П1): теневых
// полей «на провоз» нет, непонятое отброшено и названо warning'ом (П3).
// Warning'и о неизвестных ключах и об extensions выдаёт Parse — он один видит
// сырой JSON; здесь они не дублируются.
//
// Порядок разделов значим: источники раньше правил, потому что правило может
// ссылаться на тег, который приезжает вместе с источником.
func Import(s *state.State, b *Backup, opts ImportOptions) (*ImportResult, error) {
	if s == nil {
		return nil, fmt.Errorf("nil state")
	}
	if b == nil {
		return nil, fmt.Errorf("nil backup")
	}
	if b.LxBackup > FormatVersion {
		return nil, fmt.Errorf("backup format v%d is newer than supported v%d — update the app",
			b.LxBackup, FormatVersion)
	}

	res := &ImportResult{}

	// Источники СЛИВАЮТСЯ по идентичности, а не замещаются (D-095, §9):
	// локальное, чего в файле нет, остаётся жить; совпавшее получает
	// настройки файла, не теряя истории; несовпавшее дописывается в конец.
	//
	// rules[] — ЕДИНСТВЕННАЯ секция полной замены (§9 п. 7): ось порядка у
	// сторон своя, и «долить» чужие номера в неё нечем. DNS и warp[] ниже
	// сливаются, каждая по своему ключу.
	s.Rules = nil

	var cnt mergeCounters
	mergeSubscriptions(s, b.Subscriptions, &res.Warnings, &cnt)
	// Серверы: запись с пометкой folder уходит не в корень списка, а в
	// папку с этим именем (контракт 0.12). Порядок записей в файле
	// нормативен, дедуп — по ТЕЛУ записи (§9 пп. 2–3).
	mergeServers(s, b.Servers, &res.Warnings, &cnt)

	res.AddedSubscriptions = cnt.AddedSubscriptions
	res.UpdatedSubscriptions = cnt.UpdatedSubscriptions
	res.AddedServers = cnt.AddedServers
	res.SkippedServers = cnt.SkippedServers
	res.AddedFolders = cnt.AddedFolders
	res.AppliedSources += cnt.AddedSubscriptions + cnt.UpdatedSubscriptions +
		cnt.AddedServers + cnt.AddedFolders

	// SPEC 104: Направления импортируются ДО правил и пополняют список
	// известных целей — иначе правило, чья цель приехала в этом же файле,
	// импортировалось бы выключенным.
	//
	// Существующий тег не трогаем: у принимающей стороны своё Направление с
	// этим именем, и перезапись стёрла бы его настройки.
	existing := make(map[string]bool, len(s.Directions))
	for _, d := range s.Directions {
		existing[d.Tag] = true
	}
	knownTags := append([]string(nil), opts.KnownOutbounds...)
	// Группы, которые породит свёртка приехавших подписок (D-081): правила и
	// route.final ТОГО ЖЕ файла ссылаются на позиционный дериватив
	// `<N>:select` / `<N>:auto`, а принимающая сторона о нём знать не может —
	// её список известных целей снят ДО импорта. Без этого пополнения перенос
	// свёрнутой подписки приезжал маршрутизацией в никуда: правило
	// выключалось `backup_unknown_outbound`, final отбрасывался, и оба —
	// по причине «цель не существует», хотя цель приехала этим же файлом.
	for i, sub := range b.Subscriptions {
		for tag := range foldDerivedDirectionTags(sub, i) {
			knownTags = append(knownTags, tag)
		}
	}
	for _, in := range b.Directions {
		if in.Tag == "" {
			continue
		}
		if existing[in.Tag] {
			res.Warnings = append(res.Warnings, Warning{Code: WarnBackupDirectionExists, Detail: in.Tag})
			knownTags = append(knownTags, in.Tag)
			continue
		}
		s.Directions = append(s.Directions, importDirection(in))
		existing[in.Tag] = true
		knownTags = append(knownTags, in.Tag)
		res.AppliedDirections++
	}

	// Цепочки — ПОСЛЕ Направлений (позиция может ссылаться на Направление) и
	// ДО правил (правило может метить в цепочку как в цель — тег пополняет
	// список известных). Порядок записей нормативен и сохраняется как есть.
	// Достижимость hops здесь не проверяется: хоп — чаще всего узел подписки,
	// которого до её обновления не существует; рубеж у обеих сторон один —
	// сборка (chain_hop_missing).
	existingChains := map[string]bool{}
	for _, src := range s.Sources {
		if src.Kind == state.SourceKindChain {
			existingChains[src.NodeTagOrLabel()] = true
		}
	}
	chainIDs := takenSourceIDs(s.Sources)
	for _, in := range b.Chains {
		if in.Tag == "" || in.Chain == nil {
			continue
		}
		if existingChains[in.Tag] {
			res.Warnings = append(res.Warnings, Warning{Code: WarnBackupChainExists, Detail: in.Tag})
			knownTags = append(knownTags, in.Tag)
			continue
		}
		src, warns := importChain(in)
		// id из файла держится, пока он свободен: при коллизии с уже
		// живущим источником — свежий ULID (§9 п. 7). Два источника с одним
		// id дали бы двух владельцев одной адресации.
		src.ID = freshIDIfTaken(src.ID, chainIDs)
		chainIDs[src.ID] = true
		s.Sources = append(s.Sources, src)
		res.Warnings = append(res.Warnings, warns...)
		existingChains[in.Tag] = true
		knownTags = append(knownTags, in.Tag)
		res.AppliedSources++
		res.AddedChains++
	}

	// Позиции цепочек приехали строками (контракт 0.11 адреса папок не несёт):
	// поднимаем их до адресных ссылок по ЖИВОМУ набору — уже импортированные
	// источники плюс Направления принимающей стороны. Проход отдельный и
	// последний, потому что видеть он обязан ВЕСЬ набор: цепочка может
	// ссылаться на узел подписки, объявленной ниже неё.
	resolveImportedHops(s.Sources, s.Directions)

	known := newTagSet(knownTags)
	presets := newTagSet(opts.KnownPresets)

	for _, r := range b.Rules {
		rule, warns, err := importRule(r, known, presets)
		res.Warnings = append(res.Warnings, warns...)
		if errors.Is(err, errSkipRule) {
			continue // правило не наше — пропущено с warning, импорт живёт
		}
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", ruleLabel(r), err)
		}
		s.Rules = append(s.Rules, rule)
		res.AppliedRules++
	}

	// Ось порядка перенумеровывается: абсолютные номера у сторон свои, важен
	// лишь относительный порядок (BACKUP.md §2).
	renumberImportedRules(s.Rules)

	if b.Route != nil && b.Route.Final != "" {
		if known.empty() || known.has(b.Route.Final) {
			setConfigParam(s, "final", b.Route.Final)
		} else {
			res.Warnings = append(res.Warnings, Warning{Code: WarnBackupFinalDropped, Detail: b.Route.Final})
		}
	}

	res.Warnings = append(res.Warnings, importVars(s, b.Vars)...)

	importDNS(s, b.DNS)
	importWarp(s, b.Warp)

	return res, nil
}

// importDirections — обратная сторона exportDirections.
func importDirections(list []Direction) []configtypes.Direction {
	var out []configtypes.Direction
	for _, in := range list {
		if in.Tag == "" {
			continue
		}
		out = append(out, importDirection(in))
	}
	return out
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
	if id.UserAgent != nil {
		src.UserAgent = *id.UserAgent
	}
	if id.HWID != nil {
		src.HWID = *id.HWID
	}
	if id.SendHWID != nil {
		v := *id.SendHWID
		src.SendHWID = &v
	}
	if id.HashDeviceModel != nil {
		v := *id.HashDeviceModel
		src.HashDeviceModel = &v
	}
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

func importRule(r Rule, known, presets tagSet) (state.Rule, []Warning, error) {
	var warns []Warning
	enabled := r.Enabled == nil || *r.Enabled

	// Символическая ссылка в никуда: правило приезжает выключенным, а не
	// теряется. Ядро отвергает конфиг с несуществующим outbound целиком,
	// поэтому «оставить включённым» здесь означало бы сломать пользователю
	// весь VPN одним импортом (BACKUP.md §3).
	if r.Outbound != "" && !known.empty() && !known.has(r.Outbound) {
		enabled = false
		warns = append(warns, Warning{Code: WarnBackupUnknownOutbound, Detail: ruleLabel(r) + " → " + r.Outbound})
	}

	out := state.Rule{
		Kind:    state.RuleKind(r.Kind),
		Enabled: enabled,
	}
	if r.Num != nil {
		n := int(*r.Num)
		out.OrderNum = &n
	}

	switch RuleKind(r.Kind) {
	case RulePreset:
		out.Ref = r.Ref
		if !presets.empty() && !presets.has(r.Ref) {
			out.Enabled = false
			warns = append(warns, Warning{Code: WarnBackupUnknownPreset, Detail: r.Ref})
		}
		body := state.PresetBody{Vars: r.Vars}
		raw, err := json.Marshal(body)
		if err != nil {
			return state.Rule{}, warns, err
		}
		out.Body = raw
	case RuleInline:
		var match map[string]interface{}
		if len(r.Match) > 0 {
			if err := json.Unmarshal(r.Match, &match); err != nil {
				return state.Rule{}, warns, fmt.Errorf("match: %w", err)
			}
		}
		raw, err := json.Marshal(state.InlineBody{Name: r.Name, Match: match, Outbound: r.Outbound})
		if err != nil {
			return state.Rule{}, warns, err
		}
		out.Body = raw
	case RuleSRS:
		raw, err := json.Marshal(state.SrsBody{Name: r.Name, SrsURL: r.Ref, Outbound: r.Outbound})
		if err != nil {
			return state.Rule{}, warns, err
		}
		out.Body = raw
	case RuleJSON:
		// kind=json — сырое правило другой стороны: применять вслепую нельзя
		// (структура чужая). Но и ронять весь импорт из-за одного правила
		// нельзя — пользователь потеряет всё остальное. Правило
		// пропускается, факт называется.
		return state.Rule{}, append(warns, Warning{Code: WarnBackupUnknownField, Detail: "rules[].kind=json: " + ruleLabel(r)}), errSkipRule
	default:
		return state.Rule{}, append(warns, Warning{Code: WarnBackupUnknownField, Detail: "rules[].kind=" + string(r.Kind)}), errSkipRule
	}

	return out, warns, nil
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

// renumberImportedRules перенумеровывает ось порядка, сохраняя относительный
// порядок: у сторон свои диапазоны, а важен лишь порядок следования.
//
// SPEC 113-C §1: перенумерация заканчивается пересортировкой массива. Иначе
// импорт оставлял бы файл, где номера говорят одно, а порядок записей другое —
// а закон оси запрещает читать позицию в слайсе как самостоятельный смысл.
func renumberImportedRules(rules []state.Rule) {
	idx := make([]int, 0, len(rules))
	for i, r := range rules {
		if r.OrderNum != nil {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return *rules[idx[a]].OrderNum < *rules[idx[b]].OrderNum
	})
	for pos, i := range idx {
		n := state.UserRuleNumStart + pos
		rules[i].OrderNum = &n
	}

	// Неразмеченные (бэкап без num) уезжают в хвост, сохраняя взаимный
	// порядок: разметку им раздаст MarkRuleOrder на первой загрузке, и она
	// пойдёт от конца занятой части — иначе они перебили бы перенумерованных.
	sort.SliceStable(rules, func(a, b int) bool {
		return importedAxisNum(rules[a]) < importedAxisNum(rules[b])
	})
}

// importedAxisNum — номер для сортировки импортированных: неразмеченное
// правило считается стоящим за всеми размеченными.
func importedAxisNum(r state.Rule) int {
	if r.OrderNum == nil {
		return state.UserRuleNumEnd + 1
	}
	return *r.OrderNum
}

func importVars(s *state.State, vars map[string]string) []Warning {
	if len(vars) == 0 {
		return nil
	}
	var warns []Warning
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !IsPortableVar(name) {
			// Непереносимое имя на этой машине значит другое (путь,
			// интерфейс, платформенный флаг) — применять нельзя.
			warns = append(warns, Warning{Code: WarnBackupVarSkipped, Detail: name})
			continue
		}
		setVar(s, name, vars[name])
	}
	return warns
}

func setVar(s *state.State, name, value string) {
	for i := range s.Vars {
		if s.Vars[i].Name == name {
			s.Vars[i].Value = value
			return
		}
	}
	s.Vars = append(s.Vars, state.SettingVar{Name: name, Value: value})
}

func setConfigParam(s *state.State, name, value string) {
	for i := range s.ConfigParams {
		if s.ConfigParams[i].Name == name {
			s.ConfigParams[i].Value = value
			return
		}
	}
	s.ConfigParams = append(s.ConfigParams, state.ConfigParam{Name: name, Value: value})
}

// tagSet — множество известных тегов с нормализацией регистра.
type tagSet map[string]struct{}

func newTagSet(items []string) tagSet {
	if len(items) == 0 {
		return nil
	}
	out := make(tagSet, len(items))
	for _, it := range items {
		out[strings.ToLower(strings.TrimSpace(it))] = struct{}{}
	}
	return out
}

func (t tagSet) empty() bool { return len(t) == 0 }

func (t tagSet) has(tag string) bool {
	if t.empty() {
		return false
	}
	// Зарезервированные литералы существуют всегда: их не нужно объявлять.
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "direct", "block", "reject", "drop", "dns-out":
		return true
	}
	_, ok := t[strings.ToLower(strings.TrimSpace(tag))]
	return ok
}

// importDNS СЛИВАЕТ секцию DNS: своё сильнее, новое дописывается в конец.
//
// Не замена (уточнение D-095): список DNS-серверов у пользователя собран под
// его сеть — корпоративный резолвер, локальный DoH, порядок как рубеж
// утечки, — и снести его целиком чужим файлом значило бы поменять человеку
// разрешение имён, ничего об этом не сказав. Совпавшая запись остаётся
// ЛОКАЛЬНОЙ (её тело и включённость — решение этой машины), несовпавшая
// дописывается в конец в порядке файла.
//
// Ключ сервера — kind+tag: имя DNS-сервера и есть его идентичность (на него
// метят detour и dns-правила), а тело за ним — настройка, которую здесь
// правили. Ключ dns-правила — kind+ref+тело: своего имени у правила нет,
// и различить два правила можно только тем, что они делают.
//
// Тела переносятся только у kind=user (симметрия с exportDNS: тело
// template/preset принадлежит шаблону принимающей стороны).
//
// final и strategy — ЗАМЕЩАЮТСЯ файлом: это одиночные значения, а не список,
// и «слить» два взаимоисключающих ответа нечем.
func importDNS(s *state.State, dns *DNS) {
	if dns == nil {
		return
	}

	serverKey := func(kind, tag string) string { return kind + "\x00" + tag }
	haveServers := map[string]bool{}
	for _, srv := range s.DNS.Servers {
		haveServers[serverKey(string(srv.Kind), srv.Tag)] = true
	}
	for _, ref := range dns.Servers {
		key := serverKey(ref.Kind, ref.Name)
		if haveServers[key] {
			continue // своё сильнее
		}
		srv := state.DNSServer{
			Kind:    state.DNSServerKind(ref.Kind),
			Tag:     ref.Name,
			Ref:     ref.Ref,
			Enabled: ref.Enabled == nil || *ref.Enabled,
		}
		if ref.Kind == "user" && len(ref.Value) > 0 {
			var body map[string]interface{}
			if json.Unmarshal(ref.Value, &body) == nil {
				srv.Body = body
			}
		}
		s.DNS.Servers = append(s.DNS.Servers, srv)
		haveServers[key] = true
	}

	ruleKey := func(kind, ref string, body map[string]interface{}) string {
		raw, _ := json.Marshal(canonicalJSONValue(body))
		return kind + "\x00" + ref + "\x00" + string(raw)
	}
	haveRules := map[string]bool{}
	for _, r := range s.DNS.Rules {
		haveRules[ruleKey(string(r.Kind), r.Ref, r.Body)] = true
	}
	for _, ref := range dns.Rules {
		var body map[string]interface{}
		if ref.Kind == "user" && len(ref.Value) > 0 {
			_ = json.Unmarshal(ref.Value, &body)
		}
		key := ruleKey(ref.Kind, ref.Ref, body)
		if haveRules[key] {
			continue // своё сильнее
		}
		s.DNS.Rules = append(s.DNS.Rules, state.DNSRule{
			Kind:    state.DNSRuleKind(ref.Kind),
			Ref:     ref.Ref,
			Enabled: ref.Enabled == nil || *ref.Enabled,
			Body:    body,
		})
		haveRules[key] = true
	}
	if dns.Final != "" {
		s.DNS.Final = dns.Final
	}
	if dns.Strategy != "" {
		s.DNS.Strategy = dns.Strategy
	}
}

// importWarp восстанавливает WG/MASQUE-регистрации из warp[].
//
// ДОБАВЛЕНИЕ, а не замена (§9 п. 6): аккаунт WARP — это выданная Cloudflare
// регистрация, привязанная к ключу, и затереть здешнюю приехавшей значило бы
// осиротить узлы, которые на ней стоят. У лаунчера слот один на тип (WG и
// MASQUE), поэтому «добавить» здесь и значит «занять пустой слот»: занятый
// остаётся своим, дубль пропускается молча.
//
// Ключ аккаунта — device_id, а при его отсутствии приватный ключ: это то, чем
// регистрация опознаётся у Cloudflare. Сравнение нужно и при занятом слоте:
// свой и приехавший могут оказаться ОДНОЙ регистрацией (файл с этой же
// машины), и тогда «пропущено» — правда, а не потеря.
func importWarp(s *state.State, warp []json.RawMessage) {
	for _, raw := range warp {
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &head) != nil {
			continue
		}
		switch head.Type {
		case "wg":
			var acc state.WarpWGAccount
			if json.Unmarshal(raw, &acc) != nil || acc.PrivateKey == "" {
				continue
			}
			if s.WarpAccounts == nil {
				s.WarpAccounts = &state.WarpAccountsSection{}
			}
			// Занятый слот остаётся своим: локальная регистрация уже могла
			// раздать адреса живущим узлам.
			if s.WarpAccounts.WG == nil {
				s.WarpAccounts.WG = &acc
			}
		case "masque":
			var acc state.WarpMasqueAccount
			if json.Unmarshal(raw, &acc) != nil || acc.PrivateKeyDER == "" {
				continue
			}
			if s.WarpAccounts == nil {
				s.WarpAccounts = &state.WarpAccountsSection{}
			}
			if s.WarpAccounts.Masque == nil {
				s.WarpAccounts.Masque = &acc
			}
		}
	}
}
