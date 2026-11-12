// Notifier.app — помощник лаунчера для системных уведомлений на macOS.
//
// Лежит в бандле лаунчера: Contents/Helpers/Notifier.app, bundle id
// com.leadaxe.notifier. Лаунчер запускает его так:
//
//     Notifier.app/Contents/MacOS/notifier <заголовок> <текст>
//
// Зачем отдельное приложение — в комментарии к platform.SendNotification
// (internal/platform/notify_darwin.go): у bundle id самого лаунчера
// usernoted старый центр уведомлений не пускает, а новый не работает с
// ad-hoc подписью.
//
// Код выхода: 0 — уведомление доставлено; 2 — usernoted его не принял (нет в
// deliveredNotifications); 1 — центра уведомлений нет или неверные аргументы.
// При ненулевом коде лаунчер шлёт уведомление через osascript.

#import <Foundation/Foundation.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// Баннер показывать всегда, даже если помощник окажется на переднем плане.
@interface NotifierDelegate : NSObject <NSUserNotificationCenterDelegate>
@end

@implementation NotifierDelegate
- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center
     shouldPresentNotification:(NSUserNotification *)notification {
    return YES;
}
@end

int main(int argc, char *argv[]) {
    @autoreleasepool {
        if (argc != 3) {
            fprintf(stderr, "usage: notifier <title> <body>\n");
            return 1;
        }
        NSUserNotificationCenter *center = [NSUserNotificationCenter defaultUserNotificationCenter];
        if (center == nil) {
            return 1;
        }
        NotifierDelegate *delegate = [NotifierDelegate new];
        center.delegate = delegate;

        NSString *identifier = [[NSUUID UUID] UUIDString];
        NSUserNotification *n = [NSUserNotification new];
        n.identifier = identifier;
        n.title = [NSString stringWithUTF8String:argv[1]];
        n.informativeText = [NSString stringWithUTF8String:argv[2]];
        [center deliverNotification:n];

        // Отказ usernoted приложению не сообщается: уведомление просто не
        // появляется в deliveredNotifications.
        for (int i = 0; i < 15; i++) {
            [[NSRunLoop mainRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
            for (NSUserNotification *d in center.deliveredNotifications) {
                if ([d.identifier isEqualToString:identifier]) {
                    return 0;
                }
            }
        }
        return 2;
    }
}

#pragma clang diagnostic pop
