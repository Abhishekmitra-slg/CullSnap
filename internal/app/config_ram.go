package app

import "github.com/shirou/gopsutil/v3/mem"

func detectRAMMB() int {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return 4096
	}
	return int(vmStat.Total / 1024 / 1024)
}
