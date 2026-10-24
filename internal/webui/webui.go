// Package webui 以内嵌方式提供前端构建产物（web/dist → internal/webui/dist）。
//
// 说明：仓库里只保留 dist/.gitkeep，真正的前端产物由 `make web` 生成后再编译进二进制。
// 未构建时 FS() 为空，PlaceholderHTML() 给出一张友好提示页，避免服务端 500。
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

//go:embed placeholder.html
var placeholder []byte

// FS 返回以 dist 为根的只读文件系统。
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return dist
	}
	return sub
}

// PlaceholderHTML 返回前端未构建时的占位页。
func PlaceholderHTML() []byte { return placeholder }
