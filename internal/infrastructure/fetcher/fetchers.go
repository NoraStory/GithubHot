// Package fetcher 实现各类信源的抓取适配器（应用层 SourceFetcher 端口）。
// 所有出站请求都经 safehttp 走 SSRF 校验。
package fetcher

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// RSS RSS / Atom 抓取器。
type RSS struct{}

// Kind 实现端口。
func (RSS) Kind() source.Kind { return source.KindRSS }

// Fetch 拉取并解析 feed。
func (RSS) Fetch(ctx context.Context, s source.Source, _ time.Time) ([]application.FetchedItem, error) {
	feedURL := s.ConfigValue("url", "")
	if feedURL == "" {
		return nil, fmt.Errorf("信源 %s 缺少 url 配置", s.ID)
	}
	body, _, err := safehttp.Fetch(ctx, feedURL, map[string]string{"Accept": "application/rss+xml, application/atom+xml, application/xml, text/xml, */*"})
	if err != nil {
		return nil, fmt.Errorf("拉取 feed: %w", err)
	}
	fp := gofeed.NewParser()
	feed, err := fp.ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("解析 feed: %w", err)
	}
	limit := s.ConfigInt("max_items", 30)
	out := make([]application.FetchedItem, 0, len(feed.Items))
	for _, it := range feed.Items {
		if len(out) >= limit {
			break
		}
		if it.Link == "" {
			continue
		}
		pub := parseFeedTime(it.PublishedParsed, it.UpdatedParsed)
		out = append(out, application.FetchedItem{
			URL:         it.Link,
			Title:       strings.TrimSpace(it.Title),
			Summary:     stripHTML(it.Description),
			Content:     stripHTML(it.Content),
			Author:      feedAuthor(it),
			PublishedAt: pub,
		})
	}
	// 全文抓取（site_fulltext 同款能力）：信源 config 配 fulltext=1 时，
	// 逐条抓原文页面提取正文（上限 fulltext_items 条 / fulltext_chars 字符）
	if s.ConfigValue("fulltext", "") == "1" {
		maxItems := s.ConfigInt("fulltext_items", 8)
		maxChars := s.ConfigInt("fulltext_chars", 6000)
		limit := maxItems
		if len(out) < limit {
			limit = len(out)
		}
		for i := 0; i < limit; i++ {
			ftctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			text := fetchFulltext(ftctx, out[i].URL, maxChars)
			cancel()
			if text != "" {
				out[i].Content = text
			}
		}
	}
	return out, nil
}

// fetchFulltext 抓取文章页并提取正文文本（主内容区优先，回退全部段落）。
func fetchFulltext(ctx context.Context, pageURL string, maxChars int) string {
	body, status, err := safehttp.Fetch(ctx, pageURL, map[string]string{"Accept": "text/html"})
	if err != nil || status != 200 {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}
	container := doc.Find("article, .post-content, .article-content, .entry-content, main").First()
	if container.Length() == 0 {
		container = doc.Find("body")
	}
	var parts []string
	container.Find("p").Each(func(_ int, p *goquery.Selection) {
		t := strings.TrimSpace(p.Text())
		if len([]rune(t)) >= 20 {
			parts = append(parts, t)
		}
	})
	text := strings.Join(parts, "\n")
	r := []rune(text)
	if len(r) > maxChars {
		text = string(r[:maxChars])
	}
	return text
}

func parseFeedTime(ts ...*time.Time) time.Time {
	for _, t := range ts {
		if t != nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func feedAuthor(it *gofeed.Item) string {
	if it.Author != nil {
		return it.Author.Name
	}
	return ""
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML 去除 HTML 标签并合并空白，得到纯文本。
func stripHTML(s string) string {
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, "<"); idx < 0 {
		return strings.Join(strings.Fields(s), " ")
	}
	s = htmlTagRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// HackerNews Hacker News（Algolia API）抓取器：近 N 小时含关键词的故事。
type HackerNews struct{}

// AlgoliaEndpoint HN 搜索 API。独立变量便于测试替换。
var AlgoliaEndpoint = "https://hn.algolia.com/api/v1/search_by_date"

// Kind 实现端口。
func (HackerNews) Kind() source.Kind { return source.KindHackerNews }

// Fetch 抓取并按关键词过滤。
func (HackerNews) Fetch(ctx context.Context, s source.Source, now time.Time) ([]application.FetchedItem, error) {
	hours := s.ConfigInt("hours", 24)
	keywords := defaultHNKeywords
	if k := s.ConfigValue("keywords", ""); k != "" {
		keywords = strings.Split(strings.ToLower(k), ",")
		for i := range keywords {
			keywords[i] = strings.TrimSpace(keywords[i])
		}
	}
	since := now.Add(-time.Duration(hours) * time.Hour).Unix()
	url := fmt.Sprintf("%s?tags=story&numericFilters=created_at_i>%d&hitsPerPage=150", AlgoliaEndpoint, since)

	body, _, err := safehttp.Fetch(ctx, url, map[string]string{"Accept": "application/json"})
	if err != nil {
		return nil, fmt.Errorf("拉取 HN: %w", err)
	}
	var resp struct {
		Hits []struct {
			ObjectID    string  `json:"objectID"`
			Title       string  `json:"title"`
			URL         *string `json:"url"`
			Author      string  `json:"author"`
			CreatedAt   string  `json:"created_at"`
			Points      int     `json:"points"`
			NumComments int     `json:"num_comments"`
			StoryText   *string `json:"story_text"`
		} `json:"hits"`
	}
	if err := jsonUnmarshal(body, &resp); err != nil || len(resp.Hits) == 0 {
		// 降级：部分网络环境下 Algolia 出口返回 HTML 挑战页，改走 HN 官方 Firebase API
		return fetchHNFirebase(ctx, s, now, since, keywords)
	}
	out := make([]application.FetchedItem, 0, 20)
	seen := map[string]bool{}
	for _, h := range resp.Hits {
		if h.Title == "" || h.URL == nil || *h.URL == "" {
			continue
		}
		if seen[*h.URL] {
			continue
		}
		if !matchesKeywords(h.Title+" "+deref(h.StoryText), keywords) {
			continue
		}
		seen[*h.URL] = true
		created, _ := time.Parse(time.RFC3339, h.CreatedAt)
		summary := fmt.Sprintf("Hacker News 讨论（%d 分 / %d 评论）", h.Points, h.NumComments)
		out = append(out, application.FetchedItem{
			URL:         *h.URL,
			Title:       h.Title,
			Summary:     summary,
			Author:      h.Author,
			PublishedAt: created.UTC(),
		})
	}
	return out, nil
}

var defaultHNKeywords = []string{
	"ai", "llm", "gpt", "openai", "anthropic", "claude", "gemini", "deepseek",
	"llama", "machine learning", "neural", "diffusion", "transformer", "rag",
	"agent", "inference", "training", "mistral", "qwen", "kimi", "glm",
}

func matchesKeywords(text string, keywords []string) bool {
	lower := strings.ToLower(text)
	for _, k := range keywords {
		if k != "" && strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return stripHTML(*s)
}

// fetchHNFirebase HN 官方 Firebase API 降级路径：
// topstories → 并发拉取条目 → 时间窗 + 关键词过滤。
func fetchHNFirebase(ctx context.Context, s source.Source, now time.Time, since int64, keywords []string) ([]application.FetchedItem, error) {
	body, _, err := safehttp.Fetch(ctx, "https://hacker-news.firebaseio.com/v0/topstories.json", map[string]string{"Accept": "application/json"})
	if err != nil {
		return nil, fmt.Errorf("HN Firebase 拉取失败: %w", err)
	}
	var ids []int64
	if err := jsonUnmarshal(body, &ids); err != nil {
		return nil, fmt.Errorf("HN Firebase 响应异常: %w", err)
	}
	if len(ids) > 60 {
		ids = ids[:60]
	}
	type hnItem struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		By          string `json:"by"`
		Time        int64  `json:"time"`
		Score       int    `json:"score"`
		Descendants int    `json:"descendants"`
	}
	var (
		mu    sync.Mutex
		items []hnItem
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 8)
	)
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ictx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			ib, _, err := safehttp.Fetch(ictx, fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id), nil)
			if err != nil {
				return
			}
			var it hnItem
			if jsonUnmarshal(ib, &it) != nil || it.Title == "" || it.URL == "" {
				return
			}
			mu.Lock()
			items = append(items, it)
			mu.Unlock()
		}(id)
	}
	wg.Wait()

	out := make([]application.FetchedItem, 0, 20)
	seen := map[string]bool{}
	for _, it := range items {
		if seen[it.URL] {
			continue
		}
		if it.Time > 0 && now.Sub(time.Unix(it.Time, 0)) > time.Duration(s.ConfigInt("hours", 24))*time.Hour {
			continue
		}
		if !matchesKeywords(it.Title, keywords) {
			continue
		}
		seen[it.URL] = true
		out = append(out, application.FetchedItem{
			URL:         it.URL,
			Title:       it.Title,
			Summary:     fmt.Sprintf("Hacker News 讨论（%d 分 / %d 评论）", it.Score, it.Descendants),
			Author:      it.By,
			PublishedAt: time.Unix(it.Time, 0).UTC(),
		})
	}
	return out, nil
}

// WebList 通用网页列表抓取器（实验性）：config 指定 url 与可选 CSS 选择器。
type WebList struct{}

// Kind 实现端口。
func (WebList) Kind() source.Kind { return source.KindWebList }

// Fetch 拉取页面，按选择器抽链接列表。
func (WebList) Fetch(ctx context.Context, s source.Source, now time.Time) ([]application.FetchedItem, error) {
	pageURL := s.ConfigValue("url", "")
	if pageURL == "" {
		return nil, fmt.Errorf("信源 %s 缺少 url 配置", s.ID)
	}
	selector := s.ConfigValue("selector", "main a[href], article a[href]")
	pattern := s.ConfigValue("url_pattern", "")

	body, _, err := safehttp.Fetch(ctx, pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("拉取页面: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("解析页面: %w", err)
	}
	limit := s.ConfigInt("max_items", 30)
	var out []application.FetchedItem
	seen := map[string]bool{}
	doc.Find(selector).Each(func(_ int, node *goquery.Selection) {
		if len(out) >= limit {
			return
		}
		href, ok := node.Attr("href")
		if !ok || seen[href] {
			return
		}
		full, err := resolveURL(pageURL, href)
		if err != nil {
			return
		}
		if pattern != "" {
			re, cerr := regexp.Compile(pattern)
			if cerr != nil || !re.MatchString(full) {
				return
			}
		}
		title := strings.TrimSpace(node.Text())
		if title == "" || len([]rune(title)) < 6 {
			return
		}
		seen[href] = true
		out = append(out, application.FetchedItem{URL: full, Title: title, PublishedAt: now})
	})
	return out, nil
}

var hrefRe = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:`)

// resolveURL 相对链接转绝对链接（仅 http/https）。
func resolveURL(base, href string) (string, error) {
	if hrefRe.MatchString(href) {
		if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
			return "", fmt.Errorf("非 http 协议: %s", href)
		}
		return href, nil
	}
	return joinPath(base, href)
}

func joinPath(base, href string) (string, error) {
	b, err := parseBase(base)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(href, "//") {
		return b.Scheme + ":" + href, nil
	}
	if strings.HasPrefix(href, "/") {
		return fmt.Sprintf("%s://%s%s", b.Scheme, b.Host, href), nil
	}
	return fmt.Sprintf("%s://%s%s%s", b.Scheme, b.Host, strings.TrimSuffix(b.Path, "/"), "/"+href), nil
}
