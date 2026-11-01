package main

import (
	"context"
	"testing"

	"norvels/internal/config"
)

// 本文件锁死 P4-8 的 serve 配置装配：cmd 只把标量搬进 api.Config，
// 不依赖 internal/recommend（权重类型的叠加留在 internal/api）。

// TestBuildAPIConfigPassesWiring 三项开关与三项权重覆盖原样透传。
func TestBuildAPIConfigPassesWiring(t *testing.T) {
	w := 4.5
	cfg := config.Config{
		EnableCORS:          true,
		RequestID:           true,
		RecWeightInterestL1: &w,
	}
	apiCfg := buildAPIConfig(cfg, context.Background(), "", "/tmp/data/norvels.db")
	if !apiCfg.EnableCORS || !apiCfg.RequestID {
		t.Errorf("开发期开关没透传: CORS=%v RequestID=%v", apiCfg.EnableCORS, apiCfg.RequestID)
	}
	if apiCfg.RecWeightInterestL1 == nil || *apiCfg.RecWeightInterestL1 != 4.5 {
		t.Errorf("InterestL1 覆盖没透传: %v", apiCfg.RecWeightInterestL1)
	}
}

// TestBuildAPIConfigWiringDefaultsOff 不配置时全部为零值 / nil：默认行为不变。
func TestBuildAPIConfigWiringDefaultsOff(t *testing.T) {
	apiCfg := buildAPIConfig(config.Config{}, context.Background(), "", "/tmp/data/norvels.db")
	if apiCfg.EnableCORS || apiCfg.RequestID {
		t.Errorf("默认应全关，得到 CORS=%v RequestID=%v", apiCfg.EnableCORS, apiCfg.RequestID)
	}
	if apiCfg.RecWeightInterestL1 != nil || apiCfg.RecWeightHotReads != nil || apiCfg.RecWeightFreshness != nil {
		t.Errorf("默认权重覆盖应为 nil: %+v", apiCfg)
	}
}
