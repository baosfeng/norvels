package api

import (
	"net/http"

	"norvels/internal/store"
)

// categoryListResponse 是两级分类树：大分类下挂小分类，各自带书数。
//
// 未分类的书（l1 为空串）也保留在树里：导入时目录层级不足两级的书会落到这里，
// 前端用「未分类」文案渲染，不隐藏——否则书数对不上会让人以为丢了书。
type categoryListResponse struct {
	Items []categoryTreeItem `json:"items"`
	Total int                `json:"total"`
}

// categoryTreeItem 是一个大分类节点。
type categoryTreeItem struct {
	L1    string             `json:"l1"`
	Count int                `json:"count"`
	Items []categoryLeafItem `json:"items"`
}

// categoryLeafItem 是大分类下的小分类节点；L2 为空表示该大分类下未细分。
type categoryLeafItem struct {
	L2    string `json:"l2"`
	Count int    `json:"count"`
}

// handleListCategories 返回分类树（大分类 → 小分类 → 书数）。
func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.ListCategories(r.Context())
	if err != nil {
		s.fail(w, r, err, "分类列表不可用")
		return
	}
	writeJSON(w, http.StatusOK, categoryTree(rows))
}

// categoryTree 把扁平的 (l1,l2,count) 行折成两级树，保持 store 的字典序。
//
// 用 map 记住每个大分类在切片里的下标，避免每行线性查找（分类数会随目录增长）。
func categoryTree(rows []store.CategoryCount) categoryListResponse {
	out := categoryListResponse{Items: make([]categoryTreeItem, 0, 16)}
	idx := make(map[string]int, 16)
	for _, row := range rows {
		n, ok := idx[row.L1]
		if !ok {
			n = len(out.Items)
			idx[row.L1] = n
			out.Items = append(out.Items, categoryTreeItem{L1: row.L1})
		}
		out.Items[n].Count += row.Count
		out.Items[n].Items = append(out.Items[n].Items, categoryLeafItem{L2: row.L2, Count: row.Count})
		out.Total += row.Count
	}
	return out
}
