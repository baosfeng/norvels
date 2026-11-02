package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 账号会话（迁移 0016 user_sessions）。
//
// ---- 会话口径 ----
// token = crypto/rand 32 字节的 hex（64 字符），库里**只存它的 SHA-256**：
// 数据库被读到也无法拿去冒充登录，明文 token 只存在于用户的 HttpOnly Cookie。
// 选「服务端表」而不是 HMAC 签名：开放模式下没有可用的密钥（NORVELS_AUTH_TOKEN
// 可能为空），而服务端表能立即撤销（登出删行）、能审计、也不依赖进程密钥的持久化。

// 会话的边界：token 字节数与有效期（30 天，与旧共享口令 Cookie 一致）。
const (
	sessionTokenBytes = 32
	UserSessionTTL    = 30 * 24 * time.Hour
)

// CreateUserSession 为账号建一条会话，返回**明文 token**（只此一次，库里存哈希）。
func (s *Store) CreateUserSession(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("%w: 账号 id %d", ErrInvalidArgument, userID)
	}
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	// 建会话是低频写，顺手清过期行即可，不额外起定时任务（清理失败不影响本次登录：
	// 读会话本来就带 expires_at 过滤，残留的过期行不会让谁多出权限）。
	_ = s.purgeExpiredSessions(ctx)
	now := nowUTC()
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO user_sessions(token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		hashSessionToken(token), userID, formatTime(now), formatTime(now.Add(UserSessionTTL))); err != nil {
		return "", fmt.Errorf("store: create user session: %w", err)
	}
	return token, nil
}

// UserBySessionToken 校验会话并返回所属账号；token 无效 / 过期返回 ErrNotFound。
func (s *Store) UserBySessionToken(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, fmt.Errorf("%w: 会话不存在", ErrNotFound)
	}
	var userID int64
	err := s.db.QueryRowContext(ctx,
		"SELECT user_id FROM user_sessions WHERE token_hash = ? AND expires_at > ?",
		hashSessionToken(token), formatTime(nowUTC())).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("%w: 会话不存在或已过期", ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("store: read user session: %w", err)
	}
	return s.UserByID(ctx, userID)
}

// DeleteUserSession 撤销一条会话（登出）；本来就不存在也返回 nil（幂等）。
func (s *Store) DeleteUserSession(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM user_sessions WHERE token_hash = ?", hashSessionToken(token)); err != nil {
		return fmt.Errorf("store: delete user session: %w", err)
	}
	return nil
}

// purgeExpiredSessions 删掉已过期的会话行。
//
// 失败不阻断主流程：清理是后台维护动作，读会话本身已经有 expires_at 过滤，
// 残留的过期行不会让谁多出权限。
func (s *Store) purgeExpiredSessions(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM user_sessions WHERE expires_at <= ?", formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: purge user sessions: %w", err)
	}
	return nil
}

// newSessionToken 生成不可猜的会话 token（crypto/rand，32 字节 → 64 位 hex）。
func newSessionToken() (string, error) {
	var b [sessionTokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("store: generate session token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// hashSessionToken 是库里存储会话凭据的唯一形态（SHA-256 hex）。
func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
