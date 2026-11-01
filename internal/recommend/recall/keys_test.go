package recall

import (
	"testing"

	"norvels/internal/recstore"
)

func TestTopKeysSortsByWeightThenKey(t *testing.T) {
	interest := map[string]float64{
		"tag:武侠":   3,
		"tag:悬疑":   5,
		"tag:科幻":   3,
		"author:1": 9, // 别的维度不参与
	}
	got := topKeys(interest, "tag", 3)
	want := []string{"悬疑", "武侠", "科幻"} // 同权重按 key 字典序，顺序必须完全确定
	if len(got) != len(want) {
		t.Fatalf("topKeys = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("topKeys = %v，期望 %v", got, want)
		}
	}
}

func TestTopKeysTruncatesToN(t *testing.T) {
	interest := map[string]float64{"tag:a": 1, "tag:b": 2, "tag:c": 3}
	if got := topKeys(interest, "tag", 2); len(got) != 2 || got[0] != "c" || got[1] != "b" {
		t.Fatalf("应只取权重最高的 2 个，实际 %v", got)
	}
}

func TestTopKeysOnEmptyOrMissingDim(t *testing.T) {
	if got := topKeys(nil, "tag", 3); len(got) != 0 {
		t.Fatalf("空兴趣应返回空，实际 %v", got)
	}
	if got := topKeys(map[string]float64{"author:1": 1}, "series", 3); len(got) != 0 {
		t.Fatalf("缺失维度应返回空，实际 %v", got)
	}
}

func TestInterestCategoryNamesMergesAndDedupes(t *testing.T) {
	interest := map[string]float64{
		"category_l1:武侠": 9, "category_l1:玄幻": 8, "category_l1:都市": 7,
		"category_l2:武侠": 6, "category_l2:江湖": 5,
	}
	got := interestCategoryNames(interest)
	// 各取 top3 后合并：L1 三名 → L2 的第一个是「武侠」（去重）→ 追加「江湖」。
	want := []string{"武侠", "玄幻", "都市", "江湖"}
	if len(got) != len(want) {
		t.Fatalf("interestCategoryNames = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("interestCategoryNames = %v，期望 %v", got, want)
		}
	}
}

func TestInterestCategoryNamesEmpty(t *testing.T) {
	if got := interestCategoryNames(nil); len(got) != 0 {
		t.Fatalf("空兴趣应返回空，实际 %v", got)
	}
}

func TestKeepCooccurTakesMax(t *testing.T) {
	res := New()
	res.KeepCooccur(recstore.Cooccur{OtherID: 5, Users: 3})
	res.KeepCooccur(recstore.Cooccur{OtherID: 5, Users: 9})
	res.KeepCooccur(recstore.Cooccur{OtherID: 5, Users: 4})
	if res.cooccur[5] != 9 {
		t.Fatalf("共读人数应取最大值 9，实际 %d", res.cooccur[5])
	}
}

func TestTabConstantsMatchWireValues(t *testing.T) {
	// api 侧按这三个字面量解析 HTTP 参数：改了这里不改前端就是静默失效。
	if TabForYou != "for_you" || TabResume != "resume" || TabRandom != "random" {
		t.Fatalf("tab 字面量变了: %q %q %q", TabForYou, TabResume, TabRandom)
	}
}

func TestDimConstantsMatchRecstoreKeys(t *testing.T) {
	// 兴趣 map 的 key 由 recstore 写、本包读：两组字面量必须一致。
	want := map[string]string{
		dimCategoryL1: "category_l1",
		dimCategoryL2: "category_l2",
		dimAuthor:     "author",
		dimTag:        "tag",
		dimSeries:     "series",
	}
	for got, expect := range want {
		if got != expect {
			t.Fatalf("维度字面量漂移: %q != %q", got, expect)
		}
	}
}
