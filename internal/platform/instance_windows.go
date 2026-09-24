//go:build windows
// +build windows

package platform

// Метки экземпляра и внешний запрос «закройся» (SPEC 140 §4).
//
// Установщик (Inno Setup) находит лаунчер по мьютексу и просит его закрыться
// событием Quit. Restart Manager и taskkill для этого не годятся: крестик
// прячет окно в трей, а принудительное завершение оставляет ядро сиротой или
// снимает его аварийно — системный прокси остаётся смотреть на мёртвый порт.
// По событию лаунчер выходит сам, через GracefulExit, как Quit в трее.

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"

	"singbox-launcher/internal/debuglog"
)

// Имена объектов — общий контракт с build/installer/singbox-launcher.iss.
const (
	instanceMutexLocal  = `Local\SingboxLauncher.Instance`
	instanceMutexGlobal = `Global\SingboxLauncher.Instance`
	quitEventName       = `Local\SingboxLauncher.Quit`
)

// Явные DACL. С DACL по умолчанию объект доступен только создателю и
// SYSTEM, а установщик работает повышенным и, бывает, под другой учётной
// записью администратора: CheckForMutexes (OpenMutex с SYNCHRONIZE) не
// увидел бы мьютекс, а OpenEvent не открыл бы событие. Мьютексу хватает
// SYNCHRONIZE для всех — только обнаружение, в том числе Global\ из чужого
// сеанса. Событие меняют владелец, SYSTEM и администраторы; Local\ и так
// виден только в своём сеансе.
const (
	instanceMutexSDDL = "D:(A;;0x1F0001;;;SY)(A;;0x1F0001;;;BA)(A;;0x1F0001;;;OW)(A;;0x100000;;;WD)"
	quitEventSDDL     = "D:(A;;0x1F0003;;;SY)(A;;0x1F0003;;;BA)(A;;0x1F0003;;;OW)"
)

// instanceHandles держат объекты живыми до конца процесса: закрывает их
// система при выходе, и мьютекс исчезает ровно тогда, когда процесса нет.
var instanceHandles []windows.Handle

// RegisterInstance создаёт мьютексы экземпляра и событие Quit и ждёт его в
// горутине; по сигналу вызывает onQuit (один раз). Вызывать только в
// GUI-режиме: служебные запуски (-paths, -purge-data, -gl-probe*) не
// должны выглядеть для установщика живым лаунчером.
//
// Ни одна ошибка не роняет старт: без мьютекса установщик не найдёт
// лаунчер и закроет его страховкой Restart Manager, без события — спросит
// пользователя.
func RegisterInstance(onQuit func()) {
	createInstanceMutex(instanceMutexLocal)
	// Global\ нужен установщику, чтобы увидеть экземпляр в чужом сеансе.
	// Отказ здесь — не ошибка (SPEC 140 §4).
	createInstanceMutex(instanceMutexGlobal)

	ev, err := createNamedObject(quitEventName, quitEventSDDL, func(sa *windows.SecurityAttributes, name *uint16) (windows.Handle, error) {
		// Ручной сброс: SetEvent будит все экземпляры сеанса, а не один.
		return windows.CreateEvent(sa, 1, 0, name)
	})
	if err != nil {
		debuglog.WarnLog("instance: cannot create %s: %v — the installer will not be able to close the launcher gracefully", quitEventName, err)
		return
	}
	go func() {
		if _, err := windows.WaitForSingleObject(ev, windows.INFINITE); err != nil {
			debuglog.WarnLog("instance: waiting for %s failed: %v", quitEventName, err)
			return
		}
		debuglog.WarnLog("instance: quit requested via %s (installer) — exiting", quitEventName)
		onQuit()
	}()
	debuglog.DebugLog("instance: registered %s, %s, %s", instanceMutexLocal, instanceMutexGlobal, quitEventName)
}

func createInstanceMutex(name string) {
	_, err := createNamedObject(name, instanceMutexSDDL, func(sa *windows.SecurityAttributes, n *uint16) (windows.Handle, error) {
		return windows.CreateMutex(sa, false, n)
	})
	if err != nil {
		debuglog.InfoLog("instance: cannot create %s: %v", name, err)
	}
}

// createNamedObject создаёт (или открывает уже существующий, если лаунчер
// запущен второй раз) именованный объект с DACL из sddl и запоминает хендл.
func createNamedObject(name, sddl string, create func(*windows.SecurityAttributes, *uint16) (windows.Handle, error)) (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	sa := &windows.SecurityAttributes{}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		// Без своего DACL объект всё равно полезен создателю — с DACL по
		// умолчанию.
		debuglog.WarnLog("instance: security descriptor for %s: %v", name, err)
	} else {
		sa.SecurityDescriptor = sd
	}
	h, err := create(sa, namePtr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		// Второй экземпляр: объект общий, хендл валиден.
		err = nil
	}
	if err != nil {
		return 0, err
	}
	instanceHandles = append(instanceHandles, h)
	return h, nil
}
