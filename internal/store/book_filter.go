package store

import "strings"

// 列表筛选：把 BookQuery 编译成 WHERE 片段与参数。
//
// 单独成文件的原因：筛选维度会持续长（读完/连载/标签/作者…），
// 与列表查询、扫描、分页放在一起会把 books.go 顶过 400 行上限。
// bookFilter 把 BookQuery 编译成 WHERE 片段与参数。
func bookFilter(q BookQuery) ([]string, []any) {
	var (
		where []string
		args  []any
	)
	if s := strings.TrimSpace(q.Q); s != "" {
		pat := "%" + escapeLike(s) + "%"
		where = append(where, "(b.title LIKE ? ESCAPE '!' OR a.name LIKE ? ESCAPE '!')")
		args = append(args, pat, pat)
	}
	if q.AuthorID != 0 {
		where = append(where, "b.author_id = ?")
		args = append(args, q.AuthorID)
	}
	if t := strings.TrimSpace(q.Tag); t != "" {
		where = append(where, "EXISTS (SELECT 1 FROM book_tags bt JOIN tags t ON t.id = bt.tag_id "+
			"WHERE bt.book_id = b.id AND t.name = ?)")
		args = append(args, t)
	}
	// '' 表示不过滤（全部）：未标记不该成为一个可见筛选项，与 api 层口径一致。
	if rs := strings.TrimSpace(q.ReadStatus); rs != "" {
		where = append(where, "b.read_status = ?")
		args = append(args, rs)
	}
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
	// 书单是 M:N 关联，存在性子查询即可（collection_books 很小，不必 JOIN 去重）。
	if q.Collection != 0 {
		where = append(where, "EXISTS (SELECT 1 FROM collection_books cb "+
			"WHERE cb.book_id = b.id AND cb.collection_id = ?)")
		args = append(args, q.Collection)
	}
	// 连载状态：none 是「未知（''）」的哨兵值，其余按取值精确匹配。
	if ss := strings.TrimSpace(q.SerialStatus); ss != "" {
		if ss == SerialStatusNone {
			where = append(where, "b.serial_status = ''")
		} else {
			where = append(where, "b.serial_status = ?")
			args = append(args, ss)
		}
	}
	return where, args
}
