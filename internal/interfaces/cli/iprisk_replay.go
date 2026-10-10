package cli

// IPRiskReplay 风控参数离线回放评估（算法改进 A 批）：
// 读生产库全部违规事件与封禁记录，对 baseline 与候选参数组重放对比，
// 输出召回率 / 误报代理率 / 触发提前量表——风控调参从"手拍"升级为"看数字"。
//
// 口径说明见 internal/domain/iprisk/replay.go 顶部注释（单用户无稀释的保守上界）。
import (
	"context"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/config"
	"github.com/NoraStory/GithubHot/internal/domain/iprisk"
	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
)

func IPRiskReplay(cfg *config.Config, sweep bool) error {
	db, err := sqlite.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()

	evRows, err := db.ListIPEvents(ctx, 500000)
	if err != nil {
		return fmt.Errorf("读取事件: %w", err)
	}
	banRows, err := db.ListBans(ctx)
	if err != nil {
		return fmt.Errorf("读取封禁: %w", err)
	}
	events := make([]iprisk.ReplayEvent, 0, len(evRows))
	var minAt, maxAt time.Time
	for _, r := range evRows {
		events = append(events, iprisk.ReplayEvent{IP: r.IP, Kind: r.Kind, Score: r.Score, At: r.At})
		if minAt.IsZero() || r.At.Before(minAt) {
			minAt = r.At
		}
		if r.At.After(maxAt) {
			maxAt = r.At
		}
	}
	bans := make([]iprisk.ReplayBan, 0, len(banRows))
	for _, r := range banRows {
		bans = append(bans, iprisk.ReplayBan{IP: r.IP, BannedAt: r.BannedAt, Reason: r.Reason})
	}

	fmt.Printf("数据概览：事件 %d 条（%s ~ %s），IP %d 个，封禁标签 %d 条\n",
		len(events), minAt.Format("01-02 15:04"), maxAt.Format("01-02 15:04"), distinctIPs(events), len(bans))
	if len(events) == 0 {
		fmt.Println("无事件可回放")
		return nil
	}

	sets := []iprisk.Params{iprisk.DefaultParams()}
	names := []string{"baseline（阈值100/弱顶40/强顶70/半衰10）"}
	if sweep {
		base := iprisk.DefaultParams()
		variants := []struct {
			name string
			mut  func(*iprisk.Params)
		}{
			{"阈值90", func(p *iprisk.Params) { p.Threshold = 90 }},
			{"阈值110", func(p *iprisk.Params) { p.Threshold = 110 }},
			{"弱顶50/强顶80", func(p *iprisk.Params) { p.CapWeak = 50; p.CapStrong = 80 }},
			{"半衰15/窗口60", func(p *iprisk.Params) { p.HalfLifeMin = 15; p.DecayWindowMin = 60 }},
			{"阈值90+弱顶50", func(p *iprisk.Params) { p.Threshold = 90; p.CapWeak = 50 }},
			{"阈值110+弱顶50", func(p *iprisk.Params) { p.Threshold = 110; p.CapWeak = 50 }},
		}
		for _, v := range variants {
			p := base
			v.mut(&p)
			sets = append(sets, p)
			names = append(names, v.name)
		}
	}

	fmt.Printf("\n%-32s %6s %8s %8s %10s %10s\n", "参数组", "已封", "召回", "误报率", "软封锁误伤", "提前量中位")
	fmt.Println(string(make([]byte, 78)))
	for i, p := range sets {
		results := iprisk.Replay(events, bans, p, 500)
		s := iprisk.Summarize(results, p)
		fmt.Printf("%-32s %6d %7.0f%% %7.1f%% %10d %9.1fmin\n",
			names[i], s.BannedIPs, s.Recall()*100, s.FPProxy()*100, s.SoftFPs, s.MedianLeadMin)
	}
	fmt.Println("\n口径：回放按单用户无稀释处理（保守上界）；召回=模型在真实封禁时刻前触发；")
	fmt.Println("误报率=对从未被封 IP 的触发比例；软封锁误伤=未封 IP 有效分达 0.6×阈值。")
	return nil
}

// distinctIPs 事件流中的独立 IP 数。
func distinctIPs(events []iprisk.ReplayEvent) int {
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.IP] = true
	}
	return len(seen)
}
