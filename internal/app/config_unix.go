//go:build !windows

package app

import "syscall"

func detectFDLimit() uint64 {
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return 256
	}
	return rlimit.Cur
}
