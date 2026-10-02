package githubapi

import "testing"

// fixture 是仿 github.com/trending 页面结构的最小 HTML（article.Box-row）。
const fixture = `<!DOCTYPE html><html><body>
<main>
<article class="Box-row">
  <h2 class="h3 lh-condensed"><a href="/userone/hot-repo" class="d-inline-block">userone / hot-repo</a></h2>
  <p class="col-9 color-fg-muted my-1 pr-4">An blazing fast AI inference engine</p>
  <span itemprop="programmingLanguage">Rust</span>
  <span class="d-inline-block float-sm-right">1,234 stars today</span>
</article>
<article class="Box-row">
  <h2><a href="/usertwo/second-repo">usertwo / second-repo</a></h2>
  <p>LLM agent framework in Go</p>
  <span itemprop="programmingLanguage">Go</span>
</article>
<article class="Box-row">
  <h2><a href="/not-a-repo-link">skip me</a></h2>
</article>
</main></body></html>`

func TestParseTrendingHTML(t *testing.T) {
	repos, err := ParseTrendingHTML(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("应解析出 2 个仓库，得到 %d", len(repos))
	}
	first := repos[0]
	if first.FullName != "userone/hot-repo" {
		t.Fatalf("第一个仓库应为 userone/hot-repo，得到 %s", first.FullName)
	}
	if first.TrendingRank != 1 || repos[1].TrendingRank != 2 {
		t.Fatalf("排名应按出现顺序：得到 %d / %d", first.TrendingRank, repos[1].TrendingRank)
	}
	if first.Language != "Rust" {
		t.Fatalf("语言解析错误: %q", first.Language)
	}
	if first.Description != "An blazing fast AI inference engine" {
		t.Fatalf("描述解析错误: %q", first.Description)
	}
	if first.HTMLURL != "https://github.com/userone/hot-repo" {
		t.Fatalf("URL 错误: %s", first.HTMLURL)
	}
}

func TestParseTrendingHTMLEmpty(t *testing.T) {
	if _, err := ParseTrendingHTML("<html><body>blocked</body></html>"); err == nil {
		t.Fatal("无仓库结果应报错（页面结构变化的信号）")
	}
}
