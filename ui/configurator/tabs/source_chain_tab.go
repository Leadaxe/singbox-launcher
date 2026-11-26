// File source_chain_tab.go — вкладка «Цепочка» в окне источника
// (SPEC 110).
//
// Форма правит configtypes.SourceChain: список позиций и настройки
// звеньев. Всё, что она показывает, ограничено тем, что ядро реально
// принимает, — форма обязана не давать собрать конфиг, на котором ядро не
// стартует, а не ловить это в рантайме.
//
// Порядок позиций подписан сверху и это не украшение: `chain` читается в
// порядке пакета (первый хоп — от вас), а `detour` — наоборот, «кто через
// кого». Это единственное различие между двумя механизмами при чтении, и
// единственное, в чём легко ошибиться (SPEC 110 T3).
//
// Цепочка — источник, а не Направление: она описывает МАРШРУТ, а точка
// выбора между маршрутами это Направление. В Направления цепочка попадает
// узлом, наравне с серверами подписки.
package tabs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	chainConflictDetourEntryText = "The first position (%s) dials through its own detour — the real path is longer than shown: one more hop precedes it that is not in this list."
	chainConflictForwardRefText  = "Chains %s are declared below this one in the sources list — a chain may only reference chains above it. Drag them higher, or this chain will not build."
	chainConflictTagTakenText    = "The name “%s” is already taken by another node, Direction or chain — two outbounds with one tag cannot coexist, this chain will be dropped at build."
	chainNoteDetourIgnoredText   = "Positions %s have their own detour — it does not apply inside a chain: a link always dials through the previous position. The path is exactly as shown."
)

// chainForm — состояние вкладки «Цепочка».
type chainForm struct {
	// hops — позиции ССЫЛКАМИ (SPEC 118 Т8). Строкой позицию хранить нельзя:
	// финальный тег узла папки вычисляется её тег-политикой на сборке, и
	// сохранённый протух бы от правки префикса.
	hops   []corestate.NodeLink
	lookup map[string]chainHopCandidate
	cands  []chainHopCandidate

	hopsBox  *fyne.Container
	parent   fyne.Window
	onChange func()

	// built — собранное содержимое вкладки. Content() создаёт виджеты, и
	// звать его дважды значило бы получить вторую форму, не связанную с
	// загруженными данными.
	built fyne.CanvasObject

	tagEntry *widget.Entry
	// originalTag — имя на момент открытия окна, и referencedBy — кто на
	// него ссылается. Нужны, чтобы предупредить о разрыве ссылок при
	// переименовании: цепочки указывают друг на друга по имени.
	originalTag  string
	referencedBy map[string][]string
	interrupt    *widget.Check // interrupt_exist_connections (SPEC 124)

	// rewrite — таблица переопределений опций звена. Раньше эти настройки
	// правились только на вкладке JSON, и форма их лишь проносила мимо
	// себя, чтобы не потерять.
	rewrite *rewriteEditor

	// detourTags — узлы со своим detour. Что он значит внутри цепочки,
	// зависит от позиции: на входе (позиция 1) он работает и удлиняет путь,
	// на звеньях (позиция ≥ 2) перезаписывается и не действует вовсе — см.
	// conflicts(), где это различие и разводится (T7).
	detourTags map[string]bool

	// nodeTypes — тип протокола по тегу узла (vless, wireguard, …). Нужен
	// подсказкой в таблице переопределений: ключ верхнего уровня `rewrite`
	// — это тип outbound'а, и опечатка в нём означает правило, которое
	// молча не применяется.
	nodeTypes map[string]string

	// nodesKnown — разобран ли кэш узлов. Пока нет, позиция, которой не
	// видно среди кандидатов, — «ещё не знаем», а не «потеряна».
	nodesKnown bool

	// graph — исходящие и endpoints последней сборки (строки модели): по ним
	// редактор видит группы и чужие цепочки и проверяет перезапись цепочки
	// ядром (SPEC 189) до сохранения. nil — проверка не делается.
	graph func() (outbounds, endpoints []string)

	// unsupported — причина, по которой ядро не примет цепочку. Пусто, если
	// примет. Показывается предупреждением сверху: настроить маршрут,
	// который не попадёт в конфиг, пользователь вправе (ядро обновляется
	// отдельно), но узнать об этом он должен здесь, а не по факту.
	unsupported string
}

// newChainForm собирает форму. candidates — что можно поставить позицией.
func newChainForm(parent fyne.Window, cands []chainHopCandidate, detourTags map[string]bool, nodeTypes map[string]string, unsupported string, onChange func()) *chainForm {
	f := &chainForm{
		nodeTypes:   nodeTypes,
		detourTags:  detourTags,
		cands:       cands,
		lookup:      chainHopLookup(cands),
		parent:      parent,
		onChange:    onChange,
		hopsBox:     container.NewVBox(),
		unsupported: unsupported,
	}
	f.rewrite = newRewriteEditor(func() { f.changed() })
	return f
}

// SetTag показывает имя цепочки в форме.
//
// Имя живёт не в SourceChain, а в источнике (Source.Label) — оттуда его и
// берёт сборка, превращая в тег узла. Форма правит его наравне с
// позициями, потому что для пользователя это одно и то же окно.
func (f *chainForm) SetTag(tag string) {
	if f.tagEntry == nil {
		return
	}
	f.originalTag = tag
	prev := f.tagEntry.OnChanged
	f.tagEntry.OnChanged = nil
	f.tagEntry.SetText(tag)
	f.tagEntry.OnChanged = prev
}

// SetCandidates заменяет список кандидатов — например, когда кэш узлов
// дозагрузился уже после открытия окна.
//
// detourTags обновляются ВМЕСТЕ с кандидатами: они считаются из тех же
// NodePool, и прежняя сигнатура их выбрасывала — окно, открытое до разбора
// подписок, навсегда оставалось с пустым множеством, и предупреждение T7
// (detour на позиции) молчало именно там, где оно нужнее всего.
func (f *chainForm) SetCandidates(cands []chainHopCandidate, detourTags map[string]bool, nodeTypes map[string]string, nodesKnown bool) {
	f.cands = cands
	f.lookup = chainHopLookup(cands)
	f.detourTags = detourTags
	f.nodeTypes = nodeTypes
	f.nodesKnown = nodesKnown
	f.rebuildHops()
}

// SetReferencedBy сообщает форме, кто ссылается на цепочки по имени.
func (f *chainForm) SetReferencedBy(m map[string][]string) { f.referencedBy = m }

// Tag — имя, введённое пользователем.
func (f *chainForm) Tag() string {
	if f.tagEntry == nil {
		return ""
	}
	return strings.TrimSpace(f.tagEntry.Text)
}

// Load заполняет форму содержимым цепочки. nil = пустая цепочка.
//
// hops приходят отдельно от настроек: в каноне v7 позиции — ссылки модели,
// а `configtypes.SourceChain` это форма ЯДРА, где позиция уже строка.
func (f *chainForm) Load(c *configtypes.SourceChain, hops []corestate.NodeLink) {
	f.hops = append([]corestate.NodeLink(nil), hops...)
	var rewrite map[string]interface{}
	if c != nil {
		rewrite = c.Rewrite
	}
	if f.rewrite != nil {
		f.rewrite.Load(rewrite)
		f.syncRewriteTypes()
	}

	if f.interrupt != nil {
		prev := f.interrupt.OnChanged
		f.interrupt.OnChanged = nil
		f.interrupt.SetChecked(c != nil && c.InterruptExistConnections)
		f.interrupt.OnChanged = prev
	}
	f.rebuildHops()
}

// CollectLinks — позиции формы в том виде, в каком их хранит модель.
//
// Это ЕДИНСТВЕННЫЙ источник позиций для сохранения: Collect ниже отдаёт
// форму ядра, где позиция уже развёрнута в финальный тег — годится для
// валидации и превью JSON, но не для записи в состояние.
func (f *chainForm) CollectLinks() []corestate.NodeLink {
	out := make([]corestate.NodeLink, 0, len(f.hops))
	for _, h := range f.hops {
		if strings.TrimSpace(h.Tag) != "" {
			out = append(out, h)
		}
	}
	return out
}

// Collect возвращает цепочку из формы в форме ЯДРА: позиции развёрнуты в
// финальные теги (так их видит эмиттер и валидатор). nil, если позиций нет
// вовсе — пустая цепочка это не настройка, а её отсутствие.
//
// Ссылка, которой нет среди кандидатов (узел исчез из подписки), разворачивается
// в свой сырой тег: показать её потерянной честнее, чем выбросить — цепочка
// с висячей позицией всё равно не соберётся, и пользователь обязан увидеть, что
// именно пропало.
func (f *chainForm) Collect() *configtypes.SourceChain {
	hops := make([]string, 0, len(f.hops))
	for _, h := range f.hops {
		if tag := f.displayTag(h); tag != "" {
			hops = append(hops, tag)
		}
	}
	if len(hops) == 0 {
		return nil
	}
	c := &configtypes.SourceChain{Hops: hops}
	if f.rewrite != nil {
		c.Rewrite = f.rewrite.Collect()
	}
	if f.interrupt != nil {
		c.InterruptExistConnections = f.interrupt.Checked
	}
	return c
}

// displayTag — финальный тег позиции: как её зовут в конфиге.
//
// Проверки формы (reality, свой detour, тип протокола) считаются по
// эмитированному пулу, где ключ — финальный тег, а не ссылка модели: одну и ту
// же папку под другим префиксом ядро увидит под другими именами.
func (f *chainForm) displayTag(link corestate.NodeLink) string {
	if c, ok := f.lookup[hopLinkKey(link)]; ok {
		return c.Tag
	}
	return strings.TrimSpace(link.Tag)
}

// syncRewriteTypes отдаёт редактору типы протоколов, реально стоящих в
// цепочке.
//
// Список подсказки, а не ограничение: узел нужного типа может появиться
// позже (подписка обновится), и запретить вписать «vless», когда его сейчас
// нет в позициях, значило бы заставить ждать обновления ради настройки.
//
// Позиции, не являющиеся узлами (Направление, группа, другая цепочка),
// собственного типа не имеют — их звенья разворачиваются в конкретные узлы
// только в рантайме ядра, и здесь о них ничего не известно.
func (f *chainForm) syncRewriteTypes() {
	if f.rewrite == nil {
		return
	}
	seen := make(map[string]bool, len(f.hops))
	var types []string
	for _, link := range f.hops {
		c, ok := f.lookup[hopLinkKey(link)]
		if !ok || c.Kind != hopKindNode {
			continue
		}
		t := f.nodeTypes[c.Tag]
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		types = append(types, t)
	}
	sort.Strings(types)
	f.rewrite.SetTypes(types)
}

// Content — содержимое вкладки.
func (f *chainForm) Content() fyne.CanvasObject {
	var head []fyne.CanvasObject

	if f.unsupported != "" {
		warn := widget.NewLabel("⚠️ " + f.unsupported)
		warn.Wrapping = fyne.TextWrapWord
		warn.Importance = widget.WarningImportance
		head = append(head, warn, widget.NewSeparator())
	}

	// Имя цепочки = ТЕГ её узла: именно на него ссылаются Направления и
	// правила, и именно он виден в списке прокси. Поле обязано быть в
	// форме — иначе переименовать созданную цепочку негде вовсе.
	f.tagEntry = widget.NewEntry()
	f.tagEntry.SetPlaceHolder(locale.T("Directions and rules reference this name"))
	f.tagEntry.OnChanged = func(string) {
		// Предупреждение о разрыве ссылок живёт в списке позиций —
		// перерисовываем его вместе с именем.
		f.rebuildHops()
		f.changed()
	}
	head = append(head,
		chainFormRow(locale.T("Chain name"), f.tagEntry),
		widget.NewSeparator())

	// Подпись направления пакета — над списком, до всего остального.
	dir := widget.NewLabel(locale.T("Packets travel top to bottom: the first hop is next to you, the last one is the address the site sees."))
	dir.Wrapping = fyne.TextWrapWord
	head = append(head, dir)

	addBtn := widget.NewButtonWithIcon(locale.T("Add hop"), theme.ContentAddIcon(), func() {
		f.pickHop()
	})
	addBtn.Importance = widget.LowImportance

	// Разрыв внешних соединений при тумблере позиции (SPEC 124). Снята по
	// умолчанию, как у ядра; как у ядра.
	f.interrupt = widget.NewCheck(locale.T("Interrupt existing connections on position toggle"), nil)
	f.interrupt.OnChanged = func(bool) { f.changed() }
	interruptHint := widget.NewLabel(locale.T("Otherwise live connections finish on the old route; only new ones take the new path."))
	interruptHint.Importance = widget.LowImportance
	interruptHint.Wrapping = fyne.TextWrapWord

	// Не Accordion: раскрываясь внутри VBox, он не получает высоту от
	// родителя — содержимое есть, но его не видно (пустой блок под
	// заголовком). Своя раскрывашка на кнопке ведёт себя предсказуемо.
	advancedBody := container.NewVBox(
		f.interrupt,
		interruptHint,
		widget.NewSeparator(),
		f.rewrite.Content(),
	)
	advancedBody.Hide()
	var advancedBtn *widget.Button
	advancedBtn = widget.NewButtonWithIcon(locale.T("Advanced"),
		theme.MenuExpandIcon(), func() {
			if advancedBody.Visible() {
				advancedBody.Hide()
				advancedBtn.SetIcon(theme.MenuExpandIcon())
			} else {
				advancedBody.Show()
				advancedBtn.SetIcon(theme.MenuDropDownIcon())
			}
		})
	advancedBtn.Alignment = widget.ButtonAlignLeading
	advancedBtn.Importance = widget.LowImportance
	advanced := container.NewVBox(advancedBtn, advancedBody)

	top := container.NewVBox(head...)
	body := container.NewVBox(f.hopsBox, container.NewCenter(addBtn), widget.NewSeparator(), advanced)
	return container.NewVBox(top, body)
}

// rebuildHops перерисовывает список позиций.
func (f *chainForm) rebuildHops() {
	if f.hopsBox == nil {
		return
	}
	f.hopsBox.Objects = f.hopsBox.Objects[:0]

	// Группа перетаскивания живёт ровно один перерисов: строки регистрируют
	// себя заново, и устаревшая геометрия пережить перестановку не может.
	dragGroup := fynewidget.NewDragReorderGroup(func(from, to int) {
		f.moveHop(from, to)
	})

	for i, link := range f.hops {
		idx := i
		cand := describeChainHop(link, f.lookup, f.nodesKnown)

		num := widget.NewLabel(fmt.Sprintf("%d.", idx+1))
		name := widget.NewLabel(cand.Display())
		if cand.Kind == hopKindUnknown {
			// Позиция, которой больше нет: цепочка с ней не соберётся.
			// Показываем, а не выбрасываем молча — иначе маршрут поменялся
			// бы без ведома пользователя.
			name.Importance = widget.DangerImportance
		}
		kind := widget.NewLabel(cand.KindText())
		kind.Importance = widget.LowImportance

		del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			f.hops = append(f.hops[:idx:idx], f.hops[idx+1:]...)
			f.rebuildHops()
			f.changed()
		})
		del.Importance = widget.LowImportance

		handle := fynewidget.NewDragHandle(dragGroup, idx, nil)

		row := container.NewBorder(nil, nil,
			container.NewHBox(handle, num),
			container.NewHBox(kind, del),
			name)
		dragGroup.Register(idx, row)
		f.hopsBox.Add(row)
	}

	for _, msg := range f.conflicts() {
		warn := widget.NewLabel("⚠️ " + msg)
		warn.Importance = widget.WarningImportance
		warn.Wrapping = fyne.TextWrapWord
		f.hopsBox.Add(warn)
	}

	if len(f.hops) == 0 {
		empty := widget.NewLabel(locale.T("No positions yet — add at least two."))
		empty.Importance = widget.LowImportance
		f.hopsBox.Add(empty)
	} else if len(f.hops) < 2 {
		// Ядру нужно минимум две позиции — и это не наша прихоть, а условие
		// старта всего конфига. Сказать об этом здесь дешевле, чем дать
		// сохранить и обнаружить на сборке.
		warn := widget.NewLabel(locale.T("One more position needed: the core rejects a single-hop chain."))
		warn.Importance = widget.WarningImportance
		warn.Wrapping = fyne.TextWrapWord
		f.hopsBox.Add(warn)
	}
	f.hopsBox.Refresh()
	f.syncRewriteTypes()
}

// blockingProblems — кольца и конфликты цепочки (SPEC 189): с ними форма не
// сохраняется — решение владельца 26.11, ядро отвергло бы весь конфиг, а
// понятного сообщения не даёт. Тексты реестра подсказывают, что скопировать.
func (f *chainForm) blockingProblems() []string {
	if f.graph == nil {
		return nil
	}
	c := f.Collect()
	if c == nil {
		return nil
	}
	obs, eps := f.graph()
	return config.ChainEditProblems(obs, eps, f.originalTag, config.ChainOutboundObject(f.Tag(), c))
}

// conflicts — что в текущем составе ядро не примет.
//
// Форма обязана предотвращать такие конфигурации, а не давать поймать их в
// рантайме: ядро отвергает конфиг ЦЕЛИКОМ, и пользователь остаётся без
// VPN, а не без одной цепочки.
func (f *chainForm) conflicts() []string {
	var out []string

	// Финальные теги позиций: множества reality/detour и типы протоколов
	// ключуются именно ими (они считаются по эмитированному пулу).
	tags := make([]string, len(f.hops))
	for i, link := range f.hops {
		tags[i] = f.displayTag(link)
	}

	// SPEC 189: ядро с перезаписью цепочки в detour (SPEC 165) отвергает
	// весь конфиг на кольце, общем узле или группе двух цепочек, локальном
	// порту узла внутри цепочки, занятом теге хопа и rewrite с detour.
	// Решение владельца 26.11: клиент такого не допускает — показываем здесь
	// той же проверкой, что снимет цепочку на сборке.
	// Ядро цели здесь не различается: пин лаунчера — ядро с перезаписью, а
	// машина на старом ядре получит те же цепочки после обновления.
	out = append(out, f.blockingProblems()...)

	// T5: вложенная цепочка допустима только первой позицией — звено это
	// «узел через предыдущую позицию», а цепочка не узел.
	var nested []string
	for i := 1; i < len(f.hops); i++ {
		if c, ok := f.lookup[hopLinkKey(f.hops[i])]; ok && c.Kind == hopKindChain {
			nested = append(nested, tags[i])
		}
	}
	if len(nested) > 0 {
		out = append(out, locale.Tf("A nested chain (%s) is only allowed at the first position — move it to the top or remove it.", strings.Join(nested, ", ")))
	}

	// Ссылка «вперёд»: сборка разрешает цепочку-позицию только если та
	// объявлена ВЫШЕ по списку источников (так циклы невозможны по
	// построению). Форма это ограничение раньше не знала — цепочка молча
	// деградировала на сборке с причиной «позиция не найдена».
	var forward []string
	for _, hop := range f.hops {
		if c, ok := f.lookup[hopLinkKey(hop)]; ok && c.Kind == hopKindChain && c.Below {
			forward = append(forward, c.Tag)
		}
	}
	if len(forward) > 0 {
		out = append(out, locale.Tf(chainConflictForwardRefText, strings.Join(forward, ", ")))
	}

	// Дубль имени: цепочка, названная как существующий узел, Направление
	// или другая цепочка, даёт два outbound'а с одним тегом — сборка её
	// деградирует, и предупредить нужно в момент переименования.
	if tag := f.Tag(); tag != "" && tag != f.originalTag {
		if _, taken := f.lookup[hopLinkKey(corestate.NodeLink{Tag: tag})]; taken {
			out = append(out, locale.Tf(chainConflictTagTakenText, tag))
		}
	}

	// Переименование рвёт ссылки: другие цепочки ставят эту позицией по
	// ИМЕНИ, и после правки их позиция указывает в никуда. Тег Направления
	// по этой причине вообще неизменяем (SPEC 104); здесь запрещать не
	// стоит — имена автогенерируются, и первое переименование законно, —
	// но предупредить обязаны.
	if tag := f.Tag(); tag != "" && f.referencedBy != nil {
		if users := f.referencedBy[f.originalTag]; len(users) > 0 && tag != f.originalTag {
			out = append(out, locale.Tf("Chain %s is used as a position by: %s. After renaming, their position will point nowhere — fix those too.",
				f.originalTag, strings.Join(users, ", ")))
		}
	}

	// T7: узел со своим detour. Что произойдёт — зависит от ПОЗИЦИИ, и
	// сказать одно и то же про обе значило бы соврать про одну из них
	// (проверено на ядре 1.14.0-lx.27-rc.5):
	//
	//   позиция 1 (вход) — узел не клонируется и идёт как есть, вместе со
	//     своим детуром. Реальный путь ДЛИННЕЕ показанного: перед первым
	//     хопом добавляется ещё один, которого в списке нет;
	//   позиция ≥ 2 (звено) — detour перезаписывается на предыдущую позицию
	//     безусловно (`protocol/chain/transform.go:110`): поле одно, и
	//     именно им цепочка выражает «через предыдущий хоп». Собственный
	//     детур узла просто не применяется — путь ровно такой, как нарисован.
	//
	// Первое — предупреждение (пользователь получит не тот путь), второе —
	// справка (всё работает, но настройка узла здесь не действует; советовать
	// «уберите detour» бессмысленно, он и так игнорируется).
	if len(tags) > 0 && f.detourTags[tags[0]] {
		out = append(out, locale.Tf(chainConflictDetourEntryText, tags[0]))
	}
	var detouredLinks []string
	for i := 1; i < len(tags); i++ {
		if f.detourTags[tags[i]] {
			detouredLinks = append(detouredLinks, tags[i])
		}
	}
	if len(detouredLinks) > 0 {
		out = append(out, locale.Tf(chainNoteDetourIgnoredText, strings.Join(detouredLinks, ", ")))
	}

	// Позиция, которой больше нет среди целей: ссылка в никуда.
	//
	// Только при готовом кэше узлов: пока он не разобран, в кандидатах одни
	// Направления, и «позиций больше нет» было бы приговором рабочей
	// цепочке.
	var missing []string
	if f.nodesKnown {
		for i, hop := range f.hops {
			if _, ok := f.lookup[hopLinkKey(hop)]; !ok {
				missing = append(missing, tags[i])
			}
		}
	}
	if len(missing) > 0 {
		out = append(out, locale.Tf("These positions are no longer among the available targets: %s. A chain with such a reference will not reach the config.", strings.Join(missing, ", ")))
	}
	return out
}

// moveHop переставляет позицию (перетаскивание).
func (f *chainForm) moveHop(from, to int) {
	if from == to || from < 0 || to < 0 || from >= len(f.hops) || to >= len(f.hops) {
		return
	}
	moved := f.hops[from]
	rest := append(f.hops[:from:from], f.hops[from+1:]...)
	out := make([]corestate.NodeLink, 0, len(f.hops))
	out = append(out, rest[:to]...)
	out = append(out, moved)
	out = append(out, rest[to:]...)
	f.hops = out
	f.rebuildHops()
	f.changed()
}

// pickHop показывает выбор позиции из существующих целей.
//
// Уже занятые исключаются: ядро отвергает цепочку с дублем
// (`protocol/chain/chain.go:96`).
func (f *chainForm) pickHop() {
	chosen := make(map[string]bool, len(f.hops))
	for _, h := range f.hops {
		chosen[hopLinkKey(h)] = true
	}
	var options []string
	byLabel := make(map[string]corestate.NodeLink, len(f.cands))
	// looped — группы свёртки, замыкающие кольцо: в списке видны с ⚠, но
	// позицией не встают (SPEC 182).
	looped := map[string]bool{}
	for _, c := range f.cands {
		if chosen[hopLinkKey(c.Link)] || c.Known {
			continue
		}
		label := c.Display() + "   —   " + c.KindText()
		if c.Loop {
			// «Здесь» — с тегом цепочки (правка владельца 26.11): окно может
			// быть не одно, и «здесь» без имени неоднозначно.
			label += "   ⚠ " + locale.Tf("leads back here (%q)", f.Tag())
		}
		// Две папки вправе нести узел под одним финальным тегом (уникализация
		// живёт на сборке, не в модели) — подпись тогда одна, а ссылки разные.
		// Разводим суффиксом, иначе вторая опция была бы недостижима.
		for n := 2; ; n++ {
			if _, dup := byLabel[label]; !dup {
				break
			}
			label = c.Display() + " (" + strconv.Itoa(n) + ")   —   " + c.KindText()
		}
		options = append(options, label)
		byLabel[label] = c.Link
		if c.Loop {
			looped[label] = true
		}
	}
	if len(options) == 0 {
		dialog.ShowInformation(locale.T("Add hop"),
			locale.T("Nothing left to add: every available target is already in the chain."), f.parent)
		return
	}
	sel := widget.NewSelect(options, nil)
	sel.SetSelected(options[0])
	dialog.ShowCustomConfirm(
		locale.T("Add hop"),
		locale.T("Save"),
		locale.T("Cancel"),
		sel,
		func(ok bool) {
			if !ok || sel.Selected == "" {
				return
			}
			link, exists := byLabel[sel.Selected]
			if !exists {
				return
			}
			if looped[sel.Selected] {
				dialog.ShowInformation(locale.T("Add hop"),
					locale.Tf("This group cannot be used here (%q): its own nodes connect through this entry, so the path would loop back.", f.Tag()), f.parent)
				return
			}
			f.hops = append(f.hops, link)
			f.rebuildHops()
			f.changed()
		}, f.parent)
}

func (f *chainForm) changed() {
	if f.onChange != nil {
		f.onChange()
	}
}

func chainFormRow(label string, field fyne.CanvasObject) fyne.CanvasObject {
	const labelWidth = 150
	l := widget.NewLabel(label)
	box := container.NewGridWrap(fyne.NewSize(labelWidth, l.MinSize().Height), l)
	return container.NewBorder(nil, nil, box, nil, field)
}

// chainNodeFlags — то, что форма знает об узлах, но сама увидеть не может:
// кто уже ходит через свой detour, и какого типа каждый узел (для таблицы
// переопределений).
func chainNodeFlags(m *wizardmodels.WizardModel) (detoured map[string]bool, types map[string]string) {
	detoured = make(map[string]bool)
	types = make(map[string]string)
	if m == nil {
		return detoured, types
	}
	for _, n := range m.NodePool {
		if n == nil {
			continue
		}
		if d, ok := n.Outbound["detour"].(string); ok && strings.TrimSpace(d) != "" {
			detoured[n.Tag] = true
		}
		// Тип из объекта outbound'а, а не Scheme: `rewrite` ключуется типом
		// SING-BOX'а (`shadowsocks`), а схема лаунчера бывает короче (`ss`).
		if t, ok := n.Outbound["type"].(string); ok && t != "" {
			types[n.Tag] = t
		}
	}
	return detoured, types
}

// chainLinksFromTags — финальные теги вкладки JSON обратно в ссылки модели.
//
// Вкладка JSON говорит формой ядра: там позиция — строка `outbounds[i]`,
// финальный тег. Модель хранит ССЫЛКУ, и перевод возможен только через
// кандидатов формы: тег `[NL] 🇳🇱-1` может принадлежать узлу папки, и
// корневая ссылка на него ведёт в никуда (fail-closed на сборке).
//
// Тег без кандидата остаётся корневой ссылкой: пользователь вправе вписать
// цель, которой ещё нет (подписка обновится), и переписывать его ввод форма
// не должна — судьбу такой позиции решит резолв сборки.
func chainLinksFromTags(f *chainForm, tags []string) []corestate.NodeLink {
	byTag := map[string]corestate.NodeLink{}
	if f != nil {
		for _, c := range f.cands {
			if _, dup := byTag[c.Tag]; !dup {
				byTag[c.Tag] = c.Link
			}
		}
	}
	out := make([]corestate.NodeLink, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if link, ok := byTag[t]; ok {
			out = append(out, link)
			continue
		}
		out = append(out, corestate.NodeLink{Tag: t})
	}
	return out
}

// chainFormSettings — НАСТРОЙКИ маршрута цепочки из канонического узла
// (SPEC 118 W5): idle_timeout, strip_evasion, каталог strip, rewrite.
//
// В модели v7 цепочка хранится узлом: настройки — в `body`, позиции —
// отдельным полем `hops []NodeLink`. Форма же говорит формой ядра
// (configtypes.SourceChain), потому что именно её проверяют валидаторы и
// эмиттер. Перевод — здесь, в одной точке на обе стороны.
//
// Позиции сюда НЕ кладутся: у формы они живут ссылками, и развернуть их в
// финальные теги может только она сама (по своим кандидатам).
func chainFormSettings(src *wizardmodels.Source) *configtypes.SourceChain {
	if src == nil {
		return nil
	}
	return configtypes.ChainFromBody(src.Body, nil)
}

// chainFormHops — позиции канонического узла как есть.
func chainFormHops(src *wizardmodels.Source) []corestate.NodeLink {
	if src == nil {
		return nil
	}
	return src.Hops
}

// applyChainFormToSource — обратная сторона: форма → канонический узел.
//
// Настройки маршрута едут в `body`, позиции — ССЫЛКАМИ как есть (SPEC 118 Т8).
// Прежняя версия складывала их корневыми ссылками по финальному тегу и тем
// самым ломала хопы на узлы папок: ссылка `{folder, "🇳🇱-1"}` превращалась в
// `{"", "[NL] 🇳🇱-1"}` — тег, за которым в корне никого нет, — и цепочка
// молча деградировала fail-closed на первой же пересборке.
func applyChainFormToSource(src *wizardmodels.Source, c *configtypes.SourceChain, hops []corestate.NodeLink) {
	if src == nil {
		return
	}
	if c == nil && len(hops) == 0 {
		src.Body = nil
		src.Hops = nil
		return
	}
	src.Body = configtypes.ChainBody(c)
	src.Hops = append([]corestate.NodeLink(nil), hops...)
}
