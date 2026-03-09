//go:build linux

package fsindex

import (
	"io/fs"
	"time"
)

// Linux does not reliably expose file birth time, fall back to mtime.
func createdAt(info fs.FileInfo) time.Time {
	return info.ModTime()
}
