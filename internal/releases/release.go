package releases

// Sections 是一个版本下的四类条目，顺序固定（新增 / 改进 / 修复 / 升级注意）。
//
// 用固定字段而不是 map：前端按固定顺序渲染，字段名就是契约（见 docs/api/api-dashboard.md）。
type Sections struct {
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	Fixed   []string `json:"fixed"`
	Upgrade []string `json:"upgrade"`
}

// Entry 是一个版本的发版说明。
type Entry struct {
	Version string `json:"version"` // 形如 0.9.0（不含 v 前缀）
	Date    string `json:"date"`    // YYYY-MM-DD，取 tag 的创建日期；缺省为空串
	// Retro 表示条目是事后从 git 历史回溯整理的（早期版本发版时没有写说明）。
	Retro    bool     `json:"retro"`
	Sections Sections `json:"sections"`
}

// sliceFor 返回某个分组的底层切片指针（解析期用来追加条目与续行）。
func (s *Sections) sliceFor(group string) *[]string {
	switch group {
	case keyAdded:
		return &s.Added
	case keyChanged:
		return &s.Changed
	case keyFixed:
		return &s.Fixed
	case keyUpgrade:
		return &s.Upgrade
	}
	return nil
}

// empty 把 nil 切片换成空切片：JSON 里必须是 []，前端不必判 null。
func (s *Sections) empty() {
	groups := []*[]string{&s.Added, &s.Changed, &s.Fixed, &s.Upgrade}
	for _, g := range groups {
		if *g == nil {
			*g = []string{}
		}
	}
}

// total 返回条目总数：一个版本一条都没有，说明格式写错了，解析按失败处理。
func (s Sections) total() int {
	return len(s.Added) + len(s.Changed) + len(s.Fixed) + len(s.Upgrade)
}
