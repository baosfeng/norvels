package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// 账号（迁移 0016）：一个入口进来，**没有密码**。会话见 user_sessions.go。
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
// ---- last_seen_at 的写入口只有一处 ----
// 见 touchUserLastSeen：登录 / 单入口 / 注册即登录都走它。曾经这条链路断在
// 「注册只建会话不刷时间」上，用户明明在用却显示「从未登录」，所以现在把
// 「会话建立」与「活跃时间落库」绑在同一个 api 层函数里（enterAs）。

// 账号名的跨边界长度（按 rune 计）。
const (
	minUserNameRunes = 1
	maxUserNameRunes = 32
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
//
// 建号**不**写 last_seen_at（列表要区分「建了号却从没进来过」）：
// 活跃时间是登录动作的产物，注册只建号——「注册即登录」由调用方另建会话时刷新，
// 见 handleRegister / handleEnter 里的 touchUserLastSeen。若在这里写，就等于把
// 「有人抢先建了同名号」也算成一次登录，列表会多出一条假的活跃记录。
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

// TouchUser 刷新这个账号的 last_seen_at 并返回落库后的账号。
//
// 只在「有人带着这个账号进来了」的路径上调用（登录 / 单入口 / 注册即登录），
// 不在每次读接口时调用：last_seen_at 的语义是「最近一次登录」，不是「最近一次翻页」。
// 返回值从库里读回，保证回显的时间与排序用的时间是同一个字符串。
func (s *Store) TouchUser(ctx context.Context, id int64) (User, error) {
	if id <= 0 {
		return User{}, fmt.Errorf("%w: 账号 id %d", ErrInvalidArgument, id)
	}
	if err := touchUserLastSeen(ctx, s.db, id, nil); err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, id)
}

// EnterUser 是单入口：账号不存在就建号并登录，存在就直接登录。created 报告是否新建。
//
// 先查后建，都在同一个事务里完成判重与插入（与 RegisterUser 同一口径：
// TRIM + 大小写折叠），所以并发两个同名 enter 也只会建出一个账号——
// 另一个撞唯一索引后回头走「已存在」分支，不会 500。
func (s *Store) EnterUser(ctx context.Context, name string) (u User, created bool, err error) {
	clean, err := cleanUserName(name)
	if err != nil {
		return User{}, false, err
	}
	if existing, err := s.userByName(ctx, clean); err == nil {
		if err := touchUserLastSeen(ctx, s.db, existing.ID, &existing); err != nil {
			return User{}, false, err
		}
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return User{}, false, err
	}
	u, err = s.RegisterUser(ctx, clean)
	if errors.Is(err, ErrConflict) {
		// 竞态：这个名字刚刚被别人建出来了（判重时还看不见）。重新读一次按「已存在」处理，
		// 不把内部碰撞丢给用户。读不到才是真异常，原样上报。
		existing, readErr := s.userByName(ctx, clean)
		if readErr != nil {
			return User{}, false, err
		}
		if err := touchUserLastSeen(ctx, s.db, existing.ID, &existing); err != nil {
			return User{}, false, err
		}
		return existing, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	// 单入口 = 建号并立刻登录：返回体里的 lastSeenAt 必须是刚落库的时间，
	// 否则「刚进来」的账号在列表里仍显示「从未登录」。
	if u, err = s.TouchUser(ctx, u.ID); err != nil {
		return User{}, false, err
	}
	return u, true, nil
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
	if err := touchUserLastSeen(ctx, s.db, u.ID, &u); err != nil {
		return User{}, err
	}
	return u, nil
}

// touchUserLastSeen 把 last_seen_at 刷成当前时间，并把同一个值回填到 u。
//
// 只在这里写时间、且只写一次：账号列表按它排序、界面按它说「上次活跃」，
// 落库值与回显值必须是同一个字符串，否则会出现「刚登录却显示几分钟前」。
//
// 不在 UserBySessionToken / 每次读接口时刷新：那是每个请求一次写放大，
// 而 last_seen_at 的语义是「最近一次登录」，不是「最近一次翻页」。
func touchUserLastSeen(ctx context.Context, ex execer, userID int64, u *User) error {
	at := formatTime(nowUTC())
	if _, err := ex.ExecContext(ctx,
		"UPDATE users SET last_seen_at = ? WHERE id = ?", at, userID); err != nil {
		return fmt.Errorf("store: touch user %d: %w", userID, err)
	}
	if u != nil {
		u.LastSeenAt = at
	}
	return nil
}

// execer 是 *sql.DB 与 *sql.Tx 的公共执行面（同一句写既能在事务里跑、也能单独跑）。
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// UserByID 按 id 读账号；不存在返回 ErrNotFound。
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	if id <= 0 {
		return User{}, fmt.Errorf("%w: 账号 id %d", ErrInvalidArgument, id)
	}
	return scanUser(s.db.QueryRowContext(ctx, userSelectColumns+" WHERE id = ?", id))
}

// maxListUsers 是账号列表一次返回的上限。
//
// 封顶 50 的理由：这个列表只服务登录页的「选账号」，一个人 / 一个家庭翻不到
// 第 51 个账号；而无上限的全量返回会让「被脚本批量建过号」的库拖慢每一次
// 打开登录页。截断时 total 仍是账号总数，前端据此提示「还有更多」。
const maxListUsers = 50

// ListUsers 返回账号列表（最多 maxListUsers 条）与账号总数。
//
// 排序是「最近活跃在前」：last_seen_at 倒序；从未登录（NULL）的排最后；
// 时间相同按 id 倒序兜底（后建的在前）。NULL 在 SQLite 的 DESC 里反而排最前，
// 所以先用 IS NULL 分组成「有时间的一批 / 没有时间的一批」，顺序才是登录页
// 想要的样子。不做分页参数：列表只服务登录页的「选账号」。
func (s *Store) ListUsers(ctx context.Context) ([]User, int, error) {
	total, err := s.countUsers(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx,
		userSelectColumns+" ORDER BY last_seen_at IS NULL, last_seen_at DESC, id DESC LIMIT ?",
		maxListUsers)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0, min(total, maxListUsers))
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: list users rows: %w", err)
	}
	return users, total, nil
}

// countUsers 数全库账号；列表被截断时 total 仍然是这个数。
func (s *Store) countUsers(ctx context.Context) (int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count users: %w", err)
	}
	return total, nil
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
