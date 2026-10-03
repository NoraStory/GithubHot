package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/NoraStory/GithubHot/internal/infrastructure/adminauth"
	"golang.org/x/term"
)

// HashAdminPassword 生成 Argon2id 密码哈希（供 .env 的 ADMIN_PASSWORD_HASH 使用）。
// 命令行传参不留密码进 shell 历史；无参数时从终端隐藏输入读取。
func HashAdminPassword(args []string) error {
	var password string
	if len(args) > 0 && args[0] != "" {
		password = args[0]
	} else {
		fmt.Fprint(os.Stderr, "输入管理端密码: ")
		b, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			// 非 TTY（管道/重定向）退化为明文行读取
			line, rerr := bufio.NewReader(os.Stdin).ReadString('\n')
			if rerr != nil && line == "" {
				return fmt.Errorf("读取密码失败: %w", err)
			}
			password = strings.TrimRight(line, "\r\n")
		} else {
			password = string(b)
		}
	}
	if password == "" {
		return fmt.Errorf("密码不能为空")
	}
	hash, err := adminauth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("生成哈希失败: %w", err)
	}
	fmt.Println(hash)
	fmt.Fprintln(os.Stderr, "\n把上面这行粘到 .env：ADMIN_PASSWORD_HASH=<这行>")
	return nil
}
