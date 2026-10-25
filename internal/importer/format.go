package importer

import (
	"path/filepath"
	"strings"
)

// 支持的导入格式（与 docs/specs/2026-10-25-import-formats.md 一致）。
const (
	FormatTXT  = "txt"
	FormatEPUB = "epub"
)

// FormatOf 返回文件名的导入格式；不支持的扩展名返回空串。
//
// 判定只看扩展名（大小写不敏感）：上传接口在「建任务前」就要把明显不支持的
// 文件挡回去，此处不做魔数嗅探；真正的容器校验在解析层（ebook.Open）。
func FormatOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt":
		return FormatTXT
	case ".epub":
		return FormatEPUB
	default:
		return ""
	}
}

// SupportedExt 报告扩展名是否可导入。
func SupportedExt(name string) bool { return FormatOf(name) != "" }

// SupportedFormats 返回面向用户的支持格式列表（错误提示用）。
func SupportedFormats() []string { return []string{"TXT", "EPUB"} }
