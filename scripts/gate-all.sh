#!/usr/bin/env bash
# 质量门禁总入口：九项依次执行，逐项打印结果，最后汇总并如实返回退出码。
#
# 为什么默认串行：九项虽然互不依赖，但都是 CPU 密集的（go test 与 vitest 各占满几个核）。
# 实测并行只快约 12s（46.6s → 34.6s），却在低核机器上偶发互扰——vitest 单例假红，串行复跑即绿。
# 门禁报红的代价是「怀疑修复本身」再查一遍，比省下的十几秒贵；--parallel 留给热缓存本地跑。
#
# 为什么不像 && 那样串起来：门禁未通过时仍要把**其余各项的数字**跑出来，
# 否则一次只能修一项。所以刻意「跑完全部」，但最终退出码 = 是否有任何一项失败，
# 不存在「失败也返回 0」的掩盖路径。
#
# 用法：
#   scripts/gate-all.sh            # 九项串行（默认，CI 与本机同口径）
#   scripts/gate-all.sh --parallel # 九项并行（本地热缓存下约快 12s，低核机器可能互扰）
#   scripts/gate-all.sh --stop     # 遇失败立即停（只对串行有意义：并行收不回已启动的项）
#
# 退出码：0 = 9 项全部通过；1 = 至少一项未通过；2 = 环境问题无法完成。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

JOBS=1
STOP_ON_FAIL=0
case "${1:-}" in
  ""|--serial) JOBS=1 ;;
  --parallel)  JOBS=9 ;;
  --stop)      JOBS=1; STOP_ON_FAIL=1 ;;
  -h|--help) sed -n '2,17p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

WORK=$(mktemp -d -t norvels-gate-all)
trap 'rm -rf "$WORK"' EXIT
ORDER_FILE="$WORK/order"
RESULTS_FILE="$WORK/results"
: > "$ORDER_FILE"
: > "$RESULTS_FILE"
TOTAL_GATES=0

print_header() {
  printf '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
  printf '▶ 门禁 %s：%s\n' "$1" "$2"
  printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
}

# record <编号> <名称> <pass|fail> <秒数>
record() { printf '%s|%s|%s|%s\n' "$1" "$2" "$3" "${4:-0}" >> "$RESULTS_FILE"; }

summary() {
  printf '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
  printf '门禁汇总\n'
  printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
  local fails=0 num name status secs
  while IFS='|' read -r num name status secs; do
    case "$status" in
      pass) printf '  ✅  %-4s %-46s %ss\n' "$num" "$name" "$secs" ;;
      fail) printf '  ❌  %-4s %-46s %ss\n' "$num" "$name" "$secs"; fails=$((fails + 1)) ;;
    esac
  done < "$RESULTS_FILE"
  printf '\n总耗时 %ss（%s）\n' "$TOTAL_SECS" "$([ "$JOBS" -eq 1 ] && echo 串行 || echo 并行)"
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

# 一次 go test ./... 供三项门禁共用：单元测试（1）、覆盖率（7）、Gherkin 验收（2）。
# 三者本来都是「跑一遍全部 Go 测试」，各跑一次等于同一套测试跑三遍。
# 实测：各 16s → 合并后 1 与 7 省 16s；门禁 2 单独跑 acceptance 要 7.5s，而 tests/acceptance
# 本来就是 go test ./... 里的一个包，只需 -v 多花 0.8s 就能拿到它的 godog 统计行。
# 产物：$WORK/go-cover.out（覆盖率 profile）、$WORK/go-test.log（-v 完整输出）。
# 并行模式下 2 与 7 会在门禁 1 产出前起跑，此时它们各自退回自己跑（行为不变）。
unit_tests() {
  go vet ./... || return 1
  # -v 输出有 1.2MB，不往终端刷：落盘，只在失败时挑出未通过的用例回显。
  if ! go test ./... -count=1 -v -covermode=atomic -coverprofile="$WORK/go-cover.out" \
      > "$WORK/go-test.log" 2>&1; then
    # 只回显失败用例本身：`--- FAIL` 行 + 紧随其后的细节。
    # 不能按 `_test.go:<行号>:` 挑——那只匹配到 t.Log 的普通日志（-v 下满屏都是），
    # 真正的失败细节反而被淹没（实测一次失败刷出 40 行无关日志）。
    printf '未通过的用例：\n'
    # 一个失败用例块的边界：以 `--- FAIL` 起，到下一个 `=== RUN` / 包级 FAIL 摘要为止。
    awk '/^--- FAIL/{show=1} /^(=== RUN|FAIL[[:space:]]|ok[[:space:]])/{if(show && !/^--- FAIL/) show=0} show' \
      "$WORK/go-test.log" | grep -avE '^[[:space:]]*--- PASS' | head -30
    printf '（完整输出：%s）\n' "$WORK/go-test.log"
    return 1
  fi
  # 包级摘要：ok / no test files 各一行，去掉 tab 与耗时列的对齐。
  grep -aE '^(ok|\?) ' "$WORK/go-test.log" | sed 's/\t/ /g' | head -40
  return 0
}

gate_unit() {
  gofmt_check || return 1
  unit_tests || return 1
  return 0
}

# 门禁 2：复用门禁 1 那次 go test 的 acceptance 输出，不重跑。
# 拿不到（并行模式 / 门禁 1 在编译阶段就挂了）时自己跑一遍——宁可慢，不可漏。
gate_acceptance() {
  local log="$WORK/go-test.log"
  if [ -s "$log" ] && grep -aq 'norvels/tests/acceptance' "$log"; then
    # godog 的统计行混在 1.2MB 的 -v 输出里：只挑验收包自己的结果行与 scenarios/steps 统计，
    # 别把其它包的 PASS 行刷出来。
    sed 's/\x1b\[[0-9;]*m//g' "$log" \
      | grep -aE '^(--- FAIL: TestFeatures|FAIL[[:space:]]+norvels/tests/acceptance|ok[[:space:]]+norvels/tests/acceptance)|^[0-9]+ (scenarios|steps) \(|^[[:space:]]*[0-9]+ (scenarios|steps) \(' \
      | head -20
    # 验收包自己失败时，go test 的失败行必须让门禁 2 红（不能只看「有没有统计行」）。
    # 注意 go test 的失败摘要是 FAILD<TAB>包名<TAB>耗时，分隔符是制表符，不能用空格匹配。
    if grep -aE '^FAIL[[:space:]]+norvels/tests/acceptance' "$log" >/dev/null; then
      printf '❌ 验收测试未通过（见门禁 1 的输出）\n'
      return 1
    fi
    printf '（复用门禁 1 的同一次 go test，未重跑）\n'
    return 0
  fi
  local out rc
  out=$(go test ./tests/acceptance/ -count=1 -v 2>&1)
  rc=$?
  # godog 的统计行在 -v 输出里，剥掉 ANSI 颜色后展示。
  printf '%s\n' "$out" | sed 's/\x1b\[[0-9;]*m//g' | grep -E '^(--- )?(PASS|FAIL|ok )|scenarios|steps \(' | head -20
  return $rc
}

# run_one <编号> <名称> <命令...>：子 shell 里跑，日志与退出码各自落盘（并行时不会串台）
run_one() {
  local num=$1 name=$2 start end rc
  shift 2
  start=$(date +%s)
  "$@" > "$WORK/$num.log" 2>&1
  rc=$?
  end=$(date +%s)
  printf '%s\n' "$rc" > "$WORK/$num.rc"
  printf '%s\n' "$((end - start))" > "$WORK/$num.secs"
}

# submit <编号> <名称> <命令...>：串行则实时流式跑，并行则后台启动并立即返回
submit() {
  local num=$1 name=$2 start end rc
  shift 2
  printf '%s|%s\n' "$num" "$name" >> "$ORDER_FILE"
  TOTAL_GATES=$((TOTAL_GATES + 1))
  if [ "$JOBS" -eq 1 ]; then
    print_header "$num" "$name"
    start=$(date +%s)
    "$@"
    rc=$?
    end=$(date +%s)
    if [ "$rc" -eq 0 ]; then
      record "$num" "$name" pass "$((end - start))"
      return 0
    fi
    record "$num" "$name" fail "$((end - start))"
    if [ "$STOP_ON_FAIL" -eq 1 ]; then
      printf '\n[--stop] 门禁 %s 未通过，后续门禁未执行\n' "$num"
      TOTAL_SECS=$(( $(date +%s) - RUN_START ))
      summary
      exit 1
    fi
    return 1
  fi
  ( run_one "$num" "$name" "$@" ) &
  return 0
}

# 并行等待：每 2s 报一次进度；上限 30 分钟，超时按未产出处理（不会静默挂死）。
wait_parallel() {
  local done_count=-1 i=0
  while [ "$i" -lt 900 ]; do
    done_count=$(ls "$WORK"/*.rc 2>/dev/null | wc -l | tr -d ' ')
    [ "$done_count" -lt "$TOTAL_GATES" ] || break
    if [ $((i % 1)) -eq 0 ] && [ "$done_count" -ne "${LAST_DONE:-0}" ]; then
      printf '⏳ 并行执行中：已完成 %s/%s 项\n' "$done_count" "$TOTAL_GATES"
      LAST_DONE=$done_count
    fi
    sleep 2
    i=$((i + 1))
  done
  wait
  printf '⏳ 并行执行完毕（%s/%s 项有结果）\n' "$done_count" "$TOTAL_GATES"
}

# 并行回放：按启动顺序逐项打印，单项输出完整，互不穿插
replay() {
  local num name rc secs
  while IFS='|' read -r num name; do
    print_header "$num" "$name"
    [ -f "$WORK/$num.log" ] && cat "$WORK/$num.log"
    if [ -f "$WORK/$num.rc" ]; then rc=$(cat "$WORK/$num.rc"); else rc=1; fi
    secs=0
    [ -f "$WORK/$num.secs" ] && secs=$(cat "$WORK/$num.secs")
    if [ "$rc" -eq 0 ]; then
      record "$num" "$name" pass "$secs"
    else
      record "$num" "$name" fail "$secs"
    fi
  done < "$ORDER_FILE"
}

RUN_START=$(date +%s)
printf '质量门禁（quality-gates 9 项）—— %s执行，逐项如实测量，不调阈值、不 skip\n' \
  "$([ "$JOBS" -eq 1 ] && echo 串行 || echo 并行)"
printf '基线 commit：%s\n' "$(git rev-parse --short HEAD 2>/dev/null || echo '（未初始化 git）')"

submit "1" "单元测试（gofmt + go vet + go test）" gate_unit
submit "2" "Gherkin 验收测试（godog）" gate_acceptance
submit "4" "圈复杂度（单函数 ≤ 10，豁免须登记）" scripts/gate-complexity.sh
submit "5" "模块大小（函数 ≤ 70 行、文件 ≤ 400 行，豁免须登记）" scripts/gate-size.sh
submit "6" "依赖结构（方向 + 循环依赖）" scripts/gate-deps.sh
if [ "$JOBS" -eq 1 ] && [ -s "$WORK/go-cover.out" ]; then
  submit "7" "覆盖率（不低于基线；85% 为渐进目标）" scripts/gate-cover.sh --profile="$WORK/go-cover.out"
else
  submit "7" "覆盖率（不低于基线；85% 为渐进目标）" scripts/gate-cover.sh
fi
submit "9" "隐私扫描（工作区全部文件）" scripts/privacy-scan.sh --all
submit "10" "构建一致性（容器工具版本 vs web/ 要求）" scripts/check-build-parity.sh
submit "11" "前端单元测试（vitest）" scripts/gate-frontend.sh

if [ "$JOBS" -eq 1 ]; then
  TOTAL_SECS=$(( $(date +%s) - RUN_START ))
else
  wait_parallel
  replay
  TOTAL_SECS=$(( $(date +%s) - RUN_START ))
fi

summary
exit $?
