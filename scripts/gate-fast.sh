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
#   scripts/gate-fast.sh --tests    # 额外跑「改动文件在模块图上相关」的单测
#   scripts/gate-fast.sh --impl     # 实现期模式：在上面基础上检查「有没有提前写测试」
#
# --impl（实现期模式）判的是开发顺序，不是代码质量：功能没定稿前应当零新测试
#   （见 leader-ops「开发顺序」/ quality-gates）。它检查**工作区相对 HEAD 的改动**里
#   是否新增了测试文件或给测试文件加了断言——有就红，提示「先交付、收齐意见再写测试」。
#   豁免：修 bug（提交信息 fix/bug）必须补复现测试，故该模式只拦 feat 类改动；
#   确实要写测试时用 NORVELS_ALLOW_TESTS=1 显式放行，并说明理由。
#
# 退出码：0 = 全过；1 = 任一项失败；2 = 环境问题（如 web/node_modules 未安装）。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

WITH_TESTS=0
IMPL_MODE=0
case "${1:-}" in
  "") ;;
  --tests) WITH_TESTS=1 ;;
  --impl)  IMPL_MODE=1; WITH_TESTS=1 ;;
  -h|--help) sed -n '2,21p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
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

# 改动的前端文件 → 该跑哪些测试。
#
# 两种改法都要有反馈，所以走 vitest 的模块图而不是文件名匹配：
#   改了测试文件 → 至少跑它自己（vitest related 也会把它带上）；
#   改了源码文件 → 跑所有 import 到它的测试（实测 turnGesture.ts → 15 个文件 130 用例，5.8s）。
# 早期只按 `*.test.ts` 挑，改源码时一个测试都不跑、静默报「跳过」——那等于快环不覆盖前端行为。
# 跑整套留给门禁 11；这里只跑模块图上真正相关的那一批。
check_web_tests() {
  local files
  files=$({ git diff --name-only HEAD 2>/dev/null || git diff --name-only 2>/dev/null
    git ls-files --others --exclude-standard 2>/dev/null
  } | grep -E '^web/src/.*\.(ts|tsx)$' | sort -u | sed 's|^web/||' | tr '\n' ' ')
  if [ -z "$files" ]; then
    printf '没有改动的前端文件，跳过\n'
    return 0
  fi
  printf '相关测试（按模块图）：%s\n' "$files"
  # shellcheck disable=SC2086
  ( cd web && ./node_modules/.bin/vitest related $files --run )
}
# 实现期检查：功能没定稿前不该新增测试。
#
# 判的是**开发顺序**，不是代码质量。判据刻意窄——只看「未跟踪的新测试文件」：
# 宽判据会误伤且没有证据支撑（实测历史 feat 提交 94% 同时带测试，那是定稿后的正常提交；
# 而「往已有测试文件里加断言」无法与「修断言」区分，不判）。
# 修 bug 例外：fix 类必须补复现测试，故本检查只对功能开发**进行中**有意义，
# 由调用方自觉用 --impl，不塞进门禁造成硬阻断。
impl_mode_check() {
  local changed nt
  changed=$({ git diff --name-only HEAD 2>/dev/null || git diff --name-only 2>/dev/null
    git ls-files --others --exclude-standard 2>/dev/null
  } | sort -u)
  [ -n "$changed" ] || { printf '工作区无改动，跳过\n'; return 0; }

  # 新测试文件 = 未跟踪 + 路径像测试。已跟踪测试文件的修改不判（可能是修断言）。
  nt=$(git ls-files --others --exclude-standard 2>/dev/null \
    | grep -E '(^|/)(.*\.(test|spec)\.(ts|tsx)|.*_test\.go)$')
  if [ -n "$nt" ] && [ "${NORVELS_ALLOW_TESTS:-0}" != "1" ]; then
    printf '实现期新增了测试文件（功能定稿前应当零新测试）：\n'
    printf '%s\n' "$nt" | sed 's/^/   /' | head -10
    printf '先交付、收齐意见再写测试。确需现在写：NORVELS_ALLOW_TESTS=1 make gate-fast-impl\n'
    return 1
  fi
  printf '未新增测试文件（改动 %s 个文件）\n' "$(printf '%s\n' "$changed" | grep -c .)"
  return 0
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
[ "$WITH_TESTS" -eq 1 ] && launch webtests '改动前端测试' check_web_tests
[ "$IMPL_MODE" -eq 1 ] && launch impl '实现期（未新增测试）' impl_mode_check
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
[ "$WITH_TESTS" -eq 1 ] && report webtests '改动前端测试（模块图上相关的文件）'
[ "$IMPL_MODE" -eq 1 ] && report impl '实现期（未新增测试文件）'

printf '\n'
if [ "$fails" -gt 0 ]; then
  printf '❌ 快环 %d 项未过（用时 %ss）\n' "$fails" "$ELAPSED"
  exit 1
fi
printf '✅ 快环通过（用时 %ss）\n' "$ELAPSED"
exit 0
