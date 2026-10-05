//go:build !windows

package httpapi

import (
	"os"
	"strconv"
	"strings"
)

// Unix 平台进程信号：/proc/self/stat（CPU）+ /proc/self/status（RSS）。
// 仅覆盖 Linux（部署目标）；其他 Unix 优雅降级（返回 false，前端显示不可用）。

// clockTicks 内核用户态时钟频率（绝大多数 Linux 为 100，见 sysconf(_SC_CLK_TCK)）。
const clockTicks = 100.0

func processCPUSeconds() (float64, bool) {
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, false
	}
	s := string(b)
	// comm 字段可能含空格与括号：取最后一个 ')' 之后的字段
	i := strings.LastIndex(s, ")")
	if i < 0 {
		return 0, false
	}
	fields := strings.Fields(s[i+1:])
	// 此后 fields[0] = state（原字段 3），原字段 14/15（utime/stime）= fields[11]/[12]
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseFloat(fields[11], 64)
	stime, err2 := strconv.ParseFloat(fields[12], 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return (utime + stime) / clockTicks, true
}

func processRSSBytes() (uint64, bool) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, false
			}
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, false
			}
			return kb * 1024, true
		}
	}
	return 0, false
}
