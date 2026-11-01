package recall

import (
	"sort"
	"strings"
)

// interestCategoryNames 合并 L1 / L2 两维的分类名（各取 top3、去重、按权重顺序）。
//
// 两个维度在存储上分开是为了看板能分层展示，但「按分类取书」并不在乎层级：
// 同一个名字在两维都出现时只查一次，避免同一批书被召回两遍。
func interestCategoryNames(interest map[string]float64) []string {
	names := make([]string, 0, recallTopCategories*2)
	seen := make(map[string]bool, recallTopCategories*2)
	for _, dim := range []string{dimCategoryL1, dimCategoryL2} {
		for _, name := range topKeys(interest, dim, recallTopCategories) {
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// topKeys 取某个兴趣维度下权重最高的前 n 个 key（去掉 "dim:" 前缀）。
//
// 同权重按 key 字典序：排序必须完全确定，否则同一次请求的召回集合会随
// map 迭代顺序漂移，测试与线上排查都复现不了。
func topKeys(interest map[string]float64, dim string, n int) []string {
	type entry struct {
		key    string
		weight float64
	}
	prefix := dim + ":"
	entries := make([]entry, 0, len(interest))
	for k, w := range interest {
		if strings.HasPrefix(k, prefix) {
			entries = append(entries, entry{key: strings.TrimPrefix(k, prefix), weight: w})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].weight != entries[j].weight {
			return entries[i].weight > entries[j].weight
		}
		return entries[i].key < entries[j].key
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.key
	}
	return out
}
