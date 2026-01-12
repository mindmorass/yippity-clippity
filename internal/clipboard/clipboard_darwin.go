// +build darwin

package clipboard

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <stdlib.h>

// Get the change count from the general pasteboard
int getChangeCount() {
    return (int)[[NSPasteboard generalPasteboard] changeCount];
}

// Read text from the pasteboard
const char* readText() {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
    NSString *text = [pasteboard stringForType:NSPasteboardTypeString];
    if (text == nil) {
        return NULL;
    }
    return strdup([text UTF8String]);
}

// Write text to the pasteboard
int writeText(const char* text) {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
    [pasteboard clearContents];
    NSString *nsText = [NSString stringWithUTF8String:text];
    BOOL success = [pasteboard setString:nsText forType:NSPasteboardTypeString];
    return success ? 1 : 0;
}

// Read image data from the pasteboard (returns PNG data)
void* readImageData(int* length) {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];

    // Try to get image data
    NSData *pngData = [pasteboard dataForType:NSPasteboardTypePNG];
    if (pngData != nil) {
        *length = (int)[pngData length];
        void *buffer = malloc(*length);
        memcpy(buffer, [pngData bytes], *length);
        return buffer;
    }

    // Try TIFF and convert to PNG
    NSData *tiffData = [pasteboard dataForType:NSPasteboardTypeTIFF];
    if (tiffData != nil) {
        NSImage *image = [[NSImage alloc] initWithData:tiffData];
        if (image != nil) {
            NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:[image TIFFRepresentation]];
            NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
            if (png != nil) {
                *length = (int)[png length];
                void *buffer = malloc(*length);
                memcpy(buffer, [png bytes], *length);
                return buffer;
            }
        }
    }

    *length = 0;
    return NULL;
}

// Write image data to the pasteboard (expects PNG data)
int writeImageData(const void* data, int length) {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
    [pasteboard clearContents];

    NSData *imageData = [NSData dataWithBytes:data length:length];
    NSImage *image = [[NSImage alloc] initWithData:imageData];
    if (image == nil) {
        return 0;
    }

    BOOL success = [pasteboard writeObjects:@[image]];
    return success ? 1 : 0;
}

// Check if pasteboard has text
int hasText() {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
    return [pasteboard canReadItemWithDataConformingToTypes:@[NSPasteboardTypeString]] ? 1 : 0;
}

// Check if pasteboard has image
int hasImage() {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
    NSArray *imageTypes = @[NSPasteboardTypePNG, NSPasteboardTypeTIFF];
    return [pasteboard canReadItemWithDataConformingToTypes:imageTypes] ? 1 : 0;
}

// Check for transient/concealed data (password managers)
int hasTransientData() {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
    NSArray *types = [pasteboard types];

    for (NSString *type in types) {
        if ([type containsString:@"org.nspasteboard.TransientType"] ||
            [type containsString:@"org.nspasteboard.ConcealedType"]) {
            return 1;
        }
    }
    return 0;
}

// Free memory allocated by C
void freeMemory(void* ptr) {
    free(ptr);
}
*/
import "C"

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"
	"unsafe"

	"github.com/google/uuid"
)

// GetChangeCount returns the current pasteboard change count
func GetChangeCount() int {
	return int(C.getChangeCount())
}

// ReadText reads text from the clipboard
func ReadText() (string, bool) {
	cstr := C.readText()
	if cstr == nil {
		return "", false
	}
	defer C.freeMemory(unsafe.Pointer(cstr))
	return C.GoString(cstr), true
}

// WriteText writes text to the clipboard
func WriteText(text string) bool {
	cstr := C.CString(text)
	defer C.free(unsafe.Pointer(cstr))
	return C.writeText(cstr) == 1
}

// ReadImageData reads image data (PNG) from the clipboard
func ReadImageData() ([]byte, bool) {
	var length C.int
	ptr := C.readImageData(&length)
	if ptr == nil || length == 0 {
		return nil, false
	}
	defer C.freeMemory(ptr)
	return C.GoBytes(ptr, length), true
}

// WriteImageData writes image data (PNG) to the clipboard
func WriteImageData(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return C.writeImageData(unsafe.Pointer(&data[0]), C.int(len(data))) == 1
}

// HasText returns true if clipboard contains text
func HasText() bool {
	return C.hasText() == 1
}

// HasImage returns true if clipboard contains an image
func HasImage() bool {
	return C.hasImage() == 1
}

// HasTransientData returns true if clipboard contains transient/concealed data
// This indicates data from password managers that shouldn't be synced
func HasTransientData() bool {
	return C.hasTransientData() == 1
}

// Read reads the current clipboard content
func Read() (*Content, error) {
	// Skip transient data (password managers)
	if HasTransientData() {
		return nil, nil
	}

	hostname, _ := os.Hostname()
	username := os.Getenv("USER")

	// Check for image first (higher priority)
	if HasImage() {
		data, ok := ReadImageData()
		if ok && len(data) > 0 {
			checksum := sha256.Sum256(data)
			return &Content{
				ID:            uuid.New().String(),
				Timestamp:     time.Now().UTC(),
				SourceMachine: hostname,
				SourceUser:    username,
				ContentType:   ContentTypeImage,
				MimeType:      "image/png",
				Checksum:      hex.EncodeToString(checksum[:]),
				Size:          int64(len(data)),
				Data:          data,
			}, nil
		}
	}

	// Check for text
	if HasText() {
		text, ok := ReadText()
		if ok && len(text) > 0 {
			data := []byte(text)
			checksum := sha256.Sum256(data)
			return &Content{
				ID:            uuid.New().String(),
				Timestamp:     time.Now().UTC(),
				SourceMachine: hostname,
				SourceUser:    username,
				ContentType:   ContentTypeText,
				MimeType:      "text/plain",
				Checksum:      hex.EncodeToString(checksum[:]),
				Size:          int64(len(data)),
				Data:          data,
			}, nil
		}
	}

	return nil, nil
}

// Write writes content to the clipboard
func Write(content *Content) bool {
	if content == nil {
		return false
	}

	switch content.ContentType {
	case ContentTypeText:
		return WriteText(string(content.Data))
	case ContentTypeImage:
		return WriteImageData(content.Data)
	default:
		return false
	}
}
