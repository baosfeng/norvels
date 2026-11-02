#!/usr/bin/env bash
# 门禁 11：前端单元测试 + 类型检查。
#
# 为什么单独成一项：在此之前 CI 与 make gate 只跑前端的 build（tsc + vite build），
# 从不跑前端测试。后果是——104 个前端测试全红，CI 依然全绿。实测踩过一次：
# 一次提交带上了未改完的断言，CI 通过但 main 上的 vitest 是失败的。
#
# build 只能证明「能编译」，证明不了「行为正确」，两者不可互相替代。
#
# 为什么还要在这里跑类型检查：vitest 走 esbuild 转译，不做类型检查，类型错误在测试里不可见。
# 实测踩过一次：相对导入路径写错 + 测试里用了不存在的 ApiError kind，vitest 1385 条全绿、
# make gate 九项全绿，但 make build（tsc -b && vite build）直接失败——门禁放行了「本地绿但构建挂」。
#
# 必须带 -p tsconfig.app.json：根 tsconfig.json 是 "files": [] + project references 的空壳，
# 裸跑 tsc --noEmit 一个文件都不检查并返回 0（这正是那次漏检的直接原因）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR/web" || { echo "[错误] 缺少 web/ 目录" >&2; exit 2; }

if [ ! -f package.json ]; then
  echo "[跳过] web/ 前端尚未落地"
  exit 0
fi

if [ ! -d node_modules ]; then
  echo "==> 安装前端依赖"
  if command -v pnpm >/dev/null 2>&1; then
    pnpm install --prefer-offline || exit 1
  else
    npm install || exit 1
  fi
fi


echo "==> 前端类型检查（tsc --noEmit -p tsconfig.app.json）"
if command -v pnpm >/dev/null 2>&1; then
  typeout=$(pnpm run typecheck 2>&1)
  typerc=$?
else
  typeout=$(npm run typecheck 2>&1)
  typerc=$?
fi
printf "%s\n" "$typeout" | grep -E "error TS" | head -20
if [ $typerc -ne 0 ]; then
  echo "❌ 前端类型检查未通过（vitest 不查类型，这类错它一定会漏）"
  echo "   本地复现：cd web && npm run typecheck"
  exit 1
fi
echo "✅ 前端类型检查通过"

echo "==> 前端单元测试（vitest）"
if command -v pnpm >/dev/null 2>&1; then
  out=$(pnpm test 2>&1)
  rc=$?
else
  out=$(npm test 2>&1)
  rc=$?
fi

printf "%s\n" "$out" | grep -E "Test Files|Tests +[0-9]+ (passed|failed)|FAIL |AssertionError" | head -20
if [ $rc -ne 0 ]; then
  echo "❌ 前端单元测试未通过"
  echo "   本地复现：cd web && pnpm test"
  exit 1
fi
echo "✅ 前端单元测试全部通过"
exit 0
