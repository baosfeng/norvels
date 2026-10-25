# norvels 构建入口
SHELL := /bin/bash
VERSION ?= 0.1.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILTAT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.builtAt=$(BUILTAT)
BIN := bin/norvels

.PHONY: all help web build build-server build-web run import test lint fmt tidy clean docker privacy gallery \
        bench-capacity verify gate gate-complexity gate-size gate-deps gate-cover gate-acceptance

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

all: build ## 默认：构建前端 + 后端

web: ## 构建前端并同步到 internal/webui/dist（go:embed 的输入）
	cd web && (command -v pnpm >/dev/null && (pnpm install --prefer-offline && pnpm build) || (npm install && npm run build))
	rsync -a --delete web/dist/ internal/webui/dist/
	@touch internal/webui/dist/.gitkeep

build: web build-server ## 完整构建（前端产物先同步进 embed 目录，二进制里才是真实页面）

build-web: ## 构建前端（产物落到 web/dist）
	@test -f web/package.json || { echo "web/ 前端尚未落地（React 实现阶段进行中）；现在可用 'make build-server' + 'make run' 看到占位页" >&2; exit 1; }
	cd web && (command -v pnpm >/dev/null && (pnpm install --prefer-offline && pnpm build) || (npm install && npm run build))

build-server: ## 编译服务端二进制（内嵌 internal/webui/dist）
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/norvels

run: ## 本地启动
	go run ./cmd/norvels serve --addr :8080 --db ./data/norvels.db

import: ## 导入示例（SOURCE=路径）
	go run ./cmd/norvels import --dir "$(SOURCE)" --db ./data/norvels.db

test: ## 运行全部 Go 测试（含 Gherkin 验收测试）
	go test ./... -count=1

fmt: ## 格式化
	gofmt -w ./cmd ./internal ./tests

tidy: ## 整理依赖
	go mod tidy

clean: ## 清理构建产物
	rm -rf bin web/dist web/node_modules

docker: ## 构建镜像（需要本机有 docker）
	docker build -f deploy/Dockerfile -t norvels:$(VERSION) .

privacy: ## 隐私扫描（工作区全部文件，提交前跑一次）
	scripts/privacy-scan.sh --all

gallery: ## 重新生成三版原型对比页 + 截图（自动相对化路径）
	scripts/gallery.sh

# ---------- 容量压测 ----------
# 默认小规模（500 本 x 500 章，约 1 GB 临时库，几秒跑完），产物是可直接贴进
# docs/data-profile.md 的 markdown 表格。大库压测请显式传参：
#   python3 scripts/bench-capacity.py --books 5000 --chapters 5000
# 脚本会先查磁盘余量，不够直接拒绝并打印需要多少 GB；跑完自动清临时库（--keep 保留）。
# 不碰 data/norvels.db，也不需要先编译 Go 二进制。
bench-capacity: ## 容量压测（500 本 x 500 章合成库，输出 markdown 表格，不碰生产库）
	@python3 scripts/bench-capacity.py

# ---------- 质量门禁（quality-gates 9 项，逐项可单独跑）----------
# 每个 gate-* 都如实报告：不达标即 exit 1，不调阈值、不 skip。详见 docs/quality-gates.md。

gate-complexity: ## 门禁 4：圈复杂度（单函数 ≤ 10，豁免须在 docs 登记）
	@scripts/gate-complexity.sh

gate-size: ## 门禁 5：模块大小（函数 ≤ 70 行、文件 ≤ 400 行，豁免须登记）
	@scripts/gate-size.sh

gate-deps: ## 门禁 6：依赖结构（方向 + 循环依赖）
	@scripts/gate-deps.sh

gate-cover: ## 门禁 7：覆盖率（不低于基线，防退化；85% 为渐进目标）
	@scripts/gate-cover.sh

gate-acceptance: ## 门禁 2：Gherkin 验收测试（godog）
	@go test ./tests/acceptance/ -count=1

gate: ## 全部质量门禁（跑完逐项打印结果，任一未通过则 exit 1）
	@scripts/gate-all.sh

verify: gate ## 提交前门禁：等同 make gate（全部 9 项）
