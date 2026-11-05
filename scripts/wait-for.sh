#!/usr/bin/env bash
# 等一个条件成立：在被等的机器上循环判定，只在成立或超时时返回一次。
#
# 为什么要有它：历史会话里「等长任务」是最贵的一块——实测 709 次 sleep 合计 18.2h、
# wait_agent 空等 18.0h、job_output 空等 7.4h，三项共 43.6h，占 agent 干活时长（120h）的 36%。
# 它们的共同形态是 `sleep 240; ssh nas '查一次进度'`：agent 自己睡、自己醒、再发一次往返，
# 每个 sleep 都是一次完整模型往返，而等待本身不产生任何信息。
# 本脚本把这套循环压进**一次调用**：条件在远端机器上轮询，agent 只等一个结果。
#
# 与 bash 工具超时的配合：调用方的超时到了会把命令转成后台 job（不是杀掉），
# 所以即使等待比预算长也不会丢结果——给 `--timeout` 一个比预期完成时间略长的值即可。
#
# 用法：
#   scripts/wait-for.sh --until 'test -f /tmp/done'                      # 本地文件
#   scripts/wait-for.sh --host nas --until 'pgrep -f run_import.py' --gone --timeout 3600
#   scripts/wait-for.sh --until 'curl -fsS localhost:8080/healthz' --interval 2
#   scripts/wait-for.sh --host nas --until 'test -f /vol1/x/done' --timeout 1800 --quiet
#
#   --host NAME     在远端等（走 ~/.ssh/config 的 ControlMaster 复用；不加则在本地等）
#   --until CMD     判定命令；退出码 0 = 条件成立（必需）
#   --gone          反转判定：命令**不再成功**时算成立（等进程/端口消失用）
#   --timeout SEC   最长等待秒数（默认 600）
#   --interval SEC  轮询间隔秒数（默认 5）
#   --quiet         只在最后打印一行结果
#
# 退出码：0 = 条件已成立；1 = 超时未成立；2 = 用法/环境问题。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

HOST=""
UNTIL=""
GONE=0
TIMEOUT=600
INTERVAL=5
QUIET=0

while [ $# -gt 0 ]; do
  case "$1" in
    --host) HOST="${2:-}"; shift 2 ;;
    --until) UNTIL="${2:-}"; shift 2 ;;
    --gone) GONE=1; shift ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 ;;
    --interval) INTERVAL="${2:-}"; shift 2 ;;
    --quiet) QUIET=1; shift ;;
    -h|--help) sed -n '2,29p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[ -n "$UNTIL" ] || { printf '[错误] 缺少 --until（判定命令）\n' >&2; exit 2; }
case "$TIMEOUT$INTERVAL" in
  ''|*[!0-9]*) printf '[错误] --timeout / --interval 需要正整数\n' >&2; exit 2 ;;
esac

# 远端循环：把整个「反复判定」放进一次 ssh 调用，省掉每次判定的握手与往返。
# 远端可能没有本地这个仓库，所以只用 POSIX sh 能保证的东西。
remote_loop() {
  local cond=$1 gone=$2 timeout=$3 interval=$4
  if [ "$gone" -eq 1 ]; then
    ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" \
      "i=0; while [ \$i -lt $timeout ]; do if $cond; then sleep $interval; else echo GONE_AFTER=\$i; exit 0; fi; i=\$((i + $interval)); sleep $interval; done; echo STILL_RUNNING; exit 1"
  else
    ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" \
      "i=0; while [ \$i -lt $timeout ]; do if $cond; then echo READY_AFTER=\$i; exit 0; fi; i=\$((i + $interval)); sleep $interval; done; echo TIMEOUT; exit 1"
  fi
}

local_loop() {
  local cond=$1 gone=$2 timeout=$3 interval=$4 i=0
  # 用 sh -c 展开：条件里常有引号与管道，直接 $cond 会被当前 shell 的引用规则吃掉。
  while [ "$i" -lt "$timeout" ]; do
    if [ "$gone" -eq 1 ]; then
      if sh -c "$cond" >/dev/null 2>&1; then sleep "$interval"; else printf 'GONE_AFTER=%s\n' "$i"; return 0; fi
    else
      if sh -c "$cond" >/dev/null 2>&1; then printf 'READY_AFTER=%s\n' "$i"; return 0; fi
    fi
    i=$((i + interval))
    sleep "$interval"
  done
  [ "$gone" -eq 1 ] && { printf 'STILL_RUNNING\n'; return 1; }
  printf 'TIMEOUT\n'
  return 1
}

START=$(date +%s)
if [ -n "$HOST" ]; then
  out=$(remote_loop "$UNTIL" "$GONE" "$TIMEOUT" "$INTERVAL" 2>&1)
else
  out=$(local_loop "$UNTIL" "$GONE" "$TIMEOUT" "$INTERVAL" 2>&1)
fi
rc=$?
ELAPSED=$(( $(date +%s) - START ))

if [ "$rc" -eq 0 ]; then
  if [ "$QUIET" -eq 0 ]; then
    printf '✅ 条件成立（%s%s，用时 %ss）：%s\n' \
      "$([ -n "${HOST}" ] && echo "远端 ${HOST}，" || echo '')" "$out" "$ELAPSED" "$UNTIL"
  fi
  exit 0
fi
printf '⏱ 等待超时（%ss，超时上限 %ss）：%s\n' "$ELAPSED" "$TIMEOUT" "$UNTIL" >&2
printf '   再等：加大 --timeout 重跑；先看进度：把这次的判定命令互换着跑一遍\n' >&2
exit 1
