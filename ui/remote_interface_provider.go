package ui

import (
	"errors"
	"time"

	"singbox-launcher/core/netiface"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/lxdclient"
	configuratortabs "singbox-launcher/ui/configurator/tabs"
)

// Подключает конфигуратору источник интерфейсов удалённой машины (поле
// «Outbound network interface» на remote-таргете).
//
// Живёт здесь, а не в пакете tabs: транспорт подключённой машины знает только
// `ui`, а он уже импортирует tabs — прямой вызов замкнул бы цикл. Регистрация
// через init(), потому что провайдер не зависит ни от окна, ни от контроллера:
// явная точка вызова означала бы лишь ещё одно место, где о ней можно забыть.
func init() {
	configuratortabs.SetRemoteInterfaceProvider(remoteInterfaceNames)
}

// interfacePickTimeout — сколько ждём список интерфейсов для ПОДСКАЗКИ в поле.
//
// Радикально меньше общего REST-дедлайна клиента (тридцать секунд): тот —
// запас для рабочих вызовов, где ответ нужен любой ценой. Здесь цена обратная:
// подсказки — удобство, поле и без них рабочее, а пока запрос в полёте, он
// держит single-flight слот машины, и подпись под полем стоит на «идёт
// загрузка». Пять секунд — предел, за которым честнее сказать «проверить
// нечем», чем продолжать ждать (SPEC 113-E M6).
const interfacePickTimeout = 5 * time.Second

// remoteInterfaceNames спрашивает у демона машины её интерфейсы и фильтрует их
// так же, как локальные: демон отдаёт ВСЁ, включая lo и туннели, и прямо
// оговаривает, что отбор — задача UI (см. lxdclient.Client.HostInterfaces).
//
// ok=false во всех случаях «спросить не у кого»: машина не подключена, демон
// старее телеметрии хоста, канал оборван. Для поля это не ошибка — подсказок
// нет, ручной ввод остаётся.
func remoteInterfaceNames(machineID string) ([]string, map[string]string, bool) {
	transport, connected := lxdOverrideTransportForID(machineID)
	if !connected {
		return nil, nil, false
	}
	res, err := transport.HostInterfacesWithin(interfacePickTimeout)
	if err != nil {
		// Старый демон — штатный случай, а не поломка: 404 отличён от обрыва
		// на уровне клиента, и оба здесь значат одно — подсказок не будет.
		if !errors.Is(err, lxdclient.ErrHostUnsupported) {
			debuglog.WarnLog("interface picker: host interfaces for %q: %v", machineID, err)
		}
		return nil, nil, false
	}

	names := make([]string, 0, len(res.Interfaces))
	hints := make(map[string]string, len(res.Interfaces))
	for _, raw := range res.Interfaces {
		ifc, ok := netiface.FromRemote(raw.Name, raw.Up, raw.Addresses)
		if !ok {
			continue
		}
		names = append(names, ifc.Name)
		hints[ifc.Name] = ifc.Label()
	}
	return names, hints, true
}
