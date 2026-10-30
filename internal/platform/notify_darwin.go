//go:build darwin

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework UserNotifications

#include <stdlib.h>

// Реализация — notify_darwin.m. Здесь только объявления: файл экспортирует
// Go-функцию (//export), и определение в преамбуле попало бы в сборку дважды.
int launcherSendNotification(char *title, char *body);
*/
import "C"

import (
	"os/exec"
	"unsafe"

	"singbox-launcher/internal/debuglog"
)

// SendNotification показывает системное уведомление. true — отправка взята
// на себя; false — платформа своей отправки не имеет, вызывающий шлёт через
// fyne.App.SendNotification.
//
// # Почему не fyne.App.SendNotification
//
// Fyne на macOS просит разрешение у UNUserNotificationCenter, а при ошибке
// сразу уходит в `osascript display notification`. Лаунчер подписан ad-hoc,
// и система отвечает ему ошибкой UNErrorDomain Code=1 всегда: каждое
// уведомление шло через osascript и показывалось от имени «Редактора
// скриптов» с его иконкой, а в настройках уведомлений лаунчера не было.
//
// Здесь между ними стоит NSUserNotificationCenter: устаревший, но разрешения
// не спрашивает, работает с ad-hoc подписью, показывает иконку лаунчера и
// заводит ему запись в настройках уведомлений. Цепочка:
//  1. UNUserNotificationCenter — если система пустила (подпись Developer ID);
//  2. NSUserNotificationCenter — если система ответила ошибкой;
//  3. osascript — если нет и его, или процесс запущен без бандла.
//
// Отказ без ошибки значит, что пользователь выключил уведомления лаунчера в
// настройках, — тогда не показывается ничего.
func SendNotification(title, content string) bool {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	cContent := C.CString(content)
	defer C.free(unsafe.Pointer(cContent))

	if C.launcherSendNotification(cTitle, cContent) == 0 {
		sendNotificationScript(title, content)
	}
	return true
}

//export launcherNotifyScript
func launcherNotifyScript(cTitle, cBody *C.char) {
	sendNotificationScript(C.GoString(cTitle), C.GoString(cBody))
}

// sendNotificationScript — последний рубеж: уведомление от имени osascript.
func sendNotificationScript(title, content string) {
	script := "display notification " + appleScriptStringLiteral(content) +
		" with title " + appleScriptStringLiteral(title)
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Start(); err != nil {
		debuglog.WarnLog("SendNotification: osascript: %v", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}
