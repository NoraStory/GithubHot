package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// ja4MappingURL FoxIO-LLC/ja4 仓库的 JA4 → 应用 映射表（UA↔TLS 交叉核验的数据源，
// 位于仓库根；JA4 本体 BSD-3，见 LICENSE-JA4——本项目只实现 JA4，不涉及 JA4+ 系列）。
const ja4MappingURL = "https://raw.githubusercontent.com/FoxIO-LLC/ja4/main/ja4plus-mapping.csv"

// ja4MappingPath 映射表落盘位置（尊重 DATA_DIR，默认 ./data）。
func ja4MappingPath() string {
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	return filepath.Join(dataDir, "ja4-mapping.csv")
}

// JA4Update 拉取 FoxIO ja4plus-mapping.csv（P3-2/P3-3 数据源）。
// 走 safehttp 出站通道；原子写。文件缺失时 UA↔TLS 核验自动降级。
func JA4Update(args []string) error {
	if len(args) > 0 && args[0] == "--help" {
		fmt.Fprintln(os.Stderr, "用法: githubhot ja4 update")
		return nil
	}
	path := ja4MappingPath()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fmt.Printf("[ja4] 下载映射表 %s\n", ja4MappingURL)
	data, status, err := safehttp.Fetch(ctx, ja4MappingURL, nil)
	if err != nil {
		return fmt.Errorf("下载失败: %w（离线部署可跳过，UA↔TLS 核验将自动降级）", err)
	}
	if status != 200 {
		return fmt.Errorf("下载失败: HTTP %d", status)
	}
	if len(data) < 1024 {
		return fmt.Errorf("映射表体积异常（%d 字节），疑似上游变更，拒绝写入", len(data))
	}
	if err := writeAtomic(path, data); err != nil {
		return fmt.Errorf("写映射表失败: %w", err)
	}
	fmt.Printf("[ja4] 映射表 → %s（%.1f KB）\n数据来源 FoxIO-LLC/ja4（BSD-3），致谢见项目 README\n",
		path, float64(len(data))/1024)
	return nil
}
