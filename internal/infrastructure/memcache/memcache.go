// Package memcache 提供进程内 TTL 缓存 + 请求合并在途去重（singleflight 语义）。
//
// 为什么不用外部 Redis：本服务是单二进制 + 单实例 + SQLite 的读多写少架构，
// 进程内缓存零网络跳数、零序列化开销、无额外运维与内存常驻进程；未来若扩为
// 多实例共享缓存再引入 Redis 不迟。API 层的"Redis 类"缓存能力由此包承担。
package memcache

import (
	"sync"
	"time"
)

// Cache 并发安全的 TTL 缓存。惰性过期 + 写入时批量清理，容量达上限时
// 驱逐最先过期的条目（无热点统计，读多写少的 API 场景足够）。
type Cache struct {
	mu       sync.RWMutex
	m        map[string]entry
	maxEntry int
	inflight inflight
}

type entry struct {
	val     any
	expires time.Time
}

// New 创建缓存；maxEntry <= 0 时取默认 4096。
func New(maxEntry int) *Cache {
	if maxEntry <= 0 {
		maxEntry = 4096
	}
	return &Cache{m: make(map[string]entry), maxEntry: maxEntry}
}

// Get 取值；过期或不存在返回 (nil, false)。
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.val, true
}

// Set 写入。写入路径顺带做一次低成本过期清扫（均摊）。
func (c *Cache) Set(key string, val any, ttl time.Duration) {
	now := time.Now()
	exp := now.Add(ttl)
	c.mu.Lock()
	c.m[key] = entry{val: val, expires: exp}
	if len(c.m) > c.maxEntry {
		c.evictLocked(now)
	}
	c.mu.Unlock()
}

// evictLocked 先清过期；仍满则驱逐最先将过期的 live 条目（近似 LRU 的次优解，
// 但无需维护访问序）。调用方须持写锁。
func (c *Cache) evictLocked(now time.Time) {
	for k, e := range c.m {
		if now.After(e.expires) {
			delete(c.m, k)
		}
	}
	for len(c.m) > c.maxEntry {
		var (
			oldestKey    string
			oldestExpiry time.Time
			first        = true
		)
		for k, e := range c.m {
			if first || e.expires.Before(oldestExpiry) {
				oldestKey, oldestExpiry, first = k, e.expires, false
			}
		}
		if first {
			break
		}
		delete(c.m, oldestKey)
	}
}

// Len 当前条目数（含未过期），供管理端诊断。
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}

// GetOrLoad 缓存读穿透：命中直接返回；未命中则同 key 并发请求合并为一次
// loader 调用（防缓存击穿），其余等待共享结果。loader 返回错误时不缓存。
func (c *Cache) GetOrLoad(key string, ttl time.Duration, load func() (any, error)) (any, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	done, isLeader := c.inflight.start(key)
	if !isLeader {
		<-done // 已有同 key 请求在跑，等它写缓存后走正常读
		if v, ok := c.Get(key); ok {
			return v, nil
		}
		// 前驱失败未落缓存：降级自己再调一次 loader（慢路径兜底，不递归合并）
		v, err := load()
		if err == nil {
			c.Set(key, v, ttl)
		}
		return v, err
	}
	v, err := load() // 首个到达者执行
	c.inflight.finish(key)
	if err != nil {
		return nil, err
	}
	c.Set(key, v, ttl)
	return v, nil
}

// inflight 同 key 在途请求去重。map[string]chan struct{}，零依赖实现
// singleflight 语义：首个到达者持锁创建 channel 并执行，后来者等待 channel。
type inflight struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}

// start 返回 (done, isLeader)。isLeader=false 时 done 是要等待的通道。
func (f *inflight) start(key string) (chan struct{}, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = make(map[string]chan struct{})
	}
	if ch, ok := f.m[key]; ok {
		return ch, false
	}
	ch := make(chan struct{})
	f.m[key] = ch
	return ch, true
}

func (f *inflight) finish(key string) {
	f.mu.Lock()
	if ch, ok := f.m[key]; ok {
		close(ch)
		delete(f.m, key)
	}
	f.mu.Unlock()
}
