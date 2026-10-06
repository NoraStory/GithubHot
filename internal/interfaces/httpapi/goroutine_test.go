package httpapi

import "testing"

// TestGoSafeRecoversPanic goSafe 内的 panic 不冒泡（否则请求路径派生的
// goroutine panic 会终止整个进程）。
func TestGoSafeRecoversPanic(t *testing.T) {
	done := make(chan struct{})
	goSafe("test-panic", func() {
		defer close(done)
		panic("boom")
	})
	<-done // panic 已被 recover， goroutine 正常退出
}
