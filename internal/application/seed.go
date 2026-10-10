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
		// 第二批（2026-10 新增，实测可用）：中文源提高同事件多源覆盖，"N 个来源"更厚
		mk("rss-qbitai", "量子位", source.KindRSS, source.TierMedia, "https://www.qbitai.com/feed", "媒体", "中文"),
		mk("rss-infoq-cn", "InfoQ 中文", source.KindRSS, source.TierMedia, "https://www.infoq.cn/feed", "媒体", "中文", "开发者"),
		mk("rss-sspai", "少数派", source.KindRSS, source.TierMedia, "https://sspai.com/feed", "个人", "中文", "工具"),
		mk("rss-wired-ai", "WIRED AI", source.KindRSS, source.TierMedia, "https://www.wired.com/feed/tag/ai/latest/rss", "媒体"),
		mk("rss-mit-tr", "MIT Technology Review", source.KindRSS, source.TierMedia, "https://www.technologyreview.com/feed/", "媒体", "研究"),
		mk("rss-ars-lab", "Ars Technica Lab", source.KindRSS, source.TierMedia, "https://feeds.arstechnica.com/arstechnica/technology-lab", "媒体"),
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

	scriptPush := source.Source{
		ID:              "script-push",
		Name:            "脚本推送（CLI/API 写入）",
		Kind:            source.KindScript,
		Config:          map[string]string{},
		Tier:            source.TierFirstParty,
		Tags:            []string{"自有"},
		IntervalMinutes: 1440,
		Enabled:         true,
		CreatedAt:       now,
	}

	// 国内热榜（hot_board 抓取器，国内直连无需代理）：百度/微博/网易/腾讯四源冗余，
	// rank/heat 元数据随条目入库，供多源共振热度算法使用。
	mkHot := func(id, name, board string, minutes int) source.Source {
		return source.Source{
			ID:   id,
			Name: name,
			Kind: source.KindHotBoard,
			Config: map[string]string{
				"board":     board,
				"max_items": "30",
			},
			Tier:            source.TierMedia,
			Tags:            []string{"国内", "热榜", "轻管道"},
			IntervalMinutes: minutes,
			Enabled:         true,
			CreatedAt:       now,
		}
	}
	// 榜单型 RSS（board=rss）：国内资讯站官方 feed，feed 顺序即榜位，同样走轻管道。
	mkHotRSS := func(id, name, url string, minutes int) source.Source {
		s := mkHot(id, name, "rss", minutes)
		s.Config["url"] = url
		return s
	}
	domestic := []source.Source{
		mkHot("hot-baidu-realtime", "百度热搜", "baidu", 15),
		mkHot("hot-weibo-search", "微博热搜", "weibo", 15),
		mkHot("hot-netease-rank", "网易新闻榜", "netease", 15),
		mkHot("hot-tencent-rank", "腾讯新闻榜", "tencent", 15),
		mkHot("hot-bilibili-rank", "bilibili 热门榜", "bilibili", 30),
		mkHotRSS("hot-ithome", "IT之家", "https://www.ithome.com/rss/", 30),
		mkHotRSS("hot-tmtpost", "钛媒体", "https://www.tmtpost.com/rss.xml", 30),
		mkHotRSS("hot-ifanr", "爱范儿", "https://www.ifanr.com/feed", 30),
		mkHotRSS("hot-geekpark", "极客公园", "https://www.geekpark.net/rss", 60),
		mkHotRSS("hot-iplaysoft", "异次元软件", "https://feed.iplaysoft.com/", 60),
		// 第三批（2026-10 第二轮扩充）：逐个 curl 实测——feed 200 且文章页免登录
		// （点击跳转无登录墙，知乎/抖音类登录墙源一律不接；V2EX 国内被墙不接；
		// 机器之心/36氪/虎嗅/cnBeta 的官方 feed 已停，不接失效源）。
		mkHotRSS("hot-solidot", "Solidot", "https://www.solidot.org/index.rss", 60),
		mkHotRSS("hot-oschina", "开源中国资讯", "https://www.oschina.net/news/rss", 60),
		mkHotRSS("hot-appinn", "小众软件", "https://www.appinn.com/feed/", 120),
		mkHotRSS("hot-leiphone", "雷峰网", "https://www.leiphone.com/feed", 90),
		// 阮一峰博客服务器偏慢（文章页实测 200 但 >25s），放长抓取间隔防超时
		mkHotRSS("hot-ruanyifeng", "阮一峰科技爱好者周刊", "https://www.ruanyifeng.com/blog/atom.xml", 240),
	}

	return append(append(rss, hn, ghSearch, ghTrend, scriptPush), domestic...)
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
