package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// jsonUnmarshal 标准库包装（集中错误信息）。
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// parseBase 解析基准 URL。
func parseBase(raw string) (*url.URL, error) { return url.Parse(raw) }

// Registry 按 Kind 分发的抓取器注册表。
type Registry struct {
	m map[source.Kind]application.SourceFetcher
}

// NewRegistry 构造并注册全部内置抓取器。
func NewRegistry() *Registry {
	return &Registry{m: map[source.Kind]application.SourceFetcher{
		source.KindRSS:        RSS{},
		source.KindHackerNews: HackerNews{},
		source.KindWebList:    WebList{},
		source.KindJSONAPI:    JSONAPI{},
	}}
}

// Fetcher 取抓取器；未注册即报适配器缺失。
func (r *Registry) Fetcher(kind source.Kind) (application.SourceFetcher, error) {
	f, ok := r.m[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %s", application.ErrAdapterNotInstalled, kind)
	}
	return f, nil
}

// Register 追加自定义抓取器（扩展点：X 账号、微信公众号适配器在此注册）。
func (r *Registry) Register(kind source.Kind, f application.SourceFetcher) { r.m[kind] = f }

// JSONAPI 通用 JSON 接口抓取器：config 提供 url 与字段映射。
// 约定响应形如 {"items":[{...}]}（可用 list_path 指定数组字段），
// 每项至少含 title 与 url（可用 title_path/url_path 改字段名）。
type JSONAPI struct{}

// Kind 实现端口。
func (JSONAPI) Kind() source.Kind { return source.KindJSONAPI }

// Fetch 拉取并映射 JSON 列表。
func (JSONAPI) Fetch(ctx context.Context, s source.Source, now time.Time) ([]application.FetchedItem, error) {
	apiURL := s.ConfigValue("url", "")
	if apiURL == "" {
		return nil, fmt.Errorf("信源 %s 缺少 url 配置", s.ID)
	}
	body, _, err := safehttpJSON(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("解析 JSON 响应: %w", err)
	}
	listKey := s.ConfigValue("list_path", "items")
	rawList, _ := root[listKey].([]any)
	if rawList == nil {
		// 顶层就是数组的情况
		var arr []any
		if err := json.Unmarshal(body, &arr); err != nil {
			return nil, fmt.Errorf("响应缺少 %s 数组字段", listKey)
		}
		rawList = arr
	}
	titleKey := s.ConfigValue("title_path", "title")
	urlKey := s.ConfigValue("url_path", "url")
	summaryKey := s.ConfigValue("summary_path", "summary")
	timeKey := s.ConfigValue("time_path", "")

	limit := s.ConfigInt("max_items", 30)
	out := make([]application.FetchedItem, 0, len(rawList))
	for _, row := range rawList {
		if len(out) >= limit {
			break
		}
		obj, ok := row.(map[string]any)
		if !ok {
			continue
		}
		title := strField(obj, titleKey)
		u := strField(obj, urlKey)
		if title == "" || u == "" {
			continue
		}
		fi := application.FetchedItem{
			URL:     u,
			Title:   title,
			Summary: strField(obj, summaryKey),
		}
		if timeKey != "" {
			if ts, err := time.Parse(time.RFC3339, strField(obj, timeKey)); err == nil {
				fi.PublishedAt = ts
			}
		}
		out = append(out, fi)
	}
	return out, nil
}

func strField(obj map[string]any, key string) string {
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}
