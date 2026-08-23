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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// chainForm — состояние вкладки «Цепочка».
type chainForm struct {
	hops   []string
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
	originalTag   string
	referencedBy  map[string][]string
	idleEntry     *widget.Entry
	stripEvasion  *widget.Check
	stripChecks   map[string]*widget.Check
	stripExplicit map[string]bool // ключ тронут пользователем → уедет в Strip

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

	// realityTags — позиции, чьи узлы поднимают reality: у них нельзя
	// снимать tls.utls, иначе ядро не стартует (T4). Считается по узлам,
	// которых сама цепочка не видит, — потому и приезжает снаружи.
	realityTags map[string]bool

	// unsupported — причина, по которой ядро не примет цепочку. Пусто, если
	// примет. Показывается предупреждением сверху: настроить маршрут,
	// который не попадёт в конфиг, пользователь вправе (ядро обновляется
	// отдельно), но узнать об этом он должен здесь, а не по факту.
	unsupported string
}

// newChainForm собирает форму. candidates — что можно поставить позицией.
func newChainForm(parent fyne.Window, cands []chainHopCandidate, realityTags, detourTags map[string]bool, nodeTypes map[string]string, unsupported string, onChange func()) *chainForm {
	f := &chainForm{
		realityTags:   realityTags,
		nodeTypes:     nodeTypes,
		detourTags:    detourTags,
		cands:         cands,
		lookup:        chainHopLookup(cands),
		parent:        parent,
		onChange:      onChange,
		hopsBox:       container.NewVBox(),
		stripChecks:   make(map[string]*widget.Check, len(configtypes.ChainStripKeys)),
		stripExplicit: make(map[string]bool, len(configtypes.ChainStripKeys)),
		unsupported:   unsupported,
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
func (f *chainForm) Load(c *configtypes.SourceChain) {
	f.hops = nil
	var rewrite map[string]interface{}
	if c != nil {
		f.hops = append([]string(nil), c.Hops...)
		rewrite = c.Rewrite
	}
	if f.rewrite != nil {
		f.rewrite.Load(rewrite)
		f.syncRewriteTypes()
	}

	if f.idleEntry != nil {
		if c != nil {
			f.idleEntry.SetText(c.IdleTimeout)
		} else {
			f.idleEntry.SetText("")
		}
	}
	if f.stripEvasion != nil {
		on := c.StripEvasionEnabled()
		// Обработчик снимаем на время установки: SetChecked зовёт OnChanged,
		// и он перерисовал бы галки каталога до того, как они загружены.
		prev := f.stripEvasion.OnChanged
		f.stripEvasion.OnChanged = nil
		f.stripEvasion.SetChecked(on)
		f.stripEvasion.OnChanged = prev
	}
	f.stripExplicit = make(map[string]bool, len(configtypes.ChainStripKeys))
	if c != nil {
		// Ключ, лежащий в патче, пользователь задал явно: при движении
		// общего переключателя его галку не трогаем.
		for k := range c.Strip {
			if _, known := configtypes.ChainStripDefault[k]; known {
				f.stripExplicit[k] = true
			}
		}
	}
	f.syncStripChecks(c)
	f.rebuildHops()
}

// syncStripChecks расставляет галки каталога: явно заданные — по патчу,
// остальные — по умолчанию ядра с учётом общего переключателя.
func (f *chainForm) syncStripChecks(c *configtypes.SourceChain) {
	evasion := c.StripEvasionEnabled()
	for _, key := range configtypes.ChainStripKeys {
		chk := f.stripChecks[key]
		if chk == nil {
			continue
		}
		want := evasion && configtypes.ChainStripDefault[key]
		if c != nil {
			if v, ok := c.Strip[key]; ok {
				want = v
			}
		}
		prev := chk.OnChanged
		chk.OnChanged = nil
		chk.SetChecked(want)
		chk.OnChanged = prev
	}
}

// Collect возвращает цепочку из формы. nil, если позиций нет вовсе —
// пустая цепочка это не настройка, а её отсутствие.
func (f *chainForm) Collect() *configtypes.SourceChain {
	hops := make([]string, 0, len(f.hops))
	for _, h := range f.hops {
		if strings.TrimSpace(h) != "" {
			hops = append(hops, h)
		}
	}
	if len(hops) == 0 {
		return nil
	}
	c := &configtypes.SourceChain{Hops: hops}
	if f.rewrite != nil {
		c.Rewrite = f.rewrite.Collect()
	}
	if f.idleEntry != nil {
		c.IdleTimeout = strings.TrimSpace(f.idleEntry.Text)
	}
	if f.stripEvasion != nil && !f.stripEvasion.Checked {
		// Пишем только выключение: nil = «умолчание ядра», и явное
		// `strip_evasion: true` было бы правкой, которой пользователь не
		// делал (та же трёхзначность, что у interrupt_exist_connections).
		off := false
		c.StripEvasion = &off
	}
	strip := make(map[string]bool, len(configtypes.ChainStripKeys))
	for _, key := range configtypes.ChainStripKeys {
		chk := f.stripChecks[key]
		if chk == nil {
			continue
		}
		def := configtypes.ChainStripDefault[key] && c.StripEvasionEnabled()
		if chk.Checked != def {
			// В патч уезжает только расхождение с умолчанием: писать весь
			// каталог значило бы зафиксировать сегодняшние умолчания ядра
			// навсегда, и смена умолчания в новой версии прошла бы мимо.
			strip[key] = chk.Checked
		}
	}
	if len(strip) > 0 {
		c.Strip = strip
	}
	return c
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
	for _, tag := range f.hops {
		c, ok := f.lookup[tag]
		if !ok || c.Kind != hopKindNode {
			continue
		}
		t := f.nodeTypes[tag]
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
	f.tagEntry.SetPlaceHolder(locale.T("wizard.chain.tag_placeholder"))
	f.tagEntry.OnChanged = func(string) {
		// Предупреждение о разрыве ссылок живёт в списке позиций —
		// перерисовываем его вместе с именем.
		f.rebuildHops()
		f.changed()
	}
	head = append(head,
		chainFormRow(locale.T("wizard.chain.tag"), f.tagEntry),
		widget.NewSeparator())

	// Подпись направления пакета — над списком, до всего остального.
	dir := widget.NewLabel(locale.T("wizard.chain.packet_order"))
	dir.Wrapping = fyne.TextWrapWord
	head = append(head, dir)

	addBtn := widget.NewButtonWithIcon(locale.T("wizard.chain.add_hop"), theme.ContentAddIcon(), func() {
		f.pickHop()
	})
	addBtn.Importance = widget.LowImportance

	f.idleEntry = widget.NewEntry()
	f.idleEntry.SetPlaceHolder(locale.T("wizard.chain.idle_placeholder"))

	f.stripEvasion = widget.NewCheck(locale.T("wizard.chain.strip_evasion"), nil)
	f.stripEvasion.SetChecked(true)
	stripRows := container.NewVBox()
	for _, key := range configtypes.ChainStripKeys {
		k := key
		chk := widget.NewCheck(k, func(bool) {
			f.stripExplicit[k] = true
			// Снятие tls.utls меняет вердикт по reality-узлам — список
			// позиций перерисовывается вместе с предупреждениями.
			f.rebuildHops()
			f.changed()
		})
		f.stripChecks[k] = chk
		hint := widget.NewLabel(chainStripHint(k))
		hint.Importance = widget.LowImportance
		stripRows.Add(container.NewBorder(nil, nil, chk, nil, hint))
	}
	// Обработчик — ПОСЛЕ SetChecked выше, иначе он сработал бы на нём.
	f.stripEvasion.OnChanged = func(bool) {
		// Общий переключатель двигает галки каталога к новым умолчаниям,
		// не трогая явно заданные пользователем: `strip` — патч поверх.
		for _, key := range configtypes.ChainStripKeys {
			if f.stripExplicit[key] {
				continue
			}
			chk := f.stripChecks[key]
			if chk == nil {
				continue
			}
			prev := chk.OnChanged
			chk.OnChanged = nil
			chk.SetChecked(f.stripEvasion.Checked && configtypes.ChainStripDefault[key])
			chk.OnChanged = prev
		}
		f.rebuildHops()
		f.changed()
	}

	// Не Accordion: раскрываясь внутри VBox, он не получает высоту от
	// родителя — содержимое есть, но его не видно (пустой блок под
	// заголовком). Своя раскрывашка на кнопке ведёт себя предсказуемо.
	advancedBody := container.NewVBox(
		chainFormRow(locale.T("wizard.chain.idle_timeout"), f.idleEntry),
		widget.NewSeparator(),
		f.stripEvasion,
		stripRows,
		widget.NewSeparator(),
		f.rewrite.Content(),
	)
	advancedBody.Hide()
	var advancedBtn *widget.Button
	advancedBtn = widget.NewButtonWithIcon(locale.T("wizard.chain.advanced"),
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

	for i, tag := range f.hops {
		idx := i
		cand := describeChainHop(tag, f.lookup)

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
		empty := widget.NewLabel(locale.T("wizard.chain.hops_empty"))
		empty.Importance = widget.LowImportance
		f.hopsBox.Add(empty)
	} else if len(f.hops) < 2 {
		// Ядру нужно минимум две позиции — и это не наша прихоть, а условие
		// старта всего конфига. Сказать об этом здесь дешевле, чем дать
		// сохранить и обнаружить на сборке.
		warn := widget.NewLabel(locale.T("wizard.chain.need_two"))
		warn.Importance = widget.WarningImportance
		warn.Wrapping = fyne.TextWrapWord
		f.hopsBox.Add(warn)
	}
	f.hopsBox.Refresh()
	f.syncRewriteTypes()
}

// conflicts — что в текущем составе ядро не примет.
//
// Форма обязана предотвращать такие конфигурации, а не давать поймать их в
// рантайме: ядро отвергает конфиг ЦЕЛИКОМ, и пользователь остаётся без
// VPN, а не без одной цепочки.
func (f *chainForm) conflicts() []string {
	var out []string

	// T4: снятый отпечаток ClientHello на reality-узле. Проверяются позиции
	// с индексом ≥ 1 — strip применяется к звеньям, а первая позиция идёт в
	// сеть как есть.
	if f.stripsUTLS() {
		var bad []string
		for i := 1; i < len(f.hops); i++ {
			if f.realityTags[f.hops[i]] {
				bad = append(bad, f.hops[i])
			}
		}
		if len(bad) > 0 {
			out = append(out, locale.Tf("wizard.chain.conflict_reality", strings.Join(bad, ", ")))
		}
	}

	// T5: вложенная цепочка допустима только первой позицией — звено это
	// «узел через предыдущую позицию», а цепочка не узел.
	var nested []string
	for i := 1; i < len(f.hops); i++ {
		if c, ok := f.lookup[f.hops[i]]; ok && c.Kind == hopKindChain {
			nested = append(nested, f.hops[i])
		}
	}
	if len(nested) > 0 {
		out = append(out, locale.Tf("wizard.chain.conflict_nested", strings.Join(nested, ", ")))
	}

	// Переименование рвёт ссылки: другие цепочки ставят эту позицией по
	// ИМЕНИ, и после правки их позиция указывает в никуда. Тег Направления
	// по этой причине вообще неизменяем (SPEC 104); здесь запрещать не
	// стоит — имена автогенерируются, и первое переименование законно, —
	// но предупредить обязаны.
	if tag := f.Tag(); tag != "" && f.referencedBy != nil {
		if users := f.referencedBy[f.originalTag]; len(users) > 0 && tag != f.originalTag {
			out = append(out, locale.Tf("wizard.chain.rename_breaks_refs",
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
	if len(f.hops) > 0 && f.detourTags[f.hops[0]] {
		out = append(out, locale.Tf("wizard.chain.conflict_detour_entry", f.hops[0]))
	}
	var detouredLinks []string
	for i := 1; i < len(f.hops); i++ {
		if f.detourTags[f.hops[i]] {
			detouredLinks = append(detouredLinks, f.hops[i])
		}
	}
	if len(detouredLinks) > 0 {
		out = append(out, locale.Tf("wizard.chain.note_detour_ignored", strings.Join(detouredLinks, ", ")))
	}

	// Позиция, которой больше нет среди целей: ссылка в никуда.
	var missing []string
	for _, tag := range f.hops {
		if _, ok := f.lookup[tag]; !ok {
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		out = append(out, locale.Tf("wizard.chain.conflict_missing", strings.Join(missing, ", ")))
	}
	return out
}

// stripsUTLS — снимается ли отпечаток ClientHello при текущих галках.
func (f *chainForm) stripsUTLS() bool {
	if chk := f.stripChecks[configtypes.ChainStripTLSUTLS]; chk != nil {
		return chk.Checked
	}
	return configtypes.ChainStripDefault[configtypes.ChainStripTLSUTLS]
}

// moveHop переставляет позицию (перетаскивание).
func (f *chainForm) moveHop(from, to int) {
	if from == to || from < 0 || to < 0 || from >= len(f.hops) || to >= len(f.hops) {
		return
	}
	tag := f.hops[from]
	rest := append(f.hops[:from:from], f.hops[from+1:]...)
	out := make([]string, 0, len(f.hops))
	out = append(out, rest[:to]...)
	out = append(out, tag)
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
	for _, t := range f.hops {
		chosen[t] = true
	}
	var options []string
	byLabel := make(map[string]string, len(f.cands))
	for _, c := range f.cands {
		if chosen[c.Tag] {
			continue
		}
		label := c.Display() + "   —   " + c.KindText()
		options = append(options, label)
		byLabel[label] = c.Tag
	}
	if len(options) == 0 {
		dialog.ShowInformation(locale.T("wizard.chain.add_hop"),
			locale.T("wizard.chain.no_candidates"), f.parent)
		return
	}
	sel := widget.NewSelect(options, nil)
	sel.SetSelected(options[0])
	dialog.ShowCustomConfirm(
		locale.T("wizard.chain.add_hop"),
		locale.T("wizard.outbound.button_save"),
		locale.T("wizard.outbound.button_cancel"),
		sel,
		func(ok bool) {
			if !ok || sel.Selected == "" {
				return
			}
			tag, exists := byLabel[sel.Selected]
			if !exists {
				return
			}
			f.hops = append(f.hops, tag)
			f.rebuildHops()
			f.changed()
		}, f.parent)
}

func (f *chainForm) changed() {
	if f.onChange != nil {
		f.onChange()
	}
}

// chainStripHint — расшифровка ключа каталога: сами ключи ядра непрозрачны,
// а выбор без понимания последствий — не выбор.
func chainStripHint(key string) string {
	switch key {
	case configtypes.ChainStripTLSFragment:
		return locale.T("wizard.chain.strip_tls_fragment")
	case configtypes.ChainStripMultiplexPadding:
		return locale.T("wizard.chain.strip_multiplex_padding")
	case configtypes.ChainStripXHTTPPadding:
		return locale.T("wizard.chain.strip_xhttp_padding")
	case configtypes.ChainStripTLSUTLS:
		return locale.T("wizard.chain.strip_tls_utls")
	}
	return ""
}

func chainFormRow(label string, field fyne.CanvasObject) fyne.CanvasObject {
	const labelWidth = 150
	l := widget.NewLabel(label)
	box := container.NewGridWrap(fyne.NewSize(labelWidth, l.MinSize().Height), l)
	return container.NewBorder(nil, nil, box, nil, field)
}

// getParserConfigForChain — ParserConfig модели для сбора кандидатов.
//
// Направления берутся оттуда: в Sources их нет, а позицией цепочки
// Направление быть обязано — это главное, чего не умеет detour.
func getParserConfigForChain(m *wizardmodels.WizardModel) *config.ParserConfig {
	if m == nil {
		return nil
	}
	if m.ParserConfig != nil {
		return m.ParserConfig
	}
	if strings.TrimSpace(m.ParserConfigJSON) == "" {
		return nil
	}
	var parsed config.ParserConfig
	if err := json.Unmarshal([]byte(m.ParserConfigJSON), &parsed); err != nil {
		return nil
	}
	return &parsed
}

// chainNodeFlags — то, что форма знает об узлах, но сама увидеть не может:
// кто поднимает reality (у них нельзя снимать tls.utls), кто уже ходит
// через свой detour, и какого типа каждый узел (для таблицы переопределений).
func chainNodeFlags(m *wizardmodels.WizardModel) (reality, detoured map[string]bool, types map[string]string) {
	reality = make(map[string]bool)
	detoured = make(map[string]bool)
	types = make(map[string]string)
	if m == nil {
		return reality, detoured, types
	}
	for _, n := range m.PreviewNodes {
		if n == nil {
			continue
		}
		if config.NodeUsesReality(n) {
			reality[n.Tag] = true
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
	return reality, detoured, types
}
