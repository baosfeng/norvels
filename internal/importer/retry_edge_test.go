package importer

import (
	"context"
	"testing"
)

// TestSourcesToCandidatesEdge 空源与重复源只记一次；不存在源记 failed。
func TestSourcesToCandidatesEdge(t *testing.T) {
	cands, issues := sourcesToCandidates([]string{"", "  ", "/nope/a.txt", "/nope/a.txt"}, t.TempDir())
	if len(cands) != 0 {
		t.Fatalf("候选数 = %d，期望 0", len(cands))
	}
	if len(issues) != 1 || issues[0].kind != issueKindFailed {
		t.Fatalf("issues = %+v，期望去重后 1 条 failed", issues)
	}
}

// TestDryRunErrors 覆盖 nil store 与源路径不存在的错误路径。
func TestDryRunErrors(t *testing.T) {
	if _, err := DryRun(context.Background(), nil, Options{}); err == nil {
		t.Error("nil store 应当报错")
	}
	if _, err := DryRun(context.Background(), newStore(t), Options{Dir: "/no/such/dir"}); err == nil {
		t.Error("不存在的目录应当报错")
	}
}
