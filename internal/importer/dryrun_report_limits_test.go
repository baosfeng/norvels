package importer

import "testing"

// TestReportErrorAndSampleCaps 预检报告的错误与样例都有上限（50 / DryRunMaxSamples），
// 超出部分直接丢弃——否则一次导入几千个坏文件就能把响应撑爆。
func TestReportErrorAndSampleCaps(t *testing.T) {
	rep := &Report{}
	for i := 0; i < DryRunMaxErrors+5; i++ {
		addReportError(rep, "书.txt", "boom")
	}
	if len(rep.Errors) != DryRunMaxErrors {
		t.Fatalf("错误条数 = %d，want %d", len(rep.Errors), DryRunMaxErrors)
	}
	for i := 0; i < DryRunMaxSamples+5; i++ {
		addReportSample(rep, parseOutcome{cand: candidate{source: "书.txt"}, books: nil}, false, "")
	}
	if len(rep.Sample) != DryRunMaxSamples {
		t.Fatalf("样例条数 = %d，want %d", len(rep.Sample), DryRunMaxSamples)
	}
	if rep.Sample[0].Source != "书.txt" {
		t.Fatalf("样例来源 = %q", rep.Sample[0].Source)
	}
}
