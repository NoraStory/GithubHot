package httpapi

import (
	"net/http"
)

// privacyPageAPI /privacy 隐私政策页面路由入口：委托 SPA 处理，
// 由前端 Vue Router 渲染实际页面（页脚"隐私协议"为全页跳转，不能返回裸 JSON）。
func (s *Server) privacyPageAPI(w http.ResponseWriter, r *http.Request) {
	s.spa(w, r)
}
