package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/geoip"
	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// GeoDownload 拉取 P2-5 GeoIP 数据库（sapics/ip-location-db，CC BY 4.0）：
//   - 国家库 geo-whois-asn-country.mmdb → GEOIP_DB_PATH（默认 data/geo.mmdb）
//   - ASN 库 dbip-asn-lite.mmdb → GEOIP_ASN_DB_PATH（默认 data/geo-asn.mmdb）
//
// 走 safehttp 出站通道（SSRF 防护）；原子写（临时文件 + rename），文件缺失时
// 引擎侧整体降级，因此下载失败不算致命。致谢要求见规格书 §12。
func GeoDownload(args []string) error {
	if len(args) > 0 && args[0] == "--help" {
		fmt.Fprintln(os.Stderr, "用法: githubhot geo download")
		return nil
	}
	countryPath, asnPath := geoip.DefaultPaths()
	targets := []struct {
		name, url, path string
	}{
		{"国家库", geoip.CountryURL, countryPath},
		{"ASN 库", geoip.ASNURL, asnPath},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, t := range targets {
		fmt.Printf("[geo] 下载%s %s\n", t.name, t.url)
		data, status, err := safehttp.Fetch(ctx, t.url, nil)
		if err != nil {
			return fmt.Errorf("下载%s失败: %w（离线部署可跳过，GeoIP 核验将自动降级）", t.name, err)
		}
		if status != 200 {
			return fmt.Errorf("下载%s失败: HTTP %d", t.name, status)
		}
		if len(data) < 100*1024 {
			return fmt.Errorf("%s体积异常（%d 字节），疑似上游变更，拒绝写入", t.name, len(data))
		}
		if err := writeAtomic(t.path, data); err != nil {
			return fmt.Errorf("写%s失败: %w", t.name, err)
		}
		fmt.Printf("[geo] %s → %s（%.1f MB）\n", t.name, t.path, float64(len(data))/1024/1024)
	}
	fmt.Println("[geo] 数据来源 sapics/ip-location-db（CC BY 4.0），部署说明需保留致谢")
	return nil
}

// writeAtomic 原子写文件：临时文件 + rename，避免半截文件被服务端当库打开。
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".geo-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
