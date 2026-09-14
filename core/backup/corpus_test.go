package backup

// Конформанс-раннер корпуса LX Backup (контракт 1.0, legacy-вход 0.x).
//
// Гоняет contract/corpus/backup/*.backup.json через Import и сверяет с
// <case>.expected.json. Тот же набор обязана проходить сторона LxBox: перенос
// настроек между приложениями имеет смысл ровно настолько, насколько обе
// стороны одинаково понимают битую ссылку, непереносимую переменную и
// упразднённый карман extensions.
//
// Формат кейса раннер НЕ выбирает: он читает файл через Parse, а тот
// опознаёт формат по `lx_backup` (1 — 0.x, 2 — контракт 1.0). Поэтому кейсы
// обоих форматов лежат вперемешку и проверяются одними и теми же ожиданиями:
// итог импорта — состояние, и оно обязано быть одинаковым независимо от того,
// каким писателем сделан файл. Кейсы 1.0 названы с префиксом `v10_` — это
// удобство чтения каталога, а не признак для раннера.
//
// Файл с `lx_backup` БОЛЬШЕ читаемого — не ошибка кейса, а чужой extension:
// такой файл сторона пропускает (corpus/README.md), и раннер, у которого
// Parse на нём отказывает, обязан пропустить кейс, а не завалить прогон.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

const backupCorpusRelPath = "../../contract/corpus/backup"

// corpusRuleExpectation — одно правило оси в ожиданиях.
type corpusRuleExpectation struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Refs — kind=srs: все URL наборов правила по порядку (D-100).
	// Необязательно: отсутствие ключа значит «не проверяем».
	Refs []string `json:"refs"`
	// Outbound — ЦЕЛЬ правила как вид, а не как кусок тела: тег outbound'а,
	// либо `reject`, либо `drop` (state v8 §0: цель живёт внутри `body` в
	// форме sing-box — `"outbound": <тег>` | `"action": "reject"` |
	// `"action": "reject", "method": "drop"`).
	//
	// Проверяется вид, а не байты тела, потому что расходятся стороны именно
	// здесь: у 0.12 цель ехала ПЛОСКИМ полем `outbound`, и обратный перевод
	// «reject/drop → action/method» (outboundutil.ApplyOutboundToRule) легко
	// сделать наполовину — тогда правило-блокировка тихо станет правилом с
	// outbound'ом по имени "reject", то есть висячей ссылкой.
	//
	// Поле необязательное: пустая строка значит «не проверяем».
	Outbound string `json:"outbound"`
	// Vars — переменные ЭТОГО правила (`kind: preset`): имя → значение.
	//
	// Сверяются именно они, а не глобальные `vars` состояния: у пресета
	// переменные — часть его настройки («строгий список» против «мягкого»),
	// и правило, приехавшее без них, делает не то, что делало на исходной
	// машине, оставаясь при этом похожим на себя именем и видом.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	Vars map[string]string `json:"vars"`
}

// corpusExpectation — форма <case>.expected.json.
type corpusExpectation struct {
	Rules []corpusRuleExpectation `json:"rules"`

	// DNS — DNS-секция состояния после импорта (контракт 1.0).
	//
	// Проверяется ВИД записи и её ссылка, а не тело: тело — это правило
	// sing-box как есть, оно едет в файле байт в байт, и дублировать его в
	// ожидании значило бы проверять encoding/json. Предмет сверки здесь
	// другой: обе стороны обязаны одинаково разложить запись на вид
	// (user/template/preset) и её имя — именно на этом расходились плоская
	// форма 0.12 и форма записей v8.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	DNS *struct {
		Servers []struct {
			Kind string `json:"kind"`
			// Tag — у kind=user|template; Ref — у kind=preset. В ожидании
			// стоит ровно один из двух: у пресета своего тега нет.
			Tag     string `json:"tag"`
			Ref     string `json:"ref"`
			Enabled *bool  `json:"enabled"`
			// Body — тело записи (форма sing-box) deep-equal.
			//
			// Тело едет в файле как есть, но «как есть» — это свойство
			// ЭТОГО кодека, а не контракта: у второй стороны тело проходит
			// через свой декодер, и потеря `detour` или `path` у резолвера
			// — расхождение, которое видно только сверкой. Поле
			// необязательное: отсутствие ключа значит «не проверяем».
			Body json.RawMessage `json:"body"`
		} `json:"servers"`
		// Strategy и Final — одиночные значения секции; файл их ЗАМЕЩАЕТ
		// (§9 п. 5). Пустая строка значит «не проверяем».
		Strategy string `json:"strategy"`
		Final    string `json:"final"`
		// DefaultDomainResolver — третий скаляр секции, по тому же правилу.
		DefaultDomainResolver string `json:"default_domain_resolver"`
		// Rules — ЧИСЛО DNS-правил. Их имена контракт не нормирует (у
		// user-правила имя необязательно), а вот потеря правила при
		// переносе — расхождение.
		Rules *int `json:"rules"`
	} `json:"dns"`

	// Sections — секции узлов после импорта: ТЕГ КОРНЕВОГО СЕРВЕРА → его
	// связка (NODE_SECTIONS.md §5). Ключ — тег, а не позиция: секция
	// принадлежит узлу, и проверять её по номеру в списке значило бы падать
	// на любой перестановке источников.
	//
	// Правила узла сверяются по имени и enabled, но НЕ по номеру: номер
	// раздаёт общая перенумерация оси (BACKUP.md §9), он зависит от того,
	// сколько корневых правил приехало тем же файлом, и в ожидании был бы
	// хрупкой копией арифметики импортёра. Положение узлового правила НА ОСИ
	// проверяется списком `rules` верхнего уровня — там оно стоит среди
	// корневых в том порядке, в каком встало.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	// Ключ — тег узла, причём ЛЮБОГО носителя секций, а не только корневого
	// сервера: NODE_SECTIONS.md §1 разрешает секции свободному узлу и в
	// корне `sources[]`, и внутри папки, у члена папки свой путь слияния, и
	// проверять половину носителей значило бы оставить вторую без сверки.
	Sections map[string]struct {
		Rules []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"rules"`
		// DNSServers — теги серверов связки по порядку.
		DNSServers []string `json:"dns_servers"`
		// DNSRules — их число (см. DNS.Rules выше).
		DNSRules *int `json:"dns_rules"`
	} `json:"sections"`
	Vars     map[string]string `json:"vars"`
	Warnings []string          `json:"warnings"`
	// WarningReasons — уточнение причин для кодов, у которых причина
	// нормирована перечнем: код → все встретившиеся `reason` по алфавиту,
	// без повторов.
	//
	// Отдельным ключом, а не внутри `warnings`, чтобы не переписывать
	// ожидания старых кейсов. Нужен там, где ОДИН код означает разные
	// вещи: `backup_section_record_dropped` c `reason` ∈ kind | rule_set |
	// not_allowed (норма B3) — множество кодов их не различает, и сторона,
	// отбросившая запись «не по той причине», проходила бы кейс зелёной,
	// показывая пользователю неверное объяснение потери.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	WarningReasons    map[string][]string `json:"warning_reasons"`
	RouteFinalApplied *bool               `json:"route_final_applied"`
	// ExtensionsDropped — файл несёт упразднённый механизм extensions
	// (схема 0.10.x). Импортёр обязан его ОТБРОСИТЬ и назвать одним
	// warning'ом на файл (BACKUP_PRINCIPLES.md П3/П4), а не провозить:
	// провоз непонятого создавал состояние-призрак. Ожидание сформулировано
	// относительно импортёра и одинаково для обеих сторон.
	ExtensionsDropped bool     `json:"extensions_dropped"`
	DisabledHashes    []string `json:"disabled_hashes"`

	// ReplaceTags — URL подписки → имя группы, которым свёртка обязана
	// материализоваться после импорта (D-081). Тега замены контракт не
	// несёт: он ПОЗИЦИОННЫЙ ДЕРИВАТИВ префикса тегов, а при пустом
	// префиксе — «<номер записи в subscriptions[]>:» плюс `select`.
	// Номер считается по секции subscriptions[], а НЕ по позиции среди
	// всех источников файла: разошедшийся счёт даёт двум сторонам разные
	// имена одной группы, и правила файла, метящие в неё, повисают.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	ReplaceTags map[string]string `json:"replace_tags"`

	// Folders — папки, которые импорт обязан собрать: имя → теги членов В
	// ПОРЯДКЕ файла (контракт 0.12). Папка не имеет секции в файле и
	// собирается ПО ИМЕНИ из записей servers[] с полем folder, поэтому
	// проверять надо именно результат сборки: потеря пометки у одной записи
	// растащила бы состав по корню списка молча.
	//
	// Порядок членов нормативен — обе стороны собирают папку в порядке
	// записей файла, иначе состав после переноса перетасовывается.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	Folders map[string][]string `json:"folders"`

	// FolderIDs — имя папки → её ULID после импорта.
	//
	// Ступень слияния «сперва по `id`, затем по имени» (§9 п. 3) иначе
	// НЕНАБЛЮДАЕМА: состав, собранный по любой из них, выглядит одинаково,
	// и сторона, знающая только имя, проходила бы кейс зелёной. Именно id
	// показывает, какая ступень сработала: совпавшая по `id` папка держит
	// ЛОКАЛЬНЫЙ id, заведённая заново — id из файла.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	FolderIDs map[string]string `json:"folder_ids"`

	// Subscriptions — подписки после импорта: URL → её настройки (D-095).
	// Ключ тот же, по которому идёт слияние, — `url` как есть.
	//
	// Список ИСЧЕРПЫВАЮЩИЙ: подписка, которой в ожиданиях нет, — это либо
	// не оставленная локальная, либо задвоенная, и обе ошибки видны только
	// сверкой всего набора.
	//
	// Enabled — указатель: умолчание схемы true, и отсутствие ключа обязано
	// значить «не проверяем», а не «ожидаем выключенной».
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	Subscriptions map[string]struct {
		Label   string `json:"label"`
		Prefix  string `json:"prefix"`
		Postfix string `json:"postfix"`
		Enabled *bool  `json:"enabled"`
		// Nodes — сырые теги узлов, которые обязаны пережить слияние:
		// состав локальной подписки в файл не едет и потеряться не вправе.
		Nodes []string `json:"nodes"`
		// PendingDisabled — отметки выключения, ждущие первого fetch.
		// Проверяются отсортированными: они объединение двух множеств, и
		// порядок в нём смысла не несёт.
		PendingDisabled []string `json:"pending_disabled"`
	} `json:"subscriptions"`

	// RootServers — теги корневых одиночных узлов В ПОРЯДКЕ состояния
	// (D-095 §9 п. 7: совпавшие держат локальную позицию, новые встают в
	// конец). Сверяется и порядок, и состав: дедуп по телу проверяется
	// именно отсутствием второй записи.
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	RootServers []string `json:"root_servers"`

	// Directions — Направления, которые импорт обязан СОЗДАТЬ (SPEC 104,
	// схема v1.1). Проверяется каноническая форма, а не внутренняя: она и
	// есть предмет договорённости между приложениями.
	Directions []struct {
		Tag           string `json:"tag"`
		Label         string `json:"label"`
		Filter        string `json:"filter"`
		Invert        bool   `json:"invert"`
		IncludeDirect bool   `json:"include_direct"`
		IncludeBlock  bool   `json:"include_block"`
		HasAuto       bool   `json:"has_auto"`
	} `json:"directions"`

	// Chains — цепочки после импорта (SPEC 110). Список ИСЧЕРПЫВАЮЩИЙ:
	// запись, пропущенная по занятому тегу, не должна материализоваться
	// второй копией. chain сверяется deep-equal канона — включая
	// null-значения rewrite (RFC 7396: null удаляет ключ и обязан пережить
	// перенос как есть). label цепочки — объявленное поле LxBox (D-094):
	// лаунчер его не хранит и не пишет, поэтому ожидание label сверяет
	// только раннер LxBox (`expected.lxbox.json`), а у лаунчера оно — ошибка
	// кейса (см. checkChains).
	//
	// Enabled — указатель, а не bool: умолчание схемы true, и отсутствие
	// ключа в ожиданиях обязано значить «не проверяем», а не «ожидаем
	// false». Обычный bool сделал бы нулевое значение требованием
	// выключенности и провалил бы все кейсы без этого поля.
	Chains []struct {
		Tag     string          `json:"tag"`
		Label   string          `json:"label"`
		Enabled *bool           `json:"enabled"`
		Chain   json.RawMessage `json:"chain"`
		// Hops — позиции цепочки как ССЫЛКИ, а не как теги: `folder_id`
		// либо имя папки, куда он обязан указывать после импорта.
		//
		// Канон `chain` выше схлопывает хопы до плоских тегов (форма 0.12,
		// chainCanon), и адрес папки в нём теряется — то есть главная
		// механика §6 (перепись `folder_id` по карте id) остаётся без
		// сверки. Здесь она и проверяется.
		//
		// Поле необязательное: отсутствие ключа значит «не проверяем».
		Hops []corpusLinkExpectation `json:"hops"`
	} `json:"chains"`

	// Detours — личный detour УЗЛОВ после импорта: тег носителя → ссылка
	// (docs/NODE_LINK.md). Носитель — любой узел: корневой сервер и член
	// папки; общий detour самого контейнера сюда не входит.
	//
	// Без этого ключа detour проверить было нечем: канон цепочки и состав
	// папок его не показывают, а перепись `detour.folder_id` по карте id
	// (BACKUP.md §6) — та же механика, что у хопов, и ошибаться в ней можно
	// так же.
	//
	// Карта ИСЧЕРПЫВАЮЩАЯ: detour у узла, которого в ожиданиях нет, —
	// ошибка (ссылка, выдуманная импортом, опаснее потерянной: трафик идёт
	// через хоп, которого пользователь не выбирал).
	//
	// Поле необязательное: отсутствие ключа значит «не проверяем».
	Detours map[string]corpusLinkExpectation `json:"detours"`
}

// corpusLinkExpectation — ссылка NodeLink после импорта (docs/NODE_LINK.md
// §2): тег плюс АДРЕС контейнера, записанный так, чтобы он не зависел от
// машины-приёмника.
//
// Адрес задаётся не больше чем одним ключом; ни одного — ссылка обязана быть
// корневой (без folder_id). Это прежнее значение пустого `folder`, и старые
// ожидания хопов читаются ровно как раньше.
type corpusLinkExpectation struct {
	Tag string `json:"tag"`
	// Folder — ИМЯ папки, на которую обязан указывать folder_id после
	// импорта. Имя, а не ULID: ULID у новой папки берётся из файла, у
	// совпавшей — локальный, и записать в ожидание можно только то, что от
	// машины не зависит.
	Folder string `json:"folder"`
	// Subscription — URL подписки, на которую обязан указывать folder_id.
	// Отдельным ключом, а не именем в `folder`: у подписки ключ слияния —
	// URL, а имя может совпасть с именем папки, и ожидание «папка X»
	// молча проходило бы на подписке X.
	Subscription string `json:"subscription"`
	// FolderID — ВИСЯЧАЯ ссылка: folder_id обязан остаться ровно таким, как
	// в файле, и контейнера с этим id после импорта быть не должно. Импорт
	// такую ссылку не снимает и не выдумывает ей адрес (BACKUP.md §6);
	// недостижимую цель разбирает сборка.
	FolderID string `json:"folder_id"`
}

func TestBackupCorpus(t *testing.T) {
	entries, err := os.ReadDir(backupCorpusRelPath)
	if err != nil {
		t.Skipf("корпус бэкапов недоступен: %v", err)
	}

	var cases []string
	for _, e := range entries {
		// `<case>.pre.backup.json` — предсостояние своего кейса, а не кейс:
		// без этого отсева оно гонялось бы отдельным прогоном и искало бы
		// несуществующий `<case>.pre.expected.json`.
		if strings.HasSuffix(e.Name(), ".pre.backup.json") {
			continue
		}
		if strings.HasSuffix(e.Name(), ".backup.json") {
			cases = append(cases, strings.TrimSuffix(e.Name(), ".backup.json"))
		}
	}
	sort.Strings(cases)
	if len(cases) == 0 {
		t.Skip("корпус пуст")
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(backupCorpusRelPath, name+".backup.json"))
			if err != nil {
				t.Fatalf("чтение кейса: %v", err)
			}
			expRaw, err := os.ReadFile(filepath.Join(backupCorpusRelPath, name+".expected.json"))
			if err != nil {
				t.Fatalf("чтение ожиданий: %v", err)
			}
			var exp corpusExpectation
			if err := json.Unmarshal(expRaw, &exp); err != nil {
				t.Fatalf("разбор ожиданий: %v", err)
			}

			b, parseWarns, err := Parse(raw)
			if err != nil {
				if corpusFormatAhead(raw) {
					// Кейс формата новее читаемого — чужой extension, а не
					// поломка (corpus/README.md): сторона его пропускает.
					t.Skipf("формат кейса новее читаемого: %v", err)
				}
				t.Fatalf("Parse: %v", err)
			}

			// Предсостояние: `<case>.pre.backup.json` импортируется в ПУСТОЕ
			// состояние первым, и уже в него сливается сам кейс. Иначе
			// проверить слияние нечем — импорт в пустоту у слияния и у
			// замены даёт один и тот же итог, и разошедшиеся стороны здесь
			// были бы неразличимы. Предупреждения предсостояния в сверку не
			// идут: оно декорация сцены, а не предмет кейса.
			dst := &state.State{}
			if pre := loadCorpusPre(t, name); pre != nil {
				if _, err := ImportFile(dst, pre, ImportOptions{}); err != nil {
					t.Fatalf("Import предсостояния: %v", err)
				}
			}
			res, err := ImportFile(dst, b, ImportOptions{
				// Принимающая сторона знает эти цели; всё прочее —
				// символическая ссылка в никуда.
				KnownOutbounds: []string{"proxy", "direct"},
			})
			if err != nil {
				t.Fatalf("Import: %v", err)
			}

			gotWarns := warnCodes(append(parseWarns, res.Warnings...))
			wantWarns := append([]string(nil), exp.Warnings...)
			sort.Strings(wantWarns)
			if !equalStrings(gotWarns, wantWarns) {
				t.Errorf("коды предупреждений: получено %v, ожидалось %v", gotWarns, wantWarns)
			}

			checkWarningReasons(t, append(parseWarns, res.Warnings...), exp)
			checkRules(t, dst, exp)
			checkDNS(t, dst, exp)
			checkSections(t, dst, exp)
			checkVars(t, dst, exp)
			checkRouteFinal(t, dst, exp)
			checkExtensionsDropped(t, dst, exp)
			checkDisabledHashes(t, dst, exp)
			checkDirections(t, dst, exp)
			checkChains(t, dst, exp)
			checkDetours(t, dst, exp)
			checkReplaceTags(t, dst, exp)
			checkFolders(t, dst, exp)
			checkSubscriptions(t, dst, exp)
			checkRootServers(t, dst, exp)
		})
	}
}

// corpusFormatAhead — у файла мажорный маркер БОЛЬШЕ читаемого этой стороной.
//
// Правило корпуса (corpus/README.md): такой файл сторона пропускает как чужой
// extension, override-файлов под него не заводят. Отличать его от битого
// кейса приходится здесь, а не по имени файла: имя — свойство каталога, а
// маркер — свойство документа, и единственная честная проверка читает документ.
func corpusFormatAhead(raw []byte) bool {
	var head struct {
		LxBackup int `json:"lx_backup"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return false
	}
	return head.LxBackup > FormatVersion10
}

// loadCorpusPre читает предсостояние кейса; nil = его нет (тогда импорт идёт
// в пустое состояние, как было до конвенции pre).
func loadCorpusPre(t *testing.T, name string) *File {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(backupCorpusRelPath, name+".pre.backup.json"))
	if err != nil {
		return nil
	}
	pre, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("разбор предсостояния: %v", err)
	}
	return pre
}

// checkSubscriptions — подписки после слияния (D-095).
//
// Проверяет ровно то, ради чего заведено слияние: локальная запись пережила
// импорт вместе с составом, а настройки на ней — из файла.
func checkSubscriptions(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.Subscriptions == nil {
		return
	}
	have := map[string]*state.Source{}
	for i := range dst.Sources {
		if dst.Sources[i].Kind != state.SourceKindSubscription {
			continue
		}
		url := dst.Sources[i].URL
		if _, dup := have[url]; dup {
			t.Errorf("подписка %s задвоена — слияние по URL не сработало", url)
		}
		have[url] = &dst.Sources[i]
	}
	for url, want := range exp.Subscriptions {
		got, ok := have[url]
		if !ok {
			t.Errorf("подписки %s нет после импорта", url)
			continue
		}
		if got.Name != want.Label {
			t.Errorf("%s: имя %q, ожидалось %q", url, got.Name, want.Label)
		}
		prefix, postfix := "", ""
		if got.TagPolicy != nil {
			prefix, postfix = got.TagPolicy.Prefix, got.TagPolicy.Postfix
		}
		if prefix != want.Prefix || postfix != want.Postfix {
			t.Errorf("%s: политика тегов (%q, %q), ожидалась (%q, %q)",
				url, prefix, postfix, want.Prefix, want.Postfix)
		}
		if want.Enabled != nil && got.Enabled != *want.Enabled {
			t.Errorf("%s: enabled=%v, ожидалось %v", url, got.Enabled, *want.Enabled)
		}
		if want.Nodes != nil {
			tags := []string{}
			for i := range got.Nodes {
				tags = append(tags, got.Nodes[i].Tag)
			}
			if !equalStrings(tags, want.Nodes) {
				t.Errorf("%s: состав %v, ожидался %v — узлы локальной подписки слияние терять не вправе",
					url, tags, want.Nodes)
			}
		}
		if want.PendingDisabled != nil {
			pending := append([]string(nil), got.PendingDisabled...)
			sort.Strings(pending)
			wantPending := append([]string(nil), want.PendingDisabled...)
			sort.Strings(wantPending)
			if !equalStrings(pending, wantPending) {
				t.Errorf("%s: отметки выключения %v, ожидались %v", url, pending, wantPending)
			}
		}
	}
	for url := range have {
		if _, ok := exp.Subscriptions[url]; !ok {
			t.Errorf("после импорта есть подписка %s, которой в ожиданиях нет", url)
		}
	}
}

// checkRootServers — корневые одиночные узлы: состав и ПОРЯДОК.
func checkRootServers(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.RootServers == nil {
		return
	}
	var got []string
	for i := range dst.Sources {
		if dst.Sources[i].Kind == state.SourceKindServer {
			got = append(got, dst.Sources[i].NodeTagOrLabel())
		}
	}
	if !equalStrings(got, exp.RootServers) {
		t.Errorf("корневые серверы %v, ожидались %v (порядок нормативен)", got, exp.RootServers)
	}
}

// checkRules — ОСЬ ПОРЯДКА целиком: корневые правила и правила, которые узлы
// носят с собой (NODE_SECTIONS.md §5).
//
// Ось одна, и проверять её половинами нельзя: узловое правило встаёт МЕЖДУ
// корневыми по относительному порядку номеров (BACKUP.md §9), и ровно это
// расхождение — «у той стороны правило узла уехало в конец» — список корневых
// правил показать не в состоянии.
func checkRules(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	// Сравниваем в порядке оси, а не в порядке файла: импортёр
	// перенумеровывает, сохраняя относительный порядок.
	type got struct {
		name     string
		enabled  bool
		num      int
		refs     []string
		outbound string
		vars     map[string]string
	}
	all := make([]got, 0, len(dst.Rules))
	add := func(r state.Rule) {
		num := 0
		if r.Num != nil {
			num = *r.Num
		}
		all = append(all, got{ruleName(r), r.Enabled, num, ruleRefs(r), ruleOutboundView(r), r.Vars})
	}
	for _, r := range dst.Rules {
		add(r)
	}
	for i := range dst.Sources {
		if sec := dst.Sources[i].Node.Sections; sec != nil {
			for _, r := range sec.Rules {
				add(r)
			}
		}
		for j := range dst.Sources[i].Nodes {
			if sec := dst.Sources[i].Nodes[j].Sections; sec != nil {
				for _, r := range sec.Rules {
					add(r)
				}
			}
		}
	}
	if len(all) != len(exp.Rules) {
		t.Fatalf("правил на оси %d, ожидалось %d", len(all), len(exp.Rules))
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].num < all[j].num })

	for i, want := range exp.Rules {
		if all[i].name != want.Name {
			t.Errorf("правило %d: имя %q, ожидалось %q", i, all[i].name, want.Name)
		}
		if all[i].enabled != want.Enabled {
			t.Errorf("правило %q: enabled=%v, ожидалось %v", want.Name, all[i].enabled, want.Enabled)
		}
		if want.Refs != nil && !equalStrings(all[i].refs, want.Refs) {
			t.Errorf("правило %q: наборы %v, ожидались %v", want.Name, all[i].refs, want.Refs)
		}
		if want.Outbound != "" && all[i].outbound != want.Outbound {
			t.Errorf("правило %q: цель %q, ожидалась %q", want.Name, all[i].outbound, want.Outbound)
		}
		if want.Vars != nil && !equalStringMaps(all[i].vars, want.Vars) {
			t.Errorf("правило %q: переменные %v, ожидались %v", want.Name, all[i].vars, want.Vars)
		}
	}
}

// equalStringMaps — сравнение карт «имя → значение» по составу.
func equalStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// ruleOutboundView — цель правила как вид: тег | "reject" | "drop".
//
// Обратная операция к outboundutil.ApplyOutboundToRule. Читает ТЕЛО, потому
// что в state v8 другого места у цели нет; у preset-правила тела нет вовсе —
// цель приходит из шаблона, и проверять её здесь нечем (пустая строка).
func ruleOutboundView(r state.Rule) string {
	if len(r.Body) == 0 {
		return ""
	}
	var body struct {
		Outbound string `json:"outbound"`
		Action   string `json:"action"`
		Method   string `json:"method"`
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return ""
	}
	if body.Action == "reject" {
		if body.Method == "drop" {
			return "drop"
		}
		return "reject"
	}
	return body.Outbound
}

// checkDNS — DNS-секция состояния после импорта (вид записи и её ссылка).
func checkDNS(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.DNS == nil {
		return
	}
	servers := dst.DNS.Servers
	if len(servers) != len(exp.DNS.Servers) {
		t.Fatalf("DNS-серверов %d, ожидалось %d", len(servers), len(exp.DNS.Servers))
	}
	for i, want := range exp.DNS.Servers {
		got := servers[i]
		if string(got.Kind) != want.Kind {
			t.Errorf("DNS-сервер %d: вид %q, ожидался %q", i, got.Kind, want.Kind)
		}
		if got.Tag != want.Tag {
			t.Errorf("DNS-сервер %d: тег %q, ожидался %q", i, got.Tag, want.Tag)
		}
		if got.Ref != want.Ref {
			t.Errorf("DNS-сервер %d: ссылка %q, ожидалась %q", i, got.Ref, want.Ref)
		}
		if want.Enabled != nil && got.Enabled != *want.Enabled {
			t.Errorf("DNS-сервер %d: enabled=%v, ожидалось %v", i, got.Enabled, *want.Enabled)
		}
		if want.Body != nil {
			gotBody, err := json.Marshal(got.Body)
			if err != nil {
				t.Fatalf("DNS-сервер %d: marshal тела: %v", i, err)
			}
			if !jsonDeepEqual(gotBody, want.Body) {
				t.Errorf("DNS-сервер %d (%s%s): тело %s, ожидалось %s",
					i, got.Tag, got.Ref, string(gotBody), string(want.Body))
			}
		}
	}
	if exp.DNS.Rules != nil && len(dst.DNS.Rules) != *exp.DNS.Rules {
		t.Errorf("DNS-правил %d, ожидалось %d", len(dst.DNS.Rules), *exp.DNS.Rules)
	}
	// Три скаляра секции — одним правилом (§9 п. 5): файл их замещает.
	if exp.DNS.Strategy != "" && dst.DNS.Strategy != exp.DNS.Strategy {
		t.Errorf("dns.strategy=%q, ожидалось %q", dst.DNS.Strategy, exp.DNS.Strategy)
	}
	if exp.DNS.Final != "" && dst.DNS.Final != exp.DNS.Final {
		t.Errorf("dns.final=%q, ожидалось %q", dst.DNS.Final, exp.DNS.Final)
	}
	if exp.DNS.DefaultDomainResolver != "" && dst.DNS.DefaultDomainResolver != exp.DNS.DefaultDomainResolver {
		t.Errorf("dns.default_domain_resolver=%q, ожидалось %q",
			dst.DNS.DefaultDomainResolver, exp.DNS.DefaultDomainResolver)
	}
}

// checkSections — связки узлов после импорта, по тегу корневого сервера.
func checkSections(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.Sections == nil {
		return
	}
	// Носители секций — ВСЕ свободные узлы: и корневые, и члены папок
	// (NODE_SECTIONS.md §1). У члена папки собственный путь слияния, и
	// обход одних корневых оставлял бы его DNS-связку без единой проверки,
	// а сторожа «есть секции, которых в ожиданиях нет» — слепым к ней.
	have := map[string]*state.NodeSections{}
	collect := func(n *state.Node) {
		if sec := n.Sections; sec != nil && !sec.IsEmpty() {
			if _, dup := have[n.Tag]; dup {
				t.Errorf("тег %q носит секции дважды — ключ связки неоднозначен", n.Tag)
			}
			have[n.Tag] = sec
		}
	}
	for i := range dst.Sources {
		if dst.Sources[i].Kind == state.SourceKindServer {
			collect(&dst.Sources[i].Node)
		}
		for j := range dst.Sources[i].Nodes {
			collect(&dst.Sources[i].Nodes[j])
		}
	}
	for tag, want := range exp.Sections {
		sec, ok := have[tag]
		if !ok {
			t.Errorf("у узла %q секций после импорта нет: есть у %v", tag, sortedKeys(have))
			continue
		}
		if len(sec.Rules) != len(want.Rules) {
			t.Errorf("%s: правил связки %d, ожидалось %d", tag, len(sec.Rules), len(want.Rules))
		} else {
			for i, wr := range want.Rules {
				if sec.Rules[i].Name != wr.Name {
					t.Errorf("%s: правило связки %d имя %q, ожидалось %q", tag, i, sec.Rules[i].Name, wr.Name)
				}
				if sec.Rules[i].Enabled != wr.Enabled {
					t.Errorf("%s: правило связки %q enabled=%v, ожидалось %v",
						tag, wr.Name, sec.Rules[i].Enabled, wr.Enabled)
				}
			}
		}
		if want.DNSServers != nil {
			var tags []string
			for _, s := range sec.DNSServers() {
				tags = append(tags, s.Tag)
			}
			if !equalStrings(tags, want.DNSServers) {
				t.Errorf("%s: DNS-серверы связки %v, ожидались %v", tag, tags, want.DNSServers)
			}
		}
		if want.DNSRules != nil && len(sec.DNSRules()) != *want.DNSRules {
			t.Errorf("%s: DNS-правил связки %d, ожидалось %d", tag, len(sec.DNSRules()), *want.DNSRules)
		}
	}
	for tag := range have {
		if _, ok := exp.Sections[tag]; !ok {
			t.Errorf("у узла %q есть секции, которых в ожиданиях нет", tag)
		}
	}
}

// sortedKeys — имена ключей карты по алфавиту (для сообщений об ошибках).
func sortedKeys(m map[string]*state.NodeSections) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ruleRefs — все URL наборов srs-правила по порядку; у прочих kind — nil.
//
// state v8: наборы — поле записи, дедуплицированное конструктором.
func ruleRefs(r state.Rule) []string {
	if r.Kind != state.RuleKindSrs {
		return nil
	}
	return r.Refs
}

// checkFolders — папки собраны по имени, с тем же составом и порядком.
func checkFolders(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.Folders == nil && exp.FolderIDs == nil {
		return
	}
	have := map[string][]string{}
	haveIDs := map[string]string{}
	for _, src := range dst.Sources {
		if src.Kind != state.SourceKindFolder {
			continue
		}
		if _, dup := have[src.Name]; dup {
			t.Errorf("папка %q собрана дважды — одно имя = одна папка", src.Name)
		}
		tags := []string{}
		for i := range src.Nodes {
			tags = append(tags, src.Nodes[i].Tag)
		}
		have[src.Name] = tags
		haveIDs[src.Name] = src.ID
	}
	for name, want := range exp.FolderIDs {
		got, ok := haveIDs[name]
		if !ok {
			t.Errorf("папка %q не собрана: нечему сверять id", name)
			continue
		}
		if got != want {
			t.Errorf("папка %q: id %q, ожидался %q (ступень слияния §9 п. 3)", name, got, want)
		}
	}
	for name, want := range exp.Folders {
		got, ok := have[name]
		if !ok {
			t.Errorf("папка %q не собрана: есть %v", name, have)
			continue
		}
		// equalStrings сравнивает В ПОРЯДКЕ, без сортировки: порядок членов
		// папки нормативен, и перетасованный состав — это расхождение.
		if !equalStrings(got, want) {
			t.Errorf("папка %q: члены %v, ожидались %v (порядок нормативен)", name, got, want)
		}
	}
	if exp.Folders != nil {
		for name := range have {
			if _, ok := exp.Folders[name]; !ok {
				t.Errorf("собрана папка %q, которой в ожиданиях нет", name)
			}
		}
	}
}

func checkVars(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.Vars == nil {
		return
	}
	have := map[string]string{}
	for _, v := range dst.Vars {
		have[v.Name] = v.Value
	}
	for name, want := range exp.Vars {
		if have[name] != want {
			t.Errorf("переменная %s = %q, ожидалось %q", name, have[name], want)
		}
	}
	for name := range have {
		if _, ok := exp.Vars[name]; !ok {
			t.Errorf("применена переменная %s, которой не должно быть", name)
		}
	}
}

func checkRouteFinal(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.RouteFinalApplied == nil {
		return
	}
	applied := false
	for _, v := range dst.Vars {
		if v.Name == "route_final" && v.Value != "" {
			applied = true
		}
	}
	if applied != *exp.RouteFinalApplied {
		t.Errorf("route.final применён=%v, ожидалось %v", applied, *exp.RouteFinalApplied)
	}
}

// checkExtensionsDropped — упразднённый карман не возвращается в экспорт.
//
// Warning об этом уже сверен общим сравнением кодов; здесь проверяется вторая
// половина П1: состояние после импорта неотличимо от настроенного руками, то
// есть следа от extensions в нём нет и повторный экспорт его не воскрешает.
func checkExtensionsDropped(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if !exp.ExtensionsDropped {
		return
	}
	back, _, err := Export10(dst, ExportOptions{AppVersion: "corpus"})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	raw, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "extensions") {
		t.Errorf("extensions вернулся в экспорт — карман провоза не закрыт: %s", raw)
	}
}

func checkDisabledHashes(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(exp.DisabledHashes) == 0 {
		return
	}
	// SPEC 118 W5: отметка выключения живёт полем node.enabled; у только что
	// импортированной подписки узлов ещё нет (nodes[] в контракт не едут),
	// поэтому отметка ждёт первого достоверного fetch в PendingDisabled.
	found := map[string]bool{}
	for _, src := range dst.Sources {
		for _, tag := range src.PendingDisabled {
			found[tag] = true
		}
		for i := range src.Nodes {
			if !src.Nodes[i].Enabled {
				found[src.Nodes[i].Tag] = true
			}
		}
	}
	for _, want := range exp.DisabledHashes {
		if !found[want] {
			t.Errorf("отметка выключенной ноды %s не перенесена", want)
		}
	}
}

// checkReplaceTags — имя группы свёрнутой подписки после импорта (D-081).
//
// Проверяется ровно тот тег, на который ссылаются правила и route.final того
// же файла: если стороны считают позиционный дериватив по-разному, здесь
// разъезд виден сразу, а не у пользователя выключенным правилом.
func checkReplaceTags(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(exp.ReplaceTags) == 0 {
		return
	}
	byURL := map[string]*state.Source{}
	for i := range dst.Sources {
		if dst.Sources[i].Kind == state.SourceKindSubscription {
			byURL[dst.Sources[i].URL] = &dst.Sources[i]
		}
	}
	for url, want := range exp.ReplaceTags {
		src, ok := byURL[url]
		if !ok {
			t.Errorf("подписка %s не приехала — тег замены проверять не на чем", url)
			continue
		}
		if src.Replace == nil {
			t.Errorf("%s: свёртка не стала заменой — группы, в которую метят правила файла, нет", url)
			continue
		}
		if src.Replace.Tag != want {
			t.Errorf("%s: тег замены %q, ожидался %q", url, src.Replace.Tag, want)
		}
	}
}

// ruleName — имя записи: у inline/srs это поле `name` (state v8 вынес его из
// тела наружу), у preset — ссылка на пресет.
func ruleName(r state.Rule) string {
	switch r.Kind {
	case state.RuleKindInline, state.RuleKindSrs:
		if r.Name != "" {
			return r.Name
		}
	case state.RuleKindPreset:
		return r.Ref
	}
	return string(r.Kind)
}

func warnCodes(warns []Warning) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range warns {
		if seen[w.Code] {
			continue
		}
		seen[w.Code] = true
		out = append(out, w.Code)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// checkChains проверяет цепочки после импорта (SPEC 110).
//
// Сверяется канон chain (deep-equal, включая null внутри rewrite), число
// записей, enabled и хопы как ссылки.
func checkChains(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(exp.Chains) == 0 {
		return
	}
	byTag := map[string]state.Source{}
	count := 0
	for _, src := range dst.Sources {
		if src.Kind == state.SourceKindChain {
			byTag[src.NodeTagOrLabel()] = src
			count++
		}
	}
	if count != len(exp.Chains) {
		t.Fatalf("цепочек %d, ожидалось %d", count, len(exp.Chains))
	}

	for _, want := range exp.Chains {
		src, ok := byTag[want.Tag]
		if !ok {
			t.Fatalf("цепочка %q не создана импортом", want.Tag)
		}
		// SPEC 118 W5: канон цепочки в модели разложен по узлу (body + hops);
		// сверяем форму контракта source_chain.schema.json, в которой
		// записаны ожидания.
		gotRaw, err := json.Marshal(chainCanon(src))
		if err != nil {
			t.Fatalf("%s: marshal канона: %v", want.Tag, err)
		}
		if !jsonDeepEqual(gotRaw, want.Chain) {
			t.Errorf("%s: канон цепочки искажён: %s, ожидалось %s", want.Tag, gotRaw, want.Chain)
		}
		// Выключенность — состояние записи, а не канона: enabled живёт в
		// обвязке chains[], и умолчание схемы (отсутствие ключа = true)
		// стороны обязаны читать одинаково.
		if want.Enabled != nil && src.Enabled != *want.Enabled {
			t.Errorf("%s: enabled=%v, ожидалось %v", want.Tag, src.Enabled, *want.Enabled)
		}
		// Хопы как ССЫЛКИ: канон выше их уже схлопнул до тегов, и адрес
		// папки (§6 — перепись folder_id по карте id) виден только здесь.
		if want.Hops != nil {
			checkChainHops(t, dst, want.Tag, src.Hops, want.Hops)
		}
		// Подписи цепочки у лаунчера нет ни в состоянии, ни в файле 1.0 (имя
		// узла одно — тег, SPEC 112; label — поле LxBox, D-094). Ожидание,
		// которое проверить нечем, не вправе пройти молча: это ошибка кейса,
		// а не зелёный результат.
		if want.Label != "" {
			t.Errorf("%s: ожидание label цепочки у лаунчера непроверяемо — подпись сверяет раннер LxBox (expected.lxbox.json)", want.Tag)
		}
	}
}

// chainCanon — цепочка модели в форме контракта source_chain.schema.json:
// настройки маршрута из тела узла плюс позиции строками (адрес папки у хопа
// в этой форме теряется — его сверяет checkChainHops).
func chainCanon(src state.Source) *configtypes.SourceChain {
	var hops []string
	for _, h := range src.Hops {
		if strings.TrimSpace(h.Tag) != "" {
			hops = append(hops, h.Tag)
		}
	}
	return configtypes.ChainFromBody(src.Body, hops)
}

// jsonDeepEqual сравнивает два JSON-фрагмента структурно, без чувствительности
// к порядку ключей и пробелам.
func jsonDeepEqual(a, b json.RawMessage) bool {
	var av, bv interface{}
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// checkDirections проверяет Направления, созданные импортом (SPEC 104).
//
// Сверяется каноническая форма, а не внутренняя структура: именно о ней
// договорились стороны, и раннер LxBox читает те же ожидания.
func checkDirections(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if len(exp.Directions) == 0 {
		return
	}
	byTag := make(map[string]configtypes.Direction, len(dst.Directions))
	for _, d := range dst.Directions {
		byTag[d.Tag] = d
	}
	for _, want := range exp.Directions {
		got, ok := byTag[want.Tag]
		if !ok {
			t.Fatalf("направление %q не создано импортом", want.Tag)
		}
		body, invert := configtypes.DirectionFilterTag(got.Filters)
		if body != want.Filter || invert != want.Invert {
			t.Errorf("%s: отбор (%q, инверсия=%v), ожидалось (%q, %v)",
				want.Tag, body, invert, want.Filter, want.Invert)
		}
		hasDirect, hasBlock := false, false
		for _, tag := range got.AddOutbounds {
			switch tag {
			case "direct-out":
				hasDirect = true
			case "block-out":
				hasBlock = true
			}
		}
		if hasDirect != want.IncludeDirect || hasBlock != want.IncludeBlock {
			t.Errorf("%s: опции (direct=%v, block=%v), ожидалось (%v, %v)",
				want.Tag, hasDirect, hasBlock, want.IncludeDirect, want.IncludeBlock)
		}
		if (got.Auto != nil) != want.HasAuto {
			t.Errorf("%s: автовыбор=%v, ожидалось %v", want.Tag, got.Auto != nil, want.HasAuto)
		}
	}
}

// checkChainHops — позиции цепочки как ссылки: тег и адрес контейнера, на
// который обязан указывать folder_id после импорта.
func checkChainHops(t *testing.T, dst *state.State, tag string, got []state.NodeLink, want []corpusLinkExpectation) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: хопов %d, ожидалось %d", tag, len(got), len(want))
		return
	}
	idx := newCorpusContainerIndex(dst)
	for i, w := range want {
		checkLinkExpectation(t, idx, fmt.Sprintf("%s: хоп %d", tag, i), got[i], w)
	}
}

// checkDetours — личный detour узлов после импорта (docs/NODE_LINK.md §4).
//
// Ключ — тег носителя, как у `sections`: тег узла и есть его имя (SPEC 112),
// и носителем бывает и корневой сервер, и член папки. Тег, который носят два
// узла с detour, делает ключ неоднозначным — это ошибка кейса, а не
// «первый победил».
func checkDetours(t *testing.T, dst *state.State, exp corpusExpectation) {
	t.Helper()
	if exp.Detours == nil {
		return
	}
	have := map[string]state.NodeLink{}
	collect := func(n *state.Node) {
		if n.Detour == nil {
			return
		}
		if _, dup := have[n.Tag]; dup {
			t.Errorf("тег %q носит detour дважды — ключ ожидания неоднозначен", n.Tag)
		}
		have[n.Tag] = *n.Detour
	}
	for i := range dst.Sources {
		// У папки и подписки собственный detour — ОБЩИЙ detour контейнера, не
		// узла: в эту карту он не входит.
		switch dst.Sources[i].Kind {
		case state.SourceKindServer, state.SourceKindChain, state.SourceKindAuto:
			collect(&dst.Sources[i].Node)
		}
		for j := range dst.Sources[i].Nodes {
			collect(&dst.Sources[i].Nodes[j])
		}
	}
	idx := newCorpusContainerIndex(dst)
	for tag, want := range exp.Detours {
		got, ok := have[tag]
		if !ok {
			t.Errorf("у узла %q после импорта нет detour", tag)
			continue
		}
		checkLinkExpectation(t, idx, fmt.Sprintf("detour узла %q", tag), got, want)
	}
	for tag := range have {
		if _, ok := exp.Detours[tag]; !ok {
			t.Errorf("у узла %q есть detour, которого в ожиданиях нет", tag)
		}
	}
}

// corpusContainerIndex — контейнеры состояния по id: папка → имя, подписка →
// URL. Разрешение «ULID → машинонезависимый адрес» делается здесь, по
// состоянию, а не в ожидании.
type corpusContainerIndex struct {
	folderName map[string]string
	subURL     map[string]string
}

func newCorpusContainerIndex(dst *state.State) corpusContainerIndex {
	idx := corpusContainerIndex{folderName: map[string]string{}, subURL: map[string]string{}}
	for i := range dst.Sources {
		switch dst.Sources[i].Kind {
		case state.SourceKindFolder:
			idx.folderName[dst.Sources[i].ID] = dst.Sources[i].Name
		case state.SourceKindSubscription:
			idx.subURL[dst.Sources[i].ID] = dst.Sources[i].URL
		}
	}
	return idx
}

// checkLinkExpectation — одна ссылка против ожидания (corpusLinkExpectation).
func checkLinkExpectation(t *testing.T, idx corpusContainerIndex, where string, got state.NodeLink, w corpusLinkExpectation) {
	t.Helper()
	if got.Tag != w.Tag {
		t.Errorf("%s: тег %q, ожидался %q", where, got.Tag, w.Tag)
	}
	addresses := 0
	for _, v := range []string{w.Folder, w.Subscription, w.FolderID} {
		if v != "" {
			addresses++
		}
	}
	if addresses > 1 {
		t.Errorf("%s: в ожидании больше одного адреса (folder/subscription/folder_id) — ошибка кейса", where)
		return
	}
	switch {
	case w.FolderID != "":
		if got.FolderID != w.FolderID {
			t.Errorf("%s: folder_id %q, ожидался %q как в файле — ссылку на неизвестный контейнер импорт ввозит как есть (§6)",
				where, got.FolderID, w.FolderID)
			return
		}
		if name, isFolder := idx.folderName[got.FolderID]; isFolder {
			t.Errorf("%s: ожидалась висячая ссылка, а папка %q с id %q есть", where, name, got.FolderID)
		}
		if url, isSub := idx.subURL[got.FolderID]; isSub {
			t.Errorf("%s: ожидалась висячая ссылка, а подписка %s с id %q есть", where, url, got.FolderID)
		}
	case w.Subscription != "":
		if got.FolderID == "" {
			t.Errorf("%s: без folder_id, ожидалась подписка %s", where, w.Subscription)
			return
		}
		url, ok := idx.subURL[got.FolderID]
		if !ok {
			t.Errorf("%s: метит в folder_id %q, подписки с таким id нет — ссылка не переписана по карте (§6)",
				where, got.FolderID)
			return
		}
		if url != w.Subscription {
			t.Errorf("%s: метит в подписку %s, ожидалась %s", where, url, w.Subscription)
		}
	case w.Folder != "":
		if got.FolderID == "" {
			t.Errorf("%s: без folder_id, ожидалась папка %q", where, w.Folder)
			return
		}
		name, ok := idx.folderName[got.FolderID]
		if !ok {
			t.Errorf("%s: метит в folder_id %q, папки с таким id нет — ссылка не переписана по карте (§6)",
				where, got.FolderID)
			return
		}
		if name != w.Folder {
			t.Errorf("%s: метит в папку %q, ожидалась %q", where, name, w.Folder)
		}
	default:
		if got.FolderID != "" {
			t.Errorf("%s: метит в контейнер %q, ожидалась корневая ссылка", where, got.FolderID)
		}
	}
}

// checkWarningReasons — причины у кодов, где причина нормирована перечнем.
//
// Множество кодов их не различает: `backup_section_record_dropped` означает
// три разные вещи (норма B3), и сторона, отбросившая запись «не по той
// причине», показывала бы пользователю неверное объяснение потери, проходя
// кейс зелёной.
func checkWarningReasons(t *testing.T, warns []Warning, exp corpusExpectation) {
	t.Helper()
	if exp.WarningReasons == nil {
		return
	}
	have := map[string][]string{}
	for _, w := range warns {
		if _, watched := exp.WarningReasons[w.Code]; !watched {
			continue
		}
		if w.Reason == "" {
			t.Errorf("%s: предупреждение без reason, а перечень причин нормирован", w.Code)
			continue
		}
		seen := false
		for _, r := range have[w.Code] {
			if r == w.Reason {
				seen = true
				break
			}
		}
		if !seen {
			have[w.Code] = append(have[w.Code], w.Reason)
		}
	}
	for code, want := range exp.WarningReasons {
		got := append([]string(nil), have[code]...)
		sort.Strings(got)
		wantSorted := append([]string(nil), want...)
		sort.Strings(wantSorted)
		if !equalStrings(got, wantSorted) {
			t.Errorf("%s: причины %v, ожидались %v", code, got, wantSorted)
		}
	}
}
