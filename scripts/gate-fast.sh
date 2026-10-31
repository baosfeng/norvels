#!/usr/bin/env bash
# 快环：本地随改随跑（目标 < 10s），只做「格式 + 编译级」的即时反馈。
#
# 为什么要有它：make gate 是提交前的完整口径（并行后仍是分钟级），改一行就跑它是浪费；
# 但「改完完全不看」会把失败推到提交前才发现。快环只跑缓存友好的三步：
#   gofmt 检查 → go vet → 前端 tsc --noEmit
# 快环通过 ≠ 门禁通过：它不跑测试、不算覆盖率、不查复杂度。提交前仍必须 make gate。
#
# 用法：
#   scripts/gate-fast.sh            # 三步并行，每项一行结果
#   scripts/gate-fast.sh --tests    # 额外跑「改动包」的单测（git diff 取包名，耗时随包数增长）
#
# 退出码：0 = 全过；1 = 任一项失败；2 = 环境问题（如 web/node_modules 未安装）。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

WITH_TESTS=0
case "${1:-}" in
  "") ;;
  --tests) WITH_TESTS=1 ;;
  -h|--help) sed -n '2,17p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

WORK=$(mktemp -d -t norvels-fast)
trap 'rm -rf "$WORK"' EXIT

# 环境问题单独用退出码 2 表达，避免和「代码不过」混为一谈。
env_problem() { printf '[环境] %s\n' "$1" >&2; exit 2; }

command -v go >/dev/null 2>&1 || env_problem "找不到 go（需要 Go 1.26）"
[ -f web/package.json ] || env_problem "缺少 web/package.json"
if [ ! -x web/node_modules/.bin/tsc ]; then
  env_problem "缺少 web/node_modules（先 cd web && pnpm install）"
fi

check_gofmt() {
  local out
  out=$(gofmt -l ./cmd ./internal ./tests 2>&1)
  if [ -n "$out" ]; then
    printf '未格式化的文件（make fmt 可修）：\n%s\n' "$out"
    return 1
  fi
  return 0
}

check_vet() { go vet ./... ; }

# 走独立 tsbuildinfo（放 node_modules/.cache，已 gitignore）：第二次起 ~1s。
# 不能复用 web/tsconfig.app.tsbuildinfo——那是 tsc -b 构建用的，两套选项混用会互相作废。
check_tsc() {
  local info="$ROOT_DIR/web/node_modules/.cache/norvels-tsc/app.tsbuildinfo"
  mkdir -p "$(dirname -- "$info")"
  ( cd web && ./node_modules/.bin/tsc --noEmit -p tsconfig.app.json \
      --incremental --tsBuildInfoFile "$info" )
}

# 改动包：只取工作区里被改的 .go 文件所属目录；验收测试留给 make gate-acceptance。
changed_pkgs() {
  { git diff --name-only HEAD 2>/dev/null || git diff --name-only 2>/dev/null
    git ls-files --others --exclude-standard 2>/dev/null
  } | awk -F/ 'NF > 1 && $NF ~ /\.go$/ { sub(/\/[^\/]*$/, "", $0); print "./" $0 }' \
    | grep -v '^\./tests/acceptance$' | sort -u | tr '\n' ' '
}

check_tests() {
  local pkgs
  pkgs=$(changed_pkgs)
  if [ -z "$pkgs" ]; then
    printf '没有改动的 Go 包，跳过单测\n'
    return 0
  fi
  printf '改动包：%s\n' "$pkgs"
  # shellcheck disable=SC2086
  go test $pkgs -count=1
}

# 并行跑三项：每项写一个日志 + 一个退出码文件，回放时按固定顺序打印。
launch() {
  local key=$1 label=$2
  shift 2
  (
    "$@" > "$WORK/$key.log" 2>&1
    printf '%s\n' $? > "$WORK/$key.rc"
    # 未产生 .rc 说明子 shell 被信号打断，按失败处理。
  ) &
}

START=$(date +%s)
launch gofmt 'gofmt' check_gofmt
launch vet 'go vet ./...' check_vet
launch tsc 'tsc --noEmit' check_tsc
[ "$WITH_TESTS" -eq 1 ] && launch tests '改动包单测' check_tests
wait
ELAPSED=$(( $(date +%s) - START ))

printf '快环（本地随改随跑）—— 通过 ≠ 门禁通过，提交前仍跑 make gate\n'
printf '基线 commit：%s\n\n' "$(git rev-parse --short HEAD 2>/dev/null || echo '（未初始化 git）')"

fails=0
report() {
  local key=$1 label=$2 rc
  if [ -f "$WORK/$key.rc" ]; then rc=$(cat "$WORK/$key.rc"); else rc=1; fi
  if [ "$rc" -eq 0 ]; then
    printf '  ✅  %s\n' "$label"
    return 0
  fi
  fails=$((fails + 1))
  printf '  ❌  %s（退出码 %s）\n' "$label" "$rc"
  sed 's/^/      /' "$WORK/$key.log" | head -30
  return 1
}

report gofmt 'gofmt（./cmd ./internal ./tests）'
report vet   'go vet ./...'
report tsc   '前端 tsc --noEmit（web/tsconfig.app.json）'
[ "$WITH_TESTS" -eq 1 ] && report tests '改动包单测'

printf '\n'
if [ "$fails" -gt 0 ]; then
  printf '❌ 快环 %d 项未过（用时 %ss）\n' "$fails" "$ELAPSED"
  exit 1
fi
printf '✅ 快环通过（用时 %ss）\n' "$ELAPSED"
exit 0
