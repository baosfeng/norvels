package store

import "strings"

// bookCategoryFilter 编译「分类维度」的筛选条件：系列名、评分、书籍大/小分类。
//
// 从 bookFilter 拆出来是为了守住门禁：这些条件只读 books 自身列，
// 不涉及作者、标签、书单这些跨表关联，拆开后两边的分支数都留有余量。
func bookCategoryFilter(q BookQuery) ([]string, []any) {
	var (
		where []string
		args  []any
	)
	// 系列名与写入侧同口径（TRIM）；空系列不参与筛选。
	if se := strings.TrimSpace(q.Series); se != "" {
		where = append(where, "TRIM(b.series) = ?")
		args = append(args, se)
	}
	// 评分 0 = 未评，不作为一个可见筛选项（与「未标记不筛」同一取舍）。
	if q.Rating > 0 {
		where = append(where, "b.rating = ?")
		args = append(args, q.Rating)
	}
	appendCategoryWhere(&where, &args, q.CategoryL1, q.CategoryL2)
	return where, args
}

// appendCategoryWhere 追加两级分类条件：只给 L1 = 该大分类全部；两级都给 = 精确到小分类。
//
// L2 依附于 L1：只给 L2 时按「不过滤」处理（前端不会出现这种组合，后端不必猜）。
func appendCategoryWhere(where *[]string, args *[]any, l1, l2 string) {
	l1 = strings.TrimSpace(l1)
	if l1 == "" {
		return
	}
	*where = append(*where, "b.category_l1 = ?")
	*args = append(*args, l1)
	if l2 = strings.TrimSpace(l2); l2 != "" {
		*where = append(*where, "b.category_l2 = ?")
		*args = append(*args, l2)
	}
}
