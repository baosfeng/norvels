package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SettingKeyReplaceRules 是全局净化规则（Legado replaceRule）在 settings 表里的键。
//
// 与 chapter_rules / tag_rules / source_profiles 同形：JSON 数组、全量替换。
const SettingKeyReplaceRules = "replace_rules"

// CleanRule 是一条净化规则（对应 Legado 的 replaceRule）。
//
// 存「用户可编辑的源数据」而不是编译结果：IsRegex=false 时 Pattern 是字面量，
// 转义与编译属于执行侧（api 编译成 parser.Filter 时做）。
type CleanRule struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Group        string `json:"group,omitempty"`
	Order        int    `json:"order"`
	Enabled      bool   `json:"enabled"`
	IsRegex      bool   `json:"isRegex"`
	Pattern      string `json:"pattern"`
	Replacement  string `json:"replacement"`
	ScopeTitle   bool   `json:"scopeTitle"`
	ScopeContent bool   `json:"scopeContent"`
}

// ListCleanRules 读取全局净化规则；未配置时返回空切片（不是 nil）。
func (s *Store) ListCleanRules(ctx context.Context) ([]CleanRule, error) {
	raw, ok, err := s.GetSetting(ctx, SettingKeyReplaceRules)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []CleanRule{}, nil
	}
	return decodeCleanRules(raw)
}

// SaveCleanRules 全量替换全局净化规则（传空切片即清空）。
//
// 条数/长度/正则合法性由 api 层在校验边界上判，这里只负责序列化落库。
func (s *Store) SaveCleanRules(ctx context.Context, rules []CleanRule) error {
	if rules == nil {
		rules = []CleanRule{}
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("store: encode clean rules: %w", err)
	}
	return s.PutSetting(ctx, SettingKeyReplaceRules, string(raw))
}

// decodeCleanRules 解析净化规则 JSON；坏数据按「配置损坏」报错而不是静默清空。
func decodeCleanRules(raw string) ([]CleanRule, error) {
	if strings.TrimSpace(raw) == "" {
		return []CleanRule{}, nil
	}
	var rules []CleanRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("store: decode clean rules: %w", err)
	}
	if rules == nil {
		return []CleanRule{}, nil
	}
	return rules, nil
}
