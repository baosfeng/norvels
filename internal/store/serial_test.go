package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestSerialStatusRoundTrip 连载状态可写可读，且不碰解析状态 status。
func TestSerialStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	book := sampleBook("连载测试", "作者乙", "/books/serial.txt", strings.Repeat("d", 64))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.SerialStatus != SerialStatusUnknown {
		t.Errorf("新书 serialStatus = %q，期望空串", detail.SerialStatus)
	}
	if detail.Status != "parsed" {
		t.Errorf("解析状态 status = %q，期望 parsed", detail.Status)
	}

	for _, want := range []string{SerialStatusOngoing, SerialStatusCompleted, SerialStatusUnknown} {
		status := want
		if err := st.UpdateBookMeta(ctx, res.BookID, BookMetaPatch{SerialStatus: &status}); err != nil {
			t.Fatalf("UpdateBookMeta(%q): %v", want, err)
		}
		got, err := st.GetBook(ctx, res.BookID)
		if err != nil {
			t.Fatalf("GetBook: %v", err)
		}
		if got.SerialStatus != want {
			t.Errorf("serialStatus = %q，期望 %q", got.SerialStatus, want)
		}
		if got.Status != "parsed" {
			t.Errorf("改连载状态不该影响解析状态 status: %q", got.Status)
		}
	}
}

// TestSerialStatusRejectsIllegal 非法取值在写入侧也拦住（api 层 400 的第二道防线）。
func TestSerialStatusRejectsIllegal(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	book := sampleBook("连载非法值", "作者丙", "/books/serial-bad.txt", strings.Repeat("e", 64))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	bad := "abc"
	err = st.UpdateBookMeta(ctx, res.BookID, BookMetaPatch{SerialStatus: &bad})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("非法 serialStatus err = %v，期望 ErrInvalidArgument", err)
	}
	got, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.SerialStatus != SerialStatusUnknown {
		t.Errorf("非法值被写进去了: %q", got.SerialStatus)
	}
}

// TestLastChapterFields 详情返回 idx 最大的章节（含未分卷的番外）。
func TestLastChapterFields(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	book := sampleBook("最新章节", "作者丁", "/books/last.txt", strings.Repeat("f", 64))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.LastChapterIdx != 2 {
		t.Errorf("lastChapterIdx = %d，期望 2", detail.LastChapterIdx)
	}
	if detail.LastChapterTitle != "番外 小记" {
		t.Errorf("lastChapterTitle = %q，期望「番外 小记」", detail.LastChapterTitle)
	}

	// 无章节的书：0 与空串，不报错。
	empty := &model.ParsedBook{
		Title: "空书", Author: "作者丁", SourcePath: "/books/empty.txt", SourceName: "empty.txt",
		Encoding: "utf-8", FileSize: 16, SHA256: strings.Repeat("0", 64),
	}
	emptyRes, err := st.SaveBook(ctx, empty, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook empty: %v", err)
	}
	emptyDetail, err := st.GetBook(ctx, emptyRes.BookID)
	if err != nil {
		t.Fatalf("GetBook empty: %v", err)
	}
	if emptyDetail.LastChapterIdx != 0 || emptyDetail.LastChapterTitle != "" {
		t.Errorf("无章节书的最后章节 = (%d, %q)，期望 (0, \"\")",
			emptyDetail.LastChapterIdx, emptyDetail.LastChapterTitle)
	}
}
