package githubapi

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/NoraStory/GithubHot/internal/application"
)

// urlEscape 标准库包装。
func urlEscape(s string) string { return url.QueryEscape(s) }

var (
	// repoPathRe 匹配仓库相对路径。
	repoPathRe = regexp.MustCompile(`^/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/?$`)
)

// ParseTrendingHTML 解析 github.com/trending 页面为项目列表。
// 独立纯函数：网络无关，可用固定 HTML 夹具做单元测试。
func ParseTrendingHTML(html string) ([]application.GitHubRepo, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("解析 HTML: %w", err)
	}
	var out []application.GitHubRepo
	doc.Find("article.Box-row").Each(func(i int, row *goquery.Selection) {
		// 仓库链接：h2 下的 a[href="/owner/repo"]
		href := ""
		row.Find("h2 a[href]").Each(func(_ int, a *goquery.Selection) {
			if href != "" {
				return
			}
			h, ok := a.Attr("href")
			if ok && repoPathRe.MatchString(h) {
				href = h
			}
		})
		if href == "" {
			return
		}
		fullName := strings.Trim(href, "/")

		desc := strings.TrimSpace(row.Find("p").First().Text())
		lang := strings.TrimSpace(row.Find("[itemprop=programmingLanguage]").First().Text())

		// 页面排名：用出现顺序
		rank := len(out) + 1

		out = append(out, application.GitHubRepo{
			FullName:     fullName,
			HTMLURL:      "https://github.com/" + fullName,
			Description:  desc,
			Language:     lang,
			Stars:        0, // trending 页不含绝对 star 数，由 enrich 阶段补全
			TrendingRank: rank,
		})
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("trending 页解析出 0 个仓库（页面结构可能已变化）")
	}
	return out, nil
}
