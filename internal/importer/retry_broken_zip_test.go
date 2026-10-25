package importer

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSourcesToCandidatesBrokenZip 损坏的 zip 重试时记 failed，不中断整批。
func TestSourcesToCandidatesBrokenZip(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.zip")
	if err := os.WriteFile(bad, []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("写坏 zip: %v", err)
	}
	cands, issues := sourcesToCandidates([]string{bad + ":inner.txt"}, dir)
	if len(cands) != 0 || len(issues) != 1 || issues[0].kind != issueKindFailed {
		t.Fatalf("cands/issues = %d/%+v，期望 0/1（failed）", len(cands), issues)
	}
}
