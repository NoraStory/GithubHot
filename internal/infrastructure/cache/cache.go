// Package cache 两级响应缓存：L1 进程内（memcache，微秒级）+ L2 Redis（跨重启
// 保温，为未来多实例共享预留）。
//
// 设计红线：
//   - Redis 是加速器不是依赖。REDIS_ADDR 未配置时纯 L1 运行；Redis 宕机/超时
//     自动降级为 L1，任何 Redis 故障不得拖慢请求（所有 L2 操作带短超时，SET
//     异步化）。
//   - 值格式 envelope：首行 Content-Type，其余为响应体——缓存回放时还原正确的
//     Content-Type（/feed/* 是 XML，/api/* 是 JSON，不能硬编码）。
package cache

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/NoraStory/GithubHot/internal/infrastructure/memcache"
)

const (
	redisOpTimeout = 100 * time.Millisecond // L2 读超时：本地 Redis 亚毫秒级，100ms 已是 100 倍余量
	l1BackfillTTL  = 30 * time.Second       // L2 命中回填 L1 的时长（短于原始 TTL，尽快回归源头的失效节奏）
)

// Entry 缓存条目：Content-Type + 响应体。
type Entry struct {
	ContentType string
	Body        []byte
}

// Encode 序列化为 envelope：首行 Content-Type + \n + body。
func (e Entry) Encode() []byte {
	b := make([]byte, 0, len(e.ContentType)+1+len(e.Body))
	b = append(b, e.ContentType...)
	b = append(b, '\n')
	return append(b, e.Body...)
}

// DecodeEntry 解析 envelope；格式非法返回 false。
func DecodeEntry(raw []byte) (Entry, bool) {
	i := bytes.IndexByte(raw, '\n')
	if i <= 0 {
		return Entry{}, false
	}
	return Entry{ContentType: string(raw[:i]), Body: raw[i+1:]}, true
}

// TwoTier 两级缓存。rdb 为 nil 时退化为纯 L1。
type TwoTier struct {
	l1  *memcache.Cache
	rdb redis.UniversalClient
	key string // Redis 键前缀，如 "gh:"

	degraded   sync.Mutex // 串行化降级日志，避免错误风暴刷屏
	lastErrAt  time.Time
	onDegraded func(msg string) // 降级回调（接启动日志），nil 则静默
}

// New 构建两级缓存。redisAddr 为空 → 纯 L1；否则立即 Ping，失败也照常启动
//（后续操作内部再探活，Redis 恢复后自动回归）。
func New(l1 *memcache.Cache, redisAddr, redisPassword string, redisDB int, keyPrefix string, onDegraded func(string)) *TwoTier {
	if l1 == nil {
		l1 = memcache.New(2048)
	}
	t := &TwoTier{l1: l1, key: strings.TrimSuffix(keyPrefix, ":"), onDegraded: onDegraded}
	if t.key != "" {
		t.key += ":"
	}
	if strings.TrimSpace(redisAddr) == "" {
		return t
	}
	t.rdb = redis.NewClient(&redis.Options{
		Addr:        strings.TrimSpace(redisAddr),
		Password:    redisPassword,
		DB:          redisDB,
		DialTimeout: 500 * time.Millisecond,
		ReadTimeout: redisOpTimeout,
		WriteTimeout: redisOpTimeout,
		PoolSize:    8,
	})
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	if err := t.rdb.Ping(ctx).Err(); err != nil {
		t.degrade("Redis Ping 失败，暂时仅 L1 运行: " + err.Error())
	}
	return t
}

// RedisEnabled 是否配置了 L2。
func (t *TwoTier) RedisEnabled() bool { return t.rdb != nil }

// degrade 降级回调节流：同一分钟内只报一次。
func (t *TwoTier) degrade(msg string) {
	if t.onDegraded == nil {
		return
	}
	t.degraded.Lock()
	defer t.degraded.Unlock()
	if time.Since(t.lastErrAt) < time.Minute {
		return
	}
	t.lastErrAt = time.Now()
	t.onDegraded(msg)
}

// Get 查找：L1 → L2（命中回填 L1）。任何 L2 错误静默降级。
func (t *TwoTier) Get(key string) (Entry, bool) {
	if v, ok := t.l1.Get(t.key + key); ok {
		if e, ok := v.(Entry); ok {
			return e, true
		}
		return Entry{}, false
	}
	if t.rdb == nil {
		return Entry{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	raw, err := t.rdb.Get(ctx, t.key+key).Bytes()
	if err != nil {
		if err != redis.Nil {
			t.degrade("Redis GET 失败（降级 L1）: " + err.Error())
		}
		return Entry{}, false
	}
	e, ok := DecodeEntry(raw)
	if !ok {
		return Entry{}, false
	}
	t.l1.Set(t.key+key, e, l1BackfillTTL)
	return e, true
}

// Set 写入 L1（同步）+ L2（异步 fire-and-forget：写 Redis 的延迟不计入请求）。
func (t *TwoTier) Set(key string, e Entry, ttl time.Duration) {
	t.l1.Set(t.key+key, e, ttl)
	if t.rdb == nil {
		return
	}
	raw := e.Encode()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
		defer cancel()
		if err := t.rdb.Set(ctx, t.key+key, raw, ttl).Err(); err != nil {
			t.degrade("Redis SET 失败（不影响服务）: " + err.Error())
		}
	}()
}

// Close 释放 Redis 连接。
func (t *TwoTier) Close() {
	if t.rdb != nil {
		_ = t.rdb.Close()
	}
}
