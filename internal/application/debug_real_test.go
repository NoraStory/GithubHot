package application_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NoraStory/GithubHot/internal/application"
	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
)

type debugClock struct{}

func (debugClock) Now() time.Time { return time.Now().UTC() }

// TestDebugStoryDetailRealDB 诊断事件详情（本地有真实库时运行，否则跳过）。
func TestDebugStoryDetailRealDB(t *testing.T) {
	dbPath := filepath.Join(".", "..", "..", "data", "githubhot.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("本地无真实数据库")
	}
	db, err := sqlite.Open(filepath.Join(".", "..", "..", "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	deps := application.Deps{
		Sources:  sqlite.NewSourceRepo(db),
		Items:    sqlite.NewItemRepo(db),
		Projects: sqlite.NewProjectRepo(db),
		Stories:  sqlite.NewStoryRepo(db),
		Clock:    debugClock{},
	}
	ctx := context.Background()
	stories, err := deps.Stories.Active(ctx, deps.Clock.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) == 0 {
		t.Skip("无活跃事件")
	}
	s := stories[0]
	t.Logf("故事 %s: members=%d", s.ID, len(s.Members))
	if len(s.Members) > 0 {
		t.Logf("member[0]: itemID=%q", s.Members[0].ItemID)
	}
	v, err := application.BuildStoryDetail(ctx, deps, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("detail: members=%d history=%d projects=%d", len(v.Members), len(v.History), len(v.Projects))
}
