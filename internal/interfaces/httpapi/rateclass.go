package httpapi

// 速率计费三分类（刷新误封修复，2026-10-11）。
//
// 此前 ipguard 的速率窗口不区分请求类别：/anzhiyu/** 主题文件（含 161 个 CJK
// 字体子集，no-store 下每次刷新全量重打）与 API 挤同一窗口，用户连续 F5 就能
// 越过 150/min 记分、600/min traffic-attack 即时封——把最合法的行为当成攻击。
// 分类后：
//
//	主窗口（导航/API/视频流）→ 150/450/600 三档速率线的唯一口径
//	静态资源 → 不参与三档线，仅 asset-flood 绊线（≥600/min +30 弱类）
//	管理端 API → 独立窗口，仅 admin-probe 判定（≥200/min +50），
//	             控制台 2 秒轮询不再污染主窗口
import (
	"path"
	"strings"
)

type rateClass uint8

const (
	rateClassMain   rateClass = iota // 页面导航 / API / 视频流：三档速率线的计费口径
	rateClassStatic                  // 静态资源：asset-flood 绊线专用
	rateClassAdmin                   // 管理端 API：admin-probe 专用
)

// staticAssetExts 末段扩展名判定为静态资源（小写）。.xml 覆盖 /feed/*.rss 订阅出口。
var staticAssetExts = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".map": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".avif": true, ".svg": true, ".ico": true, ".cur": true,
	".mp3": true, ".flac": true, ".txt": true, ".xml": true,
}

// staticAssetPrefixes 前缀判定的静态目录（无扩展名兜底也命中的资源路径）。
var staticAssetPrefixes = []string{
	"/assets/", "/anzhiyu/", "/img/", "/fonts/", "/css/", "/js/", "/favicon",
}

// classifyRate 请求的速率计费类别。
// 顺序敏感：/video/ 是代理流（带宽重）必须归主窗口，扩展名兜底放在前缀之后，
// 否则 /video/x/1.mp4 的 .mp4 会把视频流降级成静态资源。
func classifyRate(urlPath string) rateClass {
	p := path.Clean(urlPath)
	switch {
	case strings.HasPrefix(p, "/api/v1/admin"):
		return rateClassAdmin
	case strings.HasPrefix(p, "/api/v1/"), strings.HasPrefix(p, "/video/"):
		return rateClassMain
	}
	for _, prefix := range staticAssetPrefixes {
		if strings.HasPrefix(p, prefix) {
			return rateClassStatic
		}
	}
	if ext := strings.ToLower(path.Ext(p)); staticAssetExts[ext] {
		return rateClassStatic
	}
	// 其余一律按页面导航处理（SPA 回退）：归主窗口
	return rateClassMain
}
