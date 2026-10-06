package render

import (
	"strings"
	"testing"
)

// TestMDEscapeNeutralizesHTML 外部抓取标题流入日报表格 → 前端 marked+v-html，
// mdEscape 必须中和全部 HTML 触发字符（存储型 XSS 防线）。
func TestMDEscapeNeutralizesHTML(t *testing.T) {
	payloads := []string{
		`<img src=x onerror=alert(1)>`,
		`<script>alert(1)</script>`,
		`"><svg onload=alert(1)>`,
		`<a href="javascript:alert(1)">点我</a>`,
		`正常标题 | 带竖线` + "\n" + `带换行`,
	}
	for _, p := range payloads {
		out := mdEscape(p)
		if strings.ContainsAny(out, "<>") {
			t.Fatalf("转义后仍含原始尖括号: %q -> %q", p, out)
		}
	}
	// 竖线仍需转义（markdown 表格结构），换行压平
	if out := mdEscape("a|b"); !strings.Contains(out, `\|`) {
		t.Fatalf("竖线应转义为 \\|，实际 %q", out)
	}
}
