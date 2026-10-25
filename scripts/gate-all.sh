#!/usr/bin/env bash
# 质量门禁总入口：逐项跑完 11 项门禁、各打各的结果，最后汇总并如实返回退出码。
#
# 为什么不用 && 串起来：门禁未通过时仍要把**其余各项的数字**跑出来，
# 否则一次只能修一项。所以这里刻意「继续跑完」，但最终退出码 = 是否有任何一项失败，
# 不存在「失败也返回 0」的掩盖路径。
#
# 用法：
#   scripts/gate-all.sh            # 跑全部门禁
#   scripts/gate-all.sh --stop     # 遇失败立即停止（CI 里想快速失败时用）
#
# 退出码：0 = 9 项全部通过；1 = 至少一项未通过；2 = 环境问题无法完成。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

STOP_ON_FAIL=0
case "${1:-}" in
  --stop) STOP_ON_FAIL=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

# 结果累积：每行 "<门禁编号>|<名称>|<pass|fail|skip>"
RESULTS_FILE=$(mktemp -t norvels-gate-all)
trap 'rm -f "$RESULTS_FILE"' EXIT

record() { printf '%s|%s|%s\n' "$1" "$2" "$3" >> "$RESULTS_FILE"; }

# run_gate <编号> <名称> <命令...>
run_gate() {
  local num=$1 name=$2
  shift 2
  printf '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
  printf '▶ 门禁 %s：%s\n' "$num" "$name"
  printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
  if "$@"; then
    record "$num" "$name" pass
    return 0
  fi
  record "$num" "$name" fail
  if [ "$STOP_ON_FAIL" -eq 1 ]; then
    printf '\n[--stop] 门禁 %s 未通过，后续门禁未执行\n' "$num"
    summary
    exit 1
  fi
  return 1
}

summary() {
  printf '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
  printf '门禁汇总\n'
  printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
  local fails=0
  while IFS='|' read -r num name status; do
    case "$status" in
      pass) printf '  ✅  %-4s %s\n' "$num" "$name" ;;
      fail) printf '  ❌  %-4s %s\n' "$num" "$name"; fails=$((fails + 1)) ;;
      skip) printf '  ⏭️   %-4s %s\n' "$num" "$name" ;;
    esac
  done < "$RESULTS_FILE"
  printf '\n'
  if [ "$fails" -gt 0 ]; then
    printf '❌ 有 %d 项门禁未通过：门禁未全过 = 不得宣称交付完成、不得提交（见 docs/quality-gates.md）\n' "$fails"
    return 1
  fi
  printf '✅ 门禁全部通过\n'
  return 0
}

gofmt_check() {
  local out
  out=$(gofmt -l ./cmd ./internal ./tests 2>&1)
  if [ -n "$out" ]; then
    printf '未格式化的文件：\n%s\n' "$out"
    return 1
  fi
  printf 'gofmt：无需格式化的文件（./cmd ./internal ./tests 全部干净）\n'
  return 0
}

unit_tests() {
  go vet ./... || return 1
  go test ./... -count=1 || return 1
  return 0
}

gate_unit() {
  gofmt_check || return 1
  unit_tests || return 1
  return 0
}

gate_acceptance() {
  acceptance_tests
}

acceptance_tests() {
  local out rc
  out=$(go test ./tests/acceptance/ -count=1 -v 2>&1)
  rc=$?
  # godog 的统计行在 -v 输出里，剥掉 ANSI 颜色后展示。
  printf '%s\n' "$out" | sed 's/\x1b\[[0-9;]*m//g' | grep -E '^(--- )?(PASS|FAIL|ok )|scenarios|steps \(' | head -20
  return $rc
}

printf '质量门禁（quality-gates 11 项）—— 逐项如实测量，不调阈值、不 skip\n'
printf '基线 commit：%s\n' "$(git rev-parse --short HEAD 2>/dev/null || echo '（未初始化 git）')"

run_gate "1" "单元测试（gofmt + go vet + go test）" gate_unit
run_gate "2" "Gherkin 验收测试（godog）" gate_acceptance
run_gate "4" "圈复杂度（单函数 ≤ 10，豁免须登记）" scripts/gate-complexity.sh
run_gate "5" "模块大小（函数 ≤ 70 行、文件 ≤ 400 行，豁免须登记）" scripts/gate-size.sh
run_gate "6" "依赖结构（方向 + 循环依赖）" scripts/gate-deps.sh
run_gate "7" "覆盖率（不低于基线；85% 为渐进目标）" scripts/gate-cover.sh
run_gate "9" "隐私扫描（工作区全部文件）" scripts/privacy-scan.sh --all
run_gate "10" "构建一致性（容器工具版本 vs web/ 要求）" scripts/check-build-parity.sh
run_gate "11" "前端单元测试（vitest）" scripts/gate-frontend.sh

summary
exit $?