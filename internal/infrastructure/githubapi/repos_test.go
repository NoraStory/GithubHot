package githubapi

import (
	"strings"
	"testing"
	"time"
)

// 用户仓库响应解析：字段映射 + null 容错 + fork/pushed_at。
func TestParseUserReposJSON(t *testing.T) {
	body := []byte(`[
		{
			"full_name": "NoraStory/GithubHot",
			"html_url": "https://github.com/NoraStory/GithubHot",
			"description": "GitHub 热点聚合",
			"language": "Go",
			"topics": ["github", "trending"],
			"homepage": "https://www.githubhot.online",
			"stargazers_count": 1,
			"forks_count": 0,
			"fork": false,
			"pushed_at": "2026-10-12T07:07:06Z"
		},
		{
			"full_name": "NoraStory/some-fork",
			"html_url": "https://github.com/NoraStory/some-fork",
			"description": null,
			"language": null,
			"stargazers_count": 0,
			"fork": true,
			"pushed_at": "2026-09-01T00:00:00Z"
		}
	]`)
	repos, err := parseUserReposJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("应有 2 个仓库，得到 %d", len(repos))
	}
	first := repos[0]
	if first.FullName != "NoraStory/GithubHot" || first.Language != "Go" || first.IsFork {
		t.Errorf("首仓库字段错误: %+v", first)
	}
	if first.Homepage != "https://www.githubhot.online" || len(first.Topics) != 2 {
		t.Errorf("homepage/topics 解析错误: %+v", first)
	}
	if want := time.Date(2026, 10, 12, 7, 7, 6, 0, time.UTC); !first.PushedAt.Equal(want) {
		t.Errorf("pushed_at 解析错误: %v", first.PushedAt)
	}
	second := repos[1]
	if !second.IsFork || second.Description != "" || second.Language != "" {
		t.Errorf("fork 仓库字段错误: %+v", second)
	}
}

func TestParseUserReposJSONEmpty(t *testing.T) {
	repos, err := parseUserReposJSON([]byte(`[]`))
	if err != nil || len(repos) != 0 {
		t.Fatalf("空列表应返回空切片: %v %v", repos, err)
	}
	if _, err := parseUserReposJSON([]byte(`not json`)); err == nil || !strings.Contains(err.Error(), "解析用户仓库响应") {
		t.Errorf("非法 JSON 应报解析错误: %v", err)
	}
}
