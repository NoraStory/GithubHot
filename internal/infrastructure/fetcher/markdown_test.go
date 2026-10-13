package fetcher

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// htmlToMarkdown 的核心断言：结构映射正确 + 永不透传 HTML（XSS 防线）。
func TestHTMLToMarkdownStructure(t *testing.T) {
	html := `<div id="content_views">
		<h2>背景介绍</h2>
		<p>这是一段包含<strong>加粗</strong>与<em>斜体</em>的段落，附<a href="https://example.com/a">链接</a>。</p>
		<pre><code class="language-go">func main() {
	fmt.Println("hi") // 含 &lt;script&gt; 的代码
}</code></pre>
		<ul><li>第一项</li><li>第二项<ul><li>嵌套项</li></ul></li></ul>
		<ol><li>有序一</li><li>有序二</li></ol>
		<blockquote><p>引用内容</p></blockquote>
		<table><thead><tr><th>列A</th><th>列B</th></tr></thead><tbody><tr><td>a|b</td><td>2</td></tr></tbody></table>
		<img src="https://img.example.com/x.png" alt="示例图">
	</div>`
	out := htmlToMarkdown(parseSel(t, html, "#content_views"))

	for _, want := range []string{
		"## 背景介绍",
		"**加粗**",
		"*斜体*",
		"[链接](https://example.com/a)",
		"````go",
		`fmt.Println("hi") // 含 <script> 的代码`, // 实体解码后进围栏代码
		"- 第一项",
		"1. 有序一",
		"2. 有序二",
		"> 引用内容",
		"| 列A | 列B |",
		"a\\|b", // 单元格内竖线转义
		"![示例图](https://img.example.com/x.png)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q\n--- 实际输出 ---\n%s", want, out)
		}
	}
	if strings.Contains(out, "- 第二项\n- 嵌套项") {
		t.Errorf("嵌套列表应带缩进:\n%s", out)
	}
}

// parseSel 解析 HTML 片段并取选择器容器（测试辅助）。
func parseSel(t *testing.T, html, selector string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("解析 HTML: %v", err)
	}
	return doc.Find(selector).First()
}

// 安全防线：脚本/样式整体丢弃，输出不含任何裸 HTML 标签。
func TestHTMLToMarkdownDropsScriptAndRawHTML(t *testing.T) {
	html := `<div id="content_views">
		<script>alert('xss')</script>
		<style>.evil{color:red}</style>
		<p>正文<em>强调</em></p>
		<iframe src="https://evil.example"></iframe>
		<div onclick="steal()">点击</div>
	</div>`
	out := htmlToMarkdown(parseSel(t, html, "#content_views"))
	if strings.Contains(out, "alert") || strings.Contains(out, ".evil") || strings.Contains(out, "evil.example") {
		t.Errorf("脚本/样式/iframe 内容未剥离:\n%s", out)
	}
	if strings.Contains(out, "<script") || strings.Contains(out, "<iframe") || strings.Contains(out, "<div") {
		t.Errorf("输出携带裸 HTML 标签:\n%s", out)
	}
}

// 文本节点中的 < 必须实体转义（防 marked 渲染时被当作 HTML 起点）。
func TestHTMLToMarkdownEscapesTextAngleBracket(t *testing.T) {
	out := htmlToMarkdown(parseSel(t, `<div><p>复杂度 a &lt; b 且 x&lt;script&gt;注入</p></div>`, "div"))
	if !strings.Contains(out, "a &lt; b") {
		t.Errorf("文本中的 < 未转义:\n%s", out)
	}
	if strings.Contains(out, "<script>") {
		t.Errorf("实体注入未拦截:\n%s", out)
	}
}

// 段落以 Markdown 控制符开头时首字符反斜杠转义（防止字面量被解析成结构）。
func TestHTMLToMarkdownEscapesBlockStart(t *testing.T) {
	out := htmlToMarkdown(parseSel(t, `<div><p># 井号开头的内容</p><p>1. 数字列表样文本</p></div>`, "div"))
	if !strings.Contains(out, `\# 井号开头的内容`) {
		t.Errorf("行首 # 未转义:\n%s", out)
	}
	if !strings.Contains(out, `\1. 数字列表样文本`) {
		t.Errorf("行首编号未转义:\n%s", out)
	}
}

// CDN 的 UI 装饰图标（点赞/收藏/阅读数，csdnimg.cn/release/ 路径）不属于文章内容。
func TestHTMLToMarkdownSkipsCDNUIIcons(t *testing.T) {
	html := `<div id="content_views"><p>正文段落
		<img src="https://csdnimg.cn/release/blogv2/dist/pc/img/newHeart2023Active.png">
		<img src="https://i-blog.csdnimg.cn/blog_migrate/real-article-shot.png" alt="截图"></p></div>`
	out := htmlToMarkdown(parseSel(t, html, "#content_views"))
	if strings.Contains(out, "csdnimg.cn/release") {
		t.Errorf("UI 装饰图标未过滤:\n%s", out)
	}
	if !strings.Contains(out, "i-blog.csdnimg.cn/blog_migrate/real-article-shot.png") {
		t.Errorf("文章截图被误伤:\n%s", out)
	}
}

// extractMarkdown：页面里仅正文容器参与转换，页头页脚杂讯不进 Markdown。
func TestExtractMarkdown(t *testing.T) {
	page := []byte(`<html><head><title>t</title><style>body{}</style></head><body>
		<nav>站点导航 杂讯</nav>
		<div id="content_views"><h2>正文标题</h2><p>正文段落</p><script>bad()</script></div>
		<footer>页脚杂讯</footer>
	</body></html>`)
	out := extractMarkdown(page, "#content_views, article", 20000)
	if !strings.Contains(out, "## 正文标题") || !strings.Contains(out, "正文段落") {
		t.Errorf("正文缺失:\n%s", out)
	}
	if strings.Contains(out, "站点导航") || strings.Contains(out, "页脚杂讯") {
		t.Errorf("容器外杂讯混入:\n%s", out)
	}
	// 容器缺失 → 空串（详情页降级展示摘要）
	if got := extractMarkdown([]byte("<html><body>无正文容器</body></html>"), "#content_views", 20000); got != "" {
		t.Errorf("容器缺失应返回空串，得到:\n%s", got)
	}
}

// 超长正文按段落边界截断，不落半个词在截断提示前。
func TestTruncateMarkdown(t *testing.T) {
	paras := strings.Repeat("段落内容。\n\n", 500)
	out := truncateMarkdown(paras, 200)
	if len([]rune(out)) > 220 {
		t.Errorf("截断未生效，长度 %d", len([]rune(out)))
	}
	if !strings.Contains(out, "阅读全文请前往原文") {
		t.Errorf("缺少截断提示:\n%s", out)
	}
	if got := truncateMarkdown("短文", 200); got != "短文" {
		t.Errorf("短文不应截断: %q", got)
	}
}
