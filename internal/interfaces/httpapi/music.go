// 音乐馆：Meting 歌单代理。
// 浏览器直连第三方 Meting API 受 CORS 与网络环境影响，这里统一走
// 服务端 safehttp（SSRF 校验 + 代理感知）+ 短时内存缓存，前端只访问同源接口。
package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

const musicCacheTTL = 10 * time.Minute

var (
	musicIDPattern = regexp.MustCompile(`^\d{1,20}$`)
	musicServers   = map[string]bool{"netease": true, "tencent": true, "kugou": true, "baidu": true}

	musicCacheMu    sync.Mutex
	musicCacheKey   string
	musicCacheBody  []byte
	musicCacheUntil time.Time
)

// musicPlaylist 代理歌单接口：GET /api/v1/music/playlist?id=652135520&server=netease
// id 仅接受纯数字、server 仅接受白名单，二者拼入固定的 https 端点后由 safehttp 再校验。
func (s *Server) musicPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	server := r.URL.Query().Get("server")
	if !musicIDPattern.MatchString(id) || !musicServers[server] {
		writeErr(w, http.StatusBadRequest, errorString("invalid playlist id or server"))
		return
	}

	cacheKey := server + ":" + id
	musicCacheMu.Lock()
	hit := musicCacheKey == cacheKey && time.Now().Before(musicCacheUntil)
	body := musicCacheBody
	musicCacheMu.Unlock()
	if hit {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(body)
		return
	}

	endpoint := "https://api.injahow.cn/meting/?server=" + server + "&type=playlist&id=" + id
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, status, err := safehttp.Do(ctx, http.MethodGet, endpoint, map[string]string{
		"User-Agent": "GithubHot/1.0 (music proxy)",
	}, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if status != http.StatusOK {
		writeErr(w, status, errorString("meting upstream error"))
		return
	}

	musicCacheMu.Lock()
	musicCacheKey, musicCacheBody, musicCacheUntil = cacheKey, resp, time.Now().Add(musicCacheTTL)
	musicCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(resp)
}
