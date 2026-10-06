package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestUpsertFingerprintConcurrent 并发上报同一指纹：事务化后 hits 必须守恒
//（每条 +1）且无错误。修复前 SELECT 与 UPDATE 之间交错会丢失更新，
// 双新指纹并发还会 INSERT 主键冲突。
func TestUpsertFingerprintConcurrent(t *testing.T) {
	db := openTestDB(t)
	const workers = 10
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := db.UpsertFingerprint(context.Background(), "fp-concurrent",
				fmt.Sprintf("10.0.0.%d", i+1), "UA-test", nil, nil, nil, "", "", "", -1, "", "", nil)
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发上报出错（双 INSERT 冲突或丢失更新）: %v", err)
	}
	var hits int
	var ipsJSON string
	if err := db.QueryRow("SELECT hits, ips FROM ip_fingerprints WHERE fp = ?", "fp-concurrent").Scan(&hits, &ipsJSON); err != nil {
		t.Fatalf("读回指纹: %v", err)
	}
	if hits != workers {
		t.Fatalf("hits 应守恒为 %d，实际 %d（丢失更新）", workers, hits)
	}
}

// TestUpsertFingerprintIPsCapped ips 数组累积上限：超出 64 截断保留最近。
func TestUpsertFingerprintIPsCapped(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 70; i++ {
		if _, err := db.UpsertFingerprint(context.Background(), "fp-cap",
			fmt.Sprintf("10.1.%d.%d", i/250, i%250+1), "UA", nil, nil, nil, "", "", "", -1, "", "", nil); err != nil {
			t.Fatalf("第 %d 次上报: %v", i+1, err)
		}
	}
	var ipsJSON string
	if err := db.QueryRow("SELECT ips FROM ip_fingerprints WHERE fp = ?", "fp-cap").Scan(&ipsJSON); err != nil {
		t.Fatalf("读回指纹: %v", err)
	}
	// 粗断言：长度不超过 64 条记录（每条 IPv4 至少 7 字节 + 引号逗号）
	count := 0
	for _, c := range ipsJSON {
		if c == ',' {
			count++
		}
	}
	if count+1 > 64 {
		t.Fatalf("ips 应截断至 ≤64 条，实际约 %d 条", count+1)
	}
}

// TestTouchIPProfileConcurrent 并发首访同一 IP：事务化后 reqs 守恒、不报主键冲突。
func TestTouchIPProfileConcurrent(t *testing.T) {
	db := openTestDB(t)
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.TouchIPProfile(context.Background(), "10.9.9.9", "UA-a", 1); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发建档出错: %v", err)
	}
	var reqs int
	if err := db.QueryRow("SELECT reqs FROM ip_profiles WHERE ip = ?", "10.9.9.9").Scan(&reqs); err != nil {
		t.Fatalf("读回档案: %v", err)
	}
	if reqs != workers {
		t.Fatalf("reqs 应守恒为 %d，实际 %d", workers, reqs)
	}
}
