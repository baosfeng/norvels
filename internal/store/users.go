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
	"unicode/utf8"
)

// 账号与会话（迁移 0016）：只注册 + 输账号登录，**没有密码**。
//
// ---- 为什么可以无密码 ----
// 本产品的部署前提是「个人 / 家庭局域网内的私有书库」（见 deploy/nas 与 ADR-0001）。
// 这里的账号标识的是「这堆阅读记录属于谁」，不是一个跨不可信网络的凭据。
// 要暴露到公网，正确做法是换掉整套鉴权，而不是给 users 加一列 password——
// 无密码设计摆在明面上，交给部署方自己判断能不能这么用。
//
// ---- 名字口径 ----
// 去首尾空白后 1–32 字（按 rune 计），唯一性按「TRIM + 大小写折叠」比较，与
// ux_users_name 表达式唯一索引完全同一口径（见 migrations/0016）：
//   - TRIM：否则「 reader」与「reader」是两个只差空格的账号；
//   - COLLATE NOCASE：否则「Alice」与「alice」也是两个账号，登录时自己也分不清。
// 只折叠 ASCII，中文名没有大小写概念、不受影响；存储的 name 仍是用户输入的原样。
//
// ---- 会话口径 ----
// token = crypto/rand 32 字节的 hex（64 字符），库里**只存它的 SHA-256**：
// 数据库被读到也无法拿去冒充登录，明文 token 只存在于用户的 HttpOnly Cookie。
// 选「服务端表」而不是 HMAC 签名：开放模式下没有可用的密钥（NORVELS_AUTH_TOKEN
// 可能为空），而服务端表能立即撤销（登出删行）、能审计、也不依赖进程密钥的持久化。

// 账号名的跨边界长度（按 rune 计）。
const (
	minUserNameRunes = 1
	maxUserNameRunes = 32
)

// 会话的边界：token 字节数与有效期（30 天，与旧共享口令 Cookie 一致）。
const (
	sessionTokenBytes = 32
	UserSessionTTL    = 30 * 24 * time.Hour
)

// User 是一个账号。没有密码字段，也没有邮箱 / 头像之类——目前只有名字有用。
type User struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"` // 空串 = 建号后从未登录过
}

// DefaultUserID 返回存量数据归属的默认账号 id。
//
// 未配置 NORVELS_AUTH_TOKEN（开放模式）时，所有请求都按这个账号读写个人数据：
// 升级后的使用方式与单用户时代完全一致。
func DefaultUserID() int64 { return defaultUserID }

// userScopeKey 是 request context 里存放「当前账号 id」的键。
type userScopeKey struct{}

// WithUserID 把解析出的账号 id 注入 context。
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userScopeKey{}, id)
}

// ScopeUserID 取当前账号 id；没有注入时回落到默认账号。
//
// 回落而不是报错：开放模式（未配置 NORVELS_AUTH_TOKEN）下本来就没有登录这回事，
// 单用户行为必须照旧可用。所有读写个人数据的 SQL 都必须经过这里取账号，
// 不允许在别处各写一份 ctx 取值逻辑。
func ScopeUserID(ctx context.Context) int64 {
	if id, ok := ctx.Value(userScopeKey{}).(int64); ok && id > 0 {
		return id
	}
	return defaultUserID
}

// RegisterUser 建号并返回新账号；重名返回 ErrConflict（api 映射 409）。
func (s *Store) RegisterUser(ctx context.Context, name string) (User, error) {
	clean, err := cleanUserName(name)
	if err != nil {
		return User{}, err
	}
	u := User{Name: clean, CreatedAt: formatTime(nowUTC())}
	err = s.inTx(ctx, "register user", func(tx *sql.Tx) error {
		if err := ensureUserNameFree(ctx, tx, clean); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"INSERT INTO users(name, created_at) VALUES (?, ?)", clean, u.CreatedAt)
		if err != nil {
			return wrapUserNameError(err, clean)
		}
		if u.ID, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("store: register user id: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// LoginUser 按账号名登录：不存在返回 ErrNotFound（api 映射 404 + 「请先注册」），
// 存在则刷新 last_seen_at 并返回账号。
func (s *Store) LoginUser(ctx context.Context, name string) (User, error) {
	clean, err := cleanUserName(name)
	if err != nil {
		return User{}, err
	}
	u, err := s.userByName(ctx, clean)
	if err != nil {
		return User{}, err
	}
	u.LastSeenAt = formatTime(nowUTC())
	if _, err := s.db.ExecContext(ctx,
		"UPDATE users SET last_seen_at = ? WHERE id = ?", u.LastSeenAt, u.ID); err != nil {
		return User{}, fmt.Errorf("store: touch user %d: %w", u.ID, err)
	}
	return u, nil
}

// UserByID 按 id 读账号；不存在返回 ErrNotFound。
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	if id <= 0 {
		return User{}, fmt.Errorf("%w: 账号 id %d", ErrInvalidArgument, id)
	}
	return scanUser(s.db.QueryRowContext(ctx, userSelectColumns+" WHERE id = ?", id))
}

// userByName 按「TRIM + 大小写折叠」查账号（与唯一索引同一口径）。
func (s *Store) userByName(ctx context.Context, name string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		userSelectColumns+" WHERE TRIM(name) = ? COLLATE NOCASE", name))
}

// userSelectColumns 是 User 的取数列（last_seen_at 可能为 NULL）。
const userSelectColumns = "SELECT id, name, created_at, COALESCE(last_seen_at, '') FROM users"

// rowScanner 是 *sql.Row 与 *sql.Rows 的公共扫描面（包内已有同名接口时复用）。
func scanUser(sc rowScanner) (User, error) {
	var u User
	if err := sc.Scan(&u.ID, &u.Name, &u.CreatedAt, &u.LastSeenAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, fmt.Errorf("%w: 账号不存在", ErrNotFound)
		}
		return User{}, fmt.Errorf("store: read user: %w", err)
	}
	return u, nil
}

// cleanUserName 归一化并校验账号名：去首尾空白后非空、不超上限。
func cleanUserName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	if utf8.RuneCountInString(clean) < minUserNameRunes {
		return "", fmt.Errorf("%w: 账号名不能为空", ErrInvalidArgument)
	}
	if utf8.RuneCountInString(clean) > maxUserNameRunes {
		return "", fmt.Errorf("%w: 账号名不能超过 %d 字", ErrInvalidArgument, maxUserNameRunes)
	}
	return clean, nil
}

// ensureUserNameFree 事务内查重（TRIM + 大小写折叠口径，与唯一索引一致）；
// 重名返回 ErrConflict。
func ensureUserNameFree(ctx context.Context, tx *sql.Tx, name string) error {
	var id int64
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM users WHERE TRIM(name) = ? COLLATE NOCASE", name).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("store: check user name: %w", err)
	}
	return fmt.Errorf("%w: 账号名 %q 已被占用", ErrConflict, name)
}

// wrapUserNameError 把「撞唯一索引」的写错误翻成 ErrConflict（并发竞态的兜底）。
func wrapUserNameError(err error, name string) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return fmt.Errorf("%w: 账号名 %q 已被占用", ErrConflict, name)
	}
	return fmt.Errorf("store: write user: %w", err)
}

// ---------- 会话 ----------

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
