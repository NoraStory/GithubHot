// P5 标注体系（规格书 §8 P5-1）：
//   POST /api/v1/admin/fp/label  —— 管理端金标签（source=admin, confidence=1.0）
//   弱标签每日 cron 生成（RefreshRuleLabels）：封禁史 → bot(0.7)、灰度期命中 ≥2 项
//   fpb_*/botd_* → bot(0.6)、近 30 天零 flag 且有行为数据 → human(0.8)、其余 uncertain。
package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/behavior"
)

// fpLabelAPI POST /api/v1/admin/fp/label {fp, label, notes}（管理端守卫组内）。
func (s *Server) fpLabelAPI(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Fp    string `json:"fp"`
		Label string `json:"label"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&p); err != nil {
		writeErr(w, 400, errorString("请求体非法"))
		return
	}
	p.Fp = strings.TrimSpace(p.Fp)
	if len(p.Fp) < 16 || len(p.Fp) > 128 {
		writeErr(w, 400, errorString("fp 长度非法"))
		return
	}
	switch p.Label {
	case "human", "bot", "uncertain":
	default:
		writeErr(w, 400, errorString("label 须为 human|bot|uncertain"))
		return
	}
	if err := s.Guard.Store().UpsertFpLabel(r.Context(), p.Fp, p.Label, "admin", 1.0, p.Notes); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// RefreshRuleLabels 每日 cron（P5-1 弱标签）：先清 source=rule 再重建。
// 规则（规格原文）：曾入三层封禁记录 → bot(0.7)；P1 灰度期命中 ≥2 项 fpb_*/botd_* →
// bot(0.6)；近 30 天零 flag 且有行为数据 → human(0.8)；其余 uncertain（不写）。
func (g *IPGuard) RefreshRuleLabels(ctx context.Context) error {
	if g.store == nil {
		return nil
	}
	if err := g.store.DeleteFpRuleLabels(ctx); err != nil {
		return err
	}
	since := time.Now().Add(-30 * 24 * time.Hour)
	rows, err := g.store.ListFingerprintsSince(ctx, since, entropyMaxRows)
	if err != nil {
		return err
	}
	botN, humanN := 0, 0
	for _, row := range rows {
		label, conf := "", 0.0
		// ① 曾入三层封禁记录 → bot(0.7)
		if score, err := g.store.RecentIPEventsScore(ctx, firstIP(row.IPs), 30*24*3600); err == nil && score > 0 {
			if hasBanEvent(ctx, g, row.IPs) {
				label, conf = "bot", 0.7
			}
		}
		// ② 灰度期命中 ≥2 项 fpb_*/botd_* → bot(0.6)
		if label == "" {
			hits := 0
			for _, f := range row.Flags {
				if strings.HasPrefix(f, "fpb_") || strings.HasPrefix(f, "botd_") {
					hits++
				}
			}
			if hits >= 2 {
				label, conf = "bot", 0.6
			}
		}
		// ③ 近 30 天零 flag 且有行为数据 → human(0.8)
		if label == "" && len(row.Flags) == 0 && behavior.Parse(row.BehaviorJSON) != nil {
			label, conf = "human", 0.8
		}
		if label == "" {
			continue // uncertain 不写表（默认态）
		}
		if err := g.store.UpsertFpLabel(ctx, row.Fingerprint, label, "rule", conf,
			"每日弱标签 cron"); err != nil {
			return err
		}
		if label == "bot" {
			botN++
		} else {
			humanN++
		}
	}
	log.Printf("[ipguard] 弱标签刷新：bot %d / human %d", botN, humanN)
	return nil
}

func firstIP(ips []string) string {
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}

// hasBanEvent 任一关联 IP 近期有封禁记录。
func hasBanEvent(ctx context.Context, g *IPGuard, ips []string) bool {
	for _, ip := range ips {
		if b, err := g.store.FindBan(ctx, ip); err == nil && b != nil {
			return true
		}
	}
	return false
}
