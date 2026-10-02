package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
	"github.com/NoraStory/GithubHot/internal/domain/prompts"
	"github.com/NoraStory/GithubHot/internal/domain/shared"
	"github.com/NoraStory/GithubHot/internal/domain/story"
)

// ClusterStats 聚簇统计。
type ClusterStats struct {
	Written    int `json:"written"`
	NewStories int `json:"newStories"`
	Merged     int `json:"merged"`
	LLMErrors  int `json:"llmErrors"`
}

// 聚簇参数：候选召回阈值与裁决规则。
const (
	candidateThreshold  = 0.30 // 词面/向量相似度超过该值才送 LLM 裁决
	autoMergeSimilarity = 0.82 // 向量相似度极高时跳过 LLM 直接合并
)

// ClusterIntoStories 聚簇用例：把已写作的资料聚成事件。
// 候选召回：优先语义向量（余弦），未配置向量模型时退化为词面相似度；
// 合并裁决：LLM 判断 sameEvent / followUp，拿不准的不合并（宁拆错不合并错）。
func ClusterIntoStories(ctx context.Context, d Deps) (ClusterStats, error) {
	var stats ClusterStats
	written, err := d.Items.ByStage(ctx, []item.Stage{item.StageWritten}, 200)
	if err != nil {
		return stats, fmt.Errorf("读取已写作条目: %w", err)
	}
	if len(written) == 0 {
		return stats, nil
	}
	stats.Written = len(written)
	now := d.Clock.Now()

	since := now.Add(-48 * time.Hour)
	active, err := d.Stories.Active(ctx, since)
	if err != nil {
		return stats, fmt.Errorf("读取活跃事件: %w", err)
	}
	stories := map[string]*story.Story{}
	for _, s := range active {
		cp := s
		stories[cp.ID] = cp
	}

	vectors := tryEmbed(ctx, d, written)

	for _, it := range written {
		text := it.Selection.TitleZh + " " + it.Selection.SummaryZh

		var target *story.Story
		bestSim := 0.0
		for _, s := range stories {
			sim := candidateSim(vectors, it.ID, s, text)
			if sim > bestSim {
				bestSim = sim
				target = s
			}
		}

		shouldCreate := target == nil || bestSim < candidateThreshold
		if !shouldCreate {
			// 极高向量相似度直接合并；其余送 LLM 裁决，拿不准不合并
			if vectors != nil && bestSim >= autoMergeSimilarity {
				shouldCreate = false
			} else {
				same, conf, jerr := judgeMerge(ctx, d, target, it)
				if jerr != nil {
					stats.LLMErrors++
					log.Printf("[cluster] 裁决失败（按不合并处理）: %v", jerr)
					shouldCreate = true
				} else {
					shouldCreate = !(same && conf >= 0.6)
				}
			}
		}

		if shouldCreate {
			ns, nerr := story.NewNews(shared.NewID("story"), memberOf(it), now)
			if nerr != nil {
				continue
			}
			if serr := d.Stories.Save(ctx, ns); serr != nil {
				log.Printf("[cluster] 新事件入库失败: %v", serr)
				continue
			}
			stories[ns.ID] = ns
			stats.NewStories++
			continue
		}

		existing, gerr := d.Stories.FindByID(ctx, target.ID)
		if gerr != nil {
			continue
		}
		existing.Merge(mergeStoryFromItem(existing, it), now)
		if serr := d.Stories.Save(ctx, existing); serr != nil {
			log.Printf("[cluster] 合并保存失败: %v", serr)
			continue
		}
		stats.Merged++
	}
	return stats, nil
}

// judgeMerge LLM 裁决是否同一事件。
func judgeMerge(ctx context.Context, d Deps, s *story.Story, it item.Item) (same bool, conf float64, err error) {
	a := s.TitleZh + "\n" + s.SummaryZh
	if strings.TrimSpace(a) == "" && len(s.Members) > 0 {
		a = s.Members[0].TitleZh + "\n" + s.Members[0].SummaryZh
	}
	b := it.Selection.TitleZh + "\n" + it.Selection.SummaryZh
	user := prompts.RenderPrompt(prompts.ClusterJudge, "A", a)
	user = prompts.RenderPrompt(user, "B", b)
	raw, err := d.LLM.ChatJSON(ctx, prompts.System, user, d.LLM.ModelA(), 0.1)
	if err != nil {
		return false, 0, err
	}
	var out struct {
		SameEvent  bool    `json:"sameEvent"`
		FollowUp   bool    `json:"followUp"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return false, 0, fmt.Errorf("裁决 JSON 解析: %w", err)
	}
	return out.SameEvent || out.FollowUp, out.Confidence, nil
}

// memberOf 条目 → 事件成员。
func memberOf(it item.Item) story.Member {
	return story.Member{
		ItemID:      it.ID,
		URL:         it.URL,
		TitleZh:     it.Selection.TitleZh,
		SummaryZh:   it.Selection.SummaryZh,
		SourceID:    it.SourceID,
		Domain:      story.DomainOf(it.URL),
		PublishedAt: it.PublishedAt,
	}
}

// mergeStoryFromItem 构造只含单个成员的临时事件供 Merge 吸收。
func mergeStoryFromItem(s *story.Story, it item.Item) *story.Story {
	ns, _ := story.NewNews("merge-temp", memberOf(it), s.FirstSeenAt)
	return ns
}

// candidateSim 候选相似度：向量可用用余弦，否则词面 Jaccard。
func candidateSim(vectors map[string][]float32, itemID string, s *story.Story, text string) float64 {
	if v, ok := vectors[itemID]; ok && len(v) > 0 {
		var best float64
		for _, m := range s.Members {
			if mv, ok := vectors[m.ItemID]; ok && len(mv) > 0 {
				if c := story.Cosine(v, mv); c > best {
					best = c
				}
			}
		}
		if best > 0 {
			return best
		}
	}
	var best float64
	for _, m := range s.Members {
		if sim := story.LexicalSimilarity(text, m.TitleZh+" "+m.SummaryZh); sim > best {
			best = sim
		}
	}
	return best
}

// tryEmbed 尝试批量向量；未配置或失败返回 nil（静默降级）。
func tryEmbed(ctx context.Context, d Deps, items []item.Item) map[string][]float32 {
	if d.LLM == nil {
		return nil
	}
	texts := make([]string, 0, len(items))
	for _, it := range items {
		texts = append(texts, it.Selection.TitleZh+" "+it.Selection.SummaryZh)
	}
	vecs, err := d.LLM.Embed(ctx, texts)
	if err != nil {
		if err != ErrEmbeddingsUnsupported {
			log.Printf("[cluster] 向量召回不可用，退化为词面相似度: %v", err)
		}
		return nil
	}
	out := make(map[string][]float32, len(items))
	for i, it := range items {
		if i < len(vecs) {
			out[it.ID] = vecs[i]
		}
	}
	return out
}
