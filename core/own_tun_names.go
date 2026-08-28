package core

import (
	"singbox-launcher/core/config"
	"singbox-launcher/core/netiface"
	"singbox-launcher/internal/debuglog"
)

// refreshOwnTunNames сообщает netiface, как СЕЙЧАС зовётся собственный TUN
// ядра, — по свежесобранному config.json.
//
// SPEC 113-F: чужой системный туннель (wg0, awg1 на роутере) — законный аплинк
// для bind_interface, и netiface его больше не режет. Отличить наш TUN от
// чужого по префиксу имени невозможно: singbox-tun0 и tun0 ловятся одним и тем
// же "tun". Разделяет их только точное имя из конфига — его и передаём.
//
// Зовётся в двух точках: на старте (конфиг уже лежит с прошлого запуска) и
// после каждой пересборки (имя могло смениться). Реже нельзя — пикер
// интерфейсов прячет наш TUN именно по этому реестру, и устаревшее имя
// вернуло бы петлю в выбор.
//
// Ошибка чтения — не повод шуметь: конфига может не быть вовсе (первый
// запуск), и второй признак (собственная подсеть TUN) продолжает работать сам.
func (ac *AppController) refreshOwnTunNames() {
	if ac == nil || ac.FileService == nil {
		return
	}
	names, err := config.TunInterfaceNames(ac.FileService.ConfigPath)
	if err != nil {
		debuglog.DebugLog("netiface: own TUN name unknown: %v", err)
		return
	}
	netiface.SetOwnTunNames(names...)
}
