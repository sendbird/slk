//go:build darwin && cgo

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Cocoa
#import <Foundation/Foundation.h>
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

static unsigned int slk_copy_nsdata(NSData *data, void **out) {
	if (data == nil || [data length] == 0) {
		*out = NULL;
		return 0;
	}
	NSUInteger size = [data length];
	void *buf = malloc(size);
	if (buf == NULL) {
		*out = NULL;
		return 0;
	}
	[data getBytes:buf length:size];
	*out = buf;
	return (unsigned int)size;
}

unsigned int slk_clipboard_read_image_png(void **out) {
	@autoreleasepool {
		*out = NULL;
		NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];

		NSData *png = [pasteboard dataForType:NSPasteboardTypePNG];
		if (png != nil) {
			return slk_copy_nsdata(png, out);
		}

		NSData *tiff = [pasteboard dataForType:NSPasteboardTypeTIFF];
		if (tiff != nil) {
			NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
			NSData *converted = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
			if (converted != nil) {
				return slk_copy_nsdata(converted, out);
			}
		}

		NSImage *image = [[NSImage alloc] initWithPasteboard:pasteboard];
		if (image == nil) {
			return 0;
		}
		CGImageRef cgImage = [image CGImageForProposedRect:NULL context:nil hints:nil];
		if (cgImage == NULL) {
			[image release];
			return 0;
		}
		NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithCGImage:cgImage];
		NSData *converted = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		unsigned int n = slk_copy_nsdata(converted, out);
		[rep release];
		[image release];
		return n;
	}
}
*/
import "C"

import (
	"unsafe"

	"golang.design/x/clipboard"
)

func platformClipboardReader() clipboardReader {
	return func(format clipboard.Format) []byte {
		b := clipboard.Read(format)
		if format != clipboard.FmtImage || len(b) > 0 {
			return b
		}
		return readDarwinClipboardImagePNG()
	}
}

func readDarwinClipboardImagePNG() []byte {
	var data unsafe.Pointer
	n := C.slk_clipboard_read_image_png(&data)
	if data == nil || n == 0 {
		return nil
	}
	defer C.free(data)
	return C.GoBytes(data, C.int(n))
}
