package fetcher

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
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

// HotBoard 国内热榜抓取器：百度热搜 / 微博热搜 / B站热门。
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
	case "bilibili":
		return fetchBilibiliHot(ctx, limit)
	case "rss":
		return fetchHotRSS(ctx, s, limit)
	default:
		return nil, fmt.Errorf("信源 %s 缺少有效 board 配置（baidu/weibo/bilibili/rss）", s.ID)
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
// UA 必须像真实浏览器，B站对空/脚本 UA 直接返回 -352 风控。
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

// ---------- B站热门 ----------

// MixinKeyEncTab B 站 WBI 签名混淆表（官方算法，公开稳定）。
var MixinKeyEncTab = []int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35,
	27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13,
	37, 48, 7, 16, 24, 55, 40, 61, 26, 17, 0, 1, 60, 51, 30, 4,
	22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11, 36, 20, 34, 44, 52,
}

// wbiMixinKey 由 img_key + sub_key 生成 32 位混淆密钥。
func wbiMixinKey(imgKey, subKey string) string {
	raw := imgKey + subKey
	var b [64]byte
	for i, idx := range MixinKeyEncTab {
		b[i] = raw[idx]
	}
	return string(b[:32])
}

// wbiSignParams 对参数做 WBI 签名：加 wts、按键排序、值过滤 !'()*，末位附 w_rid。
func wbiSignParams(params map[string]string, mixinKey string, now time.Time) string {
	q := make(url.Values)
	for k, v := range params {
		// 值中移除 !'()*（官方要求）
		filtered := strings.Map(func(r rune) rune {
			switch r {
			case '!', '\'', '(', ')', '*':
				return -1
			}
			return r
		}, v)
		q.Set(k, filtered)
	}
	q.Set("wts", strconv.FormatInt(now.Unix(), 10))
	encoded := q.Encode()
	sum := md5.Sum([]byte(encoded + mixinKey))
	return encoded + "&w_rid=" + hex.EncodeToString(sum[:])
}

// bilibiliWBIKeys 从 nav 接口取 WBI img/sub 公钥。
func bilibiliWBIKeys(ctx context.Context) (imgKey, subKey string, err error) {
	const api = "https://api.bilibili.com/x/web-interface/nav"
	body, _, err := safehttp.Fetch(ctx, api, hotHeaders("https://www.bilibili.com/"))
	if err != nil {
		return "", "", err
	}
	var root struct {
		Code int `json:"code"`
		Data struct {
			WbiImg struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return "", "", fmt.Errorf("解析B站 nav 响应: %w", err)
	}
	imgKey = path.Base(strings.TrimSuffix(root.Data.WbiImg.ImgURL, ".png"))
	subKey = path.Base(strings.TrimSuffix(root.Data.WbiImg.SubURL, ".png"))
	if imgKey == "" || subKey == "" || imgKey == "." || subKey == "." {
		return "", "", fmt.Errorf("B站 nav 未返回 WBI 公钥（code=%d）", root.Code)
	}
	return imgKey, subKey, nil
}

func fetchBilibiliHot(ctx context.Context, limit int) ([]application.FetchedItem, error) {
	imgKey, subKey, err := bilibiliWBIKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取B站 WBI 公钥: %w", err)
	}
	signed := wbiSignParams(map[string]string{"rid": "0", "type": "all"}, wbiMixinKey(imgKey, subKey), time.Now())
	api := "https://api.bilibili.com/x/web-interface/ranking/v2?" + signed
	body, _, err := safehttp.Fetch(ctx, api, hotHeaders("https://www.bilibili.com/"))
	if err != nil {
		return nil, err
	}
	var root struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Title   string `json:"title"`
				Bvid    string `json:"bvid"`
				Tname   string `json:"tname"`
				Pubdate int64  `json:"pubdate"`
				Owner   struct {
					Name string `json:"name"`
				} `json:"owner"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("解析B站热门响应: %w", err)
	}
	if root.Code != 0 {
		return nil, fmt.Errorf("B站热门接口返回 code=%d（风控或结构变更）", root.Code)
	}
	out := make([]application.FetchedItem, 0, len(root.Data.List))
	for i, it := range root.Data.List {
		if len(out) >= limit || it.Title == "" || it.Bvid == "" {
			break
		}
		fi := application.FetchedItem{
			URL:    "https://www.bilibili.com/video/" + it.Bvid,
			Title:  it.Title,
			Author: it.Owner.Name,
			Meta:   map[string]string{"rank": strconv.Itoa(i + 1)},
		}
		if it.Tname != "" {
			fi.Summary = "分区：" + it.Tname
		}
		if it.Pubdate > 0 {
			fi.PublishedAt = time.Unix(it.Pubdate, 0)
		}
		out = append(out, fi)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("B站热门解析不到条目（结构可能已变更）")
	}
	return out, nil
}
