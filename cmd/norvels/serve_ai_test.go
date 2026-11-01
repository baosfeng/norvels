package main

import (
	"context"
	"log"
	"strings"
	"testing"

	"norvels/internal/config"
)

// 本文件锁死 serve 启动期的 AI 配置装配：日志文案（key 有没有 / 开关开没开）、
// 模型名兜底，以及 API 配置的字段映射（key 只进内存，不进任何落库结构）。

// TestLogAIState 三种启动状态各打一行，且都不打印 key 本身。
func TestLogAIState(t *testing.T) {
	var buf strings.Builder
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	logAIState(config.Config{AIAPIKey: "sk-secret", AIModel: "m1"}, true)
	logAIState(config.Config{AIAPIKey: "sk-secret"}, false)
	logAIState(config.Config{}, false)

	out := buf.String()
	if !strings.Contains(out, "AI 审核已启用") || !strings.Contains(out, "m1") {
		t.Errorf("缺「已启用」日志：%s", out)
	}
	if !strings.Contains(out, "开关未打开") {
		t.Errorf("缺「开关未打开」日志：%s", out)
	}
	if !strings.Contains(out, "未配置 key") {
		t.Errorf("缺「未配置 key」日志：%s", out)
	}
	if strings.Contains(out, "sk-secret") {
		t.Errorf("日志里出现了 key：%s", out)
	}
}

// TestAIModelLabel 未配置模型名时说明用默认值。
func TestAIModelLabel(t *testing.T) {
	if got := aiModelLabel(""); got != "内置默认" {
		t.Errorf("aiModelLabel(\"\") = %q，期望「内置默认」", got)
	}
	if got := aiModelLabel("deepseek-chat"); got != "deepseek-chat" {
		t.Errorf("aiModelLabel = %q", got)
	}
}

// TestBuildAPIConfig AI 参数原样透传给 api.Config，上传目录落在库同目录。
func TestBuildAPIConfig(t *testing.T) {
	cfg := config.Config{AIAPIKey: "sk-x", AIBaseURL: "http://127.0.0.1:1/v1", AIModel: "m2"}
	apiCfg := buildAPIConfig(cfg, context.Background(), "token", "/tmp/data/norvels.db")
	if apiCfg.AIAPIKey != "sk-x" || apiCfg.AIBaseURL != "http://127.0.0.1:1/v1" || apiCfg.AIModel != "m2" {
		t.Errorf("AI 参数没透传：%+v", apiCfg)
	}
	if apiCfg.UploadDir != "/tmp/data/uploads" {
		t.Errorf("UploadDir = %q，期望 /tmp/data/uploads", apiCfg.UploadDir)
	}
	if apiCfg.AuthToken != "token" || apiCfg.Context == nil {
		t.Errorf("鉴权 / ctx 没透传：%+v", apiCfg)
	}
}
