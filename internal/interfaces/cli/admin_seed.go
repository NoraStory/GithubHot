package cli

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

// SeedAppSign 生成 APP 请求签名种子（32 字节 base64url），供 .env 的 APP_SIGN_SEED
// 使用。种子只经环境变量在服务端与 APP 构建期共享，绝不经 HTTP 下发。
func SeedAppSign(args []string) error {
	if len(args) > 0 && args[0] == "--help" {
		fmt.Fprintln(os.Stderr, "用法: githubhot admin seed")
		return nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("生成随机种子失败: %w", err)
	}
	seed := base64.RawURLEncoding.EncodeToString(buf)
	fmt.Println(seed)
	fmt.Fprintf(os.Stderr, "\n1) 服务端：.env 增加 APP_SIGN_SEED=%s\n"+
		"2) APP 端：构建期注入同名值（Gradle buildConfigField / CI Secret，见 android-app/README）\n"+
		"3) 存量 APP 未发版前：APP_SIGN_SEED_GRACE=旧种子（如 gh-dev-seed-v1），过渡期 ≤14 天\n", seed)
	return nil
}
