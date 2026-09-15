//go:build darwin

// Делегат NSApplication лаунчера. Зачем он и чем опасна замена делегата —
// в комментарии к SetupDockReopenHandler (dock_handler.go).

#import <Cocoa/Cocoa.h>

// Реализованы в dock_handler.go (//export).
extern void callGoDockCallback(void);
extern int launcherQuitRequested(void);

@interface SingboxLauncherAppDelegate : NSObject <NSApplicationDelegate> {
    // Делегат, стоявший до нас, — GLFWApplicationDelegate. Всё, что мы не
    // реализуем сами, уходит ему.
    id _inner;
}
- (instancetype)initWithInner:(id)inner;
- (id)inner;
@end

@implementation SingboxLauncherAppDelegate

- (instancetype)initWithInner:(id)inner {
    self = [super init];
    if (self != nil) {
        _inner = [inner retain];
    }
    return self;
}

- (void)dealloc {
    [_inner release];
    [super dealloc];
}

- (id)inner {
    return _inner;
}

// NSApplication спрашивает respondsToSelector: в setDelegate: и по ответу
// подписывает делегата на свои уведомления (applicationDidChangeScreenParameters:
// и другие). Поэтому методы прежнего делегата объявляются здесь как свои,
// а сам вызов перенаправляет forwardingTargetForSelector:.
- (BOOL)respondsToSelector:(SEL)sel {
    return [super respondsToSelector:sel] || [_inner respondsToSelector:sel];
}

- (id)forwardingTargetForSelector:(SEL)sel {
    if ([_inner respondsToSelector:sel]) {
        return _inner;
    }
    return [super forwardingTargetForSelector:sel];
}

// Клик по иконке в Dock. Fyne сам скрытое окно не показывает (fyne-io/fyne#3845).
- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
    if (!flag) {
        callGoDockCallback();
    }
    return YES;
}

// Cmd+Q, «Завершить» в меню Dock, Apple Event quit, выход из системы,
// перезагрузка и выключение. NSTerminateCancel не возвращаем никогда: для
// выхода из системы это отмена самого выхода. NSTerminateLater держит
// AppKit во вложенном цикле (NSModalPanelRunLoopMode), пока Go не ответит
// launcherReplyToTerminate — Go отвечает по окончании остановки или по
// истечении бюджета.
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    return launcherQuitRequested() ? NSTerminateNow : NSTerminateLater;
}

@end

static SingboxLauncherAppDelegate *launcherDelegate = nil;

// Только с главного потока и после инициализации GLFW: иначе оборачивать
// нечего, а glfwInit потом сам заменит делегат своим.
void launcherInstallAppDelegate(void) {
    if (launcherDelegate != nil) {
        return;
    }
    launcherDelegate = [[SingboxLauncherAppDelegate alloc] initWithInner:[NSApp delegate]];
    [NSApp setDelegate:launcherDelegate];
}

void launcherRemoveAppDelegate(void) {
    if (launcherDelegate == nil) {
        return;
    }
    // После glfw.Terminate() делегата у NSApp уже нет: GLFW снял его сам
    // (_glfwTerminateCocoa) и освободил свой объект. Возвращать его на место
    // можно только пока GLFW жив.
    if ([NSApp delegate] == launcherDelegate) {
        [NSApp setDelegate:[launcherDelegate inner]];
    }
    [launcherDelegate release];
    launcherDelegate = nil;
}

// Ответ на NSTerminateLater. Вызывается из горутины; AppKit принимает его
// только на главном потоке, а главная очередь обслуживается и во вложенном
// цикле ожидания.
void launcherReplyToTerminate(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp replyToApplicationShouldTerminate:YES];
    });
}

// NSApplicationActivationPolicyAccessory: приложение работает без иконки в
// Dock (режим только трея).
void launcherHideDockIcon(void) {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

// NSApplicationActivationPolicyRegular: иконка в Dock и обычное поведение.
void launcherRestoreDockIcon(void) {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
}
