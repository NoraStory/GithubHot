package httpapi

import (
	"log"
	"runtime/debug"
)

// goSafe 请求路径派生的后台 goroutine 的 panic 隔离：chi Recoverer 只保护
// handler 栈，handler 里 `go ...` 出来的 goroutine 一旦 panic 会直接终止
// 整个进程（favicon 解码、探针抓取、指纹落库等都曾属于这类路径）。
// 代价是 goroutine 内无法直接返回错误——失败路径自行打日志。
func goSafe(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic-recovered] %s: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}
