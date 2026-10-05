package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/crypto/acme/autocert"

	"github.com/NoraStory/GithubHot/internal/config"
	"github.com/NoraStory/GithubHot/internal/domain/tlsfp"
	"github.com/NoraStory/GithubHot/internal/interfaces/httpapi"
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
	return serveWithShutdown(ctx, srv, func() error { return srv.ListenAndServe() })
}

// ja4Collector P3-2 TLS 指纹捕获：GetConfigForClient 在握手期计算 JA4，
// http.Server.ConnContext 在连接开始服务请求时把它注入 context（fp/report 读取）。
// 关联键是裸 net.Conn——hello.Conn() 与 ConnContext 收到的 tls.Conn 不是同一对象，
// 用 tls.Conn.NetConn()（Go 1.18+）取回底层连接对上键。
type ja4Collector struct {
	mu     sync.Mutex
	byConn map[net.Conn]string
}

func newJA4Collector() *ja4Collector { return &ja4Collector{byConn: map[net.Conn]string{}} }

// configForClient GetConfigForClient 回调：返回 (nil, nil) = 使用默认 TLS 配置。
// 注：本仓库工具链下 ClientHelloInfo.Conn 是导出字段（net.Conn）。
func (c *ja4Collector) configForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	if hello == nil || hello.Conn == nil {
		return nil, nil
	}
	if ja4 := tlsfp.FromClientHello(hello); ja4 != "" {
		c.mu.Lock()
		c.byConn[hello.Conn] = ja4
		c.mu.Unlock()
	}
	return nil, nil
}

// resolve 请求时刻取指纹（此时握手必已完成，条目必在；取不到返回空串并降级）。
func (c *ja4Collector) resolve(conn net.Conn) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byConn[conn]
}

// drop 连接关闭时清理（http.Server.ConnState → StateClosed）。
func (c *ja4Collector) drop(conn net.Conn) {
	if nc, ok := conn.(interface{ NetConn() net.Conn }); ok {
		conn = nc.NetConn()
	}
	c.mu.Lock()
	delete(c.byConn, conn)
	c.mu.Unlock()
}

// ja4Ref 连接上下文里的延迟解析引用（规格书 P3-2）。
type ja4Ref struct {
	c    *ja4Collector
	conn net.Conn
}

func (d *ja4Ref) ResolveJA4() string { return d.c.resolve(d.conn) }

func (c *ja4Collector) connContext(ctx context.Context, conn net.Conn) context.Context {
	if nc, ok := conn.(interface{ NetConn() net.Conn }); ok {
		conn = nc.NetConn()
	}
	// 握手在此刻尚未发生（惰性），注入引用而非值；首个请求读取时条目已在。
	return httpapi.WithJA4(ctx, &ja4Ref{c: c, conn: conn})
}

// serveHTTP P3-1 三模式启动：证书 TLS / ACME 自动签发 / 纯 HTTP（本地开发，
// JA4 随之关闭）。TLS 模式下挂 JA4 捕获；REDIRECT_HTTP=1 或 ACME 时监听 80 端口
// 做 ACME HTTP-01 挑战与 301 跳转。
func serveHTTP(ctx context.Context, cfg *config.Config, addr string, handler http.Handler) error {
	if !cfg.TLSEnabled() {
		return httpListen(ctx, addr, handler)
	}
	collector := newJA4Collector()
	tlsConf := &tls.Config{GetConfigForClient: collector.configForClient}
	var acmeMgr *autocert.Manager
	if cfg.ACMEDomains != "" {
		acmeMgr = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(filepath.Join(cfg.DataDir, "acme")),
			HostPolicy: autocert.HostWhitelist(strings.Split(cfg.ACMEDomains, ",")...),
		}
		tlsConf.GetCertificate = acmeMgr.GetCertificate
		tlsConf.NextProtos = []string{"acme-tls/1", "h2", "http/1.1"}
		fmt.Printf("[tls] ACME 已启用（%s），证书缓存 data/acme\n", cfg.ACMEDomains)
	} else {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return fmt.Errorf("加载 TLS 证书失败: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	}
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: tlsConf, ConnContext: collector.connContext,
		ConnState: func(conn net.Conn, state http.ConnState) {
			if state == http.StateClosed {
				collector.drop(conn)
			}
		}}

	// 80 端口：301 跳 HTTPS（REDIRECT_HTTP=1）；ACME 模式下必须监听（HTTP-01 挑战）。
	if cfg.RedirectHTTP || acmeMgr != nil {
		go func() {
			var h http.Handler = http.HandlerFunc(redirectHTTPS(tlsPort(addr)))
			if acmeMgr != nil {
				h = acmeMgr.HTTPHandler(h)
			}
			httpSrv := &http.Server{Addr: ":80", Handler: h, ReadHeaderTimeout: 10 * time.Second}
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("[tls] 80 端口监听失败（HTTP→HTTPS 跳转/ACME 挑战不可用）: %v\n", err)
			}
		}()
	}
	return serveWithShutdown(ctx, srv, func() error { return srv.ListenAndServeTLS("", "") })
}

// redirectHTTPS 80 → 301 → HTTPS。目标含主服务端口（非 443 时显式带上，
// 本地/沙盒的 8443 等端口同样可跳转）。
func redirectHTTPS(tlsPort string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host
		if tlsPort != "" && tlsPort != "443" {
			target += ":" + tlsPort
		}
		http.Redirect(w, r, target+r.RequestURI, http.StatusMovedPermanently)
	}
}

// tlsPort 从监听地址取端口部分。
func tlsPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}

// serveWithShutdown 共用的启动 + 优雅退出骨架。
func serveWithShutdown(ctx context.Context, srv *http.Server, serve func() error) error {
	errCh := make(chan error, 1)
	go func() {
		if err := serve(); err != nil && err != http.ErrServerClosed {
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
