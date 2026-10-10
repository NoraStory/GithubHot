package iprisk

import (
	"fmt"
	"testing"
	"time"
)

// now 固定基准时间，便于构造事件年龄。
var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func ev(kind string, score int, ageMin float64) Event {
	return Event{Kind: kind, Score: score, At: now.Add(-time.Duration(ageMin * float64(time.Minute)))}
}

// repeat 构造 n 条、间隔 gapMin 分钟、最近一条在 ageMin 前的事件。
func repeat(kind string, score int, n int, gapMin, ageMin float64) []Event {
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, ev(kind, score, ageMin+float64(i)*gapMin))
	}
	return out
}

// ---------- 1. 单信号不得封禁（结构性防误封的核心） ----------

func TestSingleKindDoesNotBan(t *testing.T) {
	cases := []struct {
		name  string
		event []Event
	}{
		{"单条 rate 60", []Event{ev("rate", 60, 0)}},
		{"单条 headless env-flag 50（旧实现 severe 秒封 7 天）", []Event{ev("env-flag", 50, 0)}},
		{"单条 app-sign-invalid 60", []Event{ev("app-sign-invalid", 60, 0)}},
		{"单条 device-mismatch 80", []Event{ev("device-mismatch", 80, 0)}},
		{"单条指纹连坐 70（指纹碰撞误判场景）", []Event{ev("fp-linked", 70, 0)}},
		{"未知类型 100（归入最弱类，需 1.5× 阈值）", []Event{ev("brand-new-rule", 100, 0)}},
		{"管理端探测 50（旧实现 +50 叠加可致封）", []Event{ev("admin-probe", 50, 0)}},
	}
	for _, c := range cases {
		d := Evaluate(c.event, now, 1)
		if d.Ban {
			t.Fatalf("%s：不应封禁（有效分 %.0f，%d 种类型）", c.name, d.Effective, d.Kinds)
		}
	}
}

func TestRepeatedSameKindBelowThreshold(t *testing.T) {
	// 同一条弱规则反复上报（环境核验 50 分 ×3）：衰减后约 110 < 弱类单类门槛 150
	d := Evaluate(repeat("env-flag", 50, 3, 5, 0), now, 1)
	if d.Ban {
		t.Fatalf("同类型反复上报不应致封（有效分 %.0f，%d 种类型）", d.Effective, d.Kinds)
	}
	if d.Kinds != 1 {
		t.Fatalf("应为 1 种类型，实际 %d", d.Kinds)
	}
}

func TestTwoWeakKindsStillBelowThreshold(t *testing.T) {
	// rate 60（封顶 40）+ env 50（封顶 40）= 80 < 100 → 不封
	d := Evaluate([]Event{ev("rate", 60, 0), ev("env-flag", 50, 0)}, now, 1)
	if d.Ban || d.Kinds != 2 {
		t.Fatalf("两类弱证据合计 %.0f 不应封禁（类型数 %d）", d.Effective, d.Kinds)
	}
}

// ---------- 2. 多证据互证 → 封（保留真实判定能力） ----------

func TestIndependentEvidenceBans(t *testing.T) {
	cases := []struct {
		name   string
		events []Event
	}{
		{"速率+环境核验+机器UA（脚本爬虫画像）",
			[]Event{ev("rate", 60, 0), ev("env-flag", 50, 0), ev("bot-ua", 25, 0)}},
		{"APP 签名无效+速率+机器UA（伪造 APP 客户端）",
			[]Event{ev("app-sign-invalid", 60, 0), ev("rate", 60, 0), ev("bot-ua", 25, 0)}},
		{"令牌漂移+设备不符+环境核验（Cookie 被盗用）",
			[]Event{ev("id-ip-drift", 60, 0), ev("device-mismatch", 80, 0), ev("env-flag", 50, 0)}},
		{"指纹连坐+指纹漂移（代理池轮换，两种强证据）",
			[]Event{ev("fp-linked", 70, 0), ev("fp-churn", 40, 0)}},
	}
	for _, c := range cases {
		d := Evaluate(c.events, now, 1)
		if !d.Ban {
			t.Fatalf("%s：应封禁（有效分 %.0f，%d 种类型）", c.name, d.Effective, d.Kinds)
		}
	}
}

func TestStrongKindSoloAccumulationBans(t *testing.T) {
	// 强证据单类型累积过阈值（设备不符 80 ×2，间隔 5 分钟）→ 单类封禁
	d := Evaluate(repeat("device-mismatch", 80, 2, 5, 0), now, 1)
	if !d.Ban {
		t.Fatalf("强证据累积应可单类封禁（有效分 %.0f）", d.Effective)
	}
}

// ---------- 3. 共享出口稀释（CGNAT / 公司 NAT） ----------

func TestSharedOutletDilutionProtects(t *testing.T) {
	events := repeat("rate", 60, 8, 5, 0) // 近 40 分钟每 5 分钟一条高倍速率违规
	solo := Evaluate(events, now, 1)
	shared := Evaluate(events, now, 20)
	if !solo.Ban {
		t.Fatalf("单人持续高倍速率滥用应封禁（有效分 %.0f）", solo.Effective)
	}
	if shared.Ban {
		t.Fatalf("20 种 UA 的共享出口不应被封（有效分 %.0f，稀释 %.2f）", shared.Effective, shared.Dilution)
	}
	if shared.Dilution != DilutionFloor {
		t.Fatalf("稀释系数应为下限 %.2f，实际 %.2f", DilutionFloor, shared.Dilution)
	}
}

func TestMediumRateSoloDoesNotBan(t *testing.T) {
	// 中等速率（150-450 req/min，真人 UA）持续：低于弱类单类门槛 → 只观察不封
	d := Evaluate(repeat("rate", 30, 8, 5, 0), now, 1)
	if d.Ban {
		t.Fatalf("中等速率持续不应单独致封（有效分 %.0f）", d.Effective)
	}
}

func TestDilutionOnlyAffectsWeakClasses(t *testing.T) {
	shared := Evaluate([]Event{ev("fp-linked", 70, 0), ev("fp-churn", 40, 0)}, now, 50)
	if !shared.Ban {
		t.Fatalf("强证据不应被出口稀释豁免（有效分 %.0f，稀释 %.2f）", shared.Effective, shared.Dilution)
	}
	if shared.Dilution == 1 {
		t.Fatalf("UA=50 应产生稀释系数")
	}
}

func TestDilutionBoundaries(t *testing.T) {
	if got := DefaultParams().dilution(1); got != 1 {
		t.Fatalf("UA=1 不应稀释，得 %.2f", got)
	}
	if got := DefaultParams().dilution(UADiversityFree); got != 1 {
		t.Fatalf("UA=%d 边界不应稀释，得 %.2f", UADiversityFree, got)
	}
	if got := DefaultParams().dilution(6); got < 0.49 || got > 0.51 {
		t.Fatalf("UA=6 应为 3/6=0.5，得 %.2f", got)
	}
	if got := DefaultParams().dilution(1000); got != DilutionFloor {
		t.Fatalf("超大出口应取稀释下限，得 %.2f", got)
	}
}

// ---------- 4. 时间半衰期 ----------

func TestDecayPreventsOldAccumulation(t *testing.T) {
	d := Evaluate([]Event{ev("rate", 60, 0), ev("env-flag", 50, 30)}, now, 1)
	if d.Ban {
		t.Fatalf("久远事件应衰减到不足以封禁（有效分 %.0f）", d.Effective)
	}
	out := Evaluate([]Event{ev("rate", 100, 45)}, now, 1)
	if out.Ban || out.Effective != 0 {
		t.Fatalf("超出统计窗口的事件应被忽略（有效分 %.0f）", out.Effective)
	}
}

func TestHalfLifeValue(t *testing.T) {
	d := Evaluate([]Event{ev("env-flag", 40, HalfLifeMin)}, now, 1)
	if d.Effective < 19.5 || d.Effective > 20.5 {
		t.Fatalf("一个半衰期后应剩 ~50%%，得 %.2f", d.Effective)
	}
}

// ---------- 5. 边界 ----------

func TestZeroScoreEventsIgnored(t *testing.T) {
	d := Evaluate([]Event{ev("id-ip-drift", 0, 0), ev("id-ip-drift", 0, 1)}, now, 1)
	if d.Ban || d.Effective != 0 || d.Kinds != 0 {
		t.Fatalf("0 分事件不应计分（有效分 %.0f，%d 种类型）", d.Effective, d.Kinds)
	}
}

func TestThresholdBoundary(t *testing.T) {
	// rate→40 + env→40 + bot-ua 20 = 100 → 封
	d := Evaluate([]Event{ev("rate", 60, 0), ev("env-flag", 50, 0), ev("bot-ua", 20, 0)}, now, 1)
	if !d.Ban {
		t.Fatalf("恰好达阈值应封禁（有效分 %.1f）", d.Effective)
	}
	// 略低：bot-ua 15 → 95 → 不封
	d2 := Evaluate([]Event{ev("rate", 60, 0), ev("env-flag", 50, 0), ev("bot-ua", 15, 0)}, now, 1)
	if d2.Ban {
		t.Fatalf("未达阈值不应封禁（有效分 %.1f）", d2.Effective)
	}
}

func TestClassOfMapping(t *testing.T) {
	cases := map[string]string{
		"rate": ClassRate, "traffic-attack": ClassRate,
		"scanner": ClassProtocol, "bot-ua": ClassProtocol, "admin-probe": ClassProtocol,
		"env-flag": ClassEnv, "env-flag:headless-ua": ClassEnv, "env-flag:navigator-webdriver": ClassEnv,
		"id-forgery": ClassIdentity, "id-token-stale": ClassIdentity,
		"id-ip-drift": ClassIdentity, "device-mismatch": ClassIdentity, "admin-brute": ClassIdentity,
		"fp-linked": ClassDevice, "fp-churn": ClassDevice, "fp-linked-watch": ClassDevice,
		"app-sign-invalid": ClassApp, "app-nonce-replay": ClassApp, "app-no-fp": ClassApp,
		"未定义类型": ClassProtocol,
	}
	for kind, want := range cases {
		if got := ClassOf(kind); got != want {
			t.Fatalf("ClassOf(%q) = %s，期望 %s", kind, got, want)
		}
	}
}

// ---------- 6. 回归：旧实现的典型误封场景 ----------

// 旧实现：rate 30 每 5 分钟叠加 + scanner 40 + bot-ua 25，很快破 100 → 封整条出口。
func TestRegressionCGNATDoesNotBanAllUsers(t *testing.T) {
	events := append(repeat("rate", 30, 8, 5, 0), ev("scanner", 40, 1), ev("bot-ua", 25, 2))
	d := Evaluate(events, now, 30) // 30 种 UA：典型运营商出口
	if d.Ban {
		t.Fatalf("大型共享出口不应被封（有效分 %.0f，稀释 %.2f）", d.Effective, d.Dilution)
	}
}

// 旧实现：headless-ua 标 severe → 立即封 7 天。
func TestRegressionHeadlessFlagAloneNoBan(t *testing.T) {
	if d := Evaluate([]Event{ev("env-flag", 50, 0)}, now, 1); d.Ban {
		t.Fatalf("单条 headless 环境核验不应封禁（有效分 %.0f）", d.Effective)
	}
}

// 旧实现：密钥轮换后老 Cookie 记 id-forgery 100 severe → 秒封。新实现该类事件按
// 15 分弱计分（引擎侧改判 id-token-stale），永不单独致封。
func TestRegressionKeyRotationStaleToken(t *testing.T) {
	d := Evaluate(repeat("id-token-stale", 15, 3, 5, 0), now, 1)
	if d.Ban {
		t.Fatalf("老 Cookie 重签类事件不应致封（有效分 %.0f）", d.Effective)
	}
}

func ExampleEvaluate() {
	d := Evaluate([]Event{
		{Kind: "rate", Score: 60, At: now},
		{Kind: "bot-ua", Score: 25, At: now},
		{Kind: "env-flag", Score: 50, At: now},
	}, now, 1)
	fmt.Printf("ban=%v effective=%.0f kinds=%d\n", d.Ban, d.Effective, d.Kinds)
	// Output: ban=true effective=105 kinds=3
}

