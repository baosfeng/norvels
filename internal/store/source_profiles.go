package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"norvels/internal/source"
)

// SettingKeySourceProfiles 是书源规则包在 settings 表里的键。
//
// 与 chapter_rules / tag_rules 同形：一个键一份 JSON 数组、全量替换、读多写极少，
// 不值得为它单开一张表（多一条迁移 + 一套 CRUD，收益为零）。
const SettingKeySourceProfiles = "source_profiles"

// ListSourceProfiles 读取书源规则包；未配置时返回空切片（不是 nil）。
//
// 返回 []source.Profile{} 而不是 nil：api 直接序列化成 {"profiles":[]}，
// 前端不必判 null。坏数据按「配置损坏」报错，不静默当成「没有规则包」——
// 静默清空等于用户的规则被无声删掉。
func (s *Store) ListSourceProfiles(ctx context.Context) ([]source.Profile, error) {
	raw, ok, err := s.GetSetting(ctx, SettingKeySourceProfiles)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []source.Profile{}, nil
	}
	return decodeSourceProfiles(raw)
}

// SaveSourceProfiles 全量替换书源规则包（传空切片即清空）。
//
// 校验（条数/重名/正则编译/字段上限）在 api 层一次做完，store 只负责序列化落库：
// 跨信任边界的校验只判一次，存储层不再重复一份可能漂移的口径。
func (s *Store) SaveSourceProfiles(ctx context.Context, profiles []source.Profile) error {
	if profiles == nil {
		profiles = []source.Profile{}
	}
	raw, err := json.Marshal(profiles)
	if err != nil {
		return fmt.Errorf("store: encode source profiles: %w", err)
	}
	return s.PutSetting(ctx, SettingKeySourceProfiles, string(raw))
}

// decodeSourceProfiles 解析规则包 JSON；空白值视为未配置。
func decodeSourceProfiles(raw string) ([]source.Profile, error) {
	if strings.TrimSpace(raw) == "" {
		return []source.Profile{}, nil
	}
	var profiles []source.Profile
	if err := json.Unmarshal([]byte(raw), &profiles); err != nil {
		return nil, fmt.Errorf("store: decode source profiles: %w", err)
	}
	if profiles == nil {
		return []source.Profile{}, nil
	}
	return profiles, nil
}
