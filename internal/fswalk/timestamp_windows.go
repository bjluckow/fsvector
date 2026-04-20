//go:build windows

package fswalk

import (
	"io/fs"
	"syscall"
	"time"
)

func createdAt(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, stat.CreationTime.Nanoseconds())
	}
	return info.ModTime()
}
