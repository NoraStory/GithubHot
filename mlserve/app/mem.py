"""RSS 内存读取：/proc（Linux）→ psapi（Windows）→ 0。不引第三方依赖。"""
import ctypes
import sys


def rss_mb() -> float:
    if sys.platform.startswith("linux"):
        try:
            with open("/proc/self/status", "r") as f:
                for line in f:
                    if line.startswith("VmRSS:"):
                        return int(line.split()[1]) / 1024.0
        except OSError:
            return 0.0
        return 0.0
    if sys.platform == "win32":
        try:
            import ctypes.wintypes as wt

            class PROCESS_MEMORY_COUNTERS(ctypes.Structure):
                _fields_ = [
                    ("cb", wt.DWORD), ("PageFaultCount", wt.DWORD),
                    ("PeakWorkingSetSize", ctypes.c_size_t),
                    ("WorkingSetSize", ctypes.c_size_t),
                    ("QuotaPeakPagedPoolUsage", ctypes.c_size_t),
                    ("QuotaPagedPoolUsage", ctypes.c_size_t),
                    ("QuotaPeakNonPagedPoolUsage", ctypes.c_size_t),
                    ("QuotaNonPagedPoolUsage", ctypes.c_size_t),
                    ("PagefileUsage", ctypes.c_size_t),
                    ("PeakPagefileUsage", ctypes.c_size_t),
                ]

            k32 = ctypes.windll.kernel32
            # 64 位下必须显式声明签名，否则伪句柄被截断成无效句柄（错误 6）
            k32.GetCurrentProcess.restype = ctypes.c_void_p
            k32.GetCurrentProcess.argtypes = []
            try:
                k32.K32GetProcessMemoryInfo.restype = wt.BOOL
                k32.K32GetProcessMemoryInfo.argtypes = [
                    ctypes.c_void_p, ctypes.POINTER(PROCESS_MEMORY_COUNTERS), wt.DWORD]
                fn = k32.K32GetProcessMemoryInfo
            except AttributeError:
                ctypes.windll.psapi.GetProcessMemoryInfo.restype = wt.BOOL
                ctypes.windll.psapi.GetProcessMemoryInfo.argtypes = [
                    ctypes.c_void_p, ctypes.POINTER(PROCESS_MEMORY_COUNTERS), wt.DWORD]
                fn = ctypes.windll.psapi.GetProcessMemoryInfo
            pmc = PROCESS_MEMORY_COUNTERS()
            pmc.cb = ctypes.sizeof(pmc)
            if fn(k32.GetCurrentProcess(), ctypes.byref(pmc), pmc.cb):
                return pmc.WorkingSetSize / (1024.0 * 1024.0)
        except Exception:
            return 0.0
    return 0.0
