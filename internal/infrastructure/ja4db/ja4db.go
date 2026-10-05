// Package ja4db — P3-2/P3-3 JA4 指纹 → 应用 映射库（规格书 §6 P3-2）。
//
// 数据源：FoxIO-LLC/ja4 仓库的 ja4plus-mapping.csv（`githubhot ja4 update` 拉取到
// data/ja4-mapping.csv）。文件缺失 → Loaded()==false，UA↔TLS 交叉核验整体降级跳过
// （离线部署纪律，同 GeoIP）。
package ja4db

import (
	"encoding/csv"
	"os"
	"strings"
)

// DB JA4 → 应用名 映射。
type DB struct {
	byJA4  map[string]string
	loaded bool
}

// Load 读取映射 CSV；文件不存在或解析失败 → 未加载（调用方降级跳过）。
// 格式容错：FoxIO 的 CSV 带表头，按表头名找 JA4 列（含 "ja4" 的第一列）与应用名列
// （含 "app" 的第一列）；无表头时回退第 0 列 = JA4、第 1 列 = 应用。
func Load(path string) *DB {
	f, err := os.Open(path)
	if err != nil {
		return &DB{}
	}
	defer f.Close()
	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1 // 容忍手工编辑的行字段数不齐（缺列按空值处理）
	rows, err := reader.ReadAll()
	if err != nil || len(rows) == 0 {
		return &DB{}
	}
	db := &DB{byJA4: map[string]string{}}
	ja4Col, appCol := -1, -1
	for i, h := range rows[0] {
		h = strings.ToLower(strings.TrimSpace(h))
		if ja4Col < 0 && strings.Contains(h, "ja4") {
			ja4Col = i
		}
		if appCol < 0 && strings.Contains(h, "app") {
			appCol = i
		}
	}
	start := 1
	if ja4Col < 0 || appCol < 0 {
		// 无表头：第 0 列 = JA4、第 1 列 = 应用
		ja4Col, appCol, start = 0, 1, 0
	}
	for _, row := range rows[start:] {
		if ja4Col >= len(row) || appCol >= len(row) {
			continue
		}
		ja4 := strings.TrimSpace(row[ja4Col])
		app := strings.TrimSpace(row[appCol])
		if ja4 == "" || app == "" {
			continue
		}
		db.byJA4[ja4] = app
	}
	db.loaded = len(db.byJA4) > 0
	return db
}

// Loaded 是否成功加载了映射数据。
func (d *DB) Loaded() bool { return d != nil && d.loaded }

// Lookup JA4 → 应用名。
func (d *DB) Lookup(ja4 string) (string, bool) {
	if d == nil || !d.loaded {
		return "", false
	}
	app, ok := d.byJA4[strings.TrimSpace(ja4)]
	return app, ok
}

// nonBrowserKeywords 已知非浏览器 TLS 栈关键词（小写匹配，规格书 §6 P3-3）：
// UA 声称浏览器而 TLS 栈命中这些 → fpb_ua_tls_mismatch（高置信 +25）。
// 宁可漏判不可误判：只收明确的命令行/HTTP 库客户端。
var nonBrowserKeywords = []string{
	"curl", "wget", "python", "urllib", "requests", "aiohttp", "httpx",
	"go-http", "go http", "golang", "okhttp", "java-", "apache-httpclient", "libwww",
	"scrapy", "node", "undici", "axios", "libcurl", "powershell",
}

// IsNonBrowserApp 映射到的应用名是否为已知非浏览器栈。
func IsNonBrowserApp(app string) bool {
	a := strings.ToLower(app)
	if a == "" {
		return false
	}
	for _, k := range nonBrowserKeywords {
		if strings.Contains(a, k) {
			return true
		}
	}
	return false
}
