//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

const char* showOpenPanel() {
    __block const char* result = NULL;

    // Use dispatch_async with a semaphore to avoid deadlock with systray
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);

    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            // Ensure app is active so the dialog appears
            [NSApp activateIgnoringOtherApps:YES];

            NSOpenPanel* panel = [NSOpenPanel openPanel];
            [panel setCanChooseFiles:NO];
            [panel setCanChooseDirectories:YES];
            [panel setAllowsMultipleSelection:NO];
            [panel setMessage:@"Choose clipboard sync location"];
            [panel setPrompt:@"Select"];
            [panel setLevel:NSFloatingWindowLevel];

            if ([panel runModal] == NSModalResponseOK) {
                NSURL* url = [[panel URLs] firstObject];
                if (url != nil) {
                    result = strdup([[url path] UTF8String]);
                }
            }
        }
        dispatch_semaphore_signal(sem);
    });

    // Wait for the panel to close
    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);

    return result;
}

void freeString(const char* str) {
    free((void*)str);
}
*/
import "C"

// ShowFolderPicker displays a native folder picker dialog
// Returns the selected path or empty string if cancelled
func ShowFolderPicker() string {
	cstr := C.showOpenPanel()
	if cstr == nil {
		return ""
	}
	defer C.freeString(cstr)
	return C.GoString(cstr)
}
