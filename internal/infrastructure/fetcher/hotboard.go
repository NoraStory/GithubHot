package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// flexibleBool 兼容各家榜单字段类型漂移（bool / 0|1 / "0"|"1"）。
type flexibleBool bool

func (b *flexibleBool) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case bool:
		*b = flexibleBool(v)
	case float64:
		*b = flexibleBool(v != 0)
	case string:
		*b = flexibleBool(v == "1" || v == "true")
	case nil:
		*b = false
	}
	return nil
}

// HotBoard 国内热榜抓取器：百度热搜 / 微博热搜 / 网易新闻 / 腾讯新闻。
// 全部国内直连、无需代理；config.board 选择榜单，config.max_items 限制条数。
// 每个榜单解析器同时产出 rank（榜上名次，从 1 开始）与 heat（平台热度值，无则空），
// 存入 FetchedItem.Meta 供后续多源共振热度算法使用。
type HotBoard struct{}

// Kind 实现端口。
func (HotBoard) Kind() source.Kind { return source.KindHotBoard }

// Fetch 按 board 分发解析。
func (HotBoard) Fetch(ctx context.Context, s source.Source, now time.Time) ([]application.FetchedItem, error) {
	board := s.ConfigValue("board", "")
	limit := s.ConfigInt("max_items", 30)
	switch board {
	case "baidu":
		return fetchBaiduHot(ctx, limit)
	case "weibo":
		return fetchWeiboHot(ctx, limit)
	case "netease":
		return fetchNeteaseHot(ctx, limit)
	case "tencent":
		return fetchTencentHot(ctx, limit)
	case "rss":
		return fetchHotRSS(ctx, s, limit)
	default:
		return nil, fmt.Errorf("信源 %s 缺少有效 board 配置（baidu/weibo/netease/tencent/rss）", s.ID)
	}
}

// fetchHotRSS 榜单型 RSS：复用 RSS 抓取器解析 feed，按条目标榜位（feed 顺序即排名）。
// 用于国内资讯源（IT之家/36氪等）接入轻管道：不走 LLM，直接进国内热榜聚簇。
func fetchHotRSS(ctx context.Context, s source.Source, limit int) ([]application.FetchedItem, error) {
	items, err := (RSS{}).Fetch(ctx, s, time.Now())
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("RSS 榜单 %s 无条目", s.ID)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	for i := range items {
		items[i].Meta = map[string]string{"rank": strconv.Itoa(i + 1)}
	}
	return items, nil
}

// hotHeaders 各家需要的反爬请求头（国内榜单 API 校验 UA/Referer）。
// UA 必须像真实浏览器，空/脚本 UA 容易被风控直接拒绝。
func hotHeaders(referer string) map[string]string {
	h := map[string]string{
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9",
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	}
	if referer != "" {
		h["Referer"] = referer
	}
	return h
}

// ---------- 百度热搜 ----------

func fetchBaiduHot(ctx context.Context, limit int) ([]application.FetchedItem, error) {
	const api = "https://top.baidu.com/api/board?platform=wise&tab=realtime"
	body, _, err := safehttp.Fetch(ctx, api, hotHeaders("https://top.baidu.com/"))
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("解析百度热搜响应: %w", err)
	}
	data, _ := root["data"].(map[string]any)
	if data == nil {
		return nil, fmt.Errorf("百度热搜响应缺少 data 字段")
	}
	// 结构历史上多次变动（cards[].content[] 或再嵌套一层），递归收集所有
	// 同时含 word+url 的对象，天然兼容各版本。
	var flat []map[string]any
	collectWordNodes(data, &flat)
	out := make([]application.FetchedItem, 0, len(flat))
	seen := map[string]bool{}
	for _, n := range flat {
		if len(out) >= limit {
			break
		}
		word, _ := n["word"].(string)
		u, _ := n["url"].(string)
		if word == "" || u == "" || seen[word] {
			continue
		}
		seen[word] = true
		desc, _ := n["desc"].(string)
		fi := application.FetchedItem{
			URL:   u,
			Title: word,
			Meta:  map[string]string{"rank": strconv.Itoa(len(out) + 1)},
		}
		if desc != "" {
			fi.Summary = desc
		}
		if hs, ok := n["hot_score"].(float64); ok && hs > 0 {
			fi.Meta["heat"] = strconv.FormatFloat(hs, 'f', 0, 64)
		}
		out = append(out, fi)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("百度热搜解析不到条目（结构可能已变更）")
	}
	return out, nil
}

// collectWordNodes 递归收集树中所有含 word 字段的对象节点。
func collectWordNodes(node any, out *[]map[string]any) {
	switch v := node.(type) {
	case map[string]any:
		if _, ok := v["word"].(string); ok {
			*out = append(*out, v)
		}
		for _, child := range v {
			collectWordNodes(child, out)
		}
	case []any:
		for _, child := range v {
			collectWordNodes(child, out)
		}
	}
}

// ---------- 微博热搜 ----------

func fetchWeiboHot(ctx context.Context, limit int) ([]application.FetchedItem, error) {
	const api = "https://weibo.com/ajax/side/hotSearch"
	body, _, err := safehttp.Fetch(ctx, api, hotHeaders("https://weibo.com/"))
	if err != nil {
		return nil, err
	}
	var root struct {
		Data struct {
			Hotgov struct {
				Word string `json:"word"`
			} `json:"hotgov"`
			Realtime []struct {
				Word  string `json:"word"`
				IsAd  flexibleBool `json:"is_ad"`
				Label string `json:"label"`
			} `json:"realtime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("解析微博热搜响应: %w", err)
	}
	out := make([]application.FetchedItem, 0, limit)
	if root.Data.Hotgov.Word != "" {
		out = append(out, weiboItem(root.Data.Hotgov.Word, 1))
	}
	for _, it := range root.Data.Realtime {
		if len(out) >= limit {
			break
		}
		if it.Word == "" || it.IsAd {
			continue
		}
		out = append(out, weiboItem(it.Word, len(out)+1))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("微博热搜解析不到条目（结构可能已变更）")
	}
	return out, nil
}

// weiboItem 微博条目：官方接口不直接给链接，用搜索页构造（ word 已含 # 时保留）。
func weiboItem(word string, rank int) application.FetchedItem {
	return application.FetchedItem{
		URL:   "https://s.weibo.com/weibo?q=" + url.QueryEscape(word),
		Title: word,
		Meta:  map[string]string{"rank": strconv.Itoa(rank)},
	}
}

// ---------- 网易新闻排行榜 ----------

// fetchNeteaseHot 抓取网易新闻排行榜（24小时点击榜，HTML 页面，GBK 编码）。
// 页面结构稳定多年：第一个 tabContents.active 表格内
// <td class="red"><span>名次</span><a href="链接">标题</a></td><td class="cBlue">点击数</td>。
func fetchNeteaseHot(ctx context.Context, limit int) ([]application.FetchedItem, error) {
	const page = "https://news.163.com/special/0001386F/rank_whole.html"
	body, _, err := safehttp.Fetch(ctx, page, hotHeaders("https://news.163.com/"))
	if err != nil {
		return nil, err
	}
	// 页面 meta 声明 GBK 但实际返回 UTF-8（实测 2026-10），按 UTF-8 直接解析，
	// 同时保留 GBK 兜底以应对编码回滚。
	text := string(body)
	if !utf8.ValidString(text) {
		if decoded, derr := decodeGBK(body); derr == nil {
			text = decoded
		}
	}
	// 只取第一个榜单表格（24小时点击榜），到表格结束为止。
	start := strings.Index(text, `class="tabContents active"`)
	if start < 0 {
		return nil, fmt.Errorf("网易排行榜解析不到榜单区域（结构可能已变更）")
	}
	rest := text[start:]
	end := strings.Index(rest, "</table>")
	if end > 0 {
		rest = rest[:end]
	}
	out := make([]application.FetchedItem, 0, limit)
	seen := map[string]bool{}
	// 逐个解析 <span>名次</span><a href="url">标题</a> …… <td class="cBlue">点击数</td>
	for len(rest) > 0 && len(out) < limit {
		sp := strings.Index(rest, "<span>")
		if sp < 0 {
			break
		}
		rest = rest[sp+len("<span>"):]
		se := strings.Index(rest, "</span>")
		if se < 0 {
			break
		}
		rank, _ := strconv.Atoi(strings.TrimSpace(rest[:se]))
		rest = rest[se+len("</span>"):]
		ap := strings.Index(rest, "<a href=\"")
		if ap < 0 {
			break
		}
		rest = rest[ap+len("<a href=\""):]
		ae := strings.Index(rest, "\"")
		if ae < 0 {
			break
		}
		u := rest[:ae]
		rest = rest[ae+1:]
		gt := strings.Index(rest, ">")
		if gt < 0 {
			break
		}
		rest = rest[gt+1:]
		te := strings.Index(rest, "</a>")
		if te < 0 {
			break
		}
		title := strings.TrimSpace(rest[:te])
		rest = rest[te+len("</a>"):]
		// 点击数（下一行 <td class="cBlue">数字</td>，拿不到不影响）。
		heat := ""
		if hp := strings.Index(rest, `class="cBlue">`); hp >= 0 && hp < 200 {
			seg := rest[hp+len(`class="cBlue">`):]
			if he := strings.Index(seg, "<"); he > 0 {
				heat = strings.TrimSpace(seg[:he])
			}
		}
		if title == "" || u == "" || !strings.HasPrefix(u, "http") || seen[title] {
			continue
		}
		seen[title] = true
		if rank <= 0 {
			rank = len(out) + 1
		}
		fi := application.FetchedItem{URL: u, Title: title, Meta: map[string]string{"rank": strconv.Itoa(rank)}}
		if heat != "" {
			fi.Meta["heat"] = heat
		}
		out = append(out, fi)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("网易排行榜解析不到条目（结构可能已变更）")
	}
	return out, nil
}

// decodeGBK GBK/GB2312 字节流转 UTF-8，非法字节替换为 U+FFFD 不失败。
func decodeGBK(b []byte) (string, error) {
	out, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ---------- 腾讯新闻热点榜 ----------

// fetchTencentHot 抓取腾讯新闻热点榜（官方 JSON 接口，约每 10 分钟更新）。
// 首条常为 TIP 运营占位（articletype=560 / id 前缀 TIP），必须过滤。
func fetchTencentHot(ctx context.Context, limit int) ([]application.FetchedItem, error) {
	const api = "https://r.inews.qq.com/gw/event/pc_hot_ranking_list?limit=100&offset=0"
	body, _, err := safehttp.Fetch(ctx, api, hotHeaders("https://news.qq.com/"))
	if err != nil {
		return nil, err
	}
	var root struct {
		Ret     int `json:"ret"`
		IDList []struct {
			NewsList []struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				URL         string `json:"url"`
				Surl        string `json:"surl"`
				Abstract    string `json:"abstract"`
				ArticleType string `json:"articletype"`
				Time        string `json:"time"`
			} `json:"newslist"`
		} `json:"idlist"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("解析腾讯热点榜响应: %w", err)
	}
	if root.Ret != 0 || len(root.IDList) == 0 {
		return nil, fmt.Errorf("腾讯热点榜接口返回 ret=%d（结构可能已变更）", root.Ret)
	}
	out := make([]application.FetchedItem, 0, limit)
	seen := map[string]bool{}
	for _, it := range root.IDList[0].NewsList {
		if len(out) >= limit {
			break
		}
		// TIP 运营位 / 非新闻条目 / 广告位过滤
		if it.Title == "" || strings.HasPrefix(it.ID, "TIP") || it.ArticleType != "0" {
			continue
		}
		u := it.URL
		if u == "" {
			u = it.Surl
		}
		if u == "" || seen[it.Title] {
			continue
		}
		seen[it.Title] = true
		fi := application.FetchedItem{
			URL:     u,
			Title:   it.Title,
			Summary: it.Abstract,
			Meta:    map[string]string{"rank": strconv.Itoa(len(out) + 1)},
		}
		if ts, perr := time.ParseInLocation("2006-01-02 15:04:05", it.Time, time.Local); perr == nil {
			fi.PublishedAt = ts
		}
		out = append(out, fi)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("腾讯热点榜解析不到条目（结构可能已变更）")
	}
	return out, nil
}
