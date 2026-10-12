package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
)

// ErrBlogNotFound 博客文章不存在（或该 id 不是 blog 阶段条目）。
var ErrBlogNotFound = errors.New("博客文章不存在")

// BlogRow 博客文章列表行（站内阅读版块）。正文不随列表下发，阅读页按 id 拉详情。
type BlogRow struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	Author      string    `json:"author,omitempty"`
	PublishedAt time.Time `json:"publishedAt"`
	HasContent  bool      `json:"hasContent"` // 站内是否已有可读正文（false = 详情页降级展示摘要）
}

// BlogView 博客版块视图。
type BlogView struct {
	Generated time.Time `json:"generatedAt"`
	Items     []BlogRow `json:"items"`
}

// BlogDetail 单篇博客文章（阅读页数据，Content 为站内加工的纯 Markdown）。
type BlogDetail struct {
	BlogRow
	Content string `json:"content"`
}

// blogBoardSize 博客列表展示上限（文章按发布时间倒序持续累积）。
const blogBoardSize = 100

// BuildBlogView 构建博客视图：StageBlog 条目按发布时间倒序。
func BuildBlogView(ctx context.Context, d Deps, limit int) (BlogView, error) {
	if limit <= 0 || limit > blogBoardSize {
		limit = blogBoardSize
	}
	view := BlogView{Generated: d.Clock.Now(), Items: []BlogRow{}}
	items, err := d.Items.ByStage(ctx, []item.Stage{item.StageBlog}, limit)
	if err != nil {
		return view, err
	}
	for _, it := range items {
		view.Items = append(view.Items, BlogRow{
			ID:          it.ID,
			Title:       it.Title,
			URL:         it.URL,
			Summary:     strings.TrimSpace(it.Summary),
			Author:      it.Author,
			PublishedAt: it.PublishedAt,
			HasContent:  strings.TrimSpace(it.Content) != "",
		})
	}
	return view, nil
}

// BuildBlogDetail 按 id 取单篇（含 Markdown 正文）；非 blog 条目按不存在处理。
func BuildBlogDetail(ctx context.Context, d Deps, id string) (BlogDetail, error) {
	var det BlogDetail
	items, err := d.Items.FindByIDs(ctx, []string{id})
	if err != nil {
		return det, err
	}
	if len(items) == 0 || items[0].Selection.Stage != item.StageBlog {
		return det, ErrBlogNotFound
	}
	it := items[0]
	det.BlogRow = BlogRow{
		ID:          it.ID,
		Title:       it.Title,
		URL:         it.URL,
		Summary:     strings.TrimSpace(it.Summary),
		Author:      it.Author,
		PublishedAt: it.PublishedAt,
		HasContent:  strings.TrimSpace(it.Content) != "",
	}
	det.Content = it.Content
	return det, nil
}
