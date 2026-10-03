package application

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/digest"
	"github.com/NoraStory/GithubHot/internal/domain/github"
	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/source"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// ---------- 内存仓储 ----------

type memSources struct {
	mu sync.Mutex
	m  map[string]source.Source
}

func newMemSources() *memSources { return &memSources{m: map[string]source.Source{}} }

func (m *memSources) Save(_ context.Context, s source.Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[s.ID] = s
	return nil
}

func (m *memSources) All(_ context.Context) ([]source.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]source.Source, 0, len(m.m))
	for _, s := range m.m {
		out = append(out, s)
	}
	return out, nil
}

func (m *memSources) FindByID(_ context.Context, id string) (source.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[id]
	if !ok {
		return source.Source{}, fmt.Errorf("not found")
	}
	return s, nil
}

func (m *memSources) MarkFetched(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[id]
	if !ok {
		return nil
	}
	s.LastFetchedAt = &at
	m.m[id] = s
	return nil
}

type memItems struct {
	mu sync.Mutex
	m  map[string]item.Item
}

func newMemItems() *memItems { return &memItems{m: map[string]item.Item{}} }

func (m *memItems) Upsert(_ context.Context, it item.Item) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.m[it.ID]; ok {
		return false, nil
	}
	m.m[it.ID] = it
	return true, nil
}

func (m *memItems) CountSince(_ context.Context, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, it := range m.m {
		if !it.FetchedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *memItems) ByStage(_ context.Context, stages []item.Stage, limit int) ([]item.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[item.Stage]bool{}
	for _, s := range stages {
		want[s] = true
	}
	var out []item.Item
	for _, it := range m.m {
		if want[it.Selection.Stage] {
			out = append(out, it)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memItems) Recent(_ context.Context, limit int) ([]item.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []item.Item
	for _, it := range m.m {
		out = append(out, it)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memItems) HotBoardSince(_ context.Context, since time.Time, limit int) ([]item.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []item.Item
	for _, it := range m.m {
		if it.Selection.Stage == item.StageHotBoard && !it.FetchedAt.Before(since) {
			out = append(out, it)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memItems) UpdateSelection(_ context.Context, id string, sel item.Selection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.m[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	it.Selection = sel
	m.m[id] = it
	return nil
}

func (m *memItems) Search(_ context.Context, q string, limit int) ([]item.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []item.Item
	for _, it := range m.m {
		if it.Selection.Stage == item.StageWritten &&
			(strings.Contains(it.Selection.TitleZh, q) || strings.Contains(it.Title, q)) {
			out = append(out, it)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memItems) FindByIDs(_ context.Context, ids []string) ([]item.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []item.Item
	for _, id := range ids {
		if it, ok := m.m[id]; ok {
			out = append(out, it)
		}
	}
	return out, nil
}

func (m *memItems) PendingContentZh(_ context.Context, limit int) ([]item.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []item.Item
	for _, it := range m.m {
		if (it.Selection.Stage == item.StageWritten || it.Selection.Stage == item.StageClustered) &&
			it.ContentZh == "" && (it.Content != "" || it.Summary != "") {
			out = append(out, it)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memItems) SaveContentZh(_ context.Context, id string, zh string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.m[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	it.ContentZh = zh
	m.m[id] = it
	return nil
}

type memProjects struct {
	mu   sync.Mutex
	m    map[string]github.Project
	snap map[string][]github.Snapshot
}

func newMemProjects() *memProjects {
	return &memProjects{m: map[string]github.Project{}, snap: map[string][]github.Snapshot{}}
}

func (m *memProjects) Upsert(_ context.Context, p github.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[p.FullName] = p
	return nil
}

func (m *memProjects) Touch(_ context.Context, p github.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[p.FullName] = p
	return nil
}

func (m *memProjects) FindByFullName(_ context.Context, fullName string) (*github.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.m[fullName]; ok {
		cp := p
		return &cp, nil
	}
	return nil, nil
}

func (m *memProjects) SaveDescriptionZh(_ context.Context, fullName, zh string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.m[fullName]
	if ok {
		p.DescriptionZh = zh
		m.m[fullName] = p
	}
	return nil
}

func (m *memProjects) All(_ context.Context) ([]github.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]github.Project, 0, len(m.m))
	for _, p := range m.m {
		out = append(out, p)
	}
	return out, nil
}

func (m *memProjects) AddSnapshot(_ context.Context, s github.Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snap[s.FullName] = append(m.snap[s.FullName], s)
	return nil
}

func (m *memProjects) SnapshotsSince(_ context.Context, fullName string, since time.Time) ([]github.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []github.Snapshot
	for _, s := range m.snap[fullName] {
		if !s.At.Before(since) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memProjects) AllSnapshotsSince(_ context.Context, since time.Time) (map[string][]github.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]github.Snapshot{}
	for name, snaps := range m.snap {
		for _, s := range snaps {
			if !s.At.Before(since) {
				out[name] = append(out[name], s)
			}
		}
	}
	return out, nil
}

type memStories struct {
	mu   sync.Mutex
	m    map[string]*story.Story
	hist map[string][]historyRow
}

type historyRow struct {
	at      time.Time
	hotness float64
}

func newMemStories() *memStories {
	return &memStories{m: map[string]*story.Story{}, hist: map[string][]historyRow{}}
}

func (m *memStories) Save(_ context.Context, s *story.Story) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *s
	m.m[s.ID] = &cp
	return nil
}

func (m *memStories) FindByID(_ context.Context, id string) (*story.Story, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.m[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *memStories) Active(_ context.Context, since time.Time) ([]*story.Story, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*story.Story
	for _, s := range m.m {
		if !s.UpdatedAt.Before(since) || !s.FirstSeenAt.Before(since) {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

// ListPage 内存版：按首次收录时间降序分页。
func (m *memStories) ListPage(_ context.Context, kind story.Kind, offset, limit int) ([]*story.Story, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []*story.Story
	for _, s := range m.m {
		if kind != "" && s.Kind != kind {
			continue
		}
		cp := *s
		all = append(all, &cp)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].FirstSeenAt.Equal(all[j].FirstSeenAt) {
			return all[i].FirstSeenAt.After(all[j].FirstSeenAt)
		}
		return all[i].ID < all[j].ID
	})
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *memStories) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, id)
	return nil
}

func (m *memStories) AddHistory(_ context.Context, storyID string, at time.Time, hotness float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hist[storyID] = append(m.hist[storyID], historyRow{at, hotness})
	return nil
}

func (m *memStories) HistoryNear(_ context.Context, storyID string, at time.Time, lookBack time.Duration) (float64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := m.hist[storyID]
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].at.Before(at.Add(-lookBack)) && !rows[i].at.After(at) {
			return rows[i].hotness, true, nil
		}
	}
	return 0, false, nil
}

func (m *memStories) SaveOverview(_ context.Context, storyID string, overview string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.m[storyID]; ok {
		s.Overview = overview
	}
	return nil
}

func (m *memStories) SetManual(_ context.Context, storyID string, manual bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.m[storyID]; ok {
		s.Manual = manual
	}
	return nil
}

func (m *memStories) HotnessHistory(_ context.Context, storyID string, limit int) ([]story.HotnessPoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := m.hist[storyID]
	out := make([]story.HotnessPoint, 0, len(rows))
	for i := len(rows) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, story.HotnessPoint{At: rows[i].at, Hotness: rows[i].hotness})
	}
	return out, nil
}

func (m *memStories) LinkProjects(_ context.Context, storyID string, fullNames []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[storyID]
	if !ok {
		return fmt.Errorf("not found")
	}
	s.Projects = append(s.Projects, fullNames...)
	return nil
}

type memDigests struct{ m map[string]digest.Digest }

func newMemDigests() *memDigests { return &memDigests{m: map[string]digest.Digest{}} }

func (m *memDigests) Save(_ context.Context, d digest.Digest) error { m.m[d.Date] = d; return nil }

func (m *memDigests) FindByDate(_ context.Context, date string) (*digest.Digest, error) {
	if d, ok := m.m[date]; ok {
		return &d, nil
	}
	return nil, nil
}

func (m *memDigests) Latest(_ context.Context, kind digest.Kind) (*digest.Digest, error) {
	var best *digest.Digest
	for k := range m.m {
		d := m.m[k]
		if d.Kind != kind {
			continue
		}
		if best == nil || d.Date > best.Date {
			cp := d
			best = &cp
		}
	}
	return best, nil
}

func (m *memDigests) List(_ context.Context, kind digest.Kind, limit int) ([]digest.Digest, error) {
	var out []digest.Digest
	for k := range m.m {
		d := m.m[k]
		if d.Kind == kind {
			out = append(out, d)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ---------- 假网关 ----------

var itemLineRe = regexp.MustCompile(`^\[([a-f0-9]+)\] `)

// fakeLLM 脚本化大模型：按提示特征返回对应 JSON。
// chatCalls 用原子计数——doubleScore 会并发调用，race 检测器会盯这里。
type fakeLLM struct{ chatCalls atomic.Int64 }

func (f *fakeLLM) ChatJSON(_ context.Context, _, user, _ string, _ float64) (string, error) {
	f.chatCalls.Add(1)
	switch {
	case strings.Contains(user, "入选标准"): // 预筛
		var out strings.Builder
		out.WriteString(`{"results":[`)
		first := true
		for _, line := range strings.Split(user, "\n") {
			m := itemLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			id := m[1]
			if !first {
				out.WriteString(",")
			}
			first = false
			if strings.Contains(line, "垃圾") {
				fmt.Fprintf(&out, `{"id":%q,"pass":false,"reason":"广告"}`, id)
			} else {
				fmt.Fprintf(&out, `{"id":%q,"pass":true,"reason":"ok"}`, id)
			}
		}
		out.WriteString(`]}`)
		return out.String(), nil

	case strings.Contains(user, "打分"): // 双评分
		return `{"score":8.0,"reason":"高分"}`, nil

	case strings.Contains(user, "改写成中文"): // 写作
		title := "默认中文标题"
		switch {
		case strings.Contains(user, "Agent框架"):
			title = "Agent框架2.0发布"
		case strings.Contains(user, "数据库"):
			title = "向量数据库大更新"
		}
		return fmt.Sprintf(`{"titleZh":%q,"summaryZh":"这是摘要。","reasonZh":"值得看","tags":["模型发布"]}`, title), nil

	case strings.Contains(user, "同一件事"): // 聚簇裁决
		// 标题同为主体的判同一事件（fake 规则：双方都含 "Agent框架" 或都含 "数据库"）
		same := false
		if (strings.Contains(user, "Agent框架") && strings.Count(user, "Agent框架") >= 2) ||
			(strings.Contains(user, "数据库") && strings.Count(user, "数据库") >= 2) {
			same = true
		}
		return fmt.Sprintf(`{"sameEvent":%t,"followUp":false,"confidence":0.95}`, same), nil

	case strings.Contains(user, "翻译成简洁中文"): // 项目描述翻译
		var out strings.Builder
		out.WriteString(`{"items":[`)
		first := true
		for _, line := range strings.Split(user, "\n") {
			m := regexp.MustCompile(`^\[([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+)\]`).FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if !first {
				out.WriteString(",")
			}
			first = false
			fmt.Fprintf(&out, `{"fullName":%q,"zh":"中文描述"}`, m[1])
		}
		out.WriteString(`]}`)
		return out.String(), nil

	case strings.Contains(user, "整合成一段事件综述"): // 事件综述
		return `{"overview":"多源报道整合的事件综述。"}`, nil

	case strings.Contains(user, "忠实翻译"): // 原文本地存档翻译
		return `{"zh":"这是原文的忠实中文译文。"}`, nil

	case strings.Contains(user, "GitHub 热门项目"): // 融合链接
		// 从提示中取第一条 story 与第一个项目
		storyID := ""
		project := ""
		for _, line := range strings.Split(user, "\n") {
			if strings.HasPrefix(line, "[story-") && storyID == "" {
				storyID = strings.Trim(strings.Split(line, "]")[0], "[]")
			}
			if m := regexp.MustCompile(`^\[([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+)\]`).FindStringSubmatch(line); m != nil && project == "" {
				project = m[1]
			}
		}
		if storyID == "" || project == "" {
			return `{"links":[]}`, nil
		}
		return fmt.Sprintf(`{"links":[{"storyId":%q,"fullName":%q,"confidence":0.9}]}`, storyID, project), nil
	}
	return `{"results":[]}`, nil
}

func (f *fakeLLM) ModelA() string { return "fake-a" }
func (f *fakeLLM) ModelB() string { return "fake-b" }

func (f *fakeLLM) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, ErrEmbeddingsUnsupported
}

// fakeGitHub 双轨：search 返回一个项目，trending 返回两个（其一与 search 重叠）。
type fakeGitHub struct{}

func (fakeGitHub) SearchNewRising(_ context.Context, _, _, _ int) ([]GitHubRepo, error) {
	return []GitHubRepo{
		{FullName: "openai/agent-kit", HTMLURL: "https://github.com/openai/agent-kit", Description: "Official agent kit", Language: "Python", Stars: 500},
	}, nil
}

func (fakeGitHub) FetchTrending(_ context.Context) ([]GitHubRepo, error) {
	return []GitHubRepo{
		{FullName: "openai/agent-kit", TrendingRank: 1, Stars: 505},
		{FullName: "newbie/tiny-db", HTMLURL: "https://github.com/newbie/tiny-db", Description: "tiny vector db", Language: "Rust", Stars: 300, TrendingRank: 2},
	}, nil
}

// fakeFetcher 固定产出三条资料：两条相关（聚簇应合并）、一条广告（预筛淘汰）。
type fakeFetcher struct{ now time.Time }

func (f fakeFetcher) Kind() source.Kind { return source.KindRSS }

func (f fakeFetcher) Fetch(_ context.Context, _ source.Source, _ time.Time) ([]FetchedItem, error) {
	return []FetchedItem{
		{URL: "https://openai.com/blog/agent-framework", Title: "Agent框架 2.0 发布", Summary: "OpenAI 发布 Agent框架 2.0", PublishedAt: f.now.Add(-2 * time.Hour)},
		{URL: "https://news.example.com/agent-framework-news", Title: "Agent框架 2.0 发布报道", Summary: "Agent框架 2.0 发布的媒体报道", PublishedAt: f.now.Add(-3 * time.Hour)},
		{URL: "https://news.example.com/vector-db", Title: "向量数据库大更新", Summary: "某向量数据库发布新版本", PublishedAt: f.now.Add(-3 * time.Hour)},
		{URL: "https://spam.example.com/ad", Title: "垃圾广告内容", Summary: "买一送一", PublishedAt: f.now.Add(-1 * time.Hour)},
	}, nil
}

type fakeRegistry struct{ f SourceFetcher }

func (r fakeRegistry) Fetcher(_ source.Kind) (SourceFetcher, error) { return r.f, nil }

// dupeVariantFetcher 产出两条同文异参条目（判重用）。
type dupeVariantFetcher struct{}

func (dupeVariantFetcher) Kind() source.Kind { return source.KindRSS }

func (dupeVariantFetcher) Fetch(_ context.Context, _ source.Source, now time.Time) ([]FetchedItem, error) {
	return []FetchedItem{
		{URL: "https://x.com/post?utm_source=a", Title: "同一篇文章", PublishedAt: now},
		{URL: "https://X.com/post/", Title: "同一篇文章", PublishedAt: now},
	}, nil
}

type fakeDigestRenderer struct{}

func (fakeDigestRenderer) Render(_ context.Context, v DigestView) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# 日报 %s\n\n", v.Date)
	for _, p := range v.GitHub {
		fmt.Fprintf(&b, "- GitHub: %s (+%d★, %.1f)\n", p.FullName, p.StarsGained, p.Hotness)
	}
	for _, s := range v.News {
		fmt.Fprintf(&b, "- AI: %s (%.1f)\n", s.TitleZh, s.Hotness)
	}
	for _, f := range v.Fusion {
		fmt.Fprintf(&b, "- 融合: %s × %s\n", f.News.TitleZh, f.Project.FullName)
	}
	return b.String(), nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// githubProject / snapshotOf 测试辅助构造。
func githubProject(t *testing.T, fullName string, stars int, firstSeen time.Time) github.Project {
	t.Helper()
	p, err := github.New(fullName, "", stars, firstSeen)
	if err != nil {
		t.Fatalf("构造项目 %s: %v", fullName, err)
	}
	return *p
}

func snapshotOf(fullName string, stars int, at time.Time) github.Snapshot {
	return github.Snapshot{FullName: fullName, At: at, Stars: stars}
}
