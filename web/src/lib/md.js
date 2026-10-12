// 站内统一的 Markdown 渲染器（marked + GFM）：期刊页与博客阅读页共用同一份
// 配置，保证排版审美同源。全局 .md-viewer 样式定义在 DigestDetail.vue 的非
// scoped 样式块中，随主包加载，所有页面可用。
//
// 安全模型：marked 不转义内嵌 HTML，防线在服务端——入库的 Markdown 由服务端
// 转换器产出（博客正文永不携带 HTML 标签），外部抓取标题经服务端 mdEscape。
import { marked } from 'marked'

marked.setOptions({ gfm: true, breaks: false })
const renderer = {
  // marked v18 的 renderer 接收 token；用 parser.parseInline 展开 cell.tokens，
  // 否则对象会被插值成 [object Object]。
  table(token) {
    const head = `<thead>${token.header.map((cell) => `<th>${this.parser.parseInline(cell.tokens)}</th>`).join('')}</thead>`
    const body = `<tbody>${token.rows.map((row) => `<tr>${row.map((cell) => `<td>${this.parser.parseInline(cell.tokens)}</td>`).join('')}</tr>`).join('')}</tbody>`
    return `<div class="table-wrap"><table>${head}${body}</table></div>`
  }
}
marked.use({ renderer })

export function renderMarkdown(md) {
  return marked.parse(md || '')
}
