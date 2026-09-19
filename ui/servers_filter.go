// File servers_filter.go — МОДЕЛЬ фильтра списка узлов вкладки Servers и его
// предикат. Окно, которое всем этим управляет, живёт в
// servers_filter_window.go.
//
// Устройство перенесено из LxBox (§048/§083/§096/§103,
// app/lib/screens/home/node_filter.dart): пять сочетаемых категорий, внутри
// категории ИЛИ, между категориями И, у каждой — своя инверсия по одной
// формуле `member == invert → не прошёл`.
//
// Отличие от LxBox одно и намеренное: несовпавшие узлы ЛАУНЧЕР СКРЫВАЕТ, а не
// приглушает. Список серверов — рабочий инструмент выбора узла, и на 500+
// узлах подписки приглушённый хвост означал бы прежнюю простыню, по которой
// снова надо скроллить.
//
// # Где живёт состояние
//
// В памяти панели и на диск не пишется — как в LxBox. Память ПЕР-ГРУППА (§083): у каждой Selector group свой снимок, при
// смене группы прежний запоминается, у новой восстанавливается свой. Один
// общий фильтр вёл бы к тому, что регулярка, набранная под пул одной страны,
// молча прятала бы весь список соседнего Направления.
//
// # Почему предикат считается пакетом, а не построчно
//
// `proxiesForListView` зовётся на КАЖДУЮ строку списка (Length, updateItem) —
// на 500 узлах это сотни вызовов за перерисовку. Компилировать регулярку и
// читать config.json там нельзя, поэтому фильтр отдаёт готовый срез
// (Apply), а панель держит его в кэше и пересчитывает только на смену
// фильтра или данных.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package ui

import (
	"regexp"
	"sort"
	"strings"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/emojitag"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// serversTestMode — режим категории «Тест».
//
// Порог пинга — ПЯТЫЙ режим этого же выбора, а не отдельная галка рядом
// (решение владельца: «вместо галки — точка»). Так у ряда ровно одно
// состояние: «ok» и «ping ≤ 300» — два ответа на один вопрос «что показывать
// по результату замера», и одновременно они не значат ничего связного.
type serversTestMode int

const (
	serversTestAny serversTestMode = iota
	serversTestOK
	serversTestError
	serversTestUntested
	// serversTestPing — «ping ≤ N ms»: порог из PingMaxMs.
	serversTestPing
)

// serversFilterDefaultPingText — значение поля порога по умолчанию.
//
// Настоящее число, а не placeholder: порог включается ВЫБОРОМ ЧИПА, и пустое
// поле в момент выбора означало бы вариант, который ничего не отбирает.
const serversFilterDefaultPingText = "300"

// serversFilterSourceNameMaxRunes — предел длины имени источника в чипе.
//
// Имена подписок бывают в пол-экрана («⬇️ Обходы белых списков ⬇️ BL: …»), и
// один такой чип растянул бы сетку. Полное имя остаётся в тултипе.
const serversFilterSourceNameMaxRunes = 16

// serversNodeFacets — то, что фильтр знает про ОДИН узел помимо его имени и
// пинга: протокол, варианты транспорта/безопасности, источник.
//
// Собирается один раз на пересчёт (см. collectServersFacets), а не на строку:
// и разбор config.json, и разбор state.json — файловые.
type serversNodeFacets struct {
	// Protocol — тип outbound'а из config.json («vless», «wireguard», …).
	// Пусто = узла в конфиге нет (гонка перегенерации, служебный outbound).
	Protocol string
	// Variants — метки транспорта и безопасности того же узла («ws», «xhttp»,
	// «Reality+Vision», «awg2»). Те же части, что показывает подзаголовок
	// строки, кроме протокола: фильтр обязан отбирать по тому, что человек
	// видит, а не по параллельному словарю.
	Variants []string
	// SourceID — ULID источника-КОНТЕЙНЕРА (подписка, папка). Пусто у узла
	// одиночного источника (server/chain/auto) и у узла, чьё происхождение
	// не раскрыто вовсе.
	SourceID string
	// SourceKnown — происхождение узла известно (неважно, контейнер или
	// одиночка).
	//
	// Различать обязательно: «неизвестно» — единственное исключение из общей
	// формулы, узел с нераскрытым тегом фильтр по источнику не отсекает (см.
	// шапку node_sources.go), а вот узел одиночного источника отсекается как
	// «не член выбора» — он известен, просто ни в один контейнер не входит.
	// Сложи их в одно «пусто» — и одиночки молча проходили бы любой выбор
	// подписки.
	SourceKnown bool
}

// serversFilterState — полное состояние фильтра одной группы.
//
// Значение, а не указатель: снимок пер-группа копируется целиком, и общая
// карта выбранного между группами была бы ровно той ошибкой, ради которой
// память заводилась.
type serversFilterState struct {
	// RegexBody — ТЕЛО регулярки без обёртки, в тех же терминах, что поле
	// фильтра Направления (configtypes.DirectionFilterPattern): флаг
	// регистронезависимости ставит компиляция, а не человек.
	RegexBody string
	// RegexInvert — галка «!» у регулярки.
	RegexInvert bool

	// Protocols / Variants — две половины ОДНОЙ категории «Протокол»: тип
	// outbound'а и метки транспорта/безопасности. Внутри половины ИЛИ, между
	// половинами И («vless» + «ws» = vless поверх ws), пустая половина не
	// ограничивает.
	//
	// Половин две, а инверсия одна (ProtocolsInvert): в окне это одна строка
	// чипов с одним «!», и он обязан переворачивать её ИТОГ. Две галки под
	// одной подписью спрашивали бы «не-vless поверх ws» или «vless поверх
	// не-ws» — вопрос, которого человек перед строкой чипов не задаёт.
	Protocols       map[string]bool
	Variants        map[string]bool
	ProtocolsInvert bool

	Sources       map[string]bool
	SourcesInvert bool

	Test serversTestMode

	// HideErrors — «глаз» панели: скрыть строки с ошибкой замера
	// (Delay == -1), оставив и измеренные, и непроверенные.
	//
	// Поле фильтра, а не отдельная переменная панели, и это важно: иначе над
	// одним и тем же рядом строк стояли бы ДВА независимых переключателя —
	// глаз и Тест, — и кто победил, зависело бы от того, что нажали последним.
	// Здесь же они связаны явно (см. SetTest / SetHideErrors), а память
	// пер-группа накрывает глаз заодно с остальным отбором.
	HideErrors bool

	// PingMaxMs — порог режима serversTestPing; 0 = число в поле не разобрано
	// (пусто или мусор), и режим не отбирает ничего.
	//
	// Отдельного флага включения нет: включает порог ВЫБОР режима, а число
	// только уточняет его. Пара «галка + режим» дала бы состояние «выбран
	// порог, но выключен», которого человек перед рядом чипов не ждёт.
	//
	// Untested (Delay == 0) порог проходят, ошибочные (-1) — нет: порог
	// отвечает на «дай быстрые», а неизмеренные прятать за неизвестность
	// значит прятать как раз те, которые стоит проверить (LxBox #11).
	PingMaxMs int
	// PingText — сырой текст поля; хранится, чтобы окно восстанавливало
	// написанное, включая недопечатанное и ошибочное.
	PingText string

	// InvertAll — инверсия ИТОГА всего фильтра, поверх категорий.
	InvertAll bool
}

// newServersFilterState — исходное состояние: всё выключено.
func newServersFilterState() serversFilterState {
	return serversFilterState{
		Protocols: map[string]bool{},
		Variants:  map[string]bool{},
		Sources:   map[string]bool{},
		Test:      serversTestAny,
		// Поле порога предзаполнено, но режим не выбран: Reset обязан вернуть
		// ровно это — пустое поле в момент выбора «ping ≤» не отбирало бы.
		PingText:  serversFilterDefaultPingText,
		PingMaxMs: 300,
	}
}

// clone — глубокая копия для снимка пер-группа.
func (f serversFilterState) clone() serversFilterState {
	out := f
	out.Protocols = copyStringBoolMap(f.Protocols)
	out.Variants = copyStringBoolMap(f.Variants)
	out.Sources = copyStringBoolMap(f.Sources)
	return out
}

func copyStringBoolMap(src map[string]bool) map[string]bool {
	out := make(map[string]bool, len(src))
	for k, v := range src {
		if v {
			out[k] = true
		}
	}
	return out
}

// selectedKeys — выбранные ключи категории (только истинные).
func selectedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return out
}

// Active сообщает, отбирает ли фильтр хоть что-нибудь.
//
// От него зависит подсветка кнопки в панели: человек обязан видеть, что
// список неполон, не открывая окна. Невалидная регулярка активной не считается
// — она и не применяется.
func (f serversFilterState) Active() bool {
	if f.InvertAll {
		return true
	}
	if body := strings.TrimSpace(f.RegexBody); body != "" {
		if _, err := compileServersFilterRegex(body); err == nil {
			return true
		}
	}
	if len(selectedKeys(f.Protocols)) > 0 ||
		len(selectedKeys(f.Variants)) > 0 ||
		len(selectedKeys(f.Sources)) > 0 {
		return true
	}
	// Режим «ping ≤» с неразобранным числом ничего не отбирает — как и
	// невалидная регулярка, активным он не считается.
	if f.Test == serversTestPing {
		return f.PingMaxMs > 0
	}
	return f.Test != serversTestAny || f.HideErrors
}

// ExcludesErrors — скрыты ли сейчас строки с ошибкой замера.
//
// По ней рисуется иконка «глаза» панели. Истинна и когда глаз нажали руками, и
// когда ошибки отсекает сам Тест: иконка обязана описывать то, что НА ЭКРАНЕ,
// а не то, какой кнопкой этого добились.
// Порог «ping ≤» сюда входит наравне с «ok»: узел с ошибкой замера (-1)
// порога не проходит, и на экране ошибок нет — значит глаз обязан гореть.
func (f serversFilterState) ExcludesErrors() bool {
	return f.HideErrors ||
		f.Test == serversTestOK ||
		f.Test == serversTestUntested ||
		f.Test == serversTestPing
}

// SetHideErrors — переключить «глаз».
//
// Тест «error» и скрытые ошибки — взаимоисключающи: вместе они дают пустой
// список, и человек читал бы это как поломку. Побеждает последнее действие,
// поэтому глаз, включаясь поверх «error», возвращает Тест в «any».
func (f *serversFilterState) SetHideErrors(on bool) {
	f.HideErrors = on
	if on && f.Test == serversTestError {
		f.Test = serversTestAny
	}
}

// SetTest — выбрать режим категории «Тест».
//
// Обратная сторона той же связи: выбор «error» гасит глаз, иначе выбранный
// режим показывал бы пустоту.
func (f *serversFilterState) SetTest(mode serversTestMode) {
	f.Test = mode
	if mode == serversTestError {
		f.HideErrors = false
	}
}

// compileServersFilterRegex — компиляция ТЕЛА.
//
// `(?i)` навешивается здесь и только здесь: в теле его быть не должно —
// то же тело уезжает в поле фильтра Направления по кнопке ⧉, а там флаг
// регистра ставит своя обёртка (configtypes.DirectionFilterPattern).
func compileServersFilterRegex(body string) (*regexp.Regexp, error) {
	return regexp.Compile("(?i)" + body)
}

// serversFilterPredicate — скомпилированный фильтр, готовый к прогону.
type serversFilterPredicate struct {
	state    serversFilterState
	re       *regexp.Regexp
	protos   map[string]bool
	variants map[string]bool
	sources  map[string]bool
}

// compileServersFilter готовит предикат один раз на пересчёт.
//
// Невалидная и пустая регулярка одинаково означают «категория выключена»:
// прятать весь список из-за недопечатанной скобки нельзя — человек печатает
// её посимвольно.
func compileServersFilter(f serversFilterState) serversFilterPredicate {
	p := serversFilterPredicate{
		state:    f,
		protos:   copyStringBoolMap(f.Protocols),
		variants: copyStringBoolMap(f.Variants),
		sources:  copyStringBoolMap(f.Sources),
	}
	if body := strings.TrimSpace(f.RegexBody); body != "" {
		if re, err := compileServersFilterRegex(body); err == nil {
			p.re = re
		}
	}
	return p
}

// passes — проходит ли узел все включённые категории.
//
// Формула инверсии одна на все категории (LxBox node_filter.dart:96):
// `member == invert → не прошёл`. Обе ветки читаются так:
//   - без инверсии: не член — не прошёл (в том числе «неизвестно»);
//   - с инверсией: член — не прошёл.
func (p serversFilterPredicate) passes(proxy api.ProxyInfo, facets serversNodeFacets) bool {
	// «Глаз» инверсии НЕ подчиняется и проверяется до неё.
	//
	// Он не категория отбора, а ярлык панели «не показывать сломанные строки»,
	// и его иконка обязана описывать экран. Попади он под «Инвертировать всё»
	// — нажатый глаз оставлял бы на экране ТОЛЬКО ошибки, продолжая рисовать
	// значок «ошибки скрыты».
	if p.state.HideErrors && proxy.Delay == -1 {
		return false
	}
	ok := p.passesCategories(proxy, facets)
	if p.state.InvertAll {
		return !ok
	}
	return ok
}

func (p serversFilterPredicate) passesCategories(proxy api.ProxyInfo, facets serversNodeFacets) bool {
	if p.re != nil {
		// Матчим по ИМЕНИ, как его видит список: человек набирает то, что
		// читает на экране.
		if p.re.MatchString(proxy.DisplayOrName()) == p.state.RegexInvert {
			return false
		}
	}
	// Объединённая категория «Протокол»: две половины через И, одна инверсия
	// на итог (см. поля состояния). Пустая половина не ограничивает, поэтому
	// проверка запускается, только если выбрано хоть что-то.
	if len(p.protos) > 0 || len(p.variants) > 0 {
		member := true
		if len(p.protos) > 0 {
			member = facets.Protocol != "" && p.protos[facets.Protocol]
		}
		if member && len(p.variants) > 0 {
			member = false
			for _, v := range facets.Variants {
				if p.variants[v] {
					member = true
					break
				}
			}
		}
		if member == p.state.ProtocolsInvert {
			return false
		}
	}
	if len(p.sources) > 0 {
		// Узел неизвестного происхождения фильтр по источнику НЕ отсекает
		// (см. шапку node_sources.go): соврать про принадлежность хуже, чем
		// промолчать. Отсюда проверка только у узлов с известным источником —
		// исключение из общей формулы, и единственное.
		// Узел одиночного источника (server/chain/auto) сюда ВХОДИТ: он
		// известен, просто не принадлежит ни одному контейнеру, и member у
		// него ложь — при непустом выборе без инверсии он не проходит, с
		// инверсией проходит. Та же формула, что у остальных.
		if facets.SourceKnown {
			member := facets.SourceID != "" && p.sources[facets.SourceID]
			if member == p.state.SourcesInvert {
				return false
			}
		}
	}
	switch p.state.Test {
	case serversTestOK:
		if proxy.Delay <= 0 {
			return false
		}
	case serversTestError:
		if proxy.Delay != -1 {
			return false
		}
	case serversTestUntested:
		if proxy.Delay != 0 {
			return false
		}
	case serversTestPing:
		// Число не разобрано (пусто/мусор) — режим не отбирает: прятать весь
		// список из-за недопечатанной цифры нельзя, ровно как у регулярки.
		if p.state.PingMaxMs > 0 {
			// Непроверенные (0) порог проходят — мерять их никто не обещал
			// (LxBox #11); ошибочные (-1) не проходят: «дай быстрые» про
			// сломанный узел не говорит ничего хорошего.
			if proxy.Delay == -1 || proxy.Delay > int64(p.state.PingMaxMs) {
				return false
			}
		}
	}
	return true
}

// serversFilterFacets — сводка по ТЕКУЩЕМУ списку: какие протоколы, варианты и
// источники в нём вообще есть, и какие эмодзи встречаются в именах.
//
// Чипы строятся только из этого: предлагать «hysteria2», когда в группе нет ни
// одного такого узла, значит предлагать заведомо пустой результат.
type serversFilterFacets struct {
	// Protocols / Variants — значения в порядке убывания частоты, при
	// равенстве — по алфавиту (как эмодзи-чипы).
	Protocols []serversFacetCount
	Variants  []serversFacetCount
	// Sources — источники, представленные в списке, по имени.
	Sources []serversFacetSource
	// Emojis — значки имён с частотой, по убыванию.
	Emojis []emojitag.Entry
	// ByName — фасеты каждого узла; используется предикатом.
	ByName map[string]serversNodeFacets
}

type serversFacetCount struct {
	Key   string
	Count int
}

type serversFacetSource struct {
	ID    string
	Name  string
	Count int
}

// applyServersFilter — срез видимых строк.
//
// keep — узлы, которые не скрываются НИКОГДА (выделение, direct-out, активный
// прокси). Контракт существующий: список без выделенной строки терял бы
// контекст ровно в тот момент, когда человек с ней работает, а спрятанный
// direct-out отнимал бы аварийный выход.
func applyServersFilter(
	all []api.ProxyInfo,
	facets map[string]serversNodeFacets,
	f serversFilterState,
	keep func(name string) bool,
) []api.ProxyInfo {
	if !f.Active() {
		return all
	}
	p := compileServersFilter(f)
	out := make([]api.ProxyInfo, 0, len(all))
	for i := range all {
		if (keep != nil && keep(all[i].Name)) || p.passes(all[i], facets[all[i].Name]) {
			out = append(out, all[i])
		}
	}
	return out
}

// visibleProxiesAfterNameSort — порядок «отсортировать полный список → отфильтровать».
// Панель Servers держит тот же пайплайн в памяти; кэш видимого среза обязан
// сбрасываться при смене порядка, иначе фильтр показывает устаревшую проекцию.
func visibleProxiesAfterNameSort(
	all []api.ProxyInfo,
	facets map[string]serversNodeFacets,
	f serversFilterState,
	keep func(name string) bool,
	ascending bool,
) []api.ProxyInfo {
	sorted := make([]api.ProxyInfo, len(all))
	copy(sorted, all)
	if ascending {
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].DisplayOrName() < sorted[j].DisplayOrName()
		})
	} else {
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].DisplayOrName() > sorted[j].DisplayOrName()
		})
	}
	return applyServersFilter(sorted, facets, f, keep)
}

// serversViewCacheKey — дешёвый ключ кэша видимого среза.
//
// Сравнимый тип (все поля — значения): панель сверяет его оператором `==`,
// без аллокаций, на каждую строку списка. Что в него входит и почему — см.
// proxiesForListView в clash_api_tab.go.
type serversViewCacheKey struct {
	total     int
	filterRev uint64
	selected  int
	active    string
	delaySum  int64
}

// truncateSourceChipName — имя источника для чипа; полное уходит в тултип.
func truncateSourceChipName(s string) string {
	return truncateRunes(strings.TrimSpace(s), serversFilterSourceNameMaxRunes)
}

// collectServersFacets — один проход по текущему списку: чем узлы вообще
// различаются и что из этого предлагать чипами.
//
// Оба файловых разбора (config.json и state.json) делаются ЗДЕСЬ, по одному
// разу на пересчёт, и берутся по scope тем же путём, что подзаголовок строки
// и её предупреждения (память `remote-override-is-global`): конфиг удалённой
// машины со состоянием локальной приписал бы её узлы чужим подпискам.
func collectServersFacets(ac *core.AppController, list []api.ProxyInfo, scope services.ProxyScope) serversFilterFacets {
	out := serversFilterFacets{ByName: make(map[string]serversNodeFacets, len(list))}
	if len(list) == 0 {
		return out
	}
	nodes := wizardbusiness.LoadConfigNodes(effectiveNodeConfigPath(ac, scope))
	sources := wizardbusiness.LoadNodeSources(effectiveNodeStatePath(ac, scope))

	protoCounts := map[string]int{}
	variantCounts := map[string]int{}
	sourceCounts := map[string]int{}
	sourceNames := map[string]string{}
	names := make([]string, 0, len(list))

	for i := range list {
		p := list[i]
		names = append(names, p.DisplayOrName())

		var facets serversNodeFacets
		if node := nodes.Lookup(p.Name); node != nil && !node.IsService() {
			// Части те же, что у подзаголовка (SubtitleParts), минус
			// протокол: он отдельной категорией. Один словарь на подпись и на
			// отбор — иначе чип обещал бы не то, что написано в строке.
			parts := node.SubtitleParts()
			facets.Protocol = node.Type
			if len(parts) > 1 {
				facets.Variants = parts[1:]
			}
		}
		src, srcOK := sources.Lookup(p.Name)
		// Происхождение известно у любого узла из индекса — и контейнерного,
		// и одиночного. Без этого флага предикат считал ВСЕ узлы «неизвестного
		// происхождения» и отбор по источнику не срабатывал вовсе.
		facets.SourceKnown = srcOK
		if srcOK && src.Container {
			// Только контейнеры (подписки и папки) — решение владельца.
			// Узловой источник в чипах не участвует, и SourceID у его узла не
			// проставляется вовсе: узел остаётся «не приписанным ни к одному
			// контейнеру» и ведёт себя по общей формуле — при пустом выборе
			// проходит, при непустом без инверсии отсекается, с инверсией
			// проходит. Отдельной ветки под него в предикате нет.
			facets.SourceID = src.ID
			sourceNames[src.ID] = src.Name
		}
		out.ByName[p.Name] = facets

		if facets.Protocol != "" {
			protoCounts[facets.Protocol]++
		}
		for _, v := range facets.Variants {
			variantCounts[v]++
		}
		if facets.SourceID != "" {
			sourceCounts[facets.SourceID]++
		}
	}

	out.Protocols = sortedFacetCounts(protoCounts)
	out.Variants = sortedFacetCounts(variantCounts)
	out.Sources = make([]serversFacetSource, 0, len(sourceCounts))
	for id, c := range sourceCounts {
		out.Sources = append(out.Sources, serversFacetSource{ID: id, Name: sourceNames[id], Count: c})
	}
	sort.Slice(out.Sources, func(i, j int) bool {
		if out.Sources[i].Name != out.Sources[j].Name {
			return out.Sources[i].Name < out.Sources[j].Name
		}
		return out.Sources[i].ID < out.Sources[j].ID
	})
	out.Emojis = emojitag.Entries(names)
	return out
}

// sortedFacetCounts — значения по убыванию частоты, при равенстве по алфавиту.
func sortedFacetCounts(counts map[string]int) []serversFacetCount {
	out := make([]serversFacetCount, 0, len(counts))
	for k, c := range counts {
		out = append(out, serversFacetCount{Key: k, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}
