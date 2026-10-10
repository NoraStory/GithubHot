// Package main 是 GithubHot 二进制入口：run（跑一轮流水线）与 serve（API+双榜+定时）。
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/NoraStory/GithubHot/internal/config"
	"github.com/NoraStory/GithubHot/internal/interfaces/cli"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "admin":
		// 管理端工具：admin hash [密码]（Argon2id 密码哈希）、admin seed（APP 签名种子）。
		// 不依赖完整配置（config.Load 会校验 LLM 配置），先于配置加载处理。
		if len(os.Args) >= 3 {
			switch os.Args[2] {
			case "hash":
				if err := cli.HashAdminPassword(os.Args[3:]); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				return
			case "seed":
				if err := cli.SeedAppSign(os.Args[3:]); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				return
			}
		}
		fmt.Fprintln(os.Stderr, "用法: githubhot admin hash [密码] | githubhot admin seed")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "geo":
		// GeoIP 数据库下载（P2-5）：不依赖完整配置，先于 config.Load 处理。
		if len(os.Args) >= 3 && os.Args[2] == "download" {
			if err := cli.GeoDownload(os.Args[3:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintln(os.Stderr, "用法: githubhot geo download")
		os.Exit(2)
	case "ml":
		// P5 机器学习数据管道：ml export（训练数据导出）/ ml check（模型门槛校验）。
		if len(os.Args) >= 3 {
			switch os.Args[2] {
			case "export":
				out := flagArg(os.Args[3:], "--out")
				if err := cli.MLExport(cfg, out); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				return
			case "export-graph":
				out := flagArg(os.Args[3:], "--out")
				if err := cli.MLExportGraph(cfg, out); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				return
			case "import-gnn":
				path := flagArg(os.Args[3:], "")
				if path == "" {
					// 取第一个非 flag 参数
					for _, a := range os.Args[3:] {
						if !strings.HasPrefix(a, "--") && !strings.HasPrefix(a, "-") {
							path = a
							break
						}
					}
				}
				if path == "" {
					fmt.Fprintln(os.Stderr, "用法: githubhot ml import-gnn <file.json>")
					os.Exit(2)
					return
				}
				if err := cli.MLImportGNN(cfg, path); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				return
			case "check":
				if err := cli.MLCheck(cfg); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				return
			}
		}
		fmt.Fprintln(os.Stderr, "用法: githubhot ml export --out data/ml/behavior.jsonl | githubhot ml check")
		os.Exit(2)
	case "ja4":
		// JA4 映射表下载（P3-3）：不依赖完整配置，先于 config.Load 处理。
		if len(os.Args) >= 3 && os.Args[2] == "update" {
			if err := cli.JA4Update(os.Args[3:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintln(os.Stderr, "用法: githubhot ja4 update")
		os.Exit(2)
	case "risk":
		// 风控参数离线回放评估（算法改进 A 批）：risk replay [--sweep]
		if len(os.Args) >= 3 && os.Args[2] == "replay" {
			sweep := false
			for _, a := range os.Args[3:] {
				if a == "--sweep" {
					sweep = true
				}
			}
			if err := cli.IPRiskReplay(cfg, sweep); err != nil {
				fmt.Fprintf(os.Stderr, "回放评估失败: %v\n", err)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintln(os.Stderr, "用法: githubhot risk replay [--sweep]")
		os.Exit(2)
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
	case "mcp":
		if err := cli.MCP(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "MCP 退出: %v\n", err)
			os.Exit(1)
		}
	case "bench":
		// 样本从 stdin 读入：cat gold.jsonl | githubhot bench
		if err := cli.Bench(cfg, os.Stdin); err != nil {
			fmt.Fprintf(os.Stderr, "校准失败: %v\n", err)
			os.Exit(1)
		}
	case "push":
		sourceID := flagArg(os.Args[2:], "--source")
		rawURL := flagArg(os.Args[2:], "--url")
		title := flagArg(os.Args[2:], "--title")
		summary := flagArg(os.Args[2:], "--summary")
		if sourceID == "" || rawURL == "" || title == "" {
			fmt.Fprintln(os.Stderr, "用法: githubhot push --source script-push --url https://... --title 标题 [--summary 摘要]")
			os.Exit(2)
		}
		if err := cli.Push(cfg, sourceID, rawURL, title, summary); err != nil {
			fmt.Fprintf(os.Stderr, "推送失败: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("GithubHot", cli.Version)
	default:
		usage()
		os.Exit(2)
	}
}

// flagArg 从参数列表取 --key value。
func flagArg(args []string, key string) string {
	for i, a := range args {
		if a == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func usage() {
	fmt.Println(`GithubHot — GitHub 开源项目热点 × AI 资讯热点

用法:
  githubhot run      跑一轮完整流水线（采集 → GitHub双轨发现 → LLM精选写作 → 聚簇 → 融合 → 热度 → 日报）
  githubhot serve    启动 API + 双榜页 + 内置定时调度（服务器常驻模式）
  githubhot mcp      以 stdio MCP 服务器运行（Claude 等 Agent 客户端接入）
  githubhot bench    SelectBench 精选校准：--file data/gold.jsonl
  githubhot push     脚本推送资料：--source script-push --url ... --title ...
  githubhot admin    管理端工具：admin hash [密码] 生成 Argon2id 哈希；admin seed 生成 APP 签名种子
  githubhot geo      GeoIP 数据：geo download 下载 ip-location-db 国家/ASN 库（P2-5，CC BY 4.0）
  githubhot ja4      TLS 指纹数据：ja4 update 下载 FoxIO JA4→应用映射表（P3-3，BSD-3）
  githubhot ml       机器学习管道：ml export / export-graph / import-gnn / check（P5/P6）
  githubhot risk     风控参数回放评估：risk replay [--sweep]（召回/误报对比，调参看数字）
  githubhot version  版本号

配置: 见 .env.example（LLM_API_KEY 必选；GITHUB_TOKEN 建议配置）`)
}
