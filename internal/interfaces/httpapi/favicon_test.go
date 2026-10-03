package httpapi

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"testing"
)

// 用真实抓取的 weibo.com ICO（favicon.im 返回，base64 内嵌于 favicon_sample_test.go）验证解码链路。
func TestDecodeWeiboICO(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(weiboICOSampleB64)
	if err != nil {
		t.Fatalf("样例解码失败: %v", err)
	}
	img, err := decodeIcon(raw)
	if err != nil || img == nil {
		t.Fatalf("decodeIcon 失败: %v", err)
	}
	b := img.Bounds()
	if b.Dx() < 16 || b.Dy() < 16 {
		t.Fatalf("图标尺寸异常: %v", b)
	}
	tile := composeTile(img, 256)
	var buf bytes.Buffer
	if err := png.Encode(&buf, tile); err != nil {
		t.Fatalf("瓦片编码失败: %v", err)
	}
	if buf.Len() < 1000 {
		t.Fatalf("瓦片过小: %d bytes", buf.Len())
	}
	t.Logf("weibo 图标 %dx%d → 瓦片 %d bytes", b.Dx(), b.Dy(), buf.Len())
}

func TestDomainOf(t *testing.T) {
	cases := map[string]string{
		"https://s.weibo.com/weibo?q=x": "s.weibo.com",
		"https://m.baidu.com/s?wd=x":    "m.baidu.com",
		"http://WWW.IThome.COM/a/1":     "www.ithome.com",
	}
	for in, want := range cases {
		if got := DomainOf(in); got != want {
			t.Errorf("DomainOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFaviconDomainValidation(t *testing.T) {
	// 注：IP 字面量（127.0.0.1 等）格式上能匹配域名正则，但在 FaviconService.Tile 里被 net.ParseIP 拦截。
	bad := []string{"weibo.com:8080", "weibo.com/path", "localhost", "a..b.com", ""}
	for _, d := range bad {
		if faviconDomainRe.MatchString(d) {
			t.Errorf("%q 不应通过校验", d)
		}
	}
	good := []string{"weibo.com", "m.baidu.com", "sub.a-b.com"}
	for _, d := range good {
		if !faviconDomainRe.MatchString(d) {
			t.Errorf("%q 应通过校验", d)
		}
	}
}
