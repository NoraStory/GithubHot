// Package main 是 GithubHot 二进制入口：run（跑一轮流水线）与 serve（API+双榜+定时）。
package main

import (
	"fmt"
	"os"

	"github.com/NoraStory/GithubHot/internal/config"
	"github.com/NoraStory/GithubHot/internal/interfaces/cli"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "run":
		if err := cli.Run(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "流水线失败: %v\n", err)
			os.Exit(1)
		}
	case "serve":
		if err := cli.Serve(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "服务退出: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("GithubHot", cli.Version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`GithubHot — GitHub 开源项目热点 × AI 资讯热点

用法:
  githubhot run      跑一轮完整流水线（采集 → GitHub双轨发现 → LLM精选写作 → 聚簇 → 融合 → 热度 → 日报）
  githubhot serve    启动 API + 双榜页 + 内置定时调度（服务器常驻模式）
  githubhot version  版本号

配置: 见 .env.example（LLM_API_KEY 必选；GITHUB_TOKEN 建议配置）`)
}
