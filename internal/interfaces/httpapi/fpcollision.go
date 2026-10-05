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

	if exp, ok := c.suspect[fp]; ok {
		if now.Before(exp) {
			return true
		}
		delete(c.suspect, fp)
	}

	// 追加并裁剪观察点（按时间顺序保留，越过 TTL 的丢弃）
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

	// 窗口内不同 IP 计数
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
}
