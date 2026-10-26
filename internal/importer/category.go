package importer

import (
	"path/filepath"
	"strings"
)

// categoryFromPath 从「相对导入根的路径」推断 (大分类, 小分类)。
//
// 规则对齐网盘按起点规范整理出的两级目录：
//   - 第一级目录 = 大分类，第二级目录 = 小分类；文件所在层级不再往下猜；
//   - 剥掉纯数字前缀（01玄幻 → 玄幻），首尾空白也去掉；
//   - 下划线开头的目录（_待删除_*）与空名目录返回空分类；
//   - 路径不足两级（文件直接放在根下）返回空分类。
func categoryFromPath(rel string) (l1, l2 string) {
	rel = strings.TrimSpace(filepath.ToSlash(filepath.Clean(rel)))
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
		return "", ""
	}
	parts := strings.Split(rel, "/")
	// 最后一段是文件名，目录层级 = len(parts)-1。
	if len(parts) < 2 {
		return "", ""
	}
	dirs := parts[:len(parts)-1]
	l1 = normalizeCategoryName(dirs[0])
	if l1 == "" {
		return "", ""
	}
	if len(dirs) >= 2 {
		l2 = normalizeCategoryName(dirs[1])
	}
	return l1, l2
}

// normalizeCategoryName 归一化单级目录名：去空白、剥数字前缀、下划线开头视为不可命名。
func normalizeCategoryName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "_") {
		return ""
	}
	trimmed := strings.TrimLeft(name, "0123456789")
	if trimmed == "" {
		return name
	}
	return strings.TrimSpace(trimmed)
}

// annotateCategories 给候选条目补上分类：按相对导入根的路径推断。
//
// zip 条目的可读路径在临时解包目录里，分类必须从包路径推断（source 冒号前的部分）；
// source 对普通文件就是文件自身路径。
func annotateCategories(cands []candidate, root string) {
	for i := range cands {
		p := cands[i].path
		if zipPath, inner, ok := strings.Cut(cands[i].source, ":"); ok && inner != "" {
			p = zipPath
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			continue
		}
		cands[i].categoryL1, cands[i].categoryL2 = categoryFromPath(rel)
	}
}
