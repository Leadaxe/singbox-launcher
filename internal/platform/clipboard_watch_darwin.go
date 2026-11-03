//go:build darwin

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

static long launcherPasteboardChangeCount(void) {
	@autoreleasepool {
		return (long)[[NSPasteboard generalPasteboard] changeCount];
	}
}

// Пометка чувствительного содержимого по nspasteboard.org: её ставят
// менеджеры паролей (1Password, Bitwarden, KeePassXC) и Связка ключей.
static int launcherPasteboardConcealed(void) {
	@autoreleasepool {
		NSArray<NSPasteboardType> *types = [[NSPasteboard generalPasteboard] types];
		return [types containsObject:@"org.nspasteboard.ConcealedType"] ||
			[types containsObject:@"org.nspasteboard.TransientType"] ? 1 : 0;
	}
}
*/
import "C"

// ClipboardChangeCount — счётчик изменений системного буфера (растёт на
// каждое копирование, даже того же текста). ok=false — платформа не умеет.
func ClipboardChangeCount() (uint64, bool) {
	return uint64(C.launcherPasteboardChangeCount()), true
}

// ClipboardConcealed — содержимое буфера помечено чувствительным
// (пароль из менеджера паролей): такое никуда не отправляется.
func ClipboardConcealed() bool {
	return C.launcherPasteboardConcealed() != 0
}
