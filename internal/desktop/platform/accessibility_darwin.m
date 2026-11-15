#import <Cocoa/Cocoa.h>
#include <stdint.h>
#include <stdbool.h>

extern void aTermVoiceOverChanged(uintptr_t handle, bool enabled);

@interface ATermVoiceOverObserver : NSObject
@property(nonatomic) uintptr_t callbackHandle;
@end

@implementation ATermVoiceOverObserver
- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
    if ([keyPath isEqualToString:@"voiceOverEnabled"]) {
        aTermVoiceOverChanged(self.callbackHandle, NSWorkspace.sharedWorkspace.isVoiceOverEnabled);
        return;
    }
    [super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
}
@end

bool ATermVoiceOverEnabled(void) {
    return NSWorkspace.sharedWorkspace.isVoiceOverEnabled;
}

// Installed and removed on the AppKit thread. Removal is synchronous, so Go
// may release its callback handle after this observer has been detached.
void *ATermObserveVoiceOver(uintptr_t handle) {
    ATermVoiceOverObserver *observer = [[ATermVoiceOverObserver alloc] init];
    observer.callbackHandle = handle;
    [NSWorkspace.sharedWorkspace addObserver:observer forKeyPath:@"voiceOverEnabled" options:0 context:NULL];
    return observer;
}

void ATermStopObservingVoiceOver(void *value) {
    ATermVoiceOverObserver *observer = (ATermVoiceOverObserver *)value;
    [NSWorkspace.sharedWorkspace removeObserver:observer forKeyPath:@"voiceOverEnabled"];
    [observer release];
}
