// Package notify 实现通知端口：日报/失败告警推送到 Webhook
// （通用 JSON / 飞书 / 企业微信 三种格式）。出站经 safehttp SSRF 校验。
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

// Webhook 通知器。
type Webhook struct {
	URL    string // 空 = 禁用
	Format string // raw（默认）/ feishu / wecom
}

// Notify 发送标题+正文。失败仅返回错误，由调用方决定是否告警。
func (w Webhook) Notify(ctx context.Context, title, text string) error {
	if w.URL == "" {
		return nil
	}
	// json.Marshal 生成合法 JSON 字符串转义（fmt %q 的 Go 转义对控制字符不是合法 JSON）
	var payload []byte
	var err error
	switch w.Format {
	case "feishu":
		payload, err = json.Marshal(map[string]any{
			"msg_type": "text",
			"content":  map[string]string{"text": title + "\n" + text},
		})
	case "wecom":
		payload, err = json.Marshal(map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"content": "**" + title + "**\n" + text},
		})
	default:
		payload, err = json.Marshal(map[string]string{
			"title":     title,
			"text":      text,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
	if err != nil {
		return fmt.Errorf("构造 webhook 请求体: %w", err)
	}
	headers := map[string]string{"Content-Type": "application/json"}
	_, status, err := safehttp.Do(ctx, "POST", w.URL, headers, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("webhook 返回 %d", status)
	}
	return nil
}
