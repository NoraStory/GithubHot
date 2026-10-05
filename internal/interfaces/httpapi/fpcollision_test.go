package httpapi

import (
	"context"
	"testing"
	"time"
)

// ---------- 碰撞检测（纯逻辑） ----------

func TestCollisionConcurrentIPs(t *testing.T) {
	c := newFPCollision()
	now := time.Now()
	// 10 分钟内 4 个不同 IP → 碰撞
	if c.observe("fp1", "1.1.1.1", now) {
		t.Fatal("首个 IP 不应判碰撞")
	}
	if c.observe("fp1", "2.2.2.2", now.Add(1*time.Minute)) {
		t.Fatal("2 个 IP 不应判碰撞")
	}
	if c.observe("fp1", "3.3.3.3", now.Add(2*time.Minute)) {
		t.Fatal("3 个 IP 不应判碰撞")
	}
	if !c.observe("fp1", "4.4.4.4", now.Add(3*time.Minute)) {
		t.Fatal("10 分钟内 4 个不同 IP 应判碰撞")
	}
	// 标记期内即便同一 IP 也继续认定为碰撞
	if !c.observe("fp1", "1.1.1.1", now.Add(4*time.Minute)) {
		t.Fatal("碰撞标记期内应保持认定")
	}
}

func TestSequentialNetworkChangeIsNotCollision(t *testing.T) {
	c := newFPCollision()
	now := time.Now()
	// 同一设备换网络：一个 IP 用一阵再换（串行），窗口内不会出现多个 IP 并发
	for i, ip := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5"} {
		if c.observe("fp2", ip, now.Add(time.Duration(i*30)*time.Minute)) {
			t.Fatalf("串行换网（每 30 分钟一个 IP）不应判碰撞：%s", ip)
		}
	}
}

func TestCollisionMarkExpires(t *testing.T) {
	c := newFPCollision()
	now := time.Now()
	for _, ip := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4"} {
		c.observe("fp3", ip, now)
	}
	if !c.observe("fp3", "5.5.5.5", now.Add(time.Hour)) {
		t.Fatal("TTL 内应保持碰撞认定")
	}
	// 越过 TTL 后重新评估：窗口内只剩一个 IP → 不再判碰撞
	if c.observe("fp3", "5.5.5.5", now.Add(collisionTTL+time.Minute)) {
		t.Fatal("TTL 过期后应重新评估，单个 IP 不判碰撞")
	}
}

func TestCollisionEmptyInput(t *testing.T) {
	c := newFPCollision()
	now := time.Now()
	if c.observe("", "1.1.1.1", now) || c.observe("fp", "", now) {
		t.Fatal("空 fp / 空 ip 不应判碰撞")
	}
}

// ---------- 碰撞豁免（引擎集成） ----------

// TestCollisionFingerprintSkipsLinkage 同一指纹被多个 IP 并发使用（同型号设备指纹重合）
// 时，不得因连坐/漂移伤及无关用户：应记 fp-collision-watch 且不封禁。
func TestCollisionFingerprintSkipsLinkage(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	t.Setenv("TRUSTED_PROXY", "127.0.0.1/32")
	g := NewIPGuard(store)
	ctx := context.Background()
	const fp = "collideCollideCollide1"

	for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4"} {
		out, _ := g.ReportFingerprint(ctx, ip, "Mozilla/5.0 Chrome/126", fp, FingerprintMeta{})
		if banned, _ := out["banned"].(bool); banned {
			t.Fatalf("%s 被误判连坐封禁", ip)
		}
	}
	watch := 0
	for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4"} {
		watch += store.kindsOf(ip)["fp-collision-watch"]
	}
	if watch == 0 {
		t.Fatal("应记录 fp-collision-watch 事件")
	}
	for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4"} {
		if ban := store.banOf(ip); ban != nil {
			t.Fatalf("%s 不应被封：%s", ip, ban.Reason)
		}
	}
}

// ---------- 管理端爆破：非 severe → 初犯档而非 7 天 ----------

func TestAdminBruteBansAtFirstStrike(t *testing.T) {
	store := newFakeStore()
	t.Setenv("IP_GUARD_ENABLED", "1")
	g := NewIPGuard(store)

	g.Event(context.Background(), "198.51.100.9", "admin-brute", "管理端密码爆破锁定", 100, false)

	ban := store.banOf("198.51.100.9")
	if ban == nil {
		t.Fatal("管理端爆破应封禁该 IP")
	}
	if ban.Strikes != 1 {
		t.Fatalf("应为初犯档（strikes=1），实际 %d", ban.Strikes)
	}
	if d := time.Until(ban.ExpiresAt); d > 31*time.Minute {
		t.Fatalf("初犯档应为 30 分钟，实际 %v", d.Round(time.Minute))
	}
}

// ---------- components 清洗 ----------

func TestSanitizeComponents(t *testing.T) {
	if got := sanitizeComponents(nil); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %v", got)
	}
	long := make(map[string]string)
	for i := 0; i < 20; i++ {
		long[string(rune('a'+i))] = "v"
	}
	if got := sanitizeComponents(long); len(got) != 16 {
		t.Fatalf("键数应截断到 16，实际 %d", len(got))
	}
	in := map[string]string{"canvas": "abc", "": "no-key", "big": string(make([]byte, 200))}
	got := sanitizeComponents(in)
	if _, ok := got[""]; ok {
		t.Fatal("空键应被丢弃")
	}
	if _, ok := got["big"]; ok {
		t.Fatal("超长值应被丢弃")
	}
	if got["canvas"] != "abc" {
		t.Fatal("合法键值应保留")
	}
}
