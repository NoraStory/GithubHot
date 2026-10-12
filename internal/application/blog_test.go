package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// blogFetcher 产出一条博客文章（URL 带信源 ID 以免两信源判重相撞）。
type blogFetcher struct{ content string }

func (f blogFetcher) Kind() source.Kind { return source.KindRSS }

func (f blogFetcher) Fetch(_ context.Context, s source.Source, now time.Time) ([]FetchedItem, error) {
	return []FetchedItem{{
		URL:         "https://blog.csdn.net/storyseekee/article/details/" + s.ID,
		Title:       "测试文章 " + s.ID,
		Summary:     "这是摘要",
		Content:     f.content,
		PublishedAt: now.Add(-time.Hour),
	}}, nil
}

// 博客源条目进专用 blog 阶段；RSS 源条目行为不变（仍进 new）。
func TestCollectBlogStageAssignment(t *testing.T) {
	now := time.Now()
	items := newMemItems()
	d := Deps{
		Items:    items,
		Sources:  newMemSources(),
		Fetchers: fakeRegistry{f: blogFetcher{content: ""}},
		Clock:    fixedClock{t: now},
	}
	blogSrc := source.Source{ID: "b1", Kind: source.KindBlog, Config: map[string]string{"url": "x"}, Tier: source.TierFirstParty, IntervalMinutes: 60}
	rssSrc := source.Source{ID: "r1", Kind: source.KindRSS, Config: map[string]string{"url": "x"}, Tier: source.TierMedia, IntervalMinutes: 60}
	for _, src := range []source.Source{blogSrc, rssSrc} {
		if _, _, err := collectOne(context.Background(), d, src, now); err != nil {
			t.Fatalf("collectOne %s: %v", src.ID, err)
		}
	}
	all, err := items.ByStage(context.Background(), []item.Stage{item.StageBlog, item.StageNew}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("应有 2 条（blog+new 各一），得到 %d", len(all))
	}
	for _, it := range all {
		if it.SourceID == "b1" && it.Selection.Stage != item.StageBlog {
			t.Errorf("博客源条目应为 blog 阶段，得到 %s", it.Selection.Stage)
		}
		if it.SourceID == "r1" && it.Selection.Stage != item.StageNew {
			t.Errorf("RSS 源条目应为 new 阶段，得到 %s", it.Selection.Stage)
		}
	}
}

// 内容回填自愈：旧行当年全文抓取失败（content 空），下一轮抓到正文自动补齐。
func TestCollectBlogContentBackfill(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	items := newMemItems()
	url := "https://blog.csdn.net/storyseekee/article/details/csdn-blog" // 与 blogFetcher 产出（base+信源ID）一致
	key, err := shared.DedupeKey(url)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := item.New(key, "csdn-blog", "T1", url, "旧文章", now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	stored.Selection.Stage = item.StageBlog
	if _, err := items.Upsert(ctx, *stored); err != nil {
		t.Fatal(err)
	}

	d := Deps{
		Items:    items,
		Sources:  newMemSources(),
		Fetchers: fakeRegistry{f: blogFetcher{content: "## 正文标题"}},
		Clock:    fixedClock{t: now},
	}
	blogSrc := source.Source{ID: "csdn-blog", Kind: source.KindBlog, Config: map[string]string{"url": "x"}, IntervalMinutes: 60}
	inserted, dupes, err := collectOne(ctx, d, blogSrc, now)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 0 || dupes != 1 {
		t.Fatalf("预期判重（inserted=0, dupes=1），得到 inserted=%d dupes=%d", inserted, dupes)
	}
	got, err := items.FindByIDs(ctx, []string{key})
	if err != nil || len(got) != 1 {
		t.Fatalf("取回条目失败: %v", err)
	}
	if got[0].Content != "## 正文标题" {
		t.Errorf("正文未回填，得到 %q", got[0].Content)
	}
}

// 博客视图：只含 blog 阶段条目，HasContent 如实标记（详情页据此降级）。
func TestBuildBlogView(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	items := newMemItems()
	mk := func(id, url, title, content string) {
		it, err := item.New(id, "csdn-blog", "T1", url, title, now.Add(-time.Hour), now)
		if err != nil {
			t.Fatal(err)
		}
		it.Summary = "摘要 " + title
		it.Content = content
		it.Selection.Stage = item.StageBlog
		if _, err := items.Upsert(ctx, *it); err != nil {
			t.Fatal(err)
		}
	}
	mk("k1", "https://blog.csdn.net/a", "有正文的文章", "## 标题")
	mk("k2", "https://blog.csdn.net/b", "没正文的文章", "")
	// 干扰项：普通资讯条目不应出现在博客视图
	other, _ := item.New("k3", "rss-x", "T2", "https://news.example.com/c", "普通资讯", now, now)
	other.Selection.Stage = item.StageNew
	items.Upsert(ctx, *other)

	d := Deps{Items: items, Clock: fixedClock{t: now}}
	v, err := BuildBlogView(ctx, d, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Items) != 2 {
		t.Fatalf("博客视图应有 2 条，得到 %d", len(v.Items))
	}
	seen := map[string]BlogRow{}
	for _, r := range v.Items {
		seen[r.ID] = r
	}
	if !seen["k1"].HasContent || seen["k2"].HasContent {
		t.Errorf("HasContent 标记错误: %+v", seen)
	}
	if seen["k1"].Summary == "" {
		t.Errorf("列表行应携带摘要")
	}
}

// RefreshMyRepos 写快照；BuildMyReposView 原创在前、fork 在后，与 push 时间无关。
func TestRefreshAndBuildMyRepos(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	settings := newMemSettings()
	d := Deps{
		GitHubOwner: "tester",
		GitHub:      fakeGitHub{},
		Settings:    settings,
		Clock:       fixedClock{t: now},
	}
	if err := RefreshMyRepos(ctx, d); err != nil {
		t.Fatalf("RefreshMyRepos: %v", err)
	}
	blob, _ := settings.Get(ctx, settingsKeyMyRepos)
	if !strings.Contains(blob, `"tester/my-site"`) {
		t.Errorf("快照未写入仓库数据: %s", blob)
	}

	v, err := BuildMyReposView(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if v.Owner != "tester" || len(v.Repos) != 2 {
		t.Fatalf("视图字段错误: owner=%s repos=%d", v.Owner, len(v.Repos))
	}
	// fake 中 fork 的 push 时间更新，但原创必须排前面
	if v.Repos[0].IsFork {
		t.Errorf("原创仓库应排在 fork 之前: %+v", v.Repos)
	}
	if v.Repos[0].FullName != "tester/my-site" {
		t.Errorf("排序结果错误: %+v", v.Repos)
	}

	// owner 为空 = 功能停用，不写快照不报错
	d2 := Deps{GitHubOwner: "", GitHub: fakeGitHub{}, Settings: newMemSettings(), Clock: fixedClock{t: now}}
	if err := RefreshMyRepos(ctx, d2); err != nil {
		t.Errorf("空 owner 应静默跳过，得到 %v", err)
	}
}
