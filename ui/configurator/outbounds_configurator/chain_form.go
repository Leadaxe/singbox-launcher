// File chain_form.go — вкладка «Цепочка» в окне правки Направления
// (SPEC 110, фаза 3).
//
// Форма правит configtypes.DirectionChain: список позиций и настройки
// звеньев. Всё, что она показывает, ограничено тем, что ядро реально
// принимает, — форма обязана не давать собрать конфиг, на котором ядро не
// стартует, а не ловить это в рантайме.
//
// Порядок позиций подписан сверху и это не украшение: `chain` читается в
// порядке пакета (первый хоп — от вас), а `detour` — наоборот, «кто через
// кого». Это единственное различие между двумя механизмами при чтении, и
// единственное, в чём легко ошибиться (SPEC 110 T3).
package outbounds_configurator

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// chainForm — состояние вкладки «Цепочка».
type chainForm struct {
	hops   []string
	lookup map[string]chainHopCandidate
	cands  []chainHopCandidate

	hopsBox  *fyne.Container
	parent   fyne.Window
	onChange func()

	idleEntry     *widget.Entry
	stripEvasion  *widget.Check
	stripChecks   map[string]*widget.Check
	stripExplicit map[string]bool // ключ тронут пользователем → уедет в Strip

	// keep — поля DirectionChain, которых форма не показывает (Rewrite).
	// Их правят на вкладке JSON, и потерять их на Load/Collect значило бы
	// стереть настройку, которую форма просто не умеет отобразить.
	keep configtypes.DirectionChain

	// detourTags — позиции, чьи узлы уже ходят через собственный detour.
	// Два механизма многохопа на одном пути дают маршрут, которого никто не
	// задумывал: цепочка ставит звено «узел через предыдущую позицию», а
	// detour узла добавляет к нему ещё один невидимый в списке хоп (T7).
	detourTags map[string]bool

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
func newChainForm(parent fyne.Window, cands []chainHopCandidate, realityTags, detourTags map[string]bool, unsupported string, onChange func()) *chainForm {
	f := &chainForm{
		realityTags:   realityTags,
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
	return f
}

// Load заполняет форму содержимым цепочки. nil = пустая цепочка.
func (f *chainForm) Load(c *configtypes.DirectionChain) {
	f.hops = nil
	f.keep = configtypes.DirectionChain{}
	if c != nil {
		f.hops = append([]string(nil), c.Hops...)
		f.keep.Rewrite = c.Rewrite
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
func (f *chainForm) syncStripChecks(c *configtypes.DirectionChain) {
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
func (f *chainForm) Collect() *configtypes.DirectionChain {
	hops := make([]string, 0, len(f.hops))
	for _, h := range f.hops {
		if strings.TrimSpace(h) != "" {
			hops = append(hops, h)
		}
	}
	if len(hops) == 0 {
		return nil
	}
	c := &configtypes.DirectionChain{
		Hops:    hops,
		Rewrite: f.keep.Rewrite,
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

// Content — содержимое вкладки.
func (f *chainForm) Content() fyne.CanvasObject {
	var head []fyne.CanvasObject

	if f.unsupported != "" {
		warn := widget.NewLabel("⚠ " + f.unsupported)
		warn.Wrapping = fyne.TextWrapWord
		warn.Importance = widget.WarningImportance
		head = append(head, warn, widget.NewSeparator())
	}

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

	rewriteNote := widget.NewLabel(locale.T("wizard.chain.rewrite_note"))
	rewriteNote.Importance = widget.LowImportance
	rewriteNote.Wrapping = fyne.TextWrapWord

	advanced := widget.NewAccordion(widget.NewAccordionItem(
		locale.T("wizard.chain.advanced"),
		container.NewVBox(
			chainFormRow(locale.T("wizard.chain.idle_timeout"), f.idleEntry),
			widget.NewSeparator(),
			f.stripEvasion,
			stripRows,
			widget.NewSeparator(),
			rewriteNote,
		),
	))

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
		warn := widget.NewLabel("⚠ " + msg)
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

	// T7: узел, у которого уже есть свой detour. Ядру это не мешает, но
	// путь получается длиннее показанного, и разбираться в нём придётся по
	// логам — предупредить дешевле.
	var detoured []string
	for _, tag := range f.hops {
		if f.detourTags[tag] {
			detoured = append(detoured, tag)
		}
	}
	if len(detoured) > 0 {
		out = append(out, locale.Tf("wizard.chain.conflict_detour", strings.Join(detoured, ", ")))
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
