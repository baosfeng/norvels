package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SettingKeyChapterRules 是自定义章节识别规则在 settings 表里的键。
//
// 用通用键值表而不是专表：规则是「一小段有序配置」，读写频率极低（用户手改几次），
// 为它单开一张表只会多一条迁移与一套 CRUD。
const SettingKeyChapterRules = "chapter_rules"

// ChapterRule 是一条自定义章节识别规则：名称用于前端展示，Pattern 是 Go 正则源串。
//
// 存源串而不是编译结果：settings 是持久化层，编译（以及编译失败的处理）属于
// 解析侧与校验侧，落库的东西必须能原样还原给用户编辑。
type ChapterRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// GetSetting 读取一个设置项；不存在返回 ok=false（不是错误）。
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, fmt.Errorf("%w: empty setting key", ErrInvalidArgument)
	}
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get setting %q: %w", key, err)
	}
	return value, true, nil
}

// PutSetting 写入（或覆盖）一个设置项，并刷新 updated_at。
func (s *Store) PutSetting(ctx context.Context, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("%w: empty setting key", ErrInvalidArgument)
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at",
		key, value, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: put setting %q: %w", key, err)
	}
	return nil
}

// ListChapterRules 读取自定义章节规则；未配置时返回空切片（不是 nil）。
//
// 返回 []ChapterRule{} 而不是 nil：api 层直接把它序列化成 {"rules":[]}，
// 让前端不必判 null。
func (s *Store) ListChapterRules(ctx context.Context) ([]ChapterRule, error) {
	raw, ok, err := s.GetSetting(ctx, SettingKeyChapterRules)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ChapterRule{}, nil
	}
	return decodeChapterRules(raw)
}

// SaveChapterRules 全量替换自定义章节规则（传空切片即清空）。
//
// 正则合法性与条数/长度上限由 api 层在校验边界上判（跨信任边界只判一次），
// 这里只负责序列化落库。
func (s *Store) SaveChapterRules(ctx context.Context, rules []ChapterRule) error {
	if rules == nil {
		rules = []ChapterRule{}
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("store: encode chapter rules: %w", err)
	}
	return s.PutSetting(ctx, SettingKeyChapterRules, string(raw))
}

// decodeChapterRules 解析规则 JSON。坏数据按「配置损坏」报错而不是静默当成空规则：
// 静默清空等于用户的规则被无声删掉，下次导入就会退回内置策略且毫无线索。
func decodeChapterRules(raw string) ([]ChapterRule, error) {
	if strings.TrimSpace(raw) == "" {
		return []ChapterRule{}, nil
	}
	var rules []ChapterRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("store: decode chapter rules: %w", err)
	}
	if rules == nil {
		return []ChapterRule{}, nil
	}
	return rules, nil
}
