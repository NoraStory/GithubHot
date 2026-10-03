package httpapi

import (
	"context"
	"net/http"
)

// ProbeRow 一条探针记录（跨层 DTO：cli 适配器从 sqlite 行转换而来）。
type ProbeRow struct {
	Target    string `json:"target"`
	Kind      string `json:"kind"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latencyMs"`
	Detail    string `json:"detail"`
	CheckedAt string `json:"checkedAt"`
}

// ProbeReader 探针结果读取 + 手动触发（cli 层装配探针服务）。
type ProbeReader interface {
	LatestProbes(ctx context.Context) ([]ProbeRow, error)
	ProbeHistory(ctx context.Context, target string, limit int) ([]ProbeRow, error)
	RunProbes(ctx context.Context)
}

// probesAPI GET /api/v1/admin/probes：全部目标最新一轮结果；
// 带 ?target= 时返回该目标的历史记录。
func (s *Server) probesAPI(w http.ResponseWriter, r *http.Request) {
	if s.Probes == nil {
		writeErr(w, 503, errorString("探针未启用"))
		return
	}
	if target := r.URL.Query().Get("target"); target != "" {
		rows, err := s.Probes.ProbeHistory(r.Context(), target, 20)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"history": rows})
		return
	}
	rows, err := s.Probes.LatestProbes(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"latest": rows})
}

// probesRunAPI POST /api/v1/admin/probes/run：立即异步跑一轮全量探测。
func (s *Server) probesRunAPI(w http.ResponseWriter, r *http.Request) {
	if s.Probes == nil {
		writeErr(w, 503, errorString("探针未启用"))
		return
	}
	go s.Probes.RunProbes(context.Background())
	writeJSON(w, 200, map[string]any{"ok": true, "started": true})
}
