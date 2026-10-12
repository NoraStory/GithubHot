package fetcher

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// 本文件实现 HTML → 纯 Markdown 转换（博客文章站内阅读管道的正文加工）。
//
// 安全模型：转换器只输出 Markdown 语法（标题/列表/围栏代码/链接/图片），
// 永不透传任何 HTML 标签与属性；文本节点中的 < 做实体转义，script/style/iframe
// 等元素整体丢弃——落库内容因此可以安全地交给前端 marked 渲染（marked 不转义
// 内嵌 HTML，防线就在这里）。

// blogFetchUA 抓取文章页用浏览器 UA：CSDN 等站点对非浏览器 UA 出验证页。
const blogFetchUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// defaultContentSelector 正文容器候选选择器：CSDN 优先，其余为常见博客平台回退链。
const defaultContentSelector = "#content_views, article, .article-content, .entry-content, main"

// dropTags 转换时整体丢弃的元素（脚本/嵌入/交互件，安全与降噪双重目的）。
var dropTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"button": true, "input": true, "select": true, "textarea": true, "form": true,
	"nav": true, "aside": true, "svg": true, "canvas": true, "noscript": true,
}

// mdTextEscaper 文本节点转义：Markdown 控制字符反斜杠转义 + < 实体转义
//（后者是 XSS 防线：裸 < 在 marked 渲染时会被当作 HTML 起点）。
var mdTextEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`<`, `&lt;`,
	`[`, `\[`,
	`]`, `\]`,
)

var codeLangRe = regexp.MustCompile(`(?:language|lang|highlight)-([A-Za-z0-9_+#-]+)`)

// htmlToMarkdown 把正文容器转换为 Markdown 文本（段落间空一行）。
func htmlToMarkdown(container *goquery.Selection) string {
	var b strings.Builder
	blocksToMD(container, &b)
	return normalizeMarkdown(b.String())
}

// blocksToMD 块级遍历：容器/透明容器的直接子节点分派到各块级处理器。
func blocksToMD(parent *goquery.Selection, b *strings.Builder) {
	parent.Contents().Each(func(_ int, s *goquery.Selection) {
		switch name := goquery.NodeName(s); name {
		case "#text":
			if t := strings.TrimSpace(s.Text()); t != "" {
				writeParagraph(b, mdTextEscaper.Replace(t))
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			writeParagraph(b, strings.Repeat("#", int(name[1]-'0'))+" "+inlineMD(s))
		case "p":
			writeParagraph(b, escapeBlockStart(inlineMD(s)))
		case "pre":
			writeParagraph(b, fenceOf(s))
		case "blockquote":
			var inner strings.Builder
			blocksToMD(s, &inner)
			if t := strings.TrimSpace(inner.String()); t != "" {
				writeParagraph(b, quotePrefix(t))
			}
		case "ul":
			listToMD(s, false, 0, b)
		case "ol":
			listToMD(s, true, 0, b)
		case "hr":
			writeParagraph(b, "---")
		case "table":
			tableToMD(s, b)
		case "img":
			if md := imageMD(s); md != "" {
				writeParagraph(b, md)
			}
		default:
			if dropTags[name] {
				return
			}
			// 未知/透明容器（div/section/figure…）：递归，内容不丢
			blocksToMD(s, b)
		}
	})
}

// inlineMD 行内转换：文本 + 强调/代码/链接/图片，未识别元素透明递归。
func inlineMD(s *goquery.Selection) string {
	// 调用方可能直接传入文本节点选择器（如列表项的裸文本）
	if goquery.NodeName(s) == "#text" {
		return mdTextEscaper.Replace(s.Text())
	}
	var b strings.Builder
	s.Contents().Each(func(_ int, c *goquery.Selection) {
		switch name := goquery.NodeName(c); name {
		case "#text":
			b.WriteString(mdTextEscaper.Replace(c.Text()))
		case "strong", "b":
			if t := strings.TrimSpace(inlineMD(c)); t != "" {
				b.WriteString("**" + t + "**")
			}
		case "em", "i":
			if t := strings.TrimSpace(inlineMD(c)); t != "" {
				b.WriteString("*" + t + "*")
			}
		case "del", "s", "strike":
			if t := strings.TrimSpace(inlineMD(c)); t != "" {
				b.WriteString("~~" + t + "~~")
			}
		case "code":
			t := strings.TrimSpace(c.Text())
			if t != "" {
				b.WriteString("`" + strings.ReplaceAll(t, "`", "'") + "`")
			}
		case "a":
			href := attrOr(c, "href")
			label := strings.TrimSpace(inlineMD(c))
			switch {
			case href == "" || strings.HasPrefix(strings.ToLower(href), "javascript"):
				b.WriteString(label)
			case label == "":
				b.WriteString(escapeURL(href))
			default:
				b.WriteString("[" + label + "](" + escapeURL(href) + ")")
			}
		case "img":
			b.WriteString(imageMD(c))
		case "br":
			b.WriteString("  \n") // GFM 硬换行（站点 marked 配置 breaks:false 也能生效）
		default:
			if dropTags[name] {
				return
			}
			b.WriteString(inlineMD(c))
		}
	})
	return b.String()
}

// imageMD 图片 → ![alt](src)；懒加载站点 src 缺失时回退 data-src。
func imageMD(s *goquery.Selection) string {
	src := attrOr(s, "src")
	if src == "" {
		src = attrOr(s, "data-src")
	}
	if src == "" {
		return ""
	}
	alt := attrOr(s, "alt")
	return "![" + alt + "](" + escapeURL(src) + ")"
}

// fenceOf 代码块 → 围栏代码（四反引号围栏，防代码内含 ``` 被截断）。
// 行号装饰元素（部分站点的行号列）先剔除再取文本。
func fenceOf(pre *goquery.Selection) string {
	lang := ""
	text := ""
	if code := pre.Find("code").First(); code.Length() > 0 {
		lang = codeLang(code)
	}
	clone := pre.Clone()
	clone.Find(".line-numbers, .hljs-ln-numbers, .code-number, .prism-line-numbers, .code-num").Remove()
	if code := clone.Find("code").First(); code.Length() > 0 {
		text = code.Text()
	} else {
		text = clone.Text()
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "````" + lang + "\n" + text + "\n````"
}

func codeLang(s *goquery.Selection) string {
	cls, _ := s.Attr("class")
	if m := codeLangRe.FindStringSubmatch(cls); m != nil {
		return m[1]
	}
	return ""
}

// listToMD 有序/无序列表；li 内嵌套列表递归缩进。
func listToMD(list *goquery.Selection, ordered bool, depth int, b *strings.Builder) {
	idx := 0
	list.Children().Each(func(_ int, li *goquery.Selection) {
		if goquery.NodeName(li) != "li" {
			return
		}
		idx++
		marker := "-"
		if ordered {
			marker = fmt.Sprintf("%d.", idx)
		}
		indent := strings.Repeat("  ", depth)
		// 拆分 li 的直接子节点：嵌套列表之外全按行内处理
		var head strings.Builder
		li.Contents().Each(func(_ int, c *goquery.Selection) {
			n := goquery.NodeName(c)
			if n == "ul" || n == "ol" || dropTags[n] {
				return
			}
			head.WriteString(inlineMD(c))
		})
		lines := strings.Split(strings.TrimSpace(head.String()), "\n")
		first := strings.TrimSpace(lines[0])
		if first == "" {
			b.WriteString(indent + marker + "\n")
		} else {
			b.WriteString(indent + marker + " " + first + "\n")
			for _, l := range lines[1:] {
				if t := strings.TrimSpace(l); t != "" {
					b.WriteString(indent + "  " + t + "\n")
				}
			}
		}
		li.Children().Each(func(_ int, c *goquery.Selection) {
			switch n := goquery.NodeName(c); n {
			case "ul":
				listToMD(c, false, depth+1, b)
			case "ol":
				listToMD(c, true, depth+1, b)
			case "pre":
				writeParagraph(b, indent+"  "+strings.ReplaceAll(fenceOf(c), "\n", "\n"+indent+"  "))
			}
		})
	})
	b.WriteString("\n")
}

// tableToMD 表格 → GFM 管道表（首行当表头；单元格内竖线转义）。
func tableToMD(tbl *goquery.Selection, b *strings.Builder) {
	var rows [][]string
	cols := 0
	tbl.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		var row []string
		tr.Children().Each(func(_ int, cell *goquery.Selection) {
			if n := goquery.NodeName(cell); n == "td" || n == "th" {
				row = append(row, strings.ReplaceAll(strings.TrimSpace(inlineMD(cell)), "|", `\|`))
			}
		})
		if len(row) > cols {
			cols = len(row)
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	})
	if len(rows) == 0 || cols == 0 {
		return
	}
	pad := func(row []string) string {
		cells := make([]string, cols)
		for i := 0; i < cols; i++ {
			if i < len(row) {
				cells[i] = row[i]
			}
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	sep := make([]string, cols)
	for i := range sep {
		sep[i] = "---"
	}
	lines := []string{pad(rows[0]), pad(sep)}
	for _, row := range rows[1:] {
		lines = append(lines, pad(row))
	}
	writeParagraph(b, strings.Join(lines, "\n"))
}

// escapeBlockStart 段落若以 Markdown 控制符开头（会被解析成标题/引用/列表），
// 首字符反斜杠转义还原字面量。
func escapeBlockStart(md string) string {
	if md == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(md, "#"), strings.HasPrefix(md, ">"),
		strings.HasPrefix(md, "-"), strings.HasPrefix(md, "*"), strings.HasPrefix(md, "+"),
		strings.HasPrefix(md, "`"), strings.HasPrefix(md, "~"):
		return `\` + md
	}
	if md[0] >= '0' && md[0] <= '9' && len(md) > 1 && (md[1] == '.' || md[1] == ')') {
		return `\` + md
	}
	return md
}

// quotePrefix 引用块：每行加 "> "。
func quotePrefix(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return strings.Join(lines, "\n")
}

func writeParagraph(b *strings.Builder, md string) {
	if strings.TrimSpace(md) == "" {
		return
	}
	b.WriteString(md + "\n\n")
}

// normalizeMarkdown 收尾：压缩 3+ 连续空行、去首尾空白。
func normalizeMarkdown(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			blank++
			if blank > 2 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// truncateMarkdown 超长正文按段落边界截断（不切进代码围栏内部）。
func truncateMarkdown(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, "\n\n"); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "\n\n……（站内存档截断，阅读全文请前往原文）"
}

func attrOr(s *goquery.Selection, key string) string {
	v, ok := s.Attr(key)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// escapeURL 链接/图片地址里的空格与括号百分号转义（Markdown 行内语法冲突字符）。
func escapeURL(u string) string {
	u = strings.ReplaceAll(u, " ", "%20")
	u = strings.ReplaceAll(u, "(", "%28")
	u = strings.ReplaceAll(u, ")", "%29")
	return u
}
