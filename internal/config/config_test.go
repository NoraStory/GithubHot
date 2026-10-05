package config

import "testing"

// TestAppSignSeedSources 种子来源：新变量优先，兼容旧名 APP_SESSION_SEED。
func TestAppSignSeedSources(t *testing.T) {
	cases := []struct {
		name, sign, session, want string
	}{
		{"新变量", "new-seed", "", "new-seed"},
		{"仅旧变量", "", "legacy-seed", "legacy-seed"},
		{"新变量优先", "new-seed", "legacy-seed", "new-seed"},
		{"都未配置", "", "", ""},
		{"两边都是空白", "   ", "  ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("APP_SIGN_SEED", c.sign)
			t.Setenv("APP_SESSION_SEED", c.session)
			if got := AppSignSeedFromEnv(); got != c.want {
				t.Fatalf("AppSignSeedFromEnv() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestAppSignSeedGraceFromEnv 过渡期种子列表：逗号分隔、忽略空白项、无配置为空。
func TestAppSignSeedGraceFromEnv(t *testing.T) {
	t.Setenv("APP_SIGN_SEED_GRACE", " gh-dev-seed-v1 , old-2 ,, ")
	got := AppSignSeedGraceFromEnv()
	if len(got) != 2 || got[0] != "gh-dev-seed-v1" || got[1] != "old-2" {
		t.Fatalf("grace = %#v", got)
	}
	t.Setenv("APP_SIGN_SEED_GRACE", "")
	if len(AppSignSeedGraceFromEnv()) != 0 {
		t.Fatalf("空配置应返回空列表")
	}
}

// TestCheckAppSignSeed serve 模式拒绝出厂默认与空种子（公开可复现 = 签名机制形同虚设）。
func TestCheckAppSignSeed(t *testing.T) {
	cases := []struct {
		name    string
		seed    string
		wantErr bool
	}{
		{"未配置", "", true},
		{"出厂默认", DevAppSignSeed, true},
		{"已轮换", "kQ3mZ0p9Xv2s6tW1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{AppSignSeed: c.seed}
			err := cfg.CheckAppSignSeed()
			if c.wantErr && err == nil {
				t.Fatalf("应报错拒绝启动")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			// 警告文案：严格校验与降级提示必须一致（serve 报错 = run/mcp 警告内容）
			warn := cfg.AppSignSeedWarning()
			if c.wantErr && warn == "" {
				t.Fatalf("run/mcp 模式应给出警告文案")
			}
			if !c.wantErr && warn != "" {
				t.Fatalf("配置合法时不应有警告，实际 %q", warn)
			}
		})
	}
}

// TestAppSignSeedGraceWarning 过渡期提示只在启用时出现。
func TestAppSignSeedGraceWarning(t *testing.T) {
	cfg := &Config{AppSignSeed: "rotated", AppSignSeedGrace: []string{DevAppSignSeed}}
	if cfg.AppSignSeedGraceWarning() == "" {
		t.Fatalf("启用过渡期应有提示")
	}
	cfg.AppSignSeedGrace = nil
	if cfg.AppSignSeedGraceWarning() != "" {
		t.Fatalf("未启用过渡期不应有提示")
	}
}
