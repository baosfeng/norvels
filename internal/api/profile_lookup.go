package api

import (
	"context"
	"fmt"
	"net/http"

	"norvels/internal/source"
)

// loadSourceProfiles 读取来源规则包；读失败只记日志、本次导入退回内置策略
// （与 loadChapterRules 的降级语义一致：一条坏配置不该让整批导入失败）。
func (s *Server) loadSourceProfiles(ctx context.Context) []source.Profile {
	profiles, err := s.st.ListSourceProfiles(ctx)
	if err != nil {
		s.logf("api: 读取来源规则包失败，本次导入只用内置策略: %v", err)
		return nil
	}
	return profiles
}

// checkProfileID 校验显式指定的规则包 id 是否存在；不存在时写 400 并返回 false。
//
// 显式指定一份不存在的规则包时**不退回 match 也不退回内置**：用户以为在用这份
// 规则、实际用了另一份，比直接报错更难排查。
func checkProfileID(w http.ResponseWriter, id string, profiles []source.Profile) bool {
	if id == "" {
		return true
	}
	for _, p := range profiles {
		if p.ID == id {
			return true
		}
	}
	writeError(w, http.StatusBadRequest, codeInvalidParam,
		fmt.Sprintf("来源规则包 id %q 未配置（GET /api/import/source-profiles 查看已配置的 id）", id))
	return false
}
