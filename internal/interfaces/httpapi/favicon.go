package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---------- 信源 favicon 代理 ----------
//
// 微博/百度热搜等条目落地页无 og:image，卡片长期无封面。
// 策略：favicon.im 取信源站 favicon（ICO/PNG），后端解码 ICO（内嵌 PNG 直接取出，
// BMP 位图手工转码），合成 256x256 白底 LOGO 瓦片 PNG，base64 存 link_images 缓存
// （key 前缀 "favicon:"，空串负缓存）。APP 卡片无图时回退到本接口，手机端不直连外网。

// FaviconService favicon 瓦片服务。
type FaviconService struct {
	cache LinkImageCache

	sem      chan struct{} // 抓取并发上限
	mu       sync.Mutex
	inflight map[string]bool
}

// NewFaviconService 构建服务；cache 为 nil 时全部返回未命中。
func NewFaviconService(cache LinkImageCache) *FaviconService {
	return &FaviconService{
		cache:    cache,
		sem:      make(chan struct{}, 8),
		inflight: map[string]bool{},
	}
}

const faviconKeyPrefix = "favicon:"

// faviconDomainRe 域名白名单格式校验（只允许纯域名，防 SSRF 内网探测）。
var faviconDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// DomainOf 从 URL 提取规范化小写域名（favicon 缓存键与 APP 回退共用同一规则）。
func DomainOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

// Tile 返回 256x256 白底 LOGO 瓦片 PNG；未命中且抓取失败返回 false。
// 首次请求同步抓取（限时），成功后入缓存；失败写入负缓存防反复打上游。
func (s *FaviconService) Tile(domain string) ([]byte, bool) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if s == nil || s.cache == nil || !faviconDomainRe.MatchString(domain) || net.ParseIP(domain) != nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if v, ok, _ := s.cache.GetLinkImage(ctx, faviconKeyPrefix+domain); ok {
		if v == "" {
			return nil, false // 负缓存：已探测但拿不到
		}
		if b, err := base64.StdEncoding.DecodeString(v); err == nil && len(b) > 0 {
			return b, true
		}
		return nil, false
	}

	s.mu.Lock()
	if s.inflight[domain] {
		s.mu.Unlock()
		return nil, false // 并发请求同域名：让先到的写缓存，本次走占位
	}
	s.inflight[domain] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inflight, domain)
		s.mu.Unlock()
	}()

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	tile := s.fetchTile(domain)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	if tile == nil {
		_ = s.cache.SetLinkImage(ctx2, faviconKeyPrefix+domain, "")
		return nil, false
	}
	if err := s.cache.SetLinkImage(ctx2, faviconKeyPrefix+domain, base64.StdEncoding.EncodeToString(tile)); err != nil {
		log.Printf("[favicon] 缓存写入失败 %s: %v", domain, err)
	}
	return tile, true
}

// fetchTile 拉取域名 favicon 并合成瓦片；失败返回 nil。
func (s *FaviconService) fetchTile(domain string) []byte {
	req, err := http.NewRequest("GET", "https://favicon.im/"+domain+"?larger=true", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	resp, err := ogHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil || len(body) < 4 {
		return nil
	}
	img, err := decodeIcon(body)
	if err != nil || img == nil {
		return nil
	}
	tile := composeTile(img, 256)
	var buf bytes.Buffer
	if err := png.Encode(&buf, tile); err != nil {
		return nil
	}
	return buf.Bytes()
}

// ---------- 图标解码 ----------

var pngMagic = []byte{0x89, 'P', 'N', 'G'}

// decodeIcon 识别 PNG / ICO（含内嵌 PNG 与 BMP DIB）并解码为 image.Image。
func decodeIcon(b []byte) (image.Image, error) {
	if bytes.Equal(b[:4], pngMagic) {
		return png.Decode(bytes.NewReader(b))
	}
	// ICO: 保留字 0 + 类型 1
	if len(b) >= 6 && binary.LittleEndian.Uint16(b[0:2]) == 0 && binary.LittleEndian.Uint16(b[2:4]) == 1 {
		return decodeICO(b)
	}
	// 兜底：其他可识别格式（GIF/JPEG 等）
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// decodeICO 取 ICO 中像素最大的图像条目并解码。
func decodeICO(b []byte) (image.Image, error) {
	if len(b) < 6 {
		return nil, fmt.Errorf("ico: 文件过短")
	}
	count := int(binary.LittleEndian.Uint16(b[4:6]))
	bestW, bestH, bestSize, bestOff := 0, 0, 0, 0
	for i := 0; i < count; i++ {
		p := 6 + i*16
		if p+16 > len(b) {
			break
		}
		w, h := int(b[p]), int(b[p+1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		size := int(binary.LittleEndian.Uint32(b[p+8:]))
		off := int(binary.LittleEndian.Uint32(b[p+12:]))
		if size <= 0 || off < 0 || off+size > len(b) {
			continue
		}
		if size > bestSize {
			bestW, bestH, bestSize, bestOff = w, h, size, off
		}
	}
	if bestSize == 0 {
		return nil, fmt.Errorf("ico: 无有效条目")
	}
	data := b[bestOff : bestOff+bestSize]
	if len(data) >= 8 && bytes.Equal(data[:4], pngMagic) {
		return png.Decode(bytes.NewReader(data))
	}
	return decodeDIB(data, bestW, bestH)
}

// decodeDIB 解码 ICO 内的 BMP 位图（支持 24/32bpp、无压缩或 BI_BITFIELDS）。
// ICO 的 DIB 高度为实际两倍（XOR+AND 掩码），biSizeImage 之后是 AND 掩码，均跳过。
func decodeDIB(d []byte, iconW, iconH int) (image.Image, error) {
	if len(d) < 40 {
		return nil, fmt.Errorf("dib: 头过短")
	}
	hdr := int(binary.LittleEndian.Uint32(d[0:4]))
	if hdr < 40 || hdr > len(d) {
		return nil, fmt.Errorf("dib: 头长度非法")
	}
	w := int(int32(binary.LittleEndian.Uint32(d[4:8])))
	h := int(int32(binary.LittleEndian.Uint32(d[8:12])))
	bpp := int(binary.LittleEndian.Uint16(d[14:16]))
	comp := binary.LittleEndian.Uint32(d[16:20])
	if w <= 0 || h <= 0 || w > 512 || h > 1024 {
		return nil, fmt.Errorf("dib: 尺寸非法 %dx%d", w, h)
	}
	// ICO 高度翻倍：取实际显示高度
	useH := h
	if useH == 2*iconH {
		useH = iconH
	}
	if useH <= 0 || useH > 512 {
		useH = iconH
	}
	off := hdr
	if comp == 3 { // BI_BITFIELDS：RGB 掩码占 12 字节
		off += 12
	}

	switch bpp {
	case 32:
		if comp != 0 && comp != 3 {
			return nil, fmt.Errorf("dib: 32bpp 不支持压缩 %d", comp)
		}
		stride := w * 4
		if off+stride*useH > len(d) {
			return nil, fmt.Errorf("dib: 像素数据不足")
		}
		// 先扫一遍 alpha：BI_RGB 的 32bpp 常把 alpha 置 0，此时按不透明处理
		allZeroA := true
		for y := 0; y < useH && allZeroA; y++ {
			row := d[off+(useH-1-y)*stride:]
			for x := 0; x < w; x++ {
				if row[x*4+3] != 0 {
					allZeroA = false
					break
				}
			}
		}
		img := image.NewNRGBA(image.Rect(0, 0, w, useH))
		for y := 0; y < useH; y++ {
			row := d[off+(useH-1-y)*stride:]
			for x := 0; x < w; x++ {
				bb, g, r, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
				if allZeroA {
					a = 255
				}
				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: bb, A: a})
			}
		}
		return img, nil
	case 24:
		if comp != 0 {
			return nil, fmt.Errorf("dib: 24bpp 不支持压缩 %d", comp)
		}
		stride := ((w*3 + 3) / 4) * 4
		if off+stride*useH > len(d) {
			return nil, fmt.Errorf("dib: 像素数据不足")
		}
		img := image.NewNRGBA(image.Rect(0, 0, w, useH))
		for y := 0; y < useH; y++ {
			row := d[off+(useH-1-y)*stride:]
			for x := 0; x < w; x++ {
				bb, g, r := row[x*3], row[x*3+1], row[x*3+2]
				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: bb, A: 255})
			}
		}
		return img, nil
	}
	return nil, fmt.Errorf("dib: 不支持的位深 %d", bpp)
}

// ---------- 瓦片合成 ----------

// composeTile 把图标双线性缩放到边长 2/3 处，居中贴在白底瓦片上。
func composeTile(img image.Image, size int) *image.NRGBA {
	tile := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(tile, tile.Bounds(), &image.Uniform{color.NRGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return tile
	}
	box := size * 2 / 3
	scale := float64(box) / float64(max(iw, ih))
	nw := max(1, int(float64(iw)*scale+0.5))
	nh := max(1, int(float64(ih)*scale+0.5))
	scaled := scaleBilinear(img, nw, nh)
	dx := (size - nw) / 2
	dy := (size - nh) / 2
	draw.Draw(tile, image.Rect(dx, dy, dx+nw, dy+nh), scaled, image.Point{}, draw.Over)
	return tile
}

// scaleBilinear 双线性插值缩放（小图标放大用，纯标准库实现）。
func scaleBilinear(src image.Image, dw, dh int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw == 0 || sh == 0 {
		return dst
	}
	for y := 0; y < dh; y++ {
		// 目标像素中心映射回源坐标（含 0.5 偏移对齐）
		sy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
		y0 := int(sy)
		fy := sy - float64(y0)
		if y0 < 0 {
			y0, fy = 0, 0
		}
		y1 := y0 + 1
		if y1 >= sh {
			y1 = sh - 1
		}
		for x := 0; x < dw; x++ {
			sx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
			x0 := int(sx)
			fx := sx - float64(x0)
			if x0 < 0 {
				x0, fx = 0, 0
			}
			x1 := x0 + 1
			if x1 >= sw {
				x1 = sw - 1
			}
			c00 := color.NRGBAModel.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y0)).(color.NRGBA)
			c10 := color.NRGBAModel.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y0)).(color.NRGBA)
			c01 := color.NRGBAModel.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y1)).(color.NRGBA)
			c11 := color.NRGBAModel.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y1)).(color.NRGBA)
			w00 := (1 - fx) * (1 - fy)
			w10 := fx * (1 - fy)
			w01 := (1 - fx) * fy
			w11 := fx * fy
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(float64(c00.R)*w00 + float64(c10.R)*w10 + float64(c01.R)*w01 + float64(c11.R)*w11 + 0.5),
				G: uint8(float64(c00.G)*w00 + float64(c10.G)*w10 + float64(c01.G)*w01 + float64(c11.G)*w11 + 0.5),
				B: uint8(float64(c00.B)*w00 + float64(c10.B)*w10 + float64(c01.B)*w01 + float64(c11.B)*w11 + 0.5),
				A: uint8(float64(c00.A)*w00 + float64(c10.A)*w10 + float64(c01.A)*w01 + float64(c11.A)*w11 + 0.5),
			})
		}
	}
	return dst
}

// faviconAPI GET /api/v1/favicon?domain=weibo.com 或 ?url=<条目链接> → 256x256 PNG 瓦片。
func (s *Server) faviconAPI(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		domain = DomainOf(r.URL.Query().Get("url"))
	}
	png, ok := s.Favicons.Tile(domain)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(png)
}
