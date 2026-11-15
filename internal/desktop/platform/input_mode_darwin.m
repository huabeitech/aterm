#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static NSMapTable *ATermInputModes;
static id ATermInputMonitor;

static BOOL ATermIsWebResponder(NSResponder *responder) {
    if (![responder isKindOfClass:NSView.class]) return NO;
    for (NSView *view = (NSView *)responder; view != nil; view = view.superview) {
        if ([view isKindOfClass:WKWebView.class]) return YES;
    }
    return NO;
}

static SEL ATermEditAction(NSEvent *event) {
    NSEventModifierFlags flags = event.modifierFlags & NSEventModifierFlagDeviceIndependentFlagsMask;
    if (!(flags & NSEventModifierFlagCommand) || (flags & (NSEventModifierFlagControl | NSEventModifierFlagOption))) return NULL;
    NSString *key = event.charactersIgnoringModifiers.lowercaseString;
    BOOL shift = (flags & NSEventModifierFlagShift) != 0;
    if ([key isEqualToString:@"z"]) return shift ? @selector(redo:) : @selector(undo:);
    if ([key isEqualToString:@"v"]) return shift ? @selector(pasteAsPlainText:) : @selector(paste:);
    if (shift) return NULL;
    if ([key isEqualToString:@"a"]) return @selector(selectAll:);
    if ([key isEqualToString:@"c"]) return @selector(copy:);
    if ([key isEqualToString:@"x"]) return @selector(cut:);
    return NULL;
}

// NSEvent's local monitor runs before menu key-equivalent dispatch. Forward
// specialized input directly through the existing WebKit responder, preserving
// its text/IME handling while keeping application menus from claiming the key.
// No WebKit subclasses, private APIs or method swizzling are involved.
void ATermSetInputMode(void *value, const char *mode) {
    NSWindow *window = (NSWindow *)value;
    if (window == nil) return;
    if (ATermInputModes == nil) ATermInputModes = [[NSMapTable weakToStrongObjectsMapTable] retain];
    NSString *inputMode = [NSString stringWithUTF8String:mode];
    if ([inputMode isEqualToString:@"normal"]) [ATermInputModes removeObjectForKey:window];
    else [ATermInputModes setObject:inputMode forKey:window];
    if (ATermInputMonitor != nil) return;
    ATermInputMonitor = [[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown handler:^NSEvent *(NSEvent *event) {
        // Only menu-capable modifier combinations need to bypass key-equivalent
        // dispatch. Ordinary characters (including Unicode injection and IME)
        // must retain Cocoa's complete text-services input path.
        if (!(event.modifierFlags & (NSEventModifierFlagCommand | NSEventModifierFlagControl))) return event;
        NSWindow *target = event.window;
        if (target == nil) return event;
        NSString *current = [ATermInputModes objectForKey:target];
        NSResponder *responder = target.firstResponder;
        if (current == nil || !ATermIsWebResponder(responder)) return event;
        BOOL composing = [responder respondsToSelector:@selector(hasMarkedText)] && [(id<NSTextInputClient>)responder hasMarkedText];
        if (!composing && [current isEqualToString:@"editable"]) {
            SEL action = ATermEditAction(event);
            if (action != NULL) {
                [NSApp sendAction:action to:nil from:nil];
                return nil;
            }
        }
        [responder keyDown:event];
        return nil;
    }] retain];
}

void ATermStopInputModes(void) {
    if (ATermInputMonitor != nil) {
        [NSEvent removeMonitor:ATermInputMonitor];
        [ATermInputMonitor release];
        ATermInputMonitor = nil;
    }
    [ATermInputModes release];
    ATermInputModes = nil;
}
