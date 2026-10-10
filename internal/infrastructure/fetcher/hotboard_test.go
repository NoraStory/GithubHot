package fetcher

import (
	"strings"
	"testing"
)

// TestParseBilibiliList 排行榜 JSON 解析（fixture 按实测响应结构截取）。
func TestParseBilibiliList(t *testing.T) {
	fixture := `{
		"code": 0,
		"data": {
			"list": [
				{"bvid": "BV1VeHQ6tEaS", "title": "超市生存挑战", "owner": {"name": "UP主甲"}, "stat": {"view": 4220616}},
				{"bvid": "BV1xx411c7mD", "title": "第二个视频", "owner": {"name": "UP主乙"}, "stat": {"view": 123456}},
				{"bvid": "", "title": "缺 bvid 应跳过", "owner": {"name": "x"}, "stat": {"view": 1}},
				{"bvid": "BV1skip00000", "title": "", "owner": {"name": "x"}, "stat": {"view": 2}}
			]
		}
	}`
	items, err := parseBilibiliList([]byte(fixture))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应保留 2 条完整条目，实际 %d", len(items))
	}
	first := items[0]
	if first.URL != "https://www.bilibili.com/video/BV1VeHQ6tEaS" {
		t.Fatalf("URL 应为完整视频页链接，实际 %s", first.URL)
	}
	if first.Meta["rank"] != "1" || first.Meta["heat"] != "4220616" {
		t.Fatalf("rank/heat 元数据不符: %v", first.Meta)
	}
	if items[1].Meta["rank"] != "2" {
		t.Fatalf("跳过缺 bvid 条目后排名应连续，实际 %s", items[1].Meta["rank"])
	}
}

// TestParseBilibiliListErrors 异常响应：code!=0、空列表、坏 JSON。
func TestParseBilibiliListErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"code 非零", `{"code":-404,"data":{"list":[]}}`},
		{"空列表", `{"code":0,"data":{"list":[]}}`},
		{"坏 JSON", `{not-json`},
	}
	for _, c := range cases {
		if _, err := parseBilibiliList([]byte(c.body)); err == nil {
			t.Fatalf("%s 应返回错误", c.name)
		} else if !strings.Contains(err.Error(), "bilibili") {
			t.Fatalf("%s 错误信息应标注 bilibili: %v", c.name, err)
		}
	}
}
