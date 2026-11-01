package recall

import (
	"testing"
	"time"

	"norvels/internal/recommend"
)

func TestAddKeepsFirstAndSkipsInvalid(t *testing.T) {
	res := New()
	res.Add(book(2))
	res.Add(book(1))
	res.Add(book(2)) // 重复
	res.Add(book(0)) // 非法
	if order := idsOf(res); len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("Add 应保序去重并跳过非法 id，实际 %v", order)
	}
}

func TestAddSimilarSkipsSelfAndKeepsFirstReason(t *testing.T) {
	res := New()
	res.selfID = 1
	reason := recommend.FixedReason(recommend.CodeSimilarAuthor)
	res.AddSimilar(book(1), reason) // 自己
	res.AddSimilar(book(2), reason)
	res.AddSimilar(book(2), recommend.FixedReason(recommend.CodeSimilarCategory)) // 不覆写
	if _, ok := res.Books[1]; ok {
		t.Fatal("种子书不该被收下")
	}
	cands := res.Candidates(Signals{})
	if len(cands) != 1 {
		t.Fatalf("应只有 1 个候选，实际 %d", len(cands))
	}
	if cands[0].SimilarReasonCode != recommend.CodeSimilarAuthor {
		t.Fatalf("先到的理由 code 不该被覆写，实际 %q", cands[0].SimilarReasonCode)
	}
	if !cands[0].Similar || cands[0].SimilarReason == "" {
		t.Fatalf("相似候选应带 Similar 标记与文案: %+v", cands[0])
	}
}

func TestCandidatesUsesFreshnessAndResumeFlags(t *testing.T) {
	res := New()
	bs := book(1)
	bs.ImportedAt = time.Now().UTC().Add(-24 * time.Hour)
	res.Add(bs)
	res.Resume[1] = 0.4
	c := res.Candidates(Signals{Now: time.Now().UTC()})[0]
	if !c.Resume {
		t.Fatal("有进度时应标记为续读候选")
	}
	if c.Fresh <= 0 || c.Fresh > 1 {
		t.Fatalf("新鲜度应落在 (0,1]，实际 %v", c.Fresh)
	}
}

func TestCandidatesEmptyResult(t *testing.T) {
	if got := New().Candidates(Signals{}); len(got) != 0 {
		t.Fatalf("空结果应产出空候选切片，实际 %d", len(got))
	}
}
