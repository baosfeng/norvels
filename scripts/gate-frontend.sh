#!/usr/bin/env bash
# 门禁 11：前端类型检查 + 单元测试 + eslint。
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
#
# 执行编排：tsc 与 eslint 并行跑完，vitest 再独占跑。
#   - tsc 走独立 tsbuildinfo：4.7s → 1~2s；eslint 走 --cache：8.3s → 0.5~1.5s。
#   - 这两项都不吃满 CPU，并行实测 1.6s（串行 6s+）。
#   - vitest 必须独占：与这两项同跑时实测出现过 `Unhandled Errors: window is not defined`
#     假红（用例计数全绿、退出码 1），正是 docs/quality-gates.md「门禁假红的识别」第 2 条。
#     假红的代价是回头怀疑修复本身，比省下的十几秒贵得多。
#   于是总耗时 = 约 2s + vitest（独占，本机 21~38s，随负载浮动）。
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

WORK=$(mktemp -d -t norvels-gate-frontend)
trap 'rm -rf "$WORK"' EXIT

# tsbuildinfo 与 eslint 缓存都放 node_modules/.cache（已 gitignore）：不进仓库、不进 CI 产物。
# tsbuildinfo 与快环用不同的文件——两边同时跑会互相作废，各自留一份。
CACHE_DIR="$ROOT_DIR/web/node_modules/.cache"
TSC_INFO="$CACHE_DIR/norvels-tsc/gate.tsbuildinfo"
mkdir -p "$(dirname -- "$TSC_INFO")" "$CACHE_DIR/eslint"

check_typecheck() {
  ./node_modules/.bin/tsc --noEmit -p tsconfig.app.json \
    --incremental --tsBuildInfoFile "$TSC_INFO"
}

# eslint 的 json 输出直接解析更准：grep 数 "severity":2 会被别的字段误伤。
# 既有告警不阻断：仓库当前有 5 条历史 error，按基线比较，只有新增才判红。
# 所以这里不看退出码（有历史 error 时恒为 1），只看解析出的 error 条数。
check_lint() {
  ./node_modules/.bin/eslint . -f json \
    --cache --cache-location "$CACHE_DIR/eslint/" > "$WORK/lint.json" 2>"$WORK/lint.err"
}

fails=0
START=$(date +%s)

# 一、静态检查并行（互不依赖、都不吃满 CPU）
( check_typecheck > "$WORK/tsc.log" 2>&1; printf '%s\n' $? > "$WORK/tsc.rc" ) &
( check_lint; printf '%s\n' $? > "$WORK/lint.rc" ) &
wait
STATIC_SECS=$(( $(date +%s) - START ))

rc_of() { [ -f "$WORK/$1.rc" ] && cat "$WORK/$1.rc" || echo 1; }

echo "==> 前端类型检查（tsc --noEmit -p tsconfig.app.json）"
grep -E "error TS" "$WORK/tsc.log" 2>/dev/null | head -20
if [ "$(rc_of tsc)" -ne 0 ]; then
  echo "❌ 前端类型检查未通过（vitest 不查类型，这类错它一定会漏）"
  sed 's/^/   /' "$WORK/tsc.log" 2>/dev/null | head -20
  echo "   本地复现：cd web && npm run typecheck"
  fails=$((fails + 1))
else
  echo "✅ 前端类型检查通过"
fi

echo "==> 前端 eslint（只判新增）"
LINT_BASELINE=5
lint_errors=$(node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{try{const r=JSON.parse(s);let n=0;for(const f of r)for(const m of f.messages)if(m.severity===2)n++;console.log(n)}catch(e){console.log("")}})' < "$WORK/lint.json")
if [ -z "$lint_errors" ]; then
  echo "⚠️  eslint 未产出结果，跳过（不阻断）"
  head -5 "$WORK/lint.err" 2>/dev/null | sed 's/^/   /'
elif [ "$lint_errors" -gt "$LINT_BASELINE" ]; then
  echo "❌ eslint 新增错误：${lint_errors} 条（基线 ${LINT_BASELINE}）"
  echo "   本地复现：cd web && npm run lint"
  fails=$((fails + 1))
else
  echo "✅ eslint 无新增错误（${lint_errors} 条 / 基线 ${LINT_BASELINE}）"
fi

# 二、vitest 独占跑（理由见文件头：与上面两项同跑会出假红）
#
# --allowOnly=false：把漏在代码里的 `.only` 当成红灯。vitest 默认只在 CI 环境变量非空时才禁，
# 本机跑全量时 .only 会把**其余用例静默跳过**而退出码仍是 0——实测一个 .only 让 6 个用例变成
# "6 skipped"、整个套件照样绿（门禁假绿，比红更糟）。没有 .only 时该参数完全无害（实测 exit 0）。
# 覆盖率与用例一起跑（--coverage 只多约 6s：21s -> 28s），用「基线只升不降」防退化。
# 为什么必须有：Go 侧删掉一个测试文件会被门禁 7 的覆盖率抓住，前端此前完全没有防线——
# 实测删掉 pageGeometry.test.ts，vitest 照常报 166/1521 全绿、退出码 0，门禁 11 全绿。
# 覆盖率把这种「静默少测」变成红灯。数值实测三次完全一致（92.11 / 89.47 / 78.99），
# 所以基线可以钉到一位小数，不会像 go 侧 importer 那样抖动。
VITEST_START=$(date +%s)
echo "==> 前端单元测试 + 覆盖率（vitest，独占）"
./node_modules/.bin/vitest run --allowOnly=false --coverage > "$WORK/vitest.log" 2>&1
vitest_rc=$?
VITEST_SECS=$(( $(date +%s) - VITEST_START ))

grep -aE "Test Files|Tests +[0-9]+ (passed|failed)|FAIL |Unhandled Errors|Unexpected .only|^All files" "$WORK/vitest.log" | head -20
if [ "$vitest_rc" -ne 0 ]; then
  echo "❌ 前端单元测试未通过（退出码 ${vitest_rc}）"
  # 用例计数全绿而退出码非 0 = Unhandled Errors，必须改代码，不是重跑能消的。
  sed 's/^/   /' "$WORK/vitest.log" | grep -aE "FAIL|AssertionError|Unhandled|Error:|threshold" | head -20
  echo "   本地复现：cd web && pnpm test  （要看退出码：echo \$?）"
  fails=$((fails + 1))
else
  echo "✅ 前端单元测试全部通过"
fi

printf '（静态检查 %ss 并行 + vitest 独占 %ss，合计 %ss）\n' "$STATIC_SECS" "$VITEST_SECS" "$(( $(date +%s) - START ))"
[ "$fails" -eq 0 ] || exit 1
exit 0
