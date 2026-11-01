package recommend

import "testing"

func scored(book, author int64, cat string, score float64) Scored {
	return Scored{
		Candidate: Candidate{BookID: book, AuthorID: author, CategoryL1: cat},
		Score:     score,
		Reason:    ReasonHot,
	}
}

func TestDiversifyEmpty(t *testing.T) {
	if got := Diversify(nil, 10, 2, 4); got != nil {
		t.Fatalf("空输入应返回 nil，得到 %v", got)
	}
	if got := Diversify([]Scored{scored(1, 0, "", 9)}, 0, 2, 4); got != nil {
		t.Fatalf("limit=0 应返回 nil，得到 %v", got)
	}
}

func TestDiversifyAuthorCap(t *testing.T) {
	items := []Scored{
		scored(1, 7, "武侠", 9),
		scored(2, 7, "都市", 8),
		scored(3, 7, "仙侠", 7),
		scored(4, 8, "科幻", 6),
	}
	got := Diversify(items, 10, 2, 4)
	if len(got) != 3 {
		t.Fatalf("同作者上限 2 时应留 3 条，得到 %d", len(got))
	}
	if got[0].BookID != 1 || got[1].BookID != 2 || got[2].BookID != 4 {
		t.Fatalf("跳过超配额项后顺序 = %v", bookIDs(got))
	}
}

func TestDiversifyCategoryCap(t *testing.T) {
	items := []Scored{
		scored(1, 1, "武侠", 9),
		scored(2, 2, "武侠", 8),
		scored(3, 3, "武侠", 7),
		scored(4, 4, "武侠", 6),
		scored(5, 5, "武侠", 5),
	}
	got := Diversify(items, 10, 2, 4)
	if len(got) != 4 {
		t.Fatalf("同分类上限 4 时应留 4 条，得到 %d", len(got))
	}
}

func TestDiversifyDefaultsWhenCapsUnset(t *testing.T) {
	authors := []Scored{
		scored(1, 7, "武侠", 9), scored(2, 7, "都市", 8), scored(3, 7, "仙侠", 7),
	}
	if got := Diversify(authors, 10, 0, 0); len(got) != 2 {
		t.Fatalf("上限传 0 时应取默认作者上限 2，得到 %d", len(got))
	}
	cats := []Scored{
		scored(1, 1, "武侠", 9), scored(2, 2, "武侠", 8), scored(3, 3, "武侠", 7),
		scored(4, 4, "武侠", 6), scored(5, 5, "武侠", 5),
	}
	if got := Diversify(cats, 10, 0, 0); len(got) != 4 {
		t.Fatalf("上限传 0 时应取默认分类上限 4，得到 %d", len(got))
	}
}

func TestDiversifyUnknownDimsDoNotConsumeCaps(t *testing.T) {
	items := []Scored{
		scored(1, 0, "", 9), scored(2, 0, "", 8), scored(3, 0, "", 7),
	}
	if got := Diversify(items, 10, 2, 4); len(got) != 3 {
		t.Fatalf("无作者/无分类的补位项不占配额，得到 %d 条", len(got))
	}
}

func TestDiversifyRespectsLimitAndOrder(t *testing.T) {
	items := []Scored{
		scored(1, 1, "武侠", 9), scored(2, 2, "都市", 8), scored(3, 3, "仙侠", 7),
		scored(4, 4, "科幻", 6),
	}
	got := Diversify(items, 2, 2, 4)
	if len(got) != 2 || got[0].BookID != 1 || got[1].BookID != 2 {
		t.Fatalf("应按原顺序取前 2 条，得到 %v", bookIDs(got))
	}
	if got[0].Score != 9 || got[1].Reason != ReasonHot {
		t.Fatalf("重排不应改写分数/理由: %+v", got)
	}
}
