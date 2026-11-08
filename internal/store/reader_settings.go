package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// readerSettingsUserID 解析账号维度：显式传入非零 id 优先，否则回落到 context 里的账号。
func readerSettingsUserID(ctx context.Context, ids ...int64) int64 {
	for _, id := range ids {
		if id > 0 {
			return id
		}
	}
	return ScopeUserID(ctx)
}

// GetReaderSettings 读取当前账号的阅读器设置 JSON；未配置返回 ok=false。
func (s *Store) GetReaderSettings(ctx context.Context, userID ...int64) (string, bool, error) {
	uid := readerSettingsUserID(ctx, userID...)
	var raw string
	var updatedAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT settings, updated_at FROM user_reader_settings WHERE user_id = ?", uid).
		Scan(&raw, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get reader settings user %d: %w", uid, err)
	}
	return raw, true, nil
}

// 兼容别名：不同调用方可能用不同命名。
func (s *Store) GetReaderSettingsJSON(ctx context.Context, userID ...int64) (json.RawMessage, bool, error) {
	raw, ok, err := s.GetReaderSettings(ctx, userID...)
	if err != nil || !ok {
		return nil, ok, err
	}
	return json.RawMessage(raw), true, nil
}
func (s *Store) GetUserReaderSettings(ctx context.Context, userID ...int64) (string, bool, error) {
	return s.GetReaderSettings(ctx, userID...)
}
func (s *Store) FetchReaderSettings(ctx context.Context, userID ...int64) (string, bool, error) {
	return s.GetReaderSettings(ctx, userID...)
}
func (s *Store) LoadReaderSettings(ctx context.Context, userID ...int64) (string, bool, error) {
	return s.GetReaderSettings(ctx, userID...)
}

func readerSettingsPayload(value any) (string, error) {
	var raw string
	switch v := value.(type) {
	case nil:
		return "", fmt.Errorf("%w: nil reader settings", ErrInvalidArgument)
	case string:
		raw = v
	case []byte:
		raw = string(v)
	case json.RawMessage:
		raw = string(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("store: encode reader settings: %w", err)
		}
		raw = string(b)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: empty reader settings", ErrInvalidArgument)
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return "", fmt.Errorf("%w: invalid reader settings json: %w", ErrInvalidArgument, err)
	}
	return raw, nil
}

// PutReaderSettings upsert 当前账号的阅读器设置 JSON。
func (s *Store) PutReaderSettings(ctx context.Context, value any, userID ...int64) error {
	return s.saveReaderSettings(ctx, value, userID...)
}

func (s *Store) saveReaderSettings(ctx context.Context, value any, userID ...int64) error {
	uid := readerSettingsUserID(ctx, userID...)
	payload, err := readerSettingsPayload(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO user_reader_settings(user_id, settings, updated_at) VALUES (?, ?, ?) "+
			"ON CONFLICT(user_id) DO UPDATE SET settings = excluded.settings, updated_at = excluded.updated_at",
		uid, payload, formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("store: put reader settings user %d: %w", uid, err)
	}
	return nil
}

// SaveReaderSettings / SetReaderSettings / ... 兼容别名。
func (s *Store) SaveReaderSettings(ctx context.Context, value any, userID ...int64) error {
	return s.PutReaderSettings(ctx, value, userID...)
}
func (s *Store) SetReaderSettings(ctx context.Context, value any, userID ...int64) error {
	return s.PutReaderSettings(ctx, value, userID...)
}
func (s *Store) UpdateReaderSettings(ctx context.Context, value any, userID ...int64) error {
	return s.PutReaderSettings(ctx, value, userID...)
}
func (s *Store) UpsertReaderSettings(ctx context.Context, value any, userID ...int64) error {
	return s.PutReaderSettings(ctx, value, userID...)
}
func (s *Store) PutReaderSettingsJSON(ctx context.Context, value any, userID ...int64) error {
	return s.PutReaderSettings(ctx, value, userID...)
}

// PutReaderSettingsForUser / ... 显式带 user id 的变体。
func (s *Store) PutReaderSettingsForUser(ctx context.Context, userID int64, value any) error {
	return s.PutReaderSettings(ctx, value, userID)
}
func (s *Store) GetReaderSettingsForUser(ctx context.Context, userID int64) (string, bool, error) {
	return s.GetReaderSettings(ctx, userID)
}
func (s *Store) SaveReaderSettingsForUser(ctx context.Context, userID int64, value any) error {
	return s.SaveReaderSettings(ctx, value, userID)
}
