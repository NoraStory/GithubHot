package cli

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/robfig/cron/v3"
)

// newScheduler 内置定时器（服务器常驻模式）。cron 表达式用本地时区，
// 默认每天 07:30（见 HOT_CRON）。
func newScheduler(spec string, job func()) *scheduler {
	c := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger), cron.DelayIfStillRunning(cron.DefaultLogger)))
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		fmt.Printf("[cron] 表达式 %q 非法，退化为每天 07:30: %v\n", spec, err)
		sched, _ = cron.ParseStandard("30 7 * * *")
	}
	c.Schedule(sched, cron.FuncJob(job))
	fmt.Printf("[cron] 已注册，下一次触发: %s（now=%s）\n", sched.Next(time.Now()).Format("2006-01-02 15:04:05"), time.Now().Format("15:04:05"))
	return &scheduler{c: c, spec: spec}
}

type scheduler struct {
	c    *cron.Cron
	spec string
}

func (s *scheduler) start() {
	s.c.Start()
	fmt.Printf("[cron] 定时调度已启动（%s，本地时区）\n", s.spec)
}

func (s *scheduler) stop() {
	ctx := s.c.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
}

// httpListen 启动 HTTP 服务并阻塞，收到信号优雅退出。
func httpListen(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
