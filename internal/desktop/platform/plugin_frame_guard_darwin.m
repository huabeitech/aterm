#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

@interface ATermFrameMessageGuard : NSObject<WKScriptMessageHandler>
@property(nonatomic,retain) id<WKScriptMessageHandler> forward;
@property(nonatomic) NSUInteger blocked;
@end
@implementation ATermFrameMessageGuard
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!message.frameInfo.isMainFrame) {
        self.blocked += 1;
        if (self.blocked == 1) NSLog(@"aTerm: blocked a subframe native bridge message");
        return;
    }
    [self.forward userContentController:controller didReceiveScriptMessage:message];
}
- (void)dealloc { self.forward=nil; [super dealloc]; }
@end

static WKWebView *ATermFindWebView(NSView *view) {
    if ([view isKindOfClass:WKWebView.class]) return (WKWebView *)view;
    for (NSView *child in view.subviews) {
        WKWebView *found=ATermFindWebView(child); if (found) return found;
    }
    return nil;
}

// Uses public WebKit APIs, without method swizzling or patching Wails. The
// adapter deliberately checks the pinned Wails delegate contract and fails
// closed if a future runtime no longer exposes that handler on its delegate.
bool ATermInstallFrameMessageGuard(void *nativeWindow) {
    NSWindow *window=(NSWindow *)nativeWindow;
    WKWebView *view=ATermFindWebView(window.contentView); if (!view) return false;
    WKUserContentController *controller=view.configuration.userContentController;
    static NSMapTable *guards=nil;
    if (!guards) guards=[[NSMapTable weakToWeakObjectsMapTable] retain];
    if ([guards objectForKey:controller]) return true;
    id delegate=view.navigationDelegate;
    if (![delegate respondsToSelector:@selector(userContentController:didReceiveScriptMessage:)]) return false;
    ATermFrameMessageGuard *guard=[[ATermFrameMessageGuard alloc] init];
    guard.forward=(id<WKScriptMessageHandler>)delegate;
    [controller removeScriptMessageHandlerForName:@"external"];
    [controller addScriptMessageHandler:guard name:@"external"];
    [guards setObject:guard forKey:controller];
    [guard release];
    return true;
}
