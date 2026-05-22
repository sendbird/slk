//go:build darwin && !cgo

package ui

import "golang.design/x/clipboard"

func platformClipboardReader() clipboardReader {
	return clipboard.Read
}
