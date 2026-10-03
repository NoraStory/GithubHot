package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/domain/digest"
)

// rssItem RSS 条目。
type rssItem struct {
	Title       string
	Link        string
	GUID        string
	PubDate     time.Time
	Description string
}

// buildRSS 生成 RSS 2.0 文档（手工转义，无外部依赖）。
func buildRSS(channelTitle, channelLink, channelDesc string, items []rssItem) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0">` + "\n<channel>\n")
	fmt.Fprintf(&b, "<title>%s</title>\n", xmlEscape(channelTitle))
	fmt.Fprintf(&b, "<link>%s</link>\n", xmlEscape(channelLink))
	fmt.Fprintf(&b, "<description>%s</description>\n", xmlEscape(channelDesc))
	fmt.Fprintf(&b, "<lastBuildDate>%s</lastBuildDate>\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "<generator>GithubHot</generator>\n")
	for _, it := range items {
		b.WriteString("<item>\n")
		fmt.Fprintf(&b, "<title>%s</title>\n", xmlEscape(it.Title))
		fmt.Fprintf(&b, "<link>%s</link>\n", xmlEscape(it.Link))
		if it.GUID != "" {
			fmt.Fprintf(&b, "<guid isPermaLink=\"false\">%s</guid>\n", xmlEscape(it.GUID))
		}
		if !it.PubDate.IsZero() {
			fmt.Fprintf(&b, "<pubDate>%s</pubDate>\n", it.PubDate.UTC().Format(time.RFC1123Z))
		}
		fmt.Fprintf(&b, "<description>%s</description>\n", xmlEscape(it.Description))
		b.WriteString("</item>\n")
	}
	b.WriteString("</channel>\n</rss>\n")
	return b.String()
}

// xmlEscape XML 文本转义。
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
	)
	return r.Replace(s)
}

// writeRSS 设置响应头并写出。
func writeRSS(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

// feedNews AI 资讯精选 feed：48h 内热度 Top 30 事件。
func (s *Server) feedNews(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	items := make([]rssItem, 0, len(v.News))
	for _, n := range v.News {
		desc := n.SummaryZh
		if n.Overview != "" {
			desc = "【综述】" + n.Overview + "\n" + desc
		}
		if len(n.SourceNames) > 0 {
			desc += "\n来源: " + strings.Join(n.SourceNames, "、")
		}
		items = append(items, rssItem{
			Title: n.TitleZh, Link: n.URL, GUID: "githubhot-story-" + n.StoryID,
			PubDate: v.Generated, Description: desc,
		})
	}
	writeRSS(w, buildRSS("GithubHot · AI 资讯热点", s.baseURL(r), "AI 资讯事件热度榜（按独立来源与时间衰减计算）", items))
}

// feedGitHub GitHub 项目榜 feed。
func (s *Server) feedGitHub(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildView()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	items := make([]rssItem, 0, len(v.GitHub))
	for _, p := range v.GitHub {
		desc := fmt.Sprintf("24h +%d★ · 热度 %.1f", p.StarsGained, p.Hotness)
		if p.DescriptionZh != "" {
			desc += " · " + p.DescriptionZh
		} else if p.Description != "" {
			desc += " · " + p.Description
		}
		items = append(items, rssItem{
			Title: fmt.Sprintf("%s（+%d★）", p.FullName, p.StarsGained),
			Link:  p.URL, GUID: "githubhot-proj-" + p.FullName,
			PubDate: v.Generated, Description: desc,
		})
	}
	writeRSS(w, buildRSS("GithubHot · GitHub 项目热点", s.baseURL(r), "GitHub 开源项目增长热度榜（快照差分口径）", items))
}

// feedDigest 期刊 feed：最近的日报/周报/月报。
func (s *Server) feedDigest(w http.ResponseWriter, r *http.Request) {
	ctx := s.ctx()
	var items []rssItem
	for _, kind := range []string{"daily", "weekly", "monthly"} {
		list, err := s.Deps.Digests.List(ctx, digestKind(kind), 5)
		if err != nil {
			continue
		}
		for _, dg := range list {
			label := "日报"
			if kind == "weekly" {
				label = "周报"
			} else if kind == "monthly" {
				label = "月报"
			}
			desc := truncStr(dg.Markdown, 900)
			items = append(items, rssItem{
				Title:       fmt.Sprintf("GithubHot %s %s", label, dg.Date),
				Link:        fmt.Sprintf("%s/api/v1/digest/%s?format=raw", s.baseURL(r), dg.Date),
				GUID:        "githubhot-digest-" + dg.Date,
				PubDate:     dg.CreatedAt,
				Description: desc,
			})
		}
	}
	writeRSS(w, buildRSS("GithubHot · 期刊", s.baseURL(r), "每日/每周/每月双热点报告", items))
}

func digestKind(s string) digest.Kind { return digest.Kind(s) }

// baseURL 站点根地址（feed channel link 用）。
func (s *Server) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

var _ = application.HotView{}
