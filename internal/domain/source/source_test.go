package source

import "testing"

func TestComputeAdaptive(t *testing.T) {
	base := 120
	// 有产出：回落基准、清零连空
	if iv, st := ComputeAdaptive(base, 3, 10); iv != base || st != 0 {
		t.Fatalf("有产出应回落基准 120/0，得到 %d/%d", iv, st)
	}
	// 首次空手：翻倍
	if iv, st := ComputeAdaptive(base, 0, 0); iv != 240 || st != 1 {
		t.Fatalf("首次空手应 240/1，得到 %d/%d", iv, st)
	}
	// 连续空手：指数退避
	if iv, _ := ComputeAdaptive(base, 2, 0); iv != 960 {
		t.Fatalf("连空 2 次应 960，得到 %d", iv)
	}
	// 上限 24h：120 << 5 溢出后截断
	if iv, _ := ComputeAdaptive(base, 10, 0); iv != 1440 {
		t.Fatalf("超长连空应封顶 1440，得到 %d", iv)
	}
}

func TestScriptKind(t *testing.T) {
	if !KindScript.Implemented() {
		t.Fatal("script 应为内置种类（push 直写）")
	}
	if KindXAccount.Implemented() || KindWechatOA.Implemented() {
		t.Fatal("x_account/wechat_oa 应保持扩展点状态")
	}
}
