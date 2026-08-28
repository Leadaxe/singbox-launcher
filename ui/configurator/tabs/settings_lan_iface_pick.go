package tabs

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/netiface"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// LAN-сторона поля tun.include_interface: список кандидатов и правила его
// стыковки с многострочным полем.
//
// Поле остаётся источником правды и принимает ручной ввод: интерфейс, в который
// воткнутся ЗАВТРА (lan2 нового роутера), в системе ещё не существует, и
// запретить его вписать значило бы отнять единственный способ настроить машину
// заранее. Кнопка «+» — только удобство поверх того же поля.

// lanIfaceVar — переменная шаблона, несущая tun.include_interface.
//
// Спецобработка по ИМЕНИ, а не по типу: тип у переменной общий (text_list), и
// вешать пикер интерфейсов на весь тип значило бы предлагать интерфейсы всюду,
// где шаблон объявит список строк — от списка доменов до списка портов.
const lanIfaceVar = "gateway_include_interface"

// lanIfacePickApplies — эту строку рисуем полем со списком LAN-кандидатов.
func lanIfacePickApplies(varName string) bool {
	return strings.TrimSpace(varName) == lanIfaceVar
}

// parseLANIfaceList разбирает содержимое поля в список имён.
//
// Формат — одно имя на строку: ровно так его читает подстановка шаблона
// (text_list разбивается по переводам строк), и придумывать здесь второй
// разделитель значило бы, что поле и конфиг понимают текст по-разному.
//
// Пустые строки отбрасываются: пользователь оставляет их, пока печатает.
func parseLANIfaceList(text string) []string {
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// appendLANIface дописывает имя в содержимое поля новой строкой.
//
// Возвращает текст целиком, а не разницу: вызывающий кладёт его в Entry одним
// SetText, и промежуточных состояний у поля не возникает.
//
// Уже присутствующее имя не дублируется — кнопка «+» и так не предлагает его,
// но текст поля правится и руками между двумя нажатиями.
func appendLANIface(text, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return text
	}
	for _, have := range parseLANIfaceList(text) {
		if strings.EqualFold(have, name) {
			return text
		}
	}
	if strings.TrimSpace(text) == "" {
		return name
	}
	// Хвостовой перевод строки не удваиваем: пользователь мог оставить его,
	// нажав Enter перед кликом по «+».
	if strings.HasSuffix(text, "\n") {
		return text + name
	}
	return text + "\n" + name
}

// lanIfaceCandidates — что предложить в выборе: имена LAN-кандидатов машины,
// уже вписанные исключены.
//
// Исключение обязательно: список выбора, предлагающий то, что уже в поле, —
// это либо дубликат в конфиге, либо клик впустую.
//
// hints — расшифровка «имя → подпись» для показа в списке. Само имя уезжает в
// поле дословно, подпись только помогает узнать порт.
//
// pending=true означает «про удалённую машину ещё спрашиваем»: список пуст не
// потому, что кандидатов нет.
func lanIfaceCandidates(model *wizardmodels.WizardModel, already []string) (names []string, hints map[string]string, pending bool) {
	hints = map[string]string{}
	have := make(map[string]struct{}, len(already))
	for _, a := range already {
		have[strings.ToLower(strings.TrimSpace(a))] = struct{}{}
	}

	var list []netiface.Iface
	if model != nil && model.Target.Normalized().IsRemote() {
		// Тот же кэш и тот же single-flight, что у поля аплинка: ответ демона
		// один на машину, различаются только фильтры поверх него.
		machineID := model.Target.Normalized().MachineIDOrEmpty()
		list, _ = cachedRemoteLANCandidates(machineID)
		pending = !remoteInterfacesSettled(machineID)
		ensureRemoteInterfaces(machineID)
	} else {
		list = netiface.ListLANCandidatesOrEmpty()
	}

	for _, ifc := range list {
		if _, dup := have[strings.ToLower(ifc.Name)]; dup {
			continue
		}
		names = append(names, ifc.Name)
		hints[ifc.Name] = ifc.Label()
	}
	// Порядок источника сохраняем: netiface уже отсортировал поднятые вперёд.
	return names, hints, pending
}

// lanIfacePickLabels — подписи для списка выбора в порядке names.
//
// Отдельный срез, а не карта: у выпадающего меню порядок пунктов свой, и
// восстанавливать его из карты пришлось бы каждому вызывающему.
func lanIfacePickLabels(names []string, hints map[string]string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if h := strings.TrimSpace(hints[n]); h != "" {
			out = append(out, h)
			continue
		}
		out = append(out, n)
	}
	return out
}

// buildLANIfacePickField оборачивает многострочное поле кнопкой «+»,
// открывающей выбор из LAN-кандидатов машины.
//
// Поле передаётся готовым и остаётся источником правды: кнопка только
// дописывает в него строку, а его OnChanged (уже настроенный вызывающим) сам
// кладёт текст в модель. Второго пути записи в переменную не заводим — иначе
// ручная правка и выбор из списка разошлись бы в том, что попадёт в конфиг.
//
// Возвращает и саму кнопку: строке нужно уметь гасить её вместе с полем.
func buildLANIfacePickField(
	gs *wizardpresentation.GUIState,
	model *wizardmodels.WizardModel,
	entry *widget.Entry,
) (fyne.CanvasObject, *ttwidget.Button) {
	var addBtn *ttwidget.Button
	addBtn = ttwidget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		showLANIfacePickMenu(gs, model, entry, addBtn)
	})
	addBtn.Importance = widget.LowImportance
	addBtn.SetToolTip(locale.T("Add an interface from the machine's list."))

	// Кнопка прижата к ВЕРХУ поля: многострочный Entry растёт вниз, и
	// растянутая по его высоте кнопка выглядела бы полосой на всю строку.
	side := container.NewVBox(addBtn)
	field := container.NewBorder(nil, nil, nil, side, entry)

	// Список интерфейсов удалённой машины приезжает в фоне. Подписка на
	// пробуждение здесь не нужна: меню собирается в момент клика — к тому
	// времени кэш либо полон, либо честно скажет, что ответа нет.
	return field, addBtn
}

// showLANIfacePickMenu показывает выбор кандидатов и дописывает выбранное имя
// в поле.
//
// Вызывается ТОЛЬКО из обработчика кнопки, то есть уже с UI-потока: ни сети, ни
// подпроцесса внутри нет — кандидаты берутся из кэша (remote) или из одного
// системного вызова (local).
func showLANIfacePickMenu(
	gs *wizardpresentation.GUIState,
	model *wizardmodels.WizardModel,
	entry *widget.Entry,
	anchor fyne.CanvasObject,
) {
	if gs == nil || gs.Window == nil || entry == nil {
		return
	}
	names, hints, pending := lanIfaceCandidates(model, parseLANIfaceList(entry.Text))
	labels := lanIfacePickLabels(names, hints)

	var items []*fyne.MenuItem
	if len(names) == 0 {
		// Пустой список — рабочее состояние, а не сбой: все кандидаты уже
		// вписаны, машина ещё отвечает, или интерфейса просто нет. Но пустое
		// меню молча ничего не объясняет, поэтому объясняет пункт-заглушка.
		msg := locale.T("No interfaces to add — type the name manually.")
		if pending {
			msg = locale.T("Reading the machine's interfaces…")
		}
		stub := fyne.NewMenuItem(msg, func() {})
		stub.Disabled = true
		items = append(items, stub)
	}
	for i, n := range names {
		name := n // захват на итерацию: пункт зовётся уже после выхода из цикла
		items = append(items, fyne.NewMenuItem(labels[i], func() {
			// Мутация виджета из обработчика меню: Fyne зовёт его с UI-потока,
			// но fyne.Do здесь дешевле рассуждений о том, кто именно вызвал
			// пункт, и безопасен при повторном входе.
			fyne.Do(func() {
				entry.SetText(appendLANIface(entry.Text, name))
			})
		}))
	}

	menu := fyne.NewMenu("", items...)
	pop := widget.NewPopUpMenu(menu, gs.Window.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+anchor.MinSize().Height))
}
