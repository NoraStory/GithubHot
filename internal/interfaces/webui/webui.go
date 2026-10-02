// Package webui 内嵌 Vue3 前端构建产物（web/dist 构建后复制到本目录 dist/）。
// 构建流程：cd web && npm ci && npm run build，然后同步 dist 到 internal/interfaces/webui/dist
package webui

import (
	"embed"
)

//go:embed all:dist
var Dist embed.FS

// FS 返回 dist 根。
func FS() embed.FS { return Dist }
