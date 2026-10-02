package application

import (
	"context"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/source"
)

// DefaultSources 默认信源清单：17 个公开 AI 资讯 RSS（沿用 AIHOT 开源示范
// 信源，致谢 KKKKhazix/AIHOT），加上 GitHub 双轨与 Hacker News。
// 这只是开箱示范；正确姿势是往 SQLite 的 sources 表里换成你自己的信源。
func DefaultSources(now time.Time) []source.Source {
	mk := func(id, name string, kind source.Kind, tier source.Tier, url string, tags ...string) source.Source {
		return source.Source{
			ID:              id,
			Name:            name,
			Kind:            kind,
			Config:          map[string]string{"url": url},
			Tier:            tier,
			Tags:            tags,
			IntervalMinutes: 120,
			Enabled:         true,
			CreatedAt:       now,
		}
	}
	rss := []source.Source{
		mk("rss-openai-news", "OpenAI News", source.KindRSS, source.TierFirstParty, "https://openai.com/news/rss.xml", "官方"),
		mk("rss-google-deepmind", "Google DeepMind", source.KindRSS, source.TierFirstParty, "https://deepmind.google/blog/rss.xml", "官方"),
		mk("rss-google-research", "Google Research", source.KindRSS, source.TierFirstParty, "https://research.google/blog/rss/", "官方", "研究"),
		mk("rss-hugging-face", "Hugging Face Blog", source.KindRSS, source.TierFirstParty, "https://huggingface.co/blog/feed.xml", "官方", "开源"),
		mk("rss-microsoft-research", "Microsoft Research", source.KindRSS, source.TierFirstParty, "https://www.microsoft.com/en-us/research/feed/", "官方", "研究"),
		mk("rss-nvidia-blog", "NVIDIA Blog", source.KindRSS, source.TierFirstParty, "https://blogs.nvidia.com/feed/", "官方"),
		mk("rss-aws-ml", "AWS Machine Learning", source.KindRSS, source.TierFirstParty, "https://aws.amazon.com/blogs/machine-learning/feed/", "官方", "教程"),
		mk("rss-github-ai", "GitHub Blog AI", source.KindRSS, source.TierFirstParty, "https://github.blog/ai-and-ml/feed/", "官方"),
		mk("rss-mistral", "Mistral AI", source.KindRSS, source.TierFirstParty, "https://mistral.ai/rss.xml", "官方"),
		mk("rss-bair", "BAIR Blog", source.KindRSS, source.TierFirstParty, "https://bair.berkeley.edu/blog/feed.xml", "研究"),
		mk("rss-the-verge-ai", "The Verge AI", source.KindRSS, source.TierMedia, "https://www.theverge.com/rss/ai-artificial-intelligence/index.xml", "媒体"),
		mk("rss-techcrunch-ai", "TechCrunch AI", source.KindRSS, source.TierMedia, "https://techcrunch.com/category/artificial-intelligence/feed/", "媒体"),
		mk("rss-ars-technica-ai", "Ars Technica AI", source.KindRSS, source.TierMedia, "https://arstechnica.com/ai/feed/", "媒体"),
		mk("rss-mit-tech-review-ai", "MIT Tech Review AI", source.KindRSS, source.TierMedia, "https://www.technologyreview.com/topic/artificial-intelligence/feed", "媒体"),
		mk("rss-the-decoder", "The Decoder", source.KindRSS, source.TierMedia, "https://the-decoder.com/feed/", "媒体"),
		mk("rss-simon-willison", "Simon Willison", source.KindRSS, source.TierMedia, "https://simonwillison.net/atom/everything/", "个人"),
		mk("rss-import-ai", "Import AI", source.KindRSS, source.TierMedia, "https://importai.substack.com/feed", "个人", "周刊"),
	}

	hn := source.Source{
		ID:   "hn-ai-keywords",
		Name: "Hacker News（AI 关键词）",
		Kind: source.KindHackerNews,
		Config: map[string]string{
			"keywords": "ai,llm,gpt,openai,anthropic,claude,gemini,deepseek,llama,machine learning,neural,diffusion,transformer,rag,agent,inference,training",
			"hours":    "24",
		},
		Tier:            source.TierMedia,
		Tags:            []string{"社区"},
		IntervalMinutes: 90,
		Enabled:         true,
		CreatedAt:       now,
	}

	ghSearch := source.Source{
		ID:   "github-search-rising",
		Name: "GitHub 新爆项目（Search API）",
		Kind: source.KindGitHubSearch,
		Config: map[string]string{
			"since_days": "7",
			"min_stars":  "50",
			"per_page":   "25",
		},
		Tier:            source.TierFirstParty,
		Tags:            []string{"开源"},
		IntervalMinutes: 240,
		Enabled:         true,
		CreatedAt:       now,
	}
	ghTrend := source.Source{
		ID:              "github-trending-daily",
		Name:            "GitHub Trending（每日榜）",
		Kind:            source.KindGitHubTrending,
		Config:          map[string]string{},
		Tier:            source.TierFirstParty,
		Tags:            []string{"开源"},
		IntervalMinutes: 240,
		Enabled:         true,
		CreatedAt:       now,
	}

	return append(rss, hn, ghSearch, ghTrend)
}

// SeedSources 首次运行时把默认信源导入仓储；已存在的 ID 不覆盖（用户改过就尊重用户）。
func SeedSources(ctx context.Context, d Deps) (imported int, err error) {
	now := d.Clock.Now()
	for _, s := range DefaultSources(now) {
		if err := s.Validate(); err != nil {
			return imported, fmt.Errorf("默认信源 %s 非法: %w", s.ID, err)
		}
		_, ferr := d.Sources.FindByID(ctx, s.ID)
		if ferr == nil {
			continue
		}
		if serr := d.Sources.Save(ctx, s); serr != nil {
			return imported, fmt.Errorf("导入信源 %s: %w", s.ID, serr)
		}
		imported++
	}
	return imported, nil
}
