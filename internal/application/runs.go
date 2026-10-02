package application

import (
	"context"
	"encoding/json"
)

// RunRepo 运行记录读取端口（写入在流水线收尾）。
type RunRepo interface {
	List(ctx context.Context, limit int) ([]RunRow, error)
}

// ListRuns 读最近 N 次运行记录。
func ListRuns(ctx context.Context, r RunRepo, limit int) ([]RunRow, error) {
	if r == nil {
		return nil, nil
	}
	return r.List(ctx, limit)
}

// DecodeRunStats 解析运行统计 JSON。
func DecodeRunStats(raw string) (collected, written, stories int) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0, 0, 0
	}
	if v, ok := m["collected"].(float64); ok {
		collected = int(v)
	}
	if v, ok := m["written"].(float64); ok {
		written = int(v)
	}
	if v, ok := m["stories"].(float64); ok {
		stories = int(v)
	}
	return
}
