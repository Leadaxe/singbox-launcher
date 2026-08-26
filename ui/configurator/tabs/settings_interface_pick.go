package tabs

import (
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/netiface"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// newInterfaceHintLabel — строка-расшифровка под полем интерфейса. Курсив, как
// у прочих пояснений в конфигураторе; текст переписывается на каждый ввод.
func newInterfaceHintLabel() *widget.Label {
	l := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

// RemoteInterfaceProvider отдаёт интерфейсы удалённой машины по её ID.
//
// Хук, а не прямой вызов: перечисление живёт в пакете `ui` (там транспорт
// подключённой машины), а `ui` уже зависит от этого пакета — прямой вызов
// замкнул бы цикл импорта. `ui` подставляет реализацию при старте.
//
// ok=false означает «спросить не у кого» — машина не подключена, демон старый
// (ErrHostUnsupported) или не ответил. Для UI это не ошибка: список подсказок
// просто пуст, а поле остаётся полноценным для ручного ввода.
type RemoteInterfaceProvider func(machineID string) (names []string, hints map[string]string, ok bool)

var (
	remoteIfaceMu       sync.RWMutex
	remoteIfaceProvider RemoteInterfaceProvider
)

// SetRemoteInterfaceProvider устанавливает источник интерфейсов удалённых
// машин. Вызывается один раз из пакета `ui` при инициализации.
func SetRemoteInterfaceProvider(p RemoteInterfaceProvider) {
	remoteIfaceMu.Lock()
	defer remoteIfaceMu.Unlock()
	remoteIfaceProvider = p
}

func remoteInterfaces(machineID string) ([]string, map[string]string, bool) {
	remoteIfaceMu.RLock()
	p := remoteIfaceProvider
	remoteIfaceMu.RUnlock()
	if p == nil || strings.TrimSpace(machineID) == "" {
		return nil, nil, false
	}
	return p(machineID)
}

// interfacePickOptions собирает подсказки для поля выбора аплинка: чистые
// имена интерфейсов для выпадающего списка и карту «имя → расшифровка»
// для строки под полем.
//
// Имена и расшифровки разделены намеренно: SelectEntry подставляет выбранный
// пункт в поле дословно, поэтому в списке не может стоять «en0 — Wi-Fi (…)» —
// эта строка уехала бы в конфиг целиком.
//
// Пустой список подсказок — рабочее состояние, а не сбой: поле остаётся
// пригодным для ручного ввода.
func interfacePickOptions(model *wizardmodels.WizardModel, current string) (names []string, hints map[string]string) {
	hints = map[string]string{}

	target := model.Target.Normalized()
	if target.IsRemote() {
		// Интерфейсы удалённой машины перечисляет её же демон: локальные
		// имена там значат другое железо.
		if remoteNames, remoteHints, ok := remoteInterfaces(target.MachineIDOrEmpty()); ok {
			names = append(names, remoteNames...)
			for k, v := range remoteHints {
				hints[k] = v
			}
		}
		return names, hints
	}

	for _, ifc := range netiface.ListOrEmpty() {
		names = append(names, ifc.Name)
		hints[ifc.Name] = ifc.Label()
	}
	return names, hints
}

// interfaceHintFor — строка под полем, объясняющая текущее значение.
//
// Три разных состояния, потому что и действия у них разные: пусто = штатный
// режим; известное имя = показать, что это за интерфейс; неизвестное =
// предупредить, но не мешать (машина может быть чужой или адаптер вынут).
func interfaceHintFor(model *wizardmodels.WizardModel, current string, hints map[string]string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return locale.T("Traffic follows the system default route.")
	}
	if h, ok := hints[current]; ok {
		return h
	}
	if model.Target.Normalized().IsRemote() {
		// Не сверяем с локальными интерфейсами: имя относится к другой машине.
		return locale.T("Cannot verify: the machine's interfaces are unavailable.")
	}
	if netiface.Exists(current) {
		return "⚠ " + locale.T("no IP address — no traffic will go through it")
	}
	return "⚠ " + locale.T("not found on this machine")
}
