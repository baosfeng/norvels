package importer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

func TestRunJudgeSkipKeepsBookOut(t *testing.T) {
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "a.txt", sampleText("甲书", "作者甲"))

	o := opts(dir)
	o.Judge = func(context.Context, *model.ParsedBook) (Verdict, error) {
		return Verdict{Import: false, Reason: "AI 判定不入库：残卷"}, nil
	}
	res, err := Run(context.Background(), st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Skipped != 1 || len(res.Imported) != 0 {
		t.Fatalf("判为不入库应为 1 跳过 0 导入，得到 %+v", res)
	}
}

func TestRunJudgeErrorIsFailClosed(t *testing.T) {
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "b.txt", sampleText("乙书", "作者乙"))

	o := opts(dir)
	o.Judge = func(context.Context, *model.ParsedBook) (Verdict, error) {
		return Verdict{}, errors.New("网络错误")
	}
	res, err := Run(context.Background(), st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Skipped != 1 || len(res.Imported) != 0 {
		t.Fatalf("判定不可用应按不入库处理（fail-closed），得到 %+v", res)
	}
}

func TestRunJudgeSeesParsedBook(t *testing.T) {
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "c.txt", sampleText("丙书", "作者丙"))

	var seen *model.ParsedBook
	o := opts(dir)
	o.Judge = func(_ context.Context, b *model.ParsedBook) (Verdict, error) {
		seen = b
		return Verdict{Import: true}, nil
	}
	res, err := Run(context.Background(), st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen == nil {
		t.Fatal("判定器没有被调用")
	}
	if strings.TrimSpace(seen.Title) == "" || len(seen.Chapters) < 3 {
		t.Fatalf("判定器看到的书不完整: title=%q chapters=%d", seen.Title, len(seen.Chapters))
	}
	if len(seen.Chapters[0].Content) == 0 {
		t.Fatal("判定器拿到的正文是空的，AI 看不到样章")
	}
	if len(res.Imported) != 1 {
		t.Fatalf("判为入库应写一本，得到 %+v", res)
	}
}

func TestRunJudgeIgnoresBelowMinChapters(t *testing.T) {
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "d.txt", sampleText("丁书", "作者丁"))

	called := false
	o := opts(dir)
	o.MinChapters = 100 // 门槛先挡掉，判定器不该被调用（省一次调用）
	o.Judge = func(context.Context, *model.ParsedBook) (Verdict, error) {
		called = true
		return Verdict{Import: true}, nil
	}
	res, err := Run(context.Background(), st, o, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called {
		t.Fatal("章节门槛应排在判定之前：被门槛挡掉的书不该花钱问判定器")
	}
	if res.Skipped != 1 || len(res.Imported) != 0 {
		t.Fatalf("得到 %+v", res)
	}
}
