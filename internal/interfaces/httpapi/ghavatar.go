// GitHub 头像服务端代理：访客浏览器可能无法直连 github.com，
// 由本服务端经 safehttp（SSRF 校验 + 代理感知）代取并短时缓存。
package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

const ghAvatarTTL = time.Hour

var (
	ghOwnerPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

	ghAvatarMu    sync.Mutex
	ghAvatarCache = map[string]ghAvatarEntry{}
)

type ghAvatarEntry struct {
	body   []byte
	until  time.Time
	status int
}

// ghAvatar 代理仓库所有者头像：GET /api/v1/gh/avatar/{owner}.png
func (s *Server) ghAvatar(w http.ResponseWriter, r *http.Request) {
	owner := strings.TrimSuffix(chi.URLParam(r, "owner"), ".png")
	if !ghOwnerPattern.MatchString(owner) {
		writeErr(w, http.StatusBadRequest, errorString("invalid owner"))
		return
	}

	key := owner
	ghAvatarMu.Lock()
	e, hit := ghAvatarCache[key]
	valid := hit && time.Now().Before(e.until)
	ghAvatarMu.Unlock()
	if valid && e.status == http.StatusOK {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=1800")
		_, _ = w.Write(e.body)
		return
	}

	endpoint := "https://github.com/" + owner + ".png?size=96"
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp, status, err := safehttp.Do(ctx, http.MethodGet, endpoint, map[string]string{
		"User-Agent": "GithubHot/1.0 (avatar proxy)",
	}, nil)
	if err != nil {
		// 拉取失败：回退 1x1 透明 PNG，前端 onerror 会再切字标封面
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write(transparentPNG)
		return
	}
	if status != http.StatusOK {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(status)
		_, _ = w.Write(transparentPNG)
		return
	}

	ghAvatarMu.Lock()
	ghAvatarCache[key] = ghAvatarEntry{body: resp, until: time.Now().Add(ghAvatarTTL), status: status}
	ghAvatarMu.Unlock()

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	_, _ = w.Write(resp)
}

// transparentPNG 1x1 全透明 PNG（上游失败时的占位）。
var transparentPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
	0x0D, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}
