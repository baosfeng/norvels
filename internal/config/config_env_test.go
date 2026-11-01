package config

import "testing"

// 本文件锁死 P4-8 新增的三个接入点：两个开发期开关 + 三项推荐权重覆盖。
// 共同约束：**默认值与接入前完全一致**（CORS 关、RequestID 关、权重走 DefaultWeights）。

// clearWiringEnv 清掉本文件涉及的变量，避免本机环境影响断言。
func clearWiringEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvEnableCORS, EnvRequestID,
		EnvRecWeightInterestL1, EnvRecWeightHotReads, EnvRecWeightFreshness,
	} {
		t.Setenv(k, "")
	}
}

// TestWiringDefaultsKeepOldBehavior 默认全关 / 全 nil：不带这些变量时行为与接入前一致。
func TestWiringDefaultsKeepOldBehavior(t *testing.T) {
	clearWiringEnv(t)
	c, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if c.EnableCORS || c.RequestID {
		t.Errorf("两个开发期开关默认必须关闭，得到 CORS=%v RequestID=%v", c.EnableCORS, c.RequestID)
	}
	if c.RecWeightInterestL1 != nil || c.RecWeightHotReads != nil || c.RecWeightFreshness != nil {
		t.Errorf("权重覆盖默认必须是 nil（叠加默认权重），得到 %+v", c)
	}
}

// TestWiringReadsBoolEnvs 两个开关都能被打开。
func TestWiringReadsBoolEnvs(t *testing.T) {
	clearWiringEnv(t)
	t.Setenv(EnvEnableCORS, "true")
	t.Setenv(EnvRequestID, "1")
	c, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !c.EnableCORS || !c.RequestID {
		t.Fatalf("开关未打开: CORS=%v RequestID=%v", c.EnableCORS, c.RequestID)
	}
}

// TestWiringRejectsBadBool 非法布尔值直接报错，不静默降级成 false。
func TestWiringRejectsBadBool(t *testing.T) {
	for _, key := range []string{EnvEnableCORS, EnvRequestID} {
		t.Run(key, func(t *testing.T) {
			clearWiringEnv(t)
			t.Setenv(key, "yes")
			if _, err := FromEnv(); err == nil {
				t.Fatalf("%s=yes 应报错（静默当 false 会让开关看起来坏了）", key)
			}
		})
	}
}

// TestWiringReadsWeights 三项权重覆盖都读得到，且 0 是合法值（显式关掉某一路信号）。
func TestWiringReadsWeights(t *testing.T) {
	clearWiringEnv(t)
	t.Setenv(EnvRecWeightInterestL1, "4.5")
	t.Setenv(EnvRecWeightHotReads, "2")
	t.Setenv(EnvRecWeightFreshness, "0")
	c, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if c.RecWeightInterestL1 == nil || *c.RecWeightInterestL1 != 4.5 {
		t.Errorf("InterestL1 覆盖 = %v", c.RecWeightInterestL1)
	}
	if c.RecWeightHotReads == nil || *c.RecWeightHotReads != 2 {
		t.Errorf("HotReads 覆盖 = %v", c.RecWeightHotReads)
	}
	if c.RecWeightFreshness == nil || *c.RecWeightFreshness != 0 {
		t.Errorf("Freshness 覆盖应为显式 0 而不是 nil: %v", c.RecWeightFreshness)
	}
}

// TestWiringRejectsBadWeights 非法权重（非数字 / NaN / Inf / 负数）一律报错。
func TestWiringRejectsBadWeights(t *testing.T) {
	for _, raw := range []string{"abc", "NaN", "Inf", "-1", "1e999"} {
		t.Run(raw, func(t *testing.T) {
			clearWiringEnv(t)
			t.Setenv(EnvRecWeightInterestL1, raw)
			if _, err := FromEnv(); err == nil {
				t.Fatalf("%s=%s 应报错", EnvRecWeightInterestL1, raw)
			}
		})
	}
}
