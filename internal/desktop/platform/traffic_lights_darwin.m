#import <Cocoa/Cocoa.h>

@interface SSHConnectTrafficLightLayout : NSObject
@property(nonatomic, assign) NSWindow *window;
@property(nonatomic) CGFloat headerHeight;
- (void)alignButtons;
- (void)windowDidLayout:(NSNotification *)notification;
@end

@implementation SSHConnectTrafficLightLayout

- (void)windowDidLayout:(NSNotification *)notification {
    [self alignButtons];
}

- (void)alignButtons {
    NSWindow *window = self.window;
    NSView *contentView = window.contentView;
    if (window == nil || contentView == nil || window.styleMask & NSWindowStyleMaskFullScreen) {
        return;
    }

    NSButton *closeButton = [window standardWindowButton:NSWindowCloseButton];
    NSButton *miniaturizeButton = [window standardWindowButton:NSWindowMiniaturizeButton];
    NSButton *zoomButton = [window standardWindowButton:NSWindowZoomButton];
    if (closeButton == nil || miniaturizeButton == nil || zoomButton == nil || closeButton.superview == nil) {
        return;
    }

    // The webview is full-size content. Convert the header's desired centre
    // from content coordinates into the native titlebar's coordinate system.
    CGFloat targetCenterY = NSHeight(contentView.bounds) - self.headerHeight / 2.0;
    NSArray<NSButton *> *buttons = @[closeButton, miniaturizeButton, zoomButton];
    for (NSButton *button in buttons) {
        NSRect frameInContent = [button.superview convertRect:button.frame toView:contentView];
        frameInContent.origin.y = targetCenterY - NSHeight(frameInContent) / 2.0;
        NSRect targetFrame = [button.superview convertRect:frameInContent fromView:contentView];
        [button setFrame:targetFrame];
    }
}

@end

static SSHConnectTrafficLightLayout *trafficLightLayout;

static void SSHConnectConfigureTrafficLights(double headerHeight, NSInteger retriesRemaining) {
    NSWindow *window = NSApp.keyWindow ?: NSApp.mainWindow;
    if (window == nil) {
        if (retriesRemaining > 0) {
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.05 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
                SSHConnectConfigureTrafficLights(headerHeight, retriesRemaining - 1);
            });
        }
        return;
    }

    if (trafficLightLayout != nil && trafficLightLayout.window != window) {
        [[NSNotificationCenter defaultCenter] removeObserver:trafficLightLayout];
        trafficLightLayout = nil;
    }
    if (trafficLightLayout == nil) {
        trafficLightLayout = [[SSHConnectTrafficLightLayout alloc] init];
        trafficLightLayout.window = window;
        NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
        [center addObserver:trafficLightLayout selector:@selector(windowDidLayout:) name:NSWindowDidResizeNotification object:window];
        [center addObserver:trafficLightLayout selector:@selector(windowDidLayout:) name:NSWindowDidEnterFullScreenNotification object:window];
        [center addObserver:trafficLightLayout selector:@selector(windowDidLayout:) name:NSWindowDidExitFullScreenNotification object:window];
    }
    trafficLightLayout.headerHeight = headerHeight;
    [trafficLightLayout alignButtons];
}

void SSHConnectAlignTrafficLights(double headerHeight) {
    dispatch_async(dispatch_get_main_queue(), ^{
        SSHConnectConfigureTrafficLights(headerHeight, 10);
    });
}
