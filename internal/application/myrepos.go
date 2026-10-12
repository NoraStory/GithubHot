package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// settingsKeyMyRepos 我的仓库快照在 settings 表中的键。
const settingsKeyMyRepos = "my_repos_v1"

// MyRepoRow 我的仓库展示行。
type MyRepoRow struct {
	FullName    string    `json:"fullName"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Language    string    `json:"language"`
	Topics      []string  `json:"topics"`
	Homepage    string    `json:"homepage,omitempty"`
	Stars       int       `json:"stars"`
	Forks       int       `json:"forks"`
	IsFork      bool      `json:"fork"`
	PushedAt    time.Time `json:"pushedAt"`
}

// MyReposView 我的仓库版块视图。
type MyReposView struct {
	Generated time.Time   `json:"generatedAt"`
	Owner     string      `json:"owner"`
	Repos     []MyRepoRow `json:"repos"`
}

// myReposSnapshot 快照存储形状（settings JSON，键 my_repos_v1）。
type myReposSnapshot struct {
	Owner     string      `json:"owner"`
	FetchedAt time.Time   `json:"fetchedAt"`
	Repos     []MyRepoRow `json:"repos"`
}

// RefreshMyRepos 刷新「我的仓库」快照：拉取 GitHub 用户公开仓库写入 settings。
// 每轮流水线调用一次；失败仅返回错误由调用方记日志——旧快照照常服务，
// 页面不因 GitHub API 抖动而空白。
func RefreshMyRepos(ctx context.Context, d Deps) error {
	owner := strings.TrimSpace(d.GitHubOwner)
	if owner == "" || d.GitHub == nil {
		return nil
	}
	repos, err := d.GitHub.ListUserRepos(ctx, owner, 100)
	if err != nil {
		return fmt.Errorf("拉取用户仓库: %w", err)
	}
	rows := make([]MyRepoRow, 0, len(repos))
	for _, r := range repos {
		rows = append(rows, MyRepoRow{
			FullName: r.FullName, URL: r.HTMLURL, Description: r.Description,
			Language: r.Language, Topics: r.Topics, Homepage: r.Homepage,
			Stars: r.Stars, Forks: r.Forks, IsFork: r.IsFork, PushedAt: r.PushedAt,
		})
	}
	blob, err := json.Marshal(myReposSnapshot{Owner: owner, FetchedAt: d.Clock.Now(), Repos: rows})
	if err != nil {
		return fmt.Errorf("序列化仓库快照: %w", err)
	}
	if err := d.Settings.Set(ctx, settingsKeyMyRepos, string(blob)); err != nil {
		return fmt.Errorf("写入仓库快照: %w", err)
	}
	return nil
}

// BuildMyReposView 构建我的仓库视图：读快照，原创在前、按最近 push 排序。
// 拉取不发生在请求路径上（页面只读快照，与站内其他数据源同一可靠性模型）。
func BuildMyReposView(ctx context.Context, d Deps) (MyReposView, error) {
	view := MyReposView{Generated: d.Clock.Now(), Repos: []MyRepoRow{}}
	blob, err := d.Settings.Get(ctx, settingsKeyMyRepos)
	if err != nil {
		return view, err
	}
	if strings.TrimSpace(blob) == "" {
		return view, nil
	}
	var snap myReposSnapshot
	if err := json.Unmarshal([]byte(blob), &snap); err != nil {
		// 快照损坏时返回空视图而非报错：个人展示页不应 500
		log.Printf("[myrepos] 仓库快照解析失败: %v", err)
		return view, nil
	}
	view.Owner = snap.Owner
	view.Repos = snap.Repos
	if view.Repos == nil {
		view.Repos = []MyRepoRow{}
	}
	sort.SliceStable(view.Repos, func(i, j int) bool {
		if view.Repos[i].IsFork != view.Repos[j].IsFork {
			return !view.Repos[i].IsFork // 原创在前
		}
		return view.Repos[i].PushedAt.After(view.Repos[j].PushedAt)
	})
	return view, nil
}
