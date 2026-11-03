//go:build darwin

// Системные уведомления лаунчера на macOS. Зачем свой путь вместо
// fyne.App.SendNotification — в комментарии к SendNotification (notify_darwin.go).

#import <Foundation/Foundation.h>

// Реализована в notify_darwin.go (//export).
extern void launcherNotifyScript(char *title, char *body);

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// Без делегата NSUserNotificationCenter не показывает баннер, пока
// приложение на переднем плане.
@interface LauncherNotificationDelegate : NSObject <NSUserNotificationCenterDelegate>
@end

@implementation LauncherNotificationDelegate
- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center
     shouldPresentNotification:(NSUserNotification *)notification {
    return YES;
}
@end

static LauncherNotificationDelegate *legacyDelegate = nil;

// usernoted отклоняет старый API этого bundle id — дальше сразу osascript.
static volatile int legacyDenied = 0;

// Сколько ждать, пока доставленное уведомление появится в
// deliveredNotifications: отказ usernoted до приложения не доходит, его
// видно только по отсутствию уведомления в этом списке.
static const int64_t deliveryCheckDelay = 1 * NSEC_PER_SEC;

static void launcherScript(NSString *title, NSString *body) {
    launcherNotifyScript((char *)[title UTF8String], (char *)[body UTF8String]);
}

// 0 — центра нет: процесс без бандла (go run) или macOS его убрала;
// отправку делает Go (osascript).
int launcherSendNotification(char *cTitle, char *cBody) {
    // Вызывается из горутины: у потока нет своего autorelease pool.
    @autoreleasepool {
        if ([[NSBundle mainBundle] bundleIdentifier] == nil) {
            return 0;
        }
        NSUserNotificationCenter *center = [NSUserNotificationCenter defaultUserNotificationCenter];
        if (center == nil || legacyDenied) {
            return 0;
        }
        @synchronized ([LauncherNotificationDelegate class]) {
            if (legacyDelegate == nil) {
                legacyDelegate = [LauncherNotificationDelegate new];
                center.delegate = legacyDelegate;
            }
        }
        NSString *title = [NSString stringWithUTF8String:cTitle];
        NSString *body = [NSString stringWithUTF8String:cBody];
        NSString *identifier = [[NSUUID UUID] UUIDString];
        NSUserNotification *n = [[NSUserNotification new] autorelease];
        n.identifier = identifier;
        n.title = title;
        n.informativeText = body;
        [center deliverNotification:n];

        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, deliveryCheckDelay),
            dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
                @autoreleasepool {
                    for (NSUserNotification *d in center.deliveredNotifications) {
                        if ([d.identifier isEqualToString:identifier]) {
                            return;
                        }
                    }
                    legacyDenied = 1;
                    launcherScript(title, body);
                }
            });
        return 1;
    }
}

#pragma clang diagnostic pop
