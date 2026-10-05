package midas

import (
	"testing"
	"time"
)

var base = time.Unix(1791300000, 0)

// TestMidasCoordinatedAttack 规格验收：20 IP × 20 fp 两时间片内互联 → 告警命中。
func TestMidasCoordinatedAttack(t *testing.T) {
	d := New(Options{SliceTTL: 60 * time.Second})
	ips := make([]string, 20)
	fps := make([]string, 20)
	for i := range ips {
		ips[i] = "10.0.0." + itoa(i)
		fps[i] = "fp-attacker-" + itoa(i)
	}
	// 时间片 1（冷启动）：正常流量建基线
	for i := 0; i < 20; i++ {
		d.Observe("normal-ip-"+itoa(i), "fp-normal-"+itoa(i), base)
	}
	// 时间片 2（攻击爆发）：20 IP × 20 fp 全量互联（400 条新边）
	attackTime := base.Add(70 * time.Second)
	maxScore1 := 0.0
	for _, ip := range ips {
		for _, fp := range fps {
			if s := d.Observe(ip, fp, attackTime); s > maxScore1 {
				maxScore1 = s
			}
		}
	}
	if maxScore1 < 3 {
		t.Fatalf("协同攻击应 >3σ 告警，最大 %.2f", maxScore1)
	}
	t.Logf("协同攻击 max score = %.1f", maxScore1)
}

// TestMidasSteadyTraffic 平稳流量：低频正常访问 → 分数远低于告警线。
func TestMidasSteadyTraffic(t *testing.T) {
	d := New(Options{SliceTTL: 60 * time.Second})
	// 时间片 1（冷启动）：正常流量建基线
	t1 := base
	for i := 0; i < 50; i++ {
		d.Observe("user-ip-"+itoa(i%10), "fp-user-"+itoa(i%5), t1)
	}
	t2 := t1.Add(70 * time.Second) // 时间片 2：正常流量继续
	maxScore := 0.0
	for i := 0; i < 50; i++ {
		if s := d.Observe("user-ip-"+itoa(i%10), "fp-user-"+itoa(i%5), t2); s > maxScore {
			maxScore = s
		}
	}
	if maxScore >= 3 {
		t.Fatalf("平稳流量不应触发告警，最大 %.2f", maxScore)
	}
}

// TestMidasRepeatedEdge 重复同边（正常重访）不误报：历史累计计数抬升后基线稳定。
func TestMidasRepeatedEdge(t *testing.T) {
	d := New(Options{SliceTTL: 60 * time.Second})
	t1 := base
	for i := 0; i < 30; i++ {
		d.Observe("user-a", "fp-a", t1) // 冷启动片
	}
	t2 := base.Add(70 * time.Second)
	maxScore := 0.0
	for i := 0; i < 30; i++ {
		sc := d.Observe("user-a", "fp-a", t2)
		if i < 3 || sc > 3 {
			cur, tot := d.DebugBin(0, d.DebugHash("user-a", "fp-a", 0))
			ct, ht := d.DebugSliceState()
			t.Logf("obs %d: score=%.1f bin cur=%d total=%d slice(ct=%d,hist=%d)", i, sc, cur, tot, ct, ht)
		}
		if sc > maxScore {
			maxScore = sc
		}
	}
	if maxScore >= 3 {
		t.Fatalf("同边重访不应告警，最大 %.2f", maxScore)
	}
}

// TestMidasSliceRoll 时间片滚动：cur 清零、total 累积。
func TestMidasSliceRoll(t *testing.T) {
	d := New(Options{SliceTTL: 60 * time.Second})
	d.Observe("a", "b", base)
	if d.CurTotal() != 1 {
		t.Fatalf("片内总数应为 1")
	}
	d.Observe("a", "b", base.Add(70*time.Second)) // 触发滚动
	if d.CurTotal() != 1 {
		t.Fatalf("滚动后 cur 应清零重计为 1，实际 %d", d.CurTotal())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
