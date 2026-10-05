package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"
)

// 资源监测（管理端 /admin/system）：进程 RSS / CPU 累计秒 / Go 运行时 / sidecar 状态。
// CPU 百分比由前端按两次轮询的差值计算（本端只报累计值，跨平台无需定时器）。
// 平台信号采集见 procsig_windows.go / procsig_unix.go。

var procStartedAt = time.Now()

func (s *Server) systemStatsAPI(w http.ResponseWriter, _ *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	resp := map[string]any{
		"pid":            os.Getpid(),
		"num_cpu":        runtime.NumCPU(),
		"go_version":     runtime.Version(),
		"uptime_s":       int(time.Since(procStartedAt).Seconds()),
		"goroutines":     runtime.NumGoroutine(),
		"heap_alloc_mb":  mb(ms.HeapAlloc),
		"heap_sys_mb":    mb(ms.HeapSys),
		"sys_mb":         mb(ms.Sys),
		"num_gc":         ms.NumGC,
		"gc_cpu_percent": ms.GCCPUFraction * 100,
	}
	if secs, ok := processCPUSeconds(); ok {
		resp["cpu_seconds_total"] = round2(secs)
	} else {
		resp["cpu_seconds_total"] = nil
	}
	if b, ok := processRSSBytes(); ok {
		resp["rss_mb"] = mb(b)
	} else {
		resp["rss_mb"] = nil
	}
	resp["sidecar"] = sidecarStatus()
	writeJSON(w, 200, resp)
}

// sidecarStatus 探测 GNN sidecar（GNN_SIDECAR_URL，见规格书 P6-3b）。
// 仅探测服务端 env 指定的本机地址（非用户输入），故不走 safehttp SSRF 白名单。
func sidecarStatus() map[string]any {
	base := os.Getenv("GNN_SIDECAR_URL")
	if base == "" {
		return map[string]any{"enabled": false}
	}
	client := &http.Client{Timeout: 800 * time.Millisecond}
	httpResp, err := client.Get(trimRightSlash(base) + "/healthz")
	if err != nil {
		return map[string]any{"enabled": true, "status": "unreachable", "error": err.Error()}
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return map[string]any{"enabled": true, "status": "unhealthy", "http_status": httpResp.StatusCode}
	}
	var h struct {
		Status       string  `json:"status"`
		ModelVersion string  `json:"model_version"`
		RSSMB        float64 `json:"rss_mb"`
		UptimeS      int     `json:"uptime_s"`
		Scored24h    int     `json:"scored_24h"`
		P99MS        int     `json:"p99_ms"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&h); err != nil {
		return map[string]any{"enabled": true, "status": "unreachable", "error": err.Error()}
	}
	return map[string]any{
		"enabled": true, "status": h.Status, "model_version": h.ModelVersion,
		"rss_mb": h.RSSMB, "uptime_s": h.UptimeS, "scored_24h": h.Scored24h, "p99_ms": h.P99MS,
	}
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func mb(b uint64) float64 { return float64(int64(b)/1024/1024*100) / 100 }

func round2(f float64) float64 { return float64(int64(f*100)) / 100 }
