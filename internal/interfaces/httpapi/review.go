// §10.2 Review 队列端点 + §10.5 ML 诊断端点（规格书 §10.2/§10.5）。
package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/behavior"
)

// reviewQueueAPI GET /api/v1/admin/review/queue?status=pending。
func (s *Server) reviewQueueAPI(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.Guard.store.ListReviewItems(r.Context(), status, limit)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	type ReviewItemEx struct {
		ID        int64              `json:"id"`
		Fp        string             `json:"fp"`
		Reasons   []string           `json:"reason_tags"`
		Scores    map[string]float64 `json:"scores"`
		Status    string             `json:"status"`
		CreatedAt string             `json:"triggered_at"`
		IpSample  string             `json:"ip_sample"`
		Ua        string             `json:"ua"`
	}
	rr := s.Guard.store
	out := make([]ReviewItemEx, 0, len(items))
	for _, item := range items {
		ex := ReviewItemEx{ID: item.ID, Fp: item.FP, Reasons: item.Reasons,
			Scores: item.Scores, Status: item.Status, CreatedAt: item.CreatedAt.Format(time.RFC3339)}
		if row, err := rr.FindFingerprint(r.Context(), item.FP); err == nil && row != nil {
			ex.IpSample = firstIP(row.IPs)
			ex.Ua = row.UA
		}
		out = append(out, ex)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

// reviewResolveAPI POST /api/v1/admin/review/resolve {ids, action}。
func (s *Server) reviewResolveAPI(w http.ResponseWriter, r *http.Request) {
	var p struct {
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&p); err != nil || len(p.IDs) == 0 {
		writeErr(w, 400, errorString("请求体需要 {ids: [...], action}"))
		return
	}
	switch p.Action {
	case "done", "ignored":
	default:
		writeErr(w, 400, errorString("action 须为 done|ignored"))
		return
	}
	if err := s.Guard.store.ResolveReviewItems(r.Context(), p.IDs, p.Action, "admin"); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "resolved": len(p.IDs)})
}

// mlDiagnosticsAPI GET /api/v1/admin/ml/diagnostics（§10.5 ML 诊断页数据源）。
func (s *Server) mlDiagnosticsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r.Context())
	defer cancel()

	// 模型健康状态（C2修复：从加载器直接获取，包含失败信息）
	modelHealth := map[string]interface{}{"enabled": false}
	if s.Guard != nil && s.Guard.mlEngine != nil {
		modelHealth = s.Guard.MLModelHealthStatus()
	}

	// PSI 简化：近 7 天行为特征变异系数
	psi := []map[string]any{}
	if s.Guard != nil && s.Guard.store != nil {
		rows, err := s.Guard.store.ListFingerprintsSince(ctx, time.Now().Add(-7*24*time.Hour), 5000)
		if err == nil && len(rows) >= 10 {
			psi = computePSI(rows)
		}
	}

	// sidecar 状态（含健康检查）
	sidecar := s.SidecarHealthStatus()

	writeJSON(w, 200, map[string]any{
		"model_health": modelHealth,
		"psi":          psi,
		"sidecar":      sidecar,
		"pipeline": map[string]string{
			"export": "githubhot ml export --out data/ml/behavior.jsonl",
			"train":  "scripts/ml/run_train.ps1",
			"check":  "githubhot ml check",
		},
	})
}

// computePSI 特征分布漂移度（简化 CV = σ/μ，全量 PSI 需训练基准分布文件）。
func computePSI(rows []FingerprintDTO) []map[string]any {
	dwellMeans, speedVars, curvatures, entropies := []float64{}, []float64{}, []float64{}, []float64{}
	for _, r := range rows {
		f := behavior.Parse(r.BehaviorJSON)
		if f == nil {
			continue
		}
		if f.Keys.Events >= 10 {
			dwellMeans = append(dwellMeans, f.Keys.DwellMean)
		}
		if f.Mouse.Events >= 20 {
			speedVars = append(speedVars, f.Mouse.SpeedVar)
			curvatures = append(curvatures, f.Mouse.CurvatureMean)
		}
		if r.EntropyBits > 0 {
			entropies = append(entropies, r.EntropyBits)
		}
	}
	feats := []struct {
		name string
		vals []float64
	}{
		{"dwell_mean", dwellMeans}, {"speed_var", speedVars},
		{"curvature_mean", curvatures}, {"entropy_bits", entropies},
	}
	out := []map[string]any{}
	for _, feat := range feats {
		if len(feat.vals) < 5 {
			continue
		}
		mean := 0.0
		for _, v := range feat.vals {
			mean += v
		}
		mean /= float64(len(feat.vals))
		std := 0.0
		for _, v := range feat.vals {
			std += (v - mean) * (v - mean)
		}
		std = math.Sqrt(std / float64(len(feat.vals)))
		if mean == 0 {
			mean = 0.001
		}
		cv := math.Abs(std / mean)
		level := "green"
		if cv > 0.2 {
			level = "red"
		} else if cv > 0.1 {
			level = "yellow"
		}
		out = append(out, map[string]any{
			"feature": feat.name, "psi": math.Round(cv*1000) / 1000,
			"level": level, "n": len(feat.vals),
		})
	}
	return out
}
