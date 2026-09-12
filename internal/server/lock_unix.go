//go:build !windows

package server

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockData(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func freeBytes(path string) (uint64, error) {
	var st unix.Statfs_t
	e := unix.Statfs(path, &st)
	return st.Bavail * uint64(st.Bsize), e
}
