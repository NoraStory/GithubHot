// Package config 加载运行配置：.env 文件（可选）+ 环境变量（优先）。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 全局配置。
type Config struct {
	DataDir      string // 数据目录（SQLite + 日报 + 站点）
	Port         string // serve 端口
	LLMBaseURL   string
	LLMAPIKey    string
	LLMModelA    string
	LLMModelB    string
	LLMEmbed     string
	EmbedBaseURL string // 向量端点；留空复用 LLMBaseURL
	EmbedAPIKey  string // 向量 Key；留空复用 LLMAPIKey
	GitHubToken  string
	CronSpec     string // serve 模式内置调度（cron 表达式，本地时区）
}

// Load 读取配置。工作目录存在 .env 时先加载（环境变量优先于 .env）。
func Load() (*Config, error) {
	loadDotenv(".env")
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dataDir := getEnv("DATA_DIR", "./data")
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(wd, dataDir)
	}
	cfg := &Config{
		DataDir:      dataDir,
		Port:         getEnv("PORT", "8787"),
		LLMBaseURL:   getEnv("LLM_BASE_URL", ""),
		LLMAPIKey:    getEnv("LLM_API_KEY", ""),
		LLMModelA:    getEnv("LLM_MODEL", ""),
		LLMModelB:    getEnv("LLM_MODEL_B", ""),
		LLMEmbed:     getEnv("LLM_EMBED_MODEL", ""),
		EmbedBaseURL: getEnv("LLM_EMBED_BASE_URL", ""),
		EmbedAPIKey:  getEnv("LLM_EMBED_API_KEY", ""),
		GitHubToken:  getEnv("GITHUB_TOKEN", ""),
		CronSpec:     getEnv("HOT_CRON", "30 7 * * *"),
	}
	if cfg.LLMBaseURL != "" || cfg.LLMAPIKey != "" || cfg.LLMModelA != "" {
		if cfg.LLMBaseURL == "" || cfg.LLMAPIKey == "" || cfg.LLMModelA == "" {
			return nil, fmt.Errorf("LLM 配置不完整：LLM_BASE_URL / LLM_API_KEY / LLM_MODEL 需要同时提供")
		}
	}
	// 向量模型可混搭服务商（LLM_EMBED_BASE_URL / LLM_EMBED_API_KEY），
	// 但配了向量模型就必须有可用的 Key
	if cfg.LLMEmbed != "" && cfg.EmbedAPIKey == "" && cfg.LLMAPIKey == "" {
		return nil, fmt.Errorf("配置了 LLM_EMBED_MODEL 但没有任何 API Key（LLM_EMBED_API_KEY / LLM_API_KEY）")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotenv 极简 .env 解析：KEY=VALUE，# 注释，不做引号展开之外的转义。
func loadDotenv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
