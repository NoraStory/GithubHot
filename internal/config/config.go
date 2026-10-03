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
	EmbedDims    int    // 输出维度；0 = 服务商默认（Qwen3-Embedding-8B 最大 4096）
	EmbedStyle   string // openai（默认）| ark-multimodal（豆包 vision 向量）
	Thinking     string // LLM_THINKING: disabled/enabled（方舟 seed 推理模型提速开关）
	// 预算熔断与成本估算
	BudgetTokensPerDay  int     // 每日 Token 上限，0 = 不熔断
	PriceInPerM         float64 // 输入单价（每百万 token），仅用于成本展示
	PriceOutPerM        float64 // 输出单价（每百万 token），仅用于成本展示
	NotifyWebhookURL    string  // 日报/告警 webhook；空 = 不推送
	NotifyWebhookFormat string  // raw（默认）/ feishu / wecom
	MusicPlaylist       string  // 背景音乐歌单 JSON（前端播放器）
	GitHubToken         string
	GitHubProxy         string // GitHub 镜像前缀（国内服务器直连失败时用，如 https://gh-proxy.com）
	CronSpec            string // serve 模式内置调度（cron 表达式，本地时区）
	ProbeIntervalHours  int    // 探针轮询间隔小时数（信源与端点健康探测，默认 6）
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
		DataDir:             dataDir,
		Port:                getEnv("PORT", "8787"),
		LLMBaseURL:          getEnv("LLM_BASE_URL", ""),
		LLMAPIKey:           getEnv("LLM_API_KEY", ""),
		LLMModelA:           getEnv("LLM_MODEL", ""),
		LLMModelB:           getEnv("LLM_MODEL_B", ""),
		LLMEmbed:            getEnv("LLM_EMBED_MODEL", ""),
		EmbedBaseURL:        getEnv("LLM_EMBED_BASE_URL", ""),
		EmbedAPIKey:         getEnv("LLM_EMBED_API_KEY", ""),
		EmbedDims:           getEnvInt("LLM_EMBED_DIMENSIONS", 0),
		EmbedStyle:          getEnv("LLM_EMBED_STYLE", ""),
		Thinking:            getEnv("LLM_THINKING", ""),
		BudgetTokensPerDay:  getEnvInt("LLM_BUDGET_TOKENS_PER_DAY", 0),
		PriceInPerM:         getEnvFloat("LLM_PRICE_IN_PER_M", 0),
		PriceOutPerM:        getEnvFloat("LLM_PRICE_OUT_PER_M", 0),
		NotifyWebhookURL:    getEnv("NOTIFY_WEBHOOK_URL", ""),
		NotifyWebhookFormat: getEnv("NOTIFY_WEBHOOK_FORMAT", ""),
		MusicPlaylist:       getEnv("MUSIC_PLAYLIST", ""),
		GitHubToken:         getEnv("GITHUB_TOKEN", ""),
		GitHubProxy:         getEnv("GITHUB_PROXY", ""),
		CronSpec:            getEnv("HOT_CRON", "30 7 * * *"),
		ProbeIntervalHours:  getEnvInt("PROBE_INTERVAL_HOURS", 6),
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

// getEnvInt 整数环境变量；非法或负值回退默认。
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n >= 0 {
			return n
		}
	}
	return fallback
}

// getEnvFloat 浮点环境变量；非法或负值回退默认。
func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err == nil && f >= 0 {
			return f
		}
	}
	return fallback
}

// loadDotenv 极简 .env 解析：KEY=VALUE，# 注释，支持 ${VAR} 引用环境变量
// （密钥可以只存在于环境变量中，.env 仅做引用，不落字面量）。
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
		if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
			ref := value[2 : len(value)-1]
			resolved := os.Getenv(ref)
			if resolved == "" {
				continue // 引用的环境变量不存在：不覆盖，让配置校验去报错
			}
			value = resolved
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
