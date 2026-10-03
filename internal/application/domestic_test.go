package application

import (
	"testing"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/item"
)

func hotItem(id, sourceID, title string, rank int, fetched time.Time) item.Item {
	it, _ := item.New(id, sourceID, "T2", "https://example.com/"+id, title, time.Time{}, fetched)
	it.Selection.Stage = item.StageHotBoard
	it.Meta = map[string]string{"rank": itoa(rank)}
	return *it
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// 同一事件被两个信源报道 → 共振入榜，热度高于任何单源条目。
func TestClusterDomesticResonance(t *testing.T) {
	now := time.Now()
	items := []item.Item{
		hotItem("a1", "hot-baidu-realtime", "国足客场战胜巴林队", 1, now.Add(-1*time.Hour)),
		hotItem("a2", "hot-weibo-search", "国足客场战胜巴林", 3, now.Add(-30*time.Minute)),
		hotItem("b1", "hot-baidu-realtime", "某明星官宣新电影上映", 10, now.Add(-1*time.Hour)),
	}
	clusters := clusterDomestic(items, now)
	if len(clusters) != 2 {
		t.Fatalf("期望 2 个簇（共振簇+单源兜底），实际 %d", len(clusters))
	}
	top := clusters[0]
	if len(top.sources) != 2 {
		t.Fatalf("期望榜首为双源共振，实际 %d 源", len(top.sources))
	}
	if top.members[0].Title != "国足客场战胜巴林队" && top.members[1].Title != "国足客场战胜巴林队" {
		t.Fatalf("榜首簇成员不对: %+v", top.members)
	}
	if top.heat <= 100 {
		t.Fatalf("共振热度应高于单源 rank1（≈100），实际 %.1f", top.heat)
	}
}

// 短标题阈值更高：泛词不误并。
func TestClusterDomesticShortTitle(t *testing.T) {
	now := time.Now()
	items := []item.Item{
		hotItem("s1", "hot-baidu-realtime", "金价大涨", 2, now.Add(-1*time.Hour)),
		hotItem("s2", "hot-weibo-search", "银价大跌", 5, now.Add(-1*time.Hour)),
	}
	clusters := clusterDomestic(items, now)
	if len(clusters) != 2 {
		t.Fatalf("短标题不应合并，期望 2 个簇，实际 %d", len(clusters))
	}
}

// 榜位分：rank1 ≈ 100，rank4 ≈ 50；时间衰减：6h 减半。
func TestClusterDomesticScoring(t *testing.T) {
	now := time.Now()
	items := []item.Item{
		hotItem("f1", "hot-baidu-realtime", "甲事件最新进展", 1, now),
		hotItem("f2", "hot-weibo-search", "乙事件完全不同的标题", 1, now.Add(-6*time.Hour)),
	}
	clusters := clusterDomestic(items, now)
	if len(clusters) != 2 {
		t.Fatalf("期望 2 个簇，实际 %d", len(clusters))
	}
	fresh, aged := clusters[0], clusters[1]
	if fresh.heat < 99 || fresh.heat > 101 {
		t.Fatalf("rank1 即时热度应≈100，实际 %.1f", fresh.heat)
	}
	if aged.heat < 49 || aged.heat > 51 {
		t.Fatalf("rank1 6h 前热度应≈50（半衰），实际 %.1f", aged.heat)
	}
}

// 无 rank 元数据按 99 处理，不会 panic。
func TestClusterDomesticMissingRank(t *testing.T) {
	now := time.Now()
	it := hotItem("m1", "hot-baidu-realtime", "无榜位元数据的事件", 0, now)
	delete(it.Meta, "rank")
	clusters := clusterDomestic([]item.Item{it}, now)
	if len(clusters) != 1 || clusters[0].heat <= 0 {
		t.Fatalf("缺 rank 时应兜底 99 且热度>0: %+v", clusters)
	}
}
