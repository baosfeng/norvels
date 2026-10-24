# norvels 构建入口
SHELL := /bin/bash
VERSION ?= 0.1.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILTAT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.builtAt=$(BUILTAT)
BIN := bin/norvels

.PHONY: all help web build build-server build-web run import test lint fmt tidy clean docker privacy verify

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-14s %s\n", $$1, $$2}'

all: build ## 默认：构建前端 + 后端

web: ## 构建前端并同步到 internal/webui/dist
	cd web && (command -v pnpm >/dev/null && pnpm install --prefer-offline && pnpm build || npm install && npm run build)
	rm -rf internal/webui/dist && cp -R web/dist internal/webui/dist

build: build-web build-server ## 完整构建

build-web: ## 构建前端（产物落到 web/dist）
	@test -f web/package.json || { echo "web/ 前端尚未落地（React 实现阶段进行中）；现在可用 'make build-server' + 'make run' 看到占位页" >&2; exit 1; }
	cd web && (command -v pnpm >/dev/null && pnpm build || npm run build)

build-server: ## 编译服务端二进制（内嵌 internal/webui/dist）
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/norvels

run: ## 本地启动
	go run ./cmd/norvels serve --addr :8080 --db ./data/norvels.db

import: ## 导入示例（SOURCE=路径）
	go run ./cmd/norvels import --dir "$(SOURCE)" --db ./data/norvels.db

test: ## 运行全部 Go 测试
	go test ./... -count=1

fmt: ## 格式化
	gofmt -w ./cmd ./internal

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

verify: fmt ## 提交前门禁：格式 + vet + 测试 + 隐私扫描
	gofmt -l ./cmd ./internal | tee /tmp/norvels-gofmt.txt | (! grep .) || (echo "gofmt 未通过，见上" && exit 1)
	go vet ./...
	go test ./... -count=1
	scripts/privacy-scan.sh --all
