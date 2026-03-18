//go:build windows

package app

func detectFDLimit() uint64 {
	return 1024
}
