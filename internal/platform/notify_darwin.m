//go:build darwin

// Системные уведомления лаунчера на macOS. Зачем своя цепочка вместо
// fyne.App.SendNotification — в комментарии к SendNotification (notify_darwin.go).

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

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
static int notifyNum = 0;

// Устаревший API, но разрешения у системы не спрашивает и работает с
// ad-hoc подписью. NO — центра нет (на будущих macOS может не оказаться).
static BOOL launcherSendLegacy(NSString *title, NSString *body) {
    NSUserNotificationCenter *center = [NSUserNotificationCenter defaultUserNotificationCenter];
    if (center == nil) {
        return NO;
    }
    @synchronized ([LauncherNotificationDelegate class]) {
        if (legacyDelegate == nil) {
            legacyDelegate = [LauncherNotificationDelegate new];
            center.delegate = legacyDelegate;
        }
    }
    NSUserNotification *n = [[NSUserNotification new] autorelease];
    n.title = title;
    n.informativeText = body;
    [center deliverNotification:n];
    return YES;
}

#pragma clang diagnostic pop

static void launcherSendFallback(NSString *title, NSString *body) {
    if (!launcherSendLegacy(title, body)) {
        launcherNotifyScript((char *)[title UTF8String], (char *)[body UTF8String]);
    }
}

// 0 — у процесса нет бандла: оба центра уведомлений без него не работают,
// отправку делает Go (osascript).
int launcherSendNotification(char *cTitle, char *cBody) {
    // Вызывается из горутины: у потока нет своего autorelease pool.
    @autoreleasepool {
    if ([[NSBundle mainBundle] bundleIdentifier] == nil) {
        return 0;
    }
    NSString *title = [NSString stringWithUTF8String:cTitle];
    NSString *body = [NSString stringWithUTF8String:cBody];

    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
    [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert
        completionHandler:^(BOOL granted, NSError *_Nullable error) {
            if (granted) {
                UNMutableNotificationContent *content = [[UNMutableNotificationContent new] autorelease];
                content.title = title;
                content.body = body;
                NSString *identifier;
                @synchronized ([LauncherNotificationDelegate class]) {
                    notifyNum++;
                    identifier = [NSString stringWithFormat:@"launcher-notify-%d", notifyNum];
                }
                UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
                    content:content trigger:nil];
                [center addNotificationRequest:request withCompletionHandler:^(NSError *_Nullable err) {
                    if (err != nil) {
                        launcherSendFallback(title, body);
                    }
                }];
                return;
            }
            // Отказ без ошибки — пользователь выключил уведомления лаунчера
            // в настройках: в обход его выбора ничего не показываем.
            if (error != nil) {
                // UNErrorDomain Code=1 — система не пускает приложение к
                // центру уведомлений (ad-hoc подпись).
                launcherSendFallback(title, body);
            }
        }];
    return 1;
    }
}
