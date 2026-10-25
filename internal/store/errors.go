package store

import (
	"errors"
	"fmt"
)

// 库层错误哨兵，供 api 层用 errors.Is 映射 HTTP 状态码。
var (
	// ErrNotFound 表示记录不存在（api 映射 404）。
	ErrNotFound = errors.New("store: not found")
	// ErrInvalidArgument 表示入参非法（api 映射 400）。
	ErrInvalidArgument = errors.New("store: invalid argument")
	// ErrConflict 表示并发冲突，例如已有 running 的导入任务（api 映射 409）。
	ErrConflict = errors.New("store: conflict")
	// ErrSchemaTooNew 表示库由更新版本迁移过：拒绝打开并写入，避免静默数据损坏。
	// 具体版本在 *SchemaVersionError 上（errors.As 可取），见 schema_guard.go。
	ErrSchemaTooNew = errors.New("store: database schema is newer than this binary")
	// ErrBlockCorrupted 表示某个压缩块解压失败或块内区间非法。
	//
	// 独立哨兵而不是复用 ErrCorruptContent：正文损坏是「读者能感知的局部故障」，
	// api 层要把它映射成明确的信息（这一章读不出来），而不是笼统的 500 内部错误。
	// 具体位置用 errors.As 取 *BlockError（含 book_id / seq / 章节范围）。
	ErrBlockCorrupted = errors.New("store: chapter block corrupted")
)

// BlockError 是一次块级损坏的定位信息。
//
// 为什么要带 book_id 和 seq：一个块约 1 MB（约 35 章）。几万本书的库里出现坏块时，
// 运维必须能直接定位「哪本书的第几块」，否则只能全库二分排查。
//
// 为什么还要带 First/LastChapterID：用户视角是「我读到某一章报错了」，把这一块
// 覆盖的章节范围一并带出来，前端才能在那一章显示「本章读取失败」，而不是
// 让整本书打不开。
type BlockError struct {
	BlockID int64 // chapter_blocks.id
	BookID  int64 // 所属书籍
	Seq     int   // 本书内块序号（从 1 开始）
	// FirstIdx / LastIdx 是本块覆盖的章节**序号**（chapters.idx，从 0 开始），
	// 与 chapter_blocks.first_idx / last_idx 同源。用 idx 而不是 chapter id，
	// 是因为运维对照目录时看到的就是「第几章」。
	FirstIdx int
	LastIdx  int
	// ChapterID 是触发这次读取的章节 id（0 表示块级校验、未指定具体章）。
	ChapterID int64
	Cause     error // 底层原因（解压失败 / 区间越界 / 记录缺失）
}

// Error 返回可直接进日志或响应的中文定位信息。
func (e *BlockError) Error() string {
	return fmt.Sprintf("压缩块损坏：book_id=%d block_seq=%d block_id=%d 章节 idx %d-%d（触发章节 id=%d）：%v",
		e.BookID, e.Seq, e.BlockID, e.FirstIdx, e.LastIdx, e.ChapterID, e.Cause)
}

// Unwrap 让 errors.Is 能同时命中 ErrBlockCorrupted 与底层原因（如 ErrCorruptContent）。
func (e *BlockError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrBlockCorrupted}
	}
	return []error{ErrBlockCorrupted, e.Cause}
}
