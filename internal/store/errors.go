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
	// ErrSchemaTooNew 表示库由更新版本迁移过：拒绝打开并写入，避免静默数据损坏。
	// 具体版本在 *SchemaVersionError 上（errors.As 可取），见 schema_guard.go。
	ErrSchemaTooNew = errors.New("store: database schema is newer than this binary")
)
