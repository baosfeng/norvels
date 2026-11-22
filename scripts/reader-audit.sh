#!/usr/bin/env bash
# 阅读器体验审计：真浏览器（390×844）+ 一组「读者能感知」的不变量。
#
# 为什么不进 make gate：它要 Chrome（agent-browser）+ 本机书库 + 真实布局，
# CI 与 jsdom 都给不了。改动阅读器时手动跑，或用 make audit-reader。
#
# 前置：scripts/preview.sh --detach（后端 :8080 + 前端 :5173）
# 退出码：0 全部通过；1 有失败；2 环境/用法问题。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

command -v node >/dev/null 2>&1 || { printf "[环境] 找不到 node\n" >&2; exit 2; }
command -v agent-browser >/dev/null 2>&1 || { printf "[环境] 找不到 agent-browser\n" >&2; exit 2; }
if ! curl -fsS -o /dev/null --max-time 2 "http://127.0.0.1:5173/" 2>/dev/null; then
  printf "[环境] 前端未起：先跑 scripts/preview.sh --detach\n" >&2
  exit 2
fi

exec node scripts/reader-audit.mjs "$@"
