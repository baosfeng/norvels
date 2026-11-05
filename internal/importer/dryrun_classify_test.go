package importer

import (
	"context"
	"testing"
)

// TestClassifyDryRunOutcomeEmptyResult 解析「成功」但一本书都没产出时记文件级失败
// 并留原因，不记成「将导入」——否则预检报告会把坏文件算成可导入。
func TestClassifyDryRunOutcomeEmptyResult(t *testing.T) {
	rep := &Report{}
	oc := parseOutcome{cand: candidate{source: "/books/空文件.txt", name: "空文件.txt"}, path: "/books/空文件.txt"}
	if err := classifyDryRunOutcome(context.Background(), nil, Options{}, oc, rep); err != nil {
		t.Fatalf("classifyDryRunOutcome: %v", err)
	}
	if rep.Files.Failed != 1 || rep.Files.WillImport != 0 {
		t.Fatalf("计数 = %+v，want failed=1 / willImport=0", rep.Files)
	}
	if len(rep.Errors) != 1 || rep.Errors[0].Reason != "parse: empty result" {
		t.Fatalf("错误样例 = %+v，want 一条 parse: empty result", rep.Errors)
	}
	if rep.Errors[0].Source != "/books/空文件.txt" {
		t.Fatalf("错误来源 = %q", rep.Errors[0].Source)
	}
}
