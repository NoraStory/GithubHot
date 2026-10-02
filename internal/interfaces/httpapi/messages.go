package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
)

// messages 留言板：极简内置实现（昵称+内容，按 IP 做内存限频）。

const messageMaxLen = 400

// 内存限频：IP -> 上次留言时间
var (
	msgMu       sync.Mutex
	msgLastPost = map[string]time.Time{}
)

func ipHash(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:8])
}

// messagesAPI GET 留言列表（分页）。
func (s *Server) messagesAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), 1)
	size := atoiDefault(q.Get("pageSize"), 20)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	// 简化为全量取最近 200 条客户端分页（留言量级小）
	rows, err := s.Deps.Messages.List(s.ctx(), 200)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	total := len(rows)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	items := []application.MessageRow{}
	if start < end {
		items = rows[start:end]
	}
	writeJSON(w, 200, map[string]any{"total": total, "page": page, "items": items})
}

// postMessage POST 留言。
func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Nickname string `json:"nickname"`
		Content  string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, err)
		return
	}
	p.Nickname = strings.TrimSpace(p.Nickname)
	p.Content = strings.TrimSpace(p.Content)
	if p.Nickname == "" {
		p.Nickname = "匿名访客"
	}
	if len([]rune(p.Nickname)) > 24 {
		writeErr(w, 400, errorString("昵称最长 24 字"))
		return
	}
	if p.Content == "" || len([]rune(p.Content)) > messageMaxLen {
		writeErr(w, 400, errorString("内容不能为空且最长 400 字"))
		return
	}
	hash := ipHash(r)
	msgMu.Lock()
	if last, ok := msgLastPost[hash]; ok && time.Since(last) < 30*time.Second {
		msgMu.Unlock()
		writeErr(w, 429, errorString("留言过于频繁，请 30 秒后再试"))
		return
	}
	msgLastPost[hash] = time.Now()
	msgMu.Unlock()

	if err := s.Deps.Messages.Add(s.ctx(), application.MessageRow{
		Nickname: p.Nickname, Content: p.Content, IPHash: hash, CreatedAt: time.Now().UTC(),
	}); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
