package httpapi

import (
	"sync"
	"time"
)

// 指纹碰撞检测。
//
// 背景：浏览器指纹由硬件与软件环境决定，**同型号手机 / 同镜像办公电脑**的指纹高度
// 重合，不同用户会得到同一个 fp。旧实现把这当成"同一台设备"，于是：
//   - 一个用户被封 → 另一台无关设备换到新 IP 时触发连坐（fp-linked）；
//   - 同一 fp 的 IP 列表快速膨胀 → 被当成代理池漂移（fp-churn）。
//
// 判据：真设备换网络是**串行**的（同一时刻只有一个 IP 在用）；指纹碰撞是**并发**的
// （短时间内多个不同 IP 同时在用同一 fp）。因此以"近 10 分钟内不同 IP 数"作为区分信号。
// 命中碰撞标记的指纹立即豁免连坐与漂移升级，直到 TTL 过期。

const (
	collisionWindow  = 10 * time.Minute // 并发观察窗口
	collisionMinIPs  = 4                // 窗口内不同 IP 达到该数即判碰撞
	collisionTTL     = 24 * time.Hour   // 碰撞标记有效期（过期后重新评估）
	collisionMaxKeep = 40               // 每个指纹保留的观察点上限
	collisionMaxFPs  = 4000             // 追踪指纹数上限（防内存膨胀）
)

type fpSighting struct {
	ip string
	at time.Time
}

type fpCollision struct {
	mu        sync.Mutex
	sightings map[string][]fpSighting
	suspect   map[string]time.Time // fp → 标记过期时间
}

func newFPCollision() *fpCollision {
	return &fpCollision{
		sightings: map[string][]fpSighting{},
		suspect:   map[string]time.Time{},
	}
}

// observe 记录一次（fp, ip）观察，返回该指纹是否应视为"碰撞指纹"。
// 判定条件任一成立：
//   - 已有未过期的碰撞标记；
//   - 近 collisionWindow 内出现 ≥ collisionMinIPs 个不同 IP。
func (c *fpCollision) observe(fp, ip string, now time.Time) bool {
	if fp == "" || ip == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已经是suspect，但不要直接返回
	alreadySuspect := false
	if exp, ok := c.suspect[fp]; ok {
		if now.Before(exp) {
			alreadySuspect = true // 记录状态但继续更新观察点
		} else {
			delete(c.suspect, fp)
		}
	}

	// 无论是否已标记，都更新观察点（保持数据新鲜）
	list := c.sightings[fp]
	list = append(list, fpSighting{ip: ip, at: now})
	cut := now.Add(-collisionTTL)
	keep := list[:0]
	for _, s := range list {
		if s.at.After(cut) {
			keep = append(keep, s)
		}
	}
	if len(keep) > collisionMaxKeep {
		keep = keep[len(keep)-collisionMaxKeep:]
	}
	c.sightings[fp] = keep

	// 如果已经是suspect，直接返回
	if alreadySuspect {
		return true
	}

	// 否则检查是否应新标记
	winCut := now.Add(-collisionWindow)
	seen := map[string]bool{}
	for _, s := range keep {
		if s.at.After(winCut) {
			seen[s.ip] = true
		}
	}
	if len(seen) >= collisionMinIPs {
		c.suspect[fp] = now.Add(collisionTTL)
		c.pruneLocked(now)
		return true
	}
	c.pruneLocked(now)
	return false
}

// pruneLocked 清理过期标记；追踪规模超限时清理已无观察点的指纹（调用方持锁）。
func (c *fpCollision) pruneLocked(now time.Time) {
	for fp, exp := range c.suspect {
		if now.After(exp) {
			delete(c.suspect, fp)
		}
	}
	if len(c.sightings) <= collisionMaxFPs {
		return
	}
	
	// 超限时主动清理
	cut := now.Add(-collisionWindow)
	for fp, list := range c.sightings {
		if len(list) == 0 {
			delete(c.sightings, fp)
			continue
		}
		if list[len(list)-1].at.Before(cut) {
			delete(c.sightings, fp)
		}
	}
	
	// 如果清理后仍然超限，清理最旧的 20%
	if len(c.sightings) > collisionMaxFPs*12/10 { // 允许 20% 超限
		type entry struct {
			fp string
			at time.Time
		}
		entries := make([]entry, 0, len(c.sightings))
		for fp, list := range c.sightings {
			if len(list) > 0 {
				entries = append(entries, entry{fp: fp, at: list[len(list)-1].at})
			}
		}
		// 按最后观察时间排序，删除最旧的
		if len(entries) > collisionMaxFPs {
			// 简单策略：删除最旧的 20%
			toDelete := len(entries) - collisionMaxFPs
			for i := 0; i < toDelete && i < len(entries); i++ {
				oldestIdx := 0
				oldestTime := entries[0].at
				for j := 1; j < len(entries); j++ {
					if entries[j].at.Before(oldestTime) {
						oldestIdx = j
						oldestTime = entries[j].at
					}
				}
				delete(c.sightings, entries[oldestIdx].fp)
				// 从 entries 中移除
				entries[oldestIdx] = entries[len(entries)-1]
				entries = entries[:len(entries)-1]
			}
		}
	}
}
