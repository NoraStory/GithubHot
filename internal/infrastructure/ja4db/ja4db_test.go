package ja4db

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureCSV 写一个 FoxIO 风格的小映射表。
func fixtureCSV(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "ja4-mapping.csv")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLoadWithHeader 带表头：按表头名定位 JA4 列与应用列。
func TestLoadWithHeader(t *testing.T) {
	p := fixtureCSV(t, "sha256,ja4_hash,original_app,notes\n"+
		"aaa,t13d1516h2_8daaf6152771_b186095e22b6,Chrome 120.0,desktop\n"+
		"bbb,t13d1713h2_5b57614c22b0_3d5424432f57,curl 8.4.0,cli\n")
	db := Load(p)
	if !db.Loaded() {
		t.Fatal("应加载成功")
	}
	if app, ok := db.Lookup("t13d1516h2_8daaf6152771_b186095e22b6"); !ok || app != "Chrome 120.0" {
		t.Fatalf("Chrome 查询失败: %q %v", app, ok)
	}
	if app, ok := db.Lookup("t13d1713h2_5b57614c22b0_3d5424432f57"); !ok || !IsNonBrowserApp(app) {
		t.Fatalf("curl 应命中非浏览器栈: %q %v", app, ok)
	}
	if _, ok := db.Lookup("t00d0000h0_000000000000_000000000000"); ok {
		t.Fatalf("未知指纹不应命中")
	}
}

// TestLoadNoHeader 无表头回退：第 0 列 = JA4、第 1 列 = 应用。
func TestLoadNoHeader(t *testing.T) {
	p := fixtureCSV(t, "t13d1516h2_xxx_yyy,Python urllib,extra\n")
	db := Load(p)
	if app, ok := db.Lookup("t13d1516h2_xxx_yyy"); !ok || !IsNonBrowserApp(app) {
		t.Fatalf("无表头回退失败: %q %v", app, ok)
	}
}

// TestLoadMissing 文件缺失 → 未加载，查询一律不命中（降级纪律）。
func TestLoadMissing(t *testing.T) {
	db := Load(filepath.Join(t.TempDir(), "nope.csv"))
	if db.Loaded() {
		t.Fatal("缺失文件应未加载")
	}
	if _, ok := db.Lookup("anything"); ok {
		t.Fatal("未加载时不应命中")
	}
	var nilDB *DB
	if nilDB.Loaded() || func() bool { _, ok := nilDB.Lookup("x"); return ok }() {
		t.Fatal("nil 接收者应安全降级")
	}
}

// TestIsNonBrowserApp 非浏览器关键词判定（大小写不敏感；浏览器名不命中）。
func TestIsNonBrowserApp(t *testing.T) {
	yes := []string{"curl 8.4.0", "Go HTTP Client", "Python 3.x urllib", "okhttp3", "PowerShell"}
	no := []string{"Chrome 120.0", "Firefox 121", "Safari 17", "Edge 120", ""}
	for _, a := range yes {
		if !IsNonBrowserApp(a) {
			t.Errorf("%q 应判为非浏览器栈", a)
		}
	}
	for _, a := range no {
		if IsNonBrowserApp(a) {
			t.Errorf("%q 不应判为非浏览器栈", a)
		}
	}
}
