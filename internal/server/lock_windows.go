//go:build windows

package server

import (
	"golang.org/x/sys/windows"
	"os"
)

func lockData(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	var overlapped windows.Overlapped
	if e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func freeBytes(path string) (uint64, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return 0, e
	}
	var free uint64
	e = windows.GetDiskFreeSpaceEx(p, &free, nil, nil)
	return free, e
}
