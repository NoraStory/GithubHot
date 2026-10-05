//go:build windows

package httpapi

import (
	"syscall"
	"unsafe"
)

// Windows 平台进程信号：GetProcessTimes（CPU）+ K32GetProcessMemoryInfo（RSS）。
// 纯 syscall，零新依赖；伪句柄直接透传 uintptr（syscall 惯例安全）。

var (
	modkernel32        = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentProc = modkernel32.NewProc("GetCurrentProcess")
	procGetProcessTimes = modkernel32.NewProc("GetProcessTimes")
	procK32GetMemInfo  = modkernel32.NewProc("K32GetProcessMemoryInfo")
)

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uint64
	WorkingSetSize             uint64
	QuotaPeakPagedPoolUsage    uint64
	QuotaPagedPoolUsage        uint64
	QuotaPeakNonPagedPoolUsage uint64
	QuotaNonPagedPoolUsage     uint64
	PagefileUsage              uint64
	PeakPagefileUsage          uint64
}

func currentProcess() uintptr {
	h, _, _ := procGetCurrentProc.Call()
	return h
}

// filetimeToSeconds 把 FILETIME（100ns 单位，high<<32|low）转成秒。
func filetimeToSeconds(ft syscall.Filetime) float64 {
	v := int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)
	return float64(v) / 1e7
}

func processCPUSeconds() (float64, bool) {
	var creation, exit, kernel, user syscall.Filetime
	r, _, _ := procGetProcessTimes.Call(
		currentProcess(),
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		return 0, false
	}
	return filetimeToSeconds(kernel) + filetimeToSeconds(user), true
}

func processRSSBytes() (uint64, bool) {
	var pmc processMemoryCounters
	pmc.Cb = uint32(unsafe.Sizeof(pmc))
	r, _, _ := procK32GetMemInfo.Call(
		currentProcess(),
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.Cb),
	)
	if r == 0 {
		return 0, false
	}
	return pmc.WorkingSetSize, true
}
