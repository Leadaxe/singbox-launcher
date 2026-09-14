package backup

// Импорт LX Backup в состояние лаунчера: ОДИН код слияния на два формата
// файла (SPEC 127 §6.2).
//
// Разрез: формат превращает файл в записи состояния (`legacy_read_0x.go` для
// 0.x, `import10.go` для 1.0 — оба отдают decodedFile), а дальше работает
// applyDecoded — единственный носитель правил §9 BACKUP.md. Правила эти не
// меняются волной 2 ни на строку: подписки по `url` байт-в-байт, серверы по
// телу, папки по имени, цепочки и Направления по тегу, DNS «своё сильнее»,
// `rules[]` — единственная полная замена, `vars` переносимые, `route.final`
// при известной цели, `warp` добавлением.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
	// Reason — ПОЧЕМУ запись отброшена, когда причин у одного кода несколько.
	//
	// Заведено под `backup_section_record_dropped` (норма B3,
	// NODE_SECTIONS.md §1): запись секции отбрасывается либо из-за чужого
	// ВИДА (`kind` — preset у правила, template/preset у DNS), либо из-за
	// `rule_set` в теле, либо из-за того, что узлу секции не положены вовсе
	// (`not_allowed`). Потеря для пользователя разная по смыслу — «вид записи
	// у узла не бывает» против «набор правил узлу не принадлежит», — а код
	// один, и различать их приклеиванием слов к Detail значило бы заставить
	// UI разбирать человеческую строку обратно.
	//
	// Значения: `kind` | `rule_set` | `not_allowed`.
	Reason string
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
	// WarnBackupSectionRecordDropped — запись ЧУЖОГО ВИДА внутри секций узла
	// (NODE_SECTIONS.md §1, D-102): у правил узла бывают только `inline` и
	// `srs`, у его DNS-записей — только `user`.
	//
	// Почему вид ограничен: `preset` означал бы ссылку на шаблон, которого на
	// принимающей машине может не быть, а `template`/`preset` у DNS — тонкие
	// ссылки, которым у узла ссылаться не на что. Такая запись применяется в
	// никуда, поэтому отбрасывается — но ОСТАЛЬНЫЕ записи узла живут: одна
	// чужая строка не стоит всей связки.
	//
	// Код на импорте, а не в состоянии: раньше отсев делал dropForeignKinds
	// (core/state/node_sections.go) и писал только в WarnLog — пользователь,
	// принёсший файл, о потере не узнавал. Detail и Kind называют узел и вид
	// отброшенной записи.
	//
	// Тем же кодом отбрасывается правило с `rule_set` в теле (норма B3) и
	// поле `sections` у узла, которому оно не положено. Три причины различает
	// поле Warning.Reason (`kind` | `rule_set` | `not_allowed`): потеря
	// одинаково называется кодом, но объясняется пользователю по-разному.
	WarnBackupSectionRecordDropped = "backup_section_record_dropped"
)

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
	// UpdatedFolders — папки, совпавшие по имени, чьи собственные настройки
	// (политика тегов, свёртка, общий detour) заместились файлом. Считается
	// только для формата 1.0: у 0.12 у папки своей записи нет вовсе.
	UpdatedFolders int
	AddedChains    int
}

// Import применяет бэкап формата 0.x к состоянию.
//
// Состояние после импорта неотличимо от настроенного руками (П1): теневых
// полей «на провоз» нет, непонятое отброшено и названо warning'ом (П3).
// Warning'и о неизвестных ключах и об extensions выдаёт Parse — он один видит
// сырой JSON; здесь они не дублируются.
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
	dec, err := decodeLegacy(b, opts)
	if err != nil {
		return nil, err
	}
	return applyDecoded(s, dec, opts)
}

// Import10 применяет бэкап формата 1.0 к состоянию.
//
// Тот же код слияния, что у 0.x: разница между форматами кончается в
// декодере. Иначе один и тот же файл, сохранённый двумя писателями, давал бы
// у пользователя два разных состояния.
func Import10(s *state.State, b *Backup10, opts ImportOptions) (*ImportResult, error) {
	if s == nil {
		return nil, fmt.Errorf("nil state")
	}
	if b == nil {
		return nil, fmt.Errorf("nil backup")
	}
	if b.LxBackup > FormatVersion10 {
		return nil, fmt.Errorf("backup format v%d is newer than supported v%d — update the app",
			b.LxBackup, FormatVersion10)
	}
	dec, err := decode10(b, opts)
	if err != nil {
		return nil, err
	}
	return applyDecoded(s, dec, opts)
}

// ImportFile применяет разобранный файл любого читаемого формата.
//
// Точка входа для тех, кто получил файл от Parse/ReadFile и про формат знать
// не обязан (UI, debug API): развилка форматов живёт здесь, а не у каждого
// вызывающего.
func ImportFile(s *state.State, f *File, opts ImportOptions) (*ImportResult, error) {
	if f == nil {
		return nil, fmt.Errorf("nil backup")
	}
	switch {
	case f.V10 != nil:
		return Import10(s, f.V10, opts)
	case f.Legacy != nil:
		return Import(s, f.Legacy, opts)
	default:
		return nil, fmt.Errorf("nil backup")
	}
}

// applyDecoded — ЕДИНСТВЕННОЕ слияние: записи файла в состояние по §9.
//
// Порядок разделов значим: источники раньше правил, потому что правило может
// ссылаться на тег, который приезжает вместе с источником; Направления раньше
// цепочек (позиция цепочки может метить в Направление).
func applyDecoded(s *state.State, dec *decodedFile, opts ImportOptions) (*ImportResult, error) {
	if s == nil {
		return nil, fmt.Errorf("nil state")
	}
	if dec == nil {
		return nil, fmt.Errorf("nil backup")
	}

	res := &ImportResult{Warnings: append([]Warning(nil), dec.Warnings...)}

	// Источники СЛИВАЮТСЯ по идентичности, а не замещаются (D-095, §9):
	// локальное, чего в файле нет, остаётся жить; совпавшее получает
	// настройки файла, не теряя истории; несовпавшее дописывается в конец.
	//
	// rules[] — ЕДИНСТВЕННАЯ секция полной замены (§9 п. 7): ось порядка у
	// сторон своя, и «долить» чужие номера в неё нечем. DNS и warp[] ниже
	// сливаются, каждая по своему ключу.
	s.Rules = nil

	// Занятые имена КОРНЕВОГО пространства снимаются ДО импорта Направлений.
	//
	// Тег Направления живёт в том же пространстве, что тег узла, и если
	// посчитать занятость после — приехавший узел-тёзка Направления из того
	// же файла получил бы суффикс `-2` там, где раньше приезжал своим именем.
	// Снимок, а не порядок разделов: источники обязаны сливаться ОДНИМ
	// проходом в порядке файла (§9 п. 8), иначе состав s.Sources после
	// импорта зависел бы от вида записи, а не от файла, и обратный экспорт
	// переставлял бы записи местами.
	rootTags := takenRootTags(s)

	// Направления — ДО источников: цепочка может встать позицией на
	// Направление, а правило — метить в него целью. Существующий тег не
	// трогаем: у принимающей стороны своё Направление с этим именем, и
	// перезапись стёрла бы его настройки.
	existing := make(map[string]bool, len(s.Directions))
	for _, d := range s.Directions {
		existing[d.Tag] = true
	}
	for _, in := range dec.Directions {
		if in.Tag == "" {
			continue
		}
		if existing[in.Tag] {
			res.Warnings = append(res.Warnings, Warning{Code: WarnBackupDirectionExists, Detail: in.Tag})
			continue
		}
		s.Directions = append(s.Directions, in)
		existing[in.Tag] = true
		res.AppliedDirections++
	}

	var cnt mergeCounters
	merged := mergeSources(s, dec.Sources, rootTags, &res.Warnings, &cnt)

	res.AddedSubscriptions = cnt.AddedSubscriptions
	res.UpdatedSubscriptions = cnt.UpdatedSubscriptions
	res.AddedServers = cnt.AddedServers
	res.SkippedServers = cnt.SkippedServers
	res.AddedFolders = cnt.AddedFolders
	res.UpdatedFolders = cnt.UpdatedFolders
	res.AddedChains = cnt.AddedChains
	res.AppliedSources += cnt.AddedSubscriptions + cnt.UpdatedSubscriptions +
		cnt.AddedServers + cnt.AddedFolders + cnt.UpdatedFolders + cnt.AddedChains

	// Ссылки на папки формата 1.0 несут ULID машины-ЭКСПОРТЁРА; совпавшая по
	// имени папка здесь держит свой. Переписка идёт последним проходом,
	// когда карта «id файла → id здесь» собрана целиком: папка может быть
	// объявлена в файле НИЖЕ записи, которая на неё ссылается.
	merged.rewriteFolderLinks(s)

	// Позиции цепочек приехали строками (контракт 0.x адреса папок не несёт):
	// поднимаем их до адресных ссылок по ЖИВОМУ набору — уже импортированные
	// источники плюс Направления принимающей стороны. Проход отдельный и
	// последний, потому что видеть он обязан ВЕСЬ набор: цепочка может
	// ссылаться на узел подписки, объявленной ниже неё. Ссылки формата 1.0
	// адрес уже несут (folder_id) и этим проходом не трогаются.
	resolveImportedHops(s.Sources, s.Directions)

	s.Rules = append(s.Rules, dec.Rules...)
	res.AppliedRules = len(dec.Rules)

	// Ось порядка перенумеровывается: абсолютные номера у сторон свои, важен
	// лишь относительный порядок (BACKUP.md §2). Правила, которые узлы носят
	// с собой, идут ТЕМ ЖЕ проходом (NODE_SECTIONS.md §5): разные проходы
	// дали бы пересечение номеров и потерю взаимного порядка.
	renumberImportedAxis(s.Rules, merged.sectionRules(s))

	if dec.RouteFinal != "" {
		known := newTagSet(importKnownTags(opts, dec, s))
		if known.empty() || known.has(dec.RouteFinal) {
			// Канонический канал лаунчера — vars["route_final"]: именно его
			// читает LoadState и пишет Save. config_params["final"] никто не
			// читал и не сохранял — Default direction из файла терялся (#111).
			setVar(s, "route_final", dec.RouteFinal)
		} else {
			res.Warnings = append(res.Warnings, Warning{Code: WarnBackupFinalDropped, Detail: dec.RouteFinal})
		}
	}

	res.Warnings = append(res.Warnings, importVars(s, dec.Vars)...)

	importDNS(s, dec.DNS)
	importWarp(s, dec.Warp)

	return res, nil
}

// importKnownTags — цели, которые считаются существующими при проверке
// route.final.
//
// Состав тот же, что у проверки правил в декодере: то, что знает принимающая
// сторона (opts), плюс теги, приехавшие ЭТИМ ЖЕ файлом. Считается уже ПОСЛЕ
// слияния, по живому состоянию: цепочка, чей тег был занят, в состояние не
// попала, и final в неё — это final в никуда.
func importKnownTags(opts ImportOptions, dec *decodedFile, s *state.State) []string {
	out := append([]string(nil), opts.KnownOutbounds...)
	out = append(out, dec.KnownTagsFromFile...)
	for _, d := range s.Directions {
		if d.Tag != "" {
			out = append(out, d.Tag)
		}
	}
	for i := range s.Sources {
		src := &s.Sources[i]
		switch src.Kind {
		case state.SourceKindChain, state.SourceKindServer, state.SourceKindAuto:
			if t := src.NodeTagOrLabel(); t != "" {
				out = append(out, t)
			}
		}
		if src.Replace != nil && src.Replace.Tag != "" {
			out = append(out, src.Replace.Tag)
			if src.Replace.Mode == state.FolderReplaceBoth {
				out = append(out, src.Replace.Tag+"-auto")
			}
		}
	}
	return out
}

// renumberImportedAxis перенумеровывает ось порядка ЦЕЛИКОМ, сохраняя
// относительный порядок: у сторон свои абсолютные диапазоны, а важен лишь
// порядок следования.
//
// Ось одна на корневые правила и на правила, которые узлы носят с собой
// (NODE_SECTIONS.md §5). Два прохода — по корню и по узлам — дали бы
// пересечение номеров: узловое правило с номером из файла встало бы посреди
// перенумерованных корневых, и порядок, который пользователь видел на той
// машине, здесь не воспроизвёлся бы.
//
// SPEC 113-C §1: перенумерация заканчивается пересортировкой КОРНЕВОГО
// массива. Иначе импорт оставлял бы состояние, где номера говорят одно, а
// порядок записей другое — а закон оси запрещает читать позицию в слайсе как
// самостоятельный смысл. Узловые правила не пересортировываются: их порядок
// внутри узла задаётся номерами, а сам узел в оси не участвует.
func renumberImportedAxis(rules []state.Rule, sectionRules []*state.Rule) {
	// Общая ось: корневые правила и правила приехавших узлов вперемешку.
	// Указатели, а не значения: номер проставляется на месте, в той записи,
	// которая уже лежит в состоянии.
	axis := make([]*state.Rule, 0, len(rules)+len(sectionRules))
	for i := range rules {
		axis = append(axis, &rules[i])
	}
	axis = append(axis, sectionRules...)

	idx := make([]int, 0, len(axis))
	for i, r := range axis {
		if r.Num != nil {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return *axis[idx[a]].Num < *axis[idx[b]].Num
	})
	for pos, i := range idx {
		n := state.UserRuleNumStart + pos
		axis[i].Num = &n
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
	if r.Num == nil {
		return state.UserRuleNumEnd + 1
	}
	return *r.Num
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
// правили. У kind=preset тега НЕТ вовсе (state.DNSServer: `ref` формы
// "<preset_id>:<local_tag>" — вот его идентичность), поэтому в ключ входит и
// `ref`: без него все preset-серверы состояния схлопывались в один ключ
// "preset\x00", и из трёх резолверов пресета импорт молча оставлял первый
// (на живом состоянии владельца из 17 DNS-серверов после импорта в пустое
// оставалось 15 — пропадали russian:yandex_doh и russian:yandex_dot).
// Ключ dns-правила — kind+ref+тело: своего имени у правила нет,
// и различить два правила можно только тем, что они делают.
//
// final и strategy — ЗАМЕЩАЮТСЯ файлом: это одиночные значения, а не список,
// и «слить» два взаимоисключающих ответа нечем.
func importDNS(s *state.State, dns *decodedDNS) {
	if dns == nil {
		return
	}

	// Ключ един для всех видов: у template/user заполнен tag и пуст ref, у
	// preset — наоборот. Разбирать по kind нечего, а один ключ на все виды
	// не даёт завести второй, расходящийся с первым.
	serverKey := func(kind, tag, ref string) string { return kind + "\x00" + tag + "\x00" + ref }
	haveServers := map[string]bool{}
	for _, srv := range s.DNS.Servers {
		haveServers[serverKey(string(srv.Kind), srv.Tag, srv.Ref)] = true
	}
	for _, srv := range dns.Servers {
		key := serverKey(string(srv.Kind), srv.Tag, srv.Ref)
		if haveServers[key] {
			continue // своё сильнее
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
	for _, r := range dns.Rules {
		key := ruleKey(string(r.Kind), r.Ref, r.Body)
		if haveRules[key] {
			continue // своё сильнее
		}
		s.DNS.Rules = append(s.DNS.Rules, r)
		haveRules[key] = true
	}
	if dns.Final != "" {
		s.DNS.Final = dns.Final
	}
	if dns.Strategy != "" {
		s.DNS.Strategy = dns.Strategy
	}
	// Третий скаляр секции — по тому же правилу, что первые два. Форма 0.12
	// его не несла и здесь всегда пуста, форма 1.0 несёт (§6.0); без этой
	// строки писатель 1.0 клал ключ в файл, а читатель терял его молча.
	if dns.DefaultDomainResolver != "" {
		s.DNS.DefaultDomainResolver = dns.DefaultDomainResolver
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
