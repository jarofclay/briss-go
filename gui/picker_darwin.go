//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#import <Cocoa/Cocoa.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdlib.h>
#include <string.h>

// brissOpenPanel shows the system Open panel for PDFs and folders and waits
// until it closes. It returns the chosen paths, each ended by a NUL byte, in
// one malloc'd buffer of *size bytes, or NULL when nothing was chosen.
static char *brissOpenPanel(int *size) {
	__block char *out = NULL;
	__block int n = 0;
	void (^run)(void) = ^{
		@autoreleasepool {
			NSWindow *window = [NSApp keyWindow];
			NSOpenPanel *panel = [NSOpenPanel openPanel];
			[panel setCanChooseFiles:YES];
			[panel setCanChooseDirectories:YES];
			[panel setAllowsMultipleSelection:YES];
			[panel setResolvesAliases:YES];
			[panel setMessage:@"Choose PDF files, or folders with PDF files"];
			[panel setAllowedContentTypes:@[UTTypePDF]];
			if ([panel runModal] == NSModalResponseOK) {
				NSMutableData *buf = [NSMutableData data];
				for (NSURL *url in [panel URLs]) {
					const char *p = [url fileSystemRepresentation];
					if (p != NULL) {
						[buf appendBytes:p length:strlen(p) + 1];
					}
				}
				if ([buf length] > 0) {
					n = (int)[buf length];
					out = malloc(n);
					memcpy(out, [buf bytes], n);
				}
			}
			[window makeKeyAndOrderFront:nil];
		}
	};
	if ([NSThread isMainThread]) {
		run();
	} else {
		dispatch_sync(dispatch_get_main_queue(), run);
	}
	*size = n;
	return out;
}
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2"
)

// openSystemPicker shows the macOS Open panel. AppKit wants it on the main
// thread, where Fyne runs its callbacks; going through fyne.Do from a new
// goroutine runs it after the tap that asked for it has been handled.
func openSystemPicker(_ fyne.Window, done func([]string, error)) {
	go fyne.Do(func() {
		var size C.int
		buf := C.brissOpenPanel(&size)
		if buf == nil {
			done(nil, nil)
			return
		}
		defer C.free(unsafe.Pointer(buf))
		done(splitNUL(C.GoBytes(unsafe.Pointer(buf), size)), nil)
	})
}
