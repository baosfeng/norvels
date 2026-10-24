package store

import "errors"

// 库层错误哨兵，供 api 层用 errors.Is 映射 HTTP 状态码。
var (
	// ErrNotFound 表示记录不存在（api 映射 404）。
	ErrNotFound = errors.New("store: not found")
	// ErrInvalidArgument 表示入参非法（api 映射 400）。
	ErrInvalidArgument = errors.New("store: invalid argument")
	// ErrConflict 表示并发冲突，例如已有 running 的导入任务（api 映射 409）。
	ErrConflict = errors.New("store: conflict")
)
