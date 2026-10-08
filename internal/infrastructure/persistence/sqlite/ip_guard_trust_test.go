package sqlite

// 渗透修复回归：components 来源信任分级（匿名更新不得覆写已有指纹基线）。

import (
	"context"
	"encoding/json"
	"testing"
)

func readComponents(t *testing.T, db *DB, fp string) map[string]string {
	t.Helper()
	var compJSON string
	if err := db.QueryRow("SELECT components FROM ip_fingerprints WHERE fp = ?", fp).Scan(&compJSON); err != nil {
		t.Fatalf("读回 components: %v", err)
	}
	comp := map[string]string{}
	_ = json.Unmarshal([]byte(compJSON), &comp)
	return comp
}

// TestComponentsTrustTiering 匿名（!trusted）更新不得覆写已有 components；
// trusted 更新与首报建档不受限。
func TestComponentsTrustTiering(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// 首报（匿名）：无基线可污染 → 建档并接受分量
	if _, err := db.UpsertFingerprint(ctx, "fp-tier", "1.2.3.4", "UA",
		nil, map[string]string{"canvas": "A"}, nil, "", "", "", -1, "", "", nil, false); err != nil {
		t.Fatal(err)
	}
	comp := readComponents(t, db, "fp-tier")
	if comp["canvas"] != "A" {
		t.Fatalf("首报建档应接受分量，实际 %v", comp)
	}

	// 匿名更新：components 被忽略（基线保持 A，不出现 attacker 值）
	if _, err := db.UpsertFingerprint(ctx, "fp-tier", "5.6.7.8", "UA",
		nil, map[string]string{"canvas": "POISON", "attacker": "yes"}, nil, "", "", "", -1, "", "", nil, false); err != nil {
		t.Fatal(err)
	}
	comp = readComponents(t, db, "fp-tier")
	if comp["canvas"] != "A" || comp["attacker"] != "" {
		t.Fatalf("匿名更新不得覆写基线，实际 %v", comp)
	}

	// 可信更新（有效会话/已验签 APP）：允许覆写
	if _, err := db.UpsertFingerprint(ctx, "fp-tier", "1.2.3.4", "UA",
		nil, map[string]string{"canvas": "B"}, nil, "", "", "", -1, "", "", nil, true); err != nil {
		t.Fatal(err)
	}
	comp = readComponents(t, db, "fp-tier")
	if comp["canvas"] != "B" {
		t.Fatalf("可信更新应允许覆写，实际 %v", comp)
	}

	// 对照组：不受信任分级影响的行为——匿名更新仍累积 hits/ips（观测不受限）
	if _, err := db.UpsertFingerprint(ctx, "fp-tier", "9.9.9.9", "UA",
		nil, nil, nil, "", "", "", -1, "", "", nil, false); err != nil {
		t.Fatal(err)
	}
	var hits int
	if err := db.QueryRow("SELECT hits FROM ip_fingerprints WHERE fp = ?", "fp-tier").Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 4 {
		t.Fatalf("hits 应累积到 4，实际 %d", hits)
	}
}
