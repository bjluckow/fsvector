//go:build !windows

package fsindex

import (
	"io/fs"
	"syscall"
	"time"
)

// createdAt returns the file creation time on Unix systems.
// Falls back to mtime if birth time is unavailable.
func createdAt(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		// Btimespec is available on macOS/BSD; Linux may not populate it
		return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
	}
	return info.ModTime()
}
