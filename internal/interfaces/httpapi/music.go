// 音乐馆：Meting 歌单代理。
// 浏览器直连第三方 Meting API 受 CORS 与网络环境影响，这里统一走
// 服务端 safehttp（SSRF 校验 + 代理感知）+ 短时内存缓存，前端只访问同源接口。
package httpapi

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/safehttp"
)

const musicCacheTTL = 10 * time.Minute

var (
	musicIDPattern = regexp.MustCompile(`^\d{1,20}$`)
	musicServers   = map[string]bool{"netease": true, "tencent": true, "kugou": true, "baidu": true}

	// 第三方 meting pic 代理只回 90x90 占位白图（实测 82% 纯像素），且多一次跳转——
	// 网易封面的 CDN 地址可由 pic id 直接算出（异或魔数 + MD5 + Base64）。
	metingNeteasePic = regexp.MustCompile(`^https://api\.injahow\.cn/meting/\?[^#]*type=pic&id=(\d{1,20})`)
	ncPicParam       = regexp.MustCompile(`param=\d+y\d+`)
	ncPicMagic       = []byte("3go8&$8*3*3h0k(2)2")

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

	// 改写封面为直连大图，再入缓存（缓存里放改写后的版本）
	resp = rewriteMusicPics(server, resp)

	musicCacheMu.Lock()
	musicCacheKey, musicCacheBody, musicCacheUntil = cacheKey, resp, time.Now().Add(musicCacheTTL)
	musicCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(resp)
}

// rewriteMusicPics 把歌单里每首歌的 pic 改写成直连大图地址（失败时原样返回）。
func rewriteMusicPics(server string, body []byte) []byte {
	var items []map[string]any
	if json.Unmarshal(body, &items) != nil || items == nil {
		return body
	}
	changed := false
	for _, it := range items {
		pic, _ := it["pic"].(string)
		if np, ok := rewritePicURL(server, pic); ok {
			it["pic"] = np
			changed = true
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(items)
	if err != nil {
		return body
	}
	return out
}

// rewritePicURL 返回改写后的封面地址；ok=false 表示沿用原地址。
func rewritePicURL(server, pic string) (string, bool) {
	if pic == "" {
		return pic, false
	}
	if server == "netease" {
		if m := metingNeteasePic.FindStringSubmatch(pic); m != nil {
			id := m[1]
			sum := md5.Sum(ncPicXOR(id))
			return "https://p3.music.126.net/" + base64.StdEncoding.EncodeToString(sum[:]) + "/" + id + ".jpg?param=300y300", true
		}
		if strings.Contains(pic, "music.126.net/") {
			return ncPic300(pic), true
		}
		return pic, false
	}
	// 其他服务器：统一升级 https，避免混合内容
	if strings.HasPrefix(pic, "http://") {
		return "https://" + strings.TrimPrefix(pic, "http://"), true
	}
	return pic, false
}

// ncPicXOR 网易封面 id 的魔数异或（digest 输入）。
func ncPicXOR(id string) []byte {
	b := []byte(id)
	for i := range b {
		b[i] ^= ncPicMagic[i%len(ncPicMagic)]
	}
	return b
}

// ncPic300 确保 126.net 封面带 300y300 尺寸参数。
func ncPic300(u string) string {
	if ncPicParam.MatchString(u) {
		return ncPicParam.ReplaceAllString(u, "param=300y300")
	}
	if strings.Contains(u, "?") {
		return u + "&param=300y300"
	}
	return u + "?param=300y300"
}
