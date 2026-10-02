// Package githubapi 实现 GitHub 双轨数据网关：
// 轨道 A：官方 Search API（近 N 天创建、按 star 排序的新项目）；
// 轨道 B：github.com/trending 每日榜（社区口径，无官方 API，HTML 解析）。
//
// 国内服务器直连 github.com 可能失败，提供两层缓解：
//  1. GITHUB_PROXY 环境变量：把 github.com / api.github.com 请求改写为
//     「镜像前缀 + 原地址」形式（如 https://gh-proxy.com/https://github.com/...）；
//  2. 对 GitHub 请求自动重试（网络错误 / 5xx / 429，最多 3 次，退避 2s/4s）。
package githubapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// Client GitHub 网关实现。Token 可选（未配置时限额 60 次/小时）。
// Proxy 可选（GITHUB_PROXY，镜像前缀，国内服务器直连失败时使用）。
type Client struct {
	Token string
	Proxy string
}

// New 构造。
func New(token, proxy string) *Client { return &Client{Token: token, Proxy: proxy} }

// url 把原始 GitHub 地址按镜像前缀改写（未配置代理时原样返回）。
func (c *Client) url(raw string) string {
	p := strings.TrimRight(strings.TrimSpace(c.Proxy), "/")
	if p == "" {
		return raw
	}
	if !strings.HasPrefix(p, "https://") && !strings.HasPrefix(p, "http://") {
		return raw
	}
	return p + "/" + raw
}

// fetchGet 带重试的 GET：网络错误 / 5xx / 429 时退避重试（2s、4s），最多 3 次。
func (c *Client) fetchGet(ctx context.Context, url string, headers map[string]string) ([]byte, int, error) {
	var body []byte
	var status int
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		b, st, err := safehttp.Fetch(ctx, c.url(url), headers)
		if err == nil && st < 500 && st != 429 {
			return b, st, nil
		}
		body, status = b, st
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("HTTP %d", st)
		}
	}
	return body, status, lastErr
}

func (c *Client) headers() map[string]string {
	h := map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}
	if c.Token != "" {
		h["Authorization"] = "Bearer " + c.Token
	}
	return h
}

// SearchNewRising 轨道 A：搜索新爆项目。
func (c *Client) SearchNewRising(ctx context.Context, sinceDays, minStars, perPage int) ([]application.GitHubRepo, error) {
	if sinceDays <= 0 {
		sinceDays = 7
	}
	if minStars <= 0 {
		minStars = 50
	}
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}
	since := time.Now().UTC().AddDate(0, 0, -sinceDays).Format("2006-01-02")
	q := fmt.Sprintf("created:>%s stars:>%d", since, minStars)
	url := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&order=desc&per_page=%d", urlQueryEscape(q), perPage)

	body, status, err := c.fetchGet(ctx, url, c.headers())
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("GitHub Search API 返回 %d: %s", status, truncateBody(body))
	}
	var resp struct {
		Items []struct {
			FullName    string   `json:"full_name"`
			HTMLURL     string   `json:"html_url"`
			Description *string  `json:"description"`
			Language    *string  `json:"language"`
			Topics      []string `json:"topics"`
			Stars       int      `json:"stargazers_count"`
			Forks       int      `json:"forks_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析 Search 响应: %w", err)
	}
	out := make([]application.GitHubRepo, 0, len(resp.Items))
	for _, it := range resp.Items {
		desc := ""
		if it.Description != nil {
			desc = *it.Description
		}
		lang := ""
		if it.Language != nil {
			lang = *it.Language
		}
		out = append(out, application.GitHubRepo{
			FullName:    it.FullName,
			HTMLURL:     it.HTMLURL,
			Description: desc,
			Language:    lang,
			Topics:      it.Topics,
			Stars:       it.Stars,
			Forks:       it.Forks,
		})
	}
	return out, nil
}

// FetchTrending 轨道 B：抓取 trending 每日榜。
// trending 页不含绝对 star 数，逐仓调用 REST API 补全（热度差分需要绝对值）。
func (c *Client) FetchTrending(ctx context.Context) ([]application.GitHubRepo, error) {
	body, status, err := c.fetchGet(ctx, "https://github.com/trending?since=daily", map[string]string{
		"Accept":          "text/html,application/xhtml+xml",
		"Accept-Language": "en-US,en;q=0.9",
	})
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("trending 页返回 %d", status)
	}
	repos, err := ParseTrendingHTML(string(body))
	if err != nil {
		return nil, err
	}
	for i := range repos {
		if repo, rerr := c.fetchRepo(ctx, repos[i].FullName); rerr == nil {
			if repo.Stars > 0 {
				repos[i].Stars = repo.Stars
			}
			if repo.Forks > 0 {
				repos[i].Forks = repo.Forks
			}
			if repos[i].Description == "" {
				repos[i].Description = repo.Description
			}
			if repos[i].Language == "" {
				repos[i].Language = repo.Language
			}
			if len(repo.Topics) > 0 {
				repos[i].Topics = repo.Topics
			}
		}
	}
	return repos, nil
}

// fetchRepo 拉取单个仓库元数据。
func (c *Client) fetchRepo(ctx context.Context, fullName string) (*application.GitHubRepo, error) {
	url := "https://api.github.com/repos/" + fullName
	body, status, err := c.fetchGet(ctx, url, c.headers())
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("GET /repos/%s 返回 %d", fullName, status)
	}
	var it struct {
		FullName    string   `json:"full_name"`
		HTMLURL     string   `json:"html_url"`
		Description *string  `json:"description"`
		Language    *string  `json:"language"`
		Topics      []string `json:"topics"`
		Stars       int      `json:"stargazers_count"`
		Forks       int      `json:"forks_count"`
	}
	if err := json.Unmarshal(body, &it); err != nil {
		return nil, err
	}
	out := &application.GitHubRepo{FullName: it.FullName, HTMLURL: it.HTMLURL, Topics: it.Topics, Stars: it.Stars, Forks: it.Forks}
	if it.Description != nil {
		out.Description = *it.Description
	}
	if it.Language != nil {
		out.Language = *it.Language
	}
	return out, nil
}

func truncateBody(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
	}
	return strings.TrimSpace(s)
}

// urlQueryEscape 查询串转义（标准库封装，便于测试）。
func urlQueryEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(urlEscape(s), "+", "%20"), "%3A", ":")
}
