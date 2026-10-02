// Package mcp 实现面向 Agent 的 MCP（Model Context Protocol）stdio 服务器：
// 新行分隔的 JSON-RPC 2.0，暴露双榜查询/搜索/日报五个工具。
// 启动：githubhot mcp（接入 Claude 等客户端时配置 command 即可）。
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/NoraStory/GithubHot/internal/application"
)

// Server MCP stdio 服务器。
type Server struct {
	Deps    application.Deps
	Version string
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// Run 阻塞读取 stdin、写 stdout，直到 EOF 或 ctx 取消。
func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	writer := bufio.NewWriter(os.Stdout)
	done := make(chan error, 1)
	go func() {
		for reader.Scan() {
			line := strings.TrimSpace(reader.Text())
			if line == "" {
				continue
			}
			var req rpcRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				continue // 非法行忽略（日志走 stderr 会污染协议，静默）
			}
			if req.ID == nil {
				continue // notification：无需响应（如 notifications/initialized）
			}
			resp := s.dispatch(ctx, req)
			if err := writeLine(writer, resp); err != nil {
				done <- err
				return
			}
		}
		done <- reader.Err()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return nil
	}
}

func writeLine(w *bufio.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) map[string]any {
	base := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	switch req.Method {
	case "initialize":
		base["result"] = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "githubhot", "version": s.Version},
		}
	case "ping":
		base["result"] = map[string]any{}
	case "tools/list":
		base["result"] = map[string]any{"tools": s.toolDefs()}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			base["error"] = map[string]any{"code": -32602, "message": "params 无效"}
			return base
		}
		text, toolErr := s.callTool(ctx, p.Name, p.Arguments)
		if toolErr != nil {
			base["result"] = map[string]any{
				"content": []map[string]any{{"type": "text", "text": toolErr.Error()}},
				"isError": true,
			}
			return base
		}
		base["result"] = map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": false,
		}
	default:
		base["error"] = map[string]any{"code": -32601, "message": "method not found: " + req.Method}
	}
	return base
}

func (s *Server) toolDefs() []map[string]any {
	obj := func(props map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": props}
	}
	intProp := map[string]any{"type": "integer", "description": "返回条数（默认 10）"}
	return []map[string]any{
		{"name": "hot_github", "description": "GitHub 开源项目热度榜（24h star 增长 + trending 口径）",
			"inputSchema": obj(map[string]any{"limit": intProp})},
		{"name": "hot_news", "description": "AI 资讯热度榜（独立来源 + 时间衰减 + 中文标题摘要）",
			"inputSchema": obj(map[string]any{"limit": intProp})},
		{"name": "hot_fusion", "description": "融合观察：AI 资讯与 GitHub 项目互相印证的配对",
			"inputSchema": obj(map[string]any{})},
		{"name": "search", "description": "站内搜索已精选的资讯与事件（中文）",
			"inputSchema": obj(map[string]any{"query": map[string]any{"type": "string", "description": "关键词"}, "limit": intProp})},
		{"name": "latest_digest", "description": "最新日报 Markdown 全文",
			"inputSchema": obj(map[string]any{})},
	}
}

func (s *Server) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	intArg := func(k string, def int) int {
		if v, ok := args[k].(float64); ok && v > 0 {
			return int(v)
		}
		return def
	}
	switch name {
	case "hot_github", "hot_news", "hot_fusion":
		view, err := application.BuildHotView(ctx, s.Deps, "daily")
		if err != nil {
			return "", err
		}
		limit := intArg("limit", 10)
		switch name {
		case "hot_github":
			if len(view.GitHub) > limit {
				view.GitHub = view.GitHub[:limit]
			}
			return marshal(view.GitHub)
		case "hot_news":
			if len(view.News) > limit {
				view.News = view.News[:limit]
			}
			return marshal(view.News)
		default:
			return marshal(view.Fusion)
		}
	case "search":
		q, _ := args["query"].(string)
		results, err := application.Search(ctx, s.Deps, q, intArg("limit", 10))
		if err != nil {
			return "", err
		}
		return marshal(results)
	case "latest_digest":
		dg, err := s.Deps.Digests.Latest(ctx, "daily")
		if err != nil {
			return "", err
		}
		if dg == nil {
			return "暂无日报", nil
		}
		return dg.Markdown, nil
	default:
		return "", fmt.Errorf("未知工具: %s", name)
	}
}

func marshal(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// 编译期检查：确保 stdout 只写协议、日志不混入。
var _ io.Writer = os.Stdout
