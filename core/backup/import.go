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

	"singbox-launcher/core/config/configtypes"
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
	// Значения у лаунчера: `kind` | `rule_set` | `not_allowed`. Перечень
	// контракта знает ещё `unknown_key` — его ставит сторона со строгим
	// разбором тела (LxBox); у лаунчера тело сырое, и такой причины нет.
	//
	// У `backup_var_skipped` (SPEC 129 §5.6) — `not_portable` | `undeclared`
	// | `superseded` | `no_record`.
	Reason string
	// Record — носитель переменной у `backup_var_skipped` (SPEC 129 §5.6):
	// `dns:<tag>` — запись шаблонного DNS-сервера, `preset:<ref>` — пресет
	// правила, пусто — корневая переменная. Отдельным полем, а не приклейкой к
	// Detail: UI различает переменную записи и корневую по полю, а не разбором
	// строки.
	Record string
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
	// WarnBackupVarSkipped — переменная не применена: корневая не в списке
	// переносимых (`not_portable`), имя записи не объявлено шаблоном
	// приёмника (`undeclared`), корневое `dns_<tag>_<var>` уступило записи
	// файла со своими vars (`superseded`) или не нашло записи (`no_record`).
	// Причина — Warning.Reason, носитель — Warning.Record (SPEC 129 §5.6).
	WarnBackupVarSkipped = "backup_var_skipped"
	// WarnBackupDNSEntrySkipped — запись шаблонного DNS-сервера, чей тег
	// шаблон приёмника не объявил: тела у неё здесь нет, и запись не
	// ввозится (SPEC 129 §5.2). Detail — `template:<tag>`.
	WarnBackupDNSEntrySkipped = "backup_dns_entry_skipped"
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
	// WarnBackupSourceKindUnsupported — запись вида, которого формат не
	// выражает. На экспорте — корневая провайдерская группа (union sources[]
	// 1.0 её не знает, export10.go): в файл она не поехала. На импорте —
	// источник вида, которого этот union не знает (import10.go): запись не
	// применена, остальные применяются.
	WarnBackupSourceKindUnsupported = "backup_source_kind_unsupported"
	// WarnBackupTagMaskDropped — `tag.mask` ПОДПИСКИ не применён: маска была
	// шаблоном имени для каждой ноды (`{$label}` и подстановки), а в модели
	// v7 у контейнера остались только prefix/postfix. У одиночного узла
	// (server/chain) маска — это имя самого узла, и она молча становится
	// его тегом; предупреждение ставится только там, где ПОТЕРЯ реальна.
	WarnBackupTagMaskDropped = "backup_tag_mask_dropped"
	// WarnBackupLocalDirectionDropped — локальное Направление источника:
	// класс упразднён (SPEC 118), переносить его некуда. С контракта 1.1.79
	// сюда попадает и пара, которую порождала свёртка 0.x
	// (`<PFX>select`/`<PFX>auto`): сама свёртка не читается.
	WarnBackupLocalDirectionDropped = "backup_local_direction_dropped"
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
	// Один код на оба направления и обе стороны. У LxBox это import_rules,
	// свои настройки папки и политика detour источника. У лаунчера им
	// называл потери писатель 0.12 (relays_in_directions подписки, настройки
	// самой папки, её члены не-server, секции узла) — в 1.0 у этих полей есть
	// дом. С 1.6.0 (контракт 1.0.1) писатель 1.0 ставит его на опции
	// Направления, которые не теги Направлений: `include` несёт только их
	// (NODE_LINK.md §8, export10.go).
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
	// WarnBackupDirectionIncludeDropped — строки `include` приехавшего
	// Направления, которые здесь не теги Направлений (этого файла или
	// приёмника) и не объявленные корневые имена: узел, чужая свёртка,
	// неизвестное имя. В опции они не попадают (NODE_LINK.md §8): узел в
	// Направление кладёт фильтр, а имени, которого нет, в конфиге не бывает.
	// Один warning на Направление; Detail — `<тег Направления>: <строки>`.
	WarnBackupDirectionIncludeDropped = "backup_direction_include_dropped"
	// WarnBackupGroupDegraded — провайдерская группа приехала в форме, которую
	// принимающая сторона не выражает, и ввезена упрощённой или не ввезена
	// вовсе. Код общий для обеих сторон, причина — в Warning.Reason: у
	// лаунчера группа по правилу (`group.members_rule` без явного состава) не
	// ввозится — пустую группу сборка выбрасывала бы на каждой сборке
	// (GroupDegradedMembersRule); у LxBox selector с `default` становится
	// urltest, умолчание отбрасывается. Detail — тег группы.
	WarnBackupGroupDegraded = "backup_group_degraded"
)

// GroupDegradedMembersRule — причина WarnBackupGroupDegraded у лаунчера:
// группа задана правилом отбора (`members_rule`), а явного состава нет.
const GroupDegradedMembersRule = "members_rule unsupported"

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
	// BlockTag — тег блокировки шаблона принимающей стороны
	// (TemplateData.DirectionBlockTag): им становится `include_block`
	// Направления. Пусто — `block-out`.
	BlockTag string
	// SystemTags — системные и шаблонные теги принимающей стороны
	// (TemplateData.SystemOutboundTags): законные строки `include` наравне с
	// тегами Направлений и свёрток (NODE_LINK.md §8). Пусто — известны только
	// `direct-out` и тег блокировки.
	SystemTags []string
	// RecordVars — объявления шаблона приёмника для значений переменных
	// записи (SPEC 129): шаблонные DNS-серверы и пресеты с умолчаниями для
	// цели состояния (template.RecordVarDeclsFor). nil — шаблона нет:
	// корневые `dns_<tag>_<var>` не переносятся, записи не нормализуются,
	// типы переменных серверов для Н9 неизвестны.
	RecordVars *state.RecordVarDecls
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

	// SPEC 129: своё хранение приёмника — к нормам записи ДО слияния.
	// Состояние, не пересохранённое после обновления (debug API читает файл
	// с диска), ещё держит корневые `dns_<tag>_<var>`; наложи файл на запись
	// без переноса — и при следующей загрузке запись «со своими vars» вытеснила
	// бы корневое значение приёмника, которого файл не называл (dns_ip).
	state.ApplyRecordVars(s, opts.RecordVars)

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
	var appliedDirections []int
	for _, in := range dec.Directions {
		if in.Tag == "" {
			continue
		}
		if existing[in.Tag] {
			res.Warnings = append(res.Warnings, Warning{Code: WarnBackupDirectionExists, Detail: in.Tag})
			continue
		}
		s.Directions = append(s.Directions, in)
		appliedDirections = append(appliedDirections, len(s.Directions)-1)
		existing[in.Tag] = true
		res.AppliedDirections++
	}

	var cnt mergeCounters
	merged := mergeSources(s, dec.Sources, rootTags, &res.Warnings, &cnt)

	// Опции приехавших Направлений — только объявленные корневые имена.
	// После слияния: свёртка, приехавшая этим же файлом, уже в состоянии.
	res.Warnings = append(res.Warnings,
		filterImportedDirectionOptions(s, dec, appliedDirections, opts)...)

	res.AddedSubscriptions = cnt.AddedSubscriptions
	res.UpdatedSubscriptions = cnt.UpdatedSubscriptions
	res.AddedServers = cnt.AddedServers
	res.SkippedServers = cnt.SkippedServers
	res.AddedFolders = cnt.AddedFolders
	res.UpdatedFolders = cnt.UpdatedFolders
	res.AddedChains = cnt.AddedChains
	res.AppliedSources += cnt.AddedSubscriptions + cnt.UpdatedSubscriptions +
		cnt.AddedServers + cnt.AddedFolders + cnt.UpdatedFolders + cnt.AddedChains

	// Ссылки файла несут адреса машины-ЭКСПОРТЁРА: id контейнеров (совпавшая
	// папка или подписка здесь держит свой) и имена узлов (слияние вправе
	// уникализировать узел или узнать его по телу под другим тегом).
	// Переписка идёт последним проходом, когда карта адресов собрана
	// целиком: цель может быть объявлена в файле НИЖЕ ссылающейся записи.
	merged.rewriteLinks(s)

	// Известные цели — ОДИН список на весь импорт (цели правил, route.final,
	// корневые имена подъёма ссылок), по состоянию ПОСЛЕ слияния: цель может
	// приехать этим же файлом, быть системным тегом шаблона приёмника или
	// корневым узлом. Слияние корневых имён дальше не меняет.
	knownTags := importKnownTags(opts, dec, s)

	// Ссылки без адреса папки поднимаются до адресных по ЖИВОМУ набору, и у
	// каждого формата свой словарь: проход отдельный и последний, потому что
	// видеть он обязан ВЕСЬ набор (цепочка может ссылаться на узел папки,
	// объявленной ниже неё).
	switch dec.Format {
	case FileFormat10:
		// Dev-формы ссылок (член группы в контейнере без folder_id, позиция на
		// группу финальным тегом) — тем же правилом, что у чтения состояния
		// (state.NormalizeNodeLinks), и ДО подъёма корневых ссылок: тот
		// работает уже с парами в норме.
		state.NormalizeNodeLinks(s.Sources, s.Directions)
		// 1.0: ссылка без folder_id адресует корень ФИНАЛЬНЫХ тегов, и на член
		// папки её поднимает только однозначный финальный тег (§4, §6).
		normalizeMemberLinks10(s, &merged, knownTags)
	default:
		// 0.x: позиции цепочек приехали строками (контракт 0.x адреса папок не
		// несёт) и сопоставляются по сырым тегам узлов контейнеров.
		resolveImportedHops(s.Sources, s.Directions, &merged)
		state.NormalizeNodeLinks(s.Sources, s.Directions)
	}

	known := newTagSet(knownTags)
	res.Warnings = append(res.Warnings, checkImportedRuleTargets(dec.Rules, known)...)
	s.Rules = append(s.Rules, dec.Rules...)
	res.AppliedRules = len(dec.Rules)
	// SPEC 129 §5.4: `vars` пресетов после замещения — необъявленные имена
	// снимаются и называются, равные умолчанию снимаются молча. Пресет вне
	// шаблона (backup_unknown_preset) объявлений не имеет — не трогается.
	var presetDrops []state.RecordVarDrop
	state.NormalizePresetRuleVars(s.Rules, opts.RecordVars, &presetDrops)
	res.Warnings = append(res.Warnings, recordVarWarnings(presetDrops)...)

	// Ось порядка встаёт номерами файла (BACKUP.md §9 п. 7): раскладка оси у
	// сторон одна, и номер несёт зону, которую порядок не передаёт. Правила,
	// которые узлы носят с собой, стоят на той же оси (NODE_SECTIONS.md §5).
	placeImportedAxis(s.Rules, merged.sectionRules(s))

	if dec.RouteFinal != "" {
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

	res.Warnings = append(res.Warnings, importDNS(s, dec.DNS, opts.RecordVars, known)...)
	importWarp(s, dec.Warp)

	return res, nil
}

// filterImportedDirectionOptions отсеивает из опций Направлений, приехавших
// этим импортом, строки, которые здесь не объявленные корневые имена
// (NODE_LINK.md §8, BACKUP.md §6).
//
// Законная строка `include` — тег Направления этого файла или приёмника, его
// `-auto`, тег свёртки результата и её `-auto`, системный тег приёмника
// (`direct-out`, тег блокировки, opts.SystemTags). Прочее — узел, свёртка,
// которой здесь нет, неизвестное имя — в опции не попадает и называется
// одним warning'ом на Направление: узел в Направление кладёт фильтр, а
// строка без цели в конфиге отвергла бы группу ядром.
func filterImportedDirectionOptions(s *state.State, dec *decodedFile, applied []int, opts ImportOptions) []Warning {
	if len(applied) == 0 {
		return nil
	}
	declared := importRootNames(opts, dec, s)

	var warns []Warning
	for _, at := range applied {
		d := &s.Directions[at]
		if len(d.AddOutbounds) == 0 {
			continue
		}
		kept := make([]string, 0, len(d.AddOutbounds))
		var dropped []string
		for _, opt := range d.AddOutbounds {
			if declared[strings.TrimSpace(opt)] {
				kept = append(kept, opt)
				continue
			}
			dropped = append(dropped, opt)
		}
		if len(dropped) == 0 {
			continue
		}
		if len(kept) == 0 {
			kept = nil
		}
		d.AddOutbounds = kept
		warns = append(warns, Warning{
			Code:   WarnBackupDirectionIncludeDropped,
			Detail: d.Tag + ": " + strings.Join(dropped, ", "),
		})
	}
	return warns
}

// importRootNames — объявленные корневые имена результата импорта
// (NODE_LINK.md §8): системные теги приёмника (`direct-out`, тег блокировки,
// opts.SystemTags — outbound'ы и endpoint'ы шаблона), теги Направлений файла
// и приёмника и их `-auto`, теги свёрток результата и их `-auto`.
//
// Источник один на опции приехавших Направлений
// (filterImportedDirectionOptions) и на известные цели (importKnownTags):
// системный тег шаблона, законный в `include`, законен и целью правила, и
// route.final.
func importRootNames(opts ImportOptions, dec *decodedFile, s *state.State) map[string]bool {
	names := map[string]bool{defaultDirectTag: true}
	add := func(tag string) {
		if tag = strings.TrimSpace(tag); tag != "" {
			names[tag] = true
		}
	}
	blockTag := opts.BlockTag
	if blockTag == "" {
		blockTag = defaultBlockTag
	}
	add(blockTag)
	for _, tag := range opts.SystemTags {
		add(tag)
	}
	addDirections := func(list []configtypes.Direction) {
		for i := range list {
			add(list[i].Tag)
			if list[i].Auto != nil && strings.TrimSpace(list[i].Tag) != "" {
				add(list[i].AutoTag())
			}
		}
	}
	if dec != nil {
		addDirections(dec.Directions)
	}
	addDirections(s.Directions)
	for i := range s.Sources {
		if r := s.Sources[i].Replace; r != nil && strings.TrimSpace(r.Tag) != "" {
			add(r.Tag)
			if r.Mode == state.FolderReplaceBoth {
				add(r.Tag + "-auto")
			}
		}
	}
	return names
}

// importKnownTags — ЕДИНСТВЕННЫЙ список известных целей импорта: цели правил
// (checkImportedRuleTargets), route.final и корневые имена подъёма ссылок 1.0
// (normalizeMemberLinks10).
//
// Состав: то, что знает принимающая сторона (opts.KnownOutbounds), объявленные корневые имена
// результата (importRootNames — с системными тегами шаблона приёмника) и
// корневые узлы. Считается ПОСЛЕ слияния, по живому состоянию: цепочка, чей
// тег был занят, в состояние не попала, и final в неё — это final в никуда.
//
// Раньше списков было два: декодер проверял цели правил ДО слияния своим
// набором (opts плюс Направления, цепочки и свёртки файла), и ни один из них
// не видел системных тегов шаблона. Импорт в пустое состояние через Debug API
// (там opts.KnownOutbounds — Направления приёмника, то есть пусто) выключал
// каждое правило на direct-out, а route.final на direct-out не применялся.
//
// nil — «проверять нечем»: ни приёмник, ни файл, ни результат слияния не
// назвали ни одного имени. Тогда цели не режутся — выключить всё подряд хуже,
// чем импортировать как есть; умолчания `direct-out` и тега блокировки такой
// список не открывают.
func importKnownTags(opts ImportOptions, dec *decodedFile, s *state.State) []string {
	out := append([]string(nil), opts.KnownOutbounds...)
	for i := range s.Sources {
		src := &s.Sources[i]
		switch src.Kind {
		case state.SourceKindChain, state.SourceKindServer, state.SourceKindAuto:
			if t := src.NodeTagOrLabel(); t != "" {
				out = append(out, t)
			}
		}
	}
	names := importRootNames(opts, dec, s)
	// Умолчания importRootNames — всегда два имени (`direct-out` и тег
	// блокировки). Сверх них ни одного — значит, сказать о целях нечего.
	onlyDefaults := len(opts.SystemTags) == 0 && strings.TrimSpace(opts.BlockTag) == "" && len(names) == 2
	if len(out) == 0 && onlyDefaults {
		return nil
	}
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// placeImportedAxis ставит правила файла на ось порядка: номер из файла
// сохраняется, неразмеченные корневые правила встают в хвост оси.
//
// Сплошной перенумерации с 1000 нет (BACKUP.md §9 п. 7). Раскладка оси у
// сторон одна (rule_order.go), и абсолютный номер значит то, чего порядок не
// передаёт:
//
//   - зону. Правило с номером ниже UserRuleNumStart сборка ставит ПЕРЕД
//     route.rules шаблона (core/build/preset_merge.go), а несортируемая голова
//     traffic-processing держит свой номер как часть инварианта
//     (rule_order.go, PlaceRuleAfter). Сплошная нумерация уводила голову и
//     якоря шаблона ниже 1000 в пользовательскую зону;
//   - якоря для того, что встанет ПОСЛЕ импорта. Пресет из библиотеки берёт
//     номер шаблона (950..995), правило нового узла — 945, новое правило —
//     максимум зоны 1000..1100 плюс один. После сплошной нумерации пресет
//     вставал перед бывшей головой (sniff переставал быть первым), а новое
//     правило — за бывшие перехватчики 1110+.
//
// Порядок файла воспроизводится: он и задан номерами, а при равных корневое
// правило стоит раньше узлового, как у сборки (rulesWithNodeSections).
// Импорт своего экспорта ось не трогает.
//
// Неразмеченным корневым (запись без num) номер раздаётся здесь: следующий за
// максимумом размеченных — корневых и узловых, — но не ниже начала
// пользовательской зоны. Разметка на загрузке (MarkRuleOrder) дала бы им
// номера от UserRuleNumStart вперемешку с размеченными. Файл, где не размечено
// ни одно корневое правило (до SPEC 106), остаётся как есть: MarkRuleOrder
// поставит пресеты на якоря шаблона, которого импорт не знает.
//
// SPEC 113-C §1: разметка заканчивается пересортировкой КОРНЕВОГО массива.
// Узловые правила не пересортировываются: их порядок внутри узла задаётся
// номерами, а сам узел в оси не участвует.
func placeImportedAxis(rules []state.Rule, sectionRules []*state.Rule) {
	var (
		last               int
		marked, rootMarked bool
	)
	see := func(r *state.Rule) {
		if r.Num == nil {
			return
		}
		if !marked || *r.Num > last {
			last = *r.Num
		}
		marked = true
	}
	for i := range rules {
		if rules[i].Num != nil {
			rootMarked = true
		}
		see(&rules[i])
	}
	for _, r := range sectionRules {
		see(r)
	}

	if rootMarked {
		next := last + 1
		if next < state.UserRuleNumStart {
			next = state.UserRuleNumStart
		}
		for i := range rules {
			if rules[i].Num != nil {
				continue
			}
			n := next
			rules[i].Num = &n
			next++
		}
	}

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
			warns = append(warns, Warning{Code: WarnBackupVarSkipped, Detail: name, Reason: VarSkippedNotPortable})
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
	if reservedTargetLiteral(tag) {
		return true
	}
	_, ok := t[strings.ToLower(strings.TrimSpace(tag))]
	return ok
}

// reservedTargetLiteral — цель, которая существует у принимающей стороны
// всегда и объявлять которую не нужно (без учёта регистра).
func reservedTargetLiteral(tag string) bool {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "direct", "block", "reject", "drop", "dns-out":
		return true
	}
	return false
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
// Запись шаблонного сервера (SPEC 129 §5.2): тег, которого шаблон приёмника
// не объявил, не ввозится (backup_dns_entry_skipped); у совпавшей записи
// `enabled` локальный, а `vars` накладываются ПО ИМЕНАМ — имя файла замещает
// значение приёмника, имён, которых в файле нет, импорт не трогает (П7:
// старый файл без vars не сбрасывает маршрут приёмника к умолчанию).
// Необъявленные имена файла называются, равные умолчанию снимаются молча —
// после наложения, поэтому значение файла, равное умолчанию, сбрасывает
// выбор приёмника (файл сказал это явно). Затем Н9: запись, которой импорт
// коснулся, с целью канала вне известных приезжает выключенной.
//
// final и strategy — ЗАМЕЩАЮТСЯ файлом: это одиночные значения, а не список,
// и «слить» два взаимоисключающих ответа нечем.
func importDNS(s *state.State, dns *decodedDNS, decls *state.RecordVarDecls, known tagSet) []Warning {
	if dns == nil {
		// Нормы записи действуют и без секции в файле: приёмник мог нести
		// сирот и умолчания — первая запись после импорта их снимает.
		if servers, changed, _ := state.NormalizeDNSServerVars(s.DNS.Servers, decls); changed {
			s.DNS.Servers = servers
		}
		return nil
	}
	var warns []Warning

	// Ключ един для всех видов: у template/user заполнен tag и пуст ref, у
	// preset — наоборот. Разбирать по kind нечего, а один ключ на все виды
	// не даёт завести второй, расходящийся с первым.
	//
	// Дубли ищутся ТОЛЬКО среди записей, которые лежали здесь ДО импорта:
	// запись файла, совпавшая с локальной, пропускается, а одинаковые записи
	// внутри самого файла ввозятся все. Файл — снимок настоящего состояния, и
	// импорт в пустое состояние обязан его воспроизвести; ключ приехавшей
	// записи в тех же наборах схлопывал два одинаковых правила файла в одно.
	serverKey := func(kind, tag, ref string) string { return kind + "\x00" + tag + "\x00" + ref }
	haveServers := map[string]bool{}
	localTemplate := map[string]int{}
	for i, srv := range s.DNS.Servers {
		haveServers[serverKey(string(srv.Kind), srv.Tag, srv.Ref)] = true
		if srv.Kind == state.DNSServerKindTemplate {
			if _, dup := localTemplate[srv.Tag]; !dup {
				localTemplate[srv.Tag] = i
			}
		}
	}
	declsKnown := decls != nil && decls.DNSServers != nil
	touched := map[int]bool{}
	var touchedOrder []int
	touch := func(i int) {
		if !touched[i] {
			touched[i] = true
			touchedOrder = append(touchedOrder, i)
		}
	}
	for _, srv := range dns.Servers {
		if srv.Kind == state.DNSServerKindTemplate {
			if !decls.DNSServerDeclared(srv.Tag) {
				warns = append(warns, Warning{
					Code:   WarnBackupDNSEntrySkipped,
					Detail: "template:" + srv.Tag,
					Kind:   warnKindDNSServer,
				})
				continue
			}
			if declsKnown {
				warns = append(warns, recordVarWarnings(state.UndeclaredRecordVars(
					srv.Vars, decls.DNSServers[srv.Tag], state.DNSVarRecord(srv.Tag)))...)
			}
			if idx, ok := localTemplate[srv.Tag]; ok {
				if overlayRecordVars(&s.DNS.Servers[idx], srv.Vars) {
					touch(idx)
				}
				continue // своё сильнее: enabled и прочие имена — локальные
			}
		} else if haveServers[serverKey(string(srv.Kind), srv.Tag, srv.Ref)] {
			continue // своё сильнее
		}
		if srv.Kind != state.DNSServerKindTemplate {
			srv.Vars = nil // Н1: vars только у записи шаблонного сервера
		}
		s.DNS.Servers = append(s.DNS.Servers, srv)
		touch(len(s.DNS.Servers) - 1)
	}

	// Н2–Н4 для записей, которых импорт коснулся, — ДО проверки целей: цель,
	// равная умолчанию, снята и целью файла не считается.
	if declsKnown {
		for _, i := range touchedOrder {
			srv := &s.DNS.Servers[i]
			if srv.Kind != state.DNSServerKindTemplate {
				continue
			}
			if vars, changed, _ := state.NormalizeRecordVarMap(srv.Vars, decls.DNSServers[srv.Tag], ""); changed {
				srv.Vars = vars
			}
		}
	}
	warns = append(warns, checkImportedDNSTargets(s.DNS.Servers, touchedOrder, decls, known)...)
	// Остальные записи приёмника — те же нормы молча (сироты, умолчания).
	if servers, changed, _ := state.NormalizeDNSServerVars(s.DNS.Servers, decls); changed {
		s.DNS.Servers = servers
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
		if haveRules[ruleKey(string(r.Kind), r.Ref, r.Body)] {
			continue // своё сильнее
		}
		s.DNS.Rules = append(s.DNS.Rules, r)
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
	return warns
}

// overlayRecordVars накладывает значения записи файла на запись приёмника по
// именам (SPEC 129 §5.2). Пустое после подрезки значение файла — «нет ключа»
// (Н3) и значение приёмника не трогает. Возвращает true, если файл назвал
// хоть одно имя.
func overlayRecordVars(dst *state.DNSServer, vars map[string]string) bool {
	named := false
	for name, raw := range vars {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if dst.Vars == nil {
			dst.Vars = map[string]string{}
		}
		dst.Vars[name] = value
		named = true
	}
	return named
}

// checkImportedDNSTargets — Н9 (SPEC 129): запись DNS-сервера, которой импорт
// коснулся, с каналом вне известных целей приезжает ВЫКЛЮЧЕННОЙ с
// backup_unknown_outbound — значение остаётся, пользователь видит, куда метил
// файл, а сборка не уведёт резолв мимо выбранного маршрута. Канал шаблонного
// сервера — значения переменных типа `outbound` (типы — из объявлений; без
// них проверять нечем), у `user` — `body.detour`. Пустой список известных —
// «проверять нечем», как у правил.
func checkImportedDNSTargets(servers []state.DNSServer, touched []int, decls *state.RecordVarDecls, known tagSet) []Warning {
	if known.empty() {
		return nil
	}
	var warns []Warning
	fail := func(srv *state.DNSServer, target string) {
		srv.Enabled = false
		warns = append(warns, Warning{
			Code:   WarnBackupUnknownOutbound,
			Detail: srv.Tag + " → " + target,
			Kind:   warnKindDNSServer,
		})
	}
	for _, i := range touched {
		srv := &servers[i]
		switch srv.Kind {
		case state.DNSServerKindTemplate:
			if decls == nil {
				continue
			}
			names := make([]string, 0, len(srv.Vars))
			for name := range srv.Vars {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				decl, ok := decls.DNSServerVarDecl(srv.Tag, name)
				if !ok || decl.Type != "outbound" {
					continue
				}
				if v := strings.TrimSpace(srv.Vars[name]); v != "" && !known.has(v) {
					fail(srv, v)
				}
			}
		case state.DNSServerKindUser:
			if det, _ := srv.Body["detour"].(string); strings.TrimSpace(det) != "" && !known.has(det) {
				fail(srv, det)
			}
		}
	}
	return warns
}

// recordVarWarnings — снятые имена переменных предупреждениями
// backup_var_skipped (SPEC 129 §5.6).
func recordVarWarnings(drops []state.RecordVarDrop) []Warning {
	if len(drops) == 0 {
		return nil
	}
	out := make([]Warning, 0, len(drops))
	for _, d := range drops {
		kind := ""
		switch {
		case strings.HasPrefix(d.Record, "dns:"):
			kind = warnKindDNSServer
		case strings.HasPrefix(d.Record, "preset:"):
			kind = warnKindPreset
		}
		out = append(out, Warning{
			Code:   WarnBackupVarSkipped,
			Detail: d.Name,
			Kind:   kind,
			Record: d.Record,
			Reason: d.Reason,
		})
	}
	return out
}

// Вид носителя в Warning.Kind у предупреждений о записях DNS и пресетов.
const (
	warnKindDNSServer = "dns_server"
	warnKindPreset    = "preset"
)

// VarSkippedNotPortable — причина backup_var_skipped у корневой переменной вне
// списка переносимых (registry/vars.json, portable=true).
const VarSkippedNotPortable = "not_portable"

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
