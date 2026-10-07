package memcache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTTLExpiry(t *testing.T) {
	c := New(0)
	c.Set("k", "v", 30*time.Millisecond)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("TTL 内应命中")
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("过期后不应命中")
	}
}

func TestGetOrLoadSingleflight(t *testing.T) {
	c := New(0)
	var calls int32
	load := func() (any, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond) // 模拟慢 loader，让并发请求重叠
		return "result", nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrLoad("hot", time.Minute, load)
			if err != nil || v != "result" {
				t.Errorf("GetOrLoad 结果异常: %v %v", v, err)
			}
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("20 个并发请求应只触发 1 次 loader，实际 %d", n)
	}
}

func TestEvictionCap(t *testing.T) {
	c := New(10)
	for i := 0; i < 50; i++ {
		c.Set(string(rune('a'+i%26))+string(rune('a'+i/26)), i, time.Duration(60+i)*time.Millisecond)
	}
	if c.Len() > 10 {
		t.Fatalf("容量应封顶 10，实际 %d", c.Len())
	}
}

func TestErrorNotCached(t *testing.T) {
	c := New(0)
	calls := 0
	load := func() (any, error) { calls++; return nil, errFake }
	if _, err := c.GetOrLoad("k", time.Minute, load); err == nil {
		t.Fatal("应透传 loader 错误")
	}
	if _, ok := c.Get("k"); ok {
		t.Fatal("错误结果不应落缓存")
	}
	// 第二次应再次执行 loader（未被错误缓存击穿阻断）
	_, _ = c.GetOrLoad("k", time.Minute, load)
	if calls != 2 {
		t.Fatalf("失败后重试应再调 loader，实际调用 %d 次", calls)
	}
}

var errFake = errors.New("fake")
