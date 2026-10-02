package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/prompts"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// FusionStats 融合链接统计。
type FusionStats struct {
	NewsConsidered int      `json:"newsConsidered"`
	ProjectsConsidered int  `json:"projectsConsidered"`
	Links          int      `json:"links"`
	LLMErrors      int      `json:"llmErrors"`
}

// fusionMinConfidence 低于该置信度的链接不建立。
const fusionMinConfidence = 0.7

// LinkFusion 融合用例："双重热点"的交汇点——判断哪些 AI 资讯事件
// 在报道哪些 GitHub 热门项目（如官方发布博客 × 项目仓库互相印证）。
// 建立链接的事件热度获得 ×1.25 加成（在 RankStories 中生效）。
func LinkFusion(ctx context.Context, d Deps, topNews, topProjects int) (FusionStats, error) {
	var stats FusionStats
	now := d.Clock.Now()

	stories, err := d.Stories.Active(ctx, now.Add(-48*time.Hour))
	if err != nil {
		return stats, fmt.Errorf("读取活跃事件: %w", err)
	}
	var news []*story.Story
	for _, s := range stories {
		if s.Kind == story.KindNews {
			news = append(news, s)
		}
	}
	if len(news) > topNews {
		news = news[:topNews]
	}
	projects, err := d.Projects.All(ctx)
	if err != nil {
		return stats, fmt.Errorf("读取项目: %w", err)
	}
	if len(projects) > topProjects {
		projects = projects[:topProjects]
	}
	stats.NewsConsidered = len(news)
	stats.ProjectsConsidered = len(projects)
	if len(news) == 0 || len(projects) == 0 {
		return stats, nil
	}

	var sb strings.Builder
	for _, s := range news {
		fmt.Fprintf(&sb, "[%s] %s\n", s.ID, s.TitleZh)
	}
	var pb strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&pb, "[%s] %s — %s\n", p.FullName, p.Description, p.Language)
	}

	user := prompts.RenderPrompt(prompts.FusionLink, "STORIES", sb.String())
	user = prompts.RenderPrompt(user, "PROJECTS", pb.String())
	raw, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.1)
	if err != nil {
		stats.LLMErrors++
		return stats, fmt.Errorf("融合判断调用: %w", err)
	}
	var out struct {
		Links []struct {
			StoryID    string  `json:"storyId"`
			FullName   string  `json:"fullName"`
			Confidence float64 `json:"confidence"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		stats.LLMErrors++
		return stats, fmt.Errorf("融合 JSON 解析: %w", err)
	}

	// 只接受真实存在的 storyId 与 fullName
	validNews := map[string]bool{}
	for _, s := range news {
		validNews[s.ID] = true
	}
	validProj := map[string]bool{}
	for _, p := range projects {
		validProj[p.FullName] = true
	}
	perStory := map[string][]string{}
	for _, l := range out.Links {
		if !validNews[l.StoryID] || !validProj[l.FullName] || l.Confidence < fusionMinConfidence {
			continue
		}
		perStory[l.StoryID] = append(perStory[l.StoryID], l.FullName)
	}
	for sid, fulls := range perStory {
		if err := d.Stories.LinkProjects(ctx, sid, fulls); err != nil {
			log.Printf("[fusion] 链接失败 %s: %v", sid, err)
			continue
		}
		stats.Links += len(fulls)
	}
	return stats, nil
}
