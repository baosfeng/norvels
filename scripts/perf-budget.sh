#!/usr/bin/env bash
# 驾驶舱性能预算：对**同一个部署**把关键 GET 各打 N 次，逐项比对 p95 上限；超预算即 exit 1。
#
# 为什么要有它：性能改造的证据链必须是「同一口径的数字 + 一条能重跑的命令」。
# /api/health/report 曾经 p95 5500 ms 却没人知道它退化了——没有预算就没有回归哨兵。
# 本脚本给每个接口一条上限（毫秒），跑一次就知道「这次改动之后驾驶舱还快不快」。
#
# 用法：
#   scripts/perf-budget.sh                                  # 打本机 :8080，每接口 10 次
#   scripts/perf-budget.sh --base http://nas:8080           # 对生产同口径复测
#   scripts/perf-budget.sh --repeat 20 --json out.json      # 采样更多 + 机器可读结果
#   scripts/perf-budget.sh --only health                    # 只跑名字含该子串的项
#
#   --base URL      部署根地址（默认 http://127.0.0.1:8080）
#   --repeat N      每接口采样次数（默认 10；p95 按 sorted[N*0.95] 取）
#   --timeout SEC   单请求超时（默认 60）
#   --json FILE     同时写出机器可读结果（name/path/budgetMs/p95Ms/ok）
#   --only STR      只跑名字含该子串的条目
#
# 退出码：0 = 全部在预算内；1 = 有接口超预算或出现 5xx/超时；2 = 用法或环境问题。
# 兼容 macOS 自带 bash 3.2（不用关联数组 / mapfile / gawk-asort）。
set -uo pipefail

BASE=http://127.0.0.1:8080
REPEAT=10
TIMEOUT=60
JSON_OUT=
ONLY=

while [ $# -gt 0 ]; do
  case "$1" in
    --base) BASE=$2; shift 2 ;;
    --repeat) REPEAT=$2; shift 2 ;;
    --timeout) TIMEOUT=$2; shift 2 ;;
    --json) JSON_OUT=$2; shift 2 ;;
    --only) ONLY=$2; shift 2 ;;
    -h|--help) sed -n '2,25p' "$0"; exit 0 ;;
    *) echo "未知参数：$1（--help 看用法）" >&2; exit 2 ;;
  esac
done

command -v curl >/dev/null || { echo "需要 curl" >&2; exit 2; }
case "$REPEAT" in ''|*[!0-9]*) echo "--repeat 必须是正整数" >&2; exit 2 ;; esac
[ "$REPEAT" -ge 1 ] || { echo "--repeat 必须是正整数" >&2; exit 2; }
BASE=$(printf '%s' "$BASE" | sed 's:/*$::')

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# 预算表：名字<TAB>路径<TAB>预算毫秒。
#
# 前三条是驾驶舱的**验收线**（生产 18.2 万本 / 5457.9 万章实测）：
#   health-report  5500 ms → 300 ms（quality 覆盖率改 0026 预计算）
#   stats           261 ms →  60 ms（books 全表扫改 0026 预计算）
#   dashboard         新端点 → 350 ms（7 个块并发 + 5 秒 TTL 缓存）
# 后五条是**回归护栏**：预算取「当前生产 p95 × 2」量级，用来抓「别处改慢了」，
# 不是本轮的优化目标（authors / categories / reading-progress 另有任务）。
ENTRIES="health-report	/api/health/report	300
stats	/api/stats	60
dashboard	/api/dashboard	350
import-trend	/api/stats/import-trend	200
reading-stats	/api/stats/reading	300
authors	/api/authors?sort=words&size=10	500
categories	/api/categories	500
reading-progress	/api/reading/progress	300"

FAILED=0
JSON_ITEMS=
printf '%-16s %8s %8s %10s %6s\n' 接口 预算ms p95ms 余量% 结论
printf '%s\n' '-------------------------------------------------------------'

while IFS='	' read -r name path budget; do
  [ -n "$name" ] || continue
  if [ -n "$ONLY" ]; then
    case "$name" in *"$ONLY"*) ;; *) continue ;; esac
  fi
  f="$TMP/$name.txt"
  : > "$f"
  codes=
  i=1
  while [ "$i" -le "$REPEAT" ]; do
    out=$(curl -s -o /dev/null --max-time "$TIMEOUT" -w '%{http_code} %{time_total}' \
      --connect-timeout 5 "$BASE$path" 2>/dev/null) || out="000 $TIMEOUT"
    code=$(printf '%s' "$out" | awk '{print $1}')
    secs=$(printf '%s' "$out" | awk '{print $2}')
    awk -v s="$secs" 'BEGIN{printf "%.1f\n", s*1000}' >> "$f"
    codes="$codes $code"
    case "$code" in 5*|000) FAILED=1 ;; esac
    i=$((i + 1))
  done
  idx=$(awk -v n="$REPEAT" 'BEGIN{i=int(n*0.95+0.999); if(i<1)i=1; if(i>n)i=n; print i}')
  p95=$(sort -n "$f" | sed -n "$idx"p)
  ok=$(awk -v p="$p95" -v b="$budget" 'BEGIN{print (p <= b) ? "ok" : "over"}')
  margin=$(awk -v p="$p95" -v b="$budget" 'BEGIN{printf "%.0f", (b-p)*100/b}')
  verdict=通过
  if [ "$ok" != "ok" ]; then
    verdict=超预算
    FAILED=1
  fi
  printf '%-16s %8s %8s %9s%% %6s\n' "$name" "$budget" "$p95" "$margin" "$verdict"
  JSON_ITEMS="$JSON_ITEMS{\"name\":\"$name\",\"path\":\"$path\",\"budgetMs\":$budget,\"p95Ms\":$p95,\"ok\":\"$ok\"},"
done <<EOF
$ENTRIES
EOF

if [ -n "$JSON_OUT" ]; then
  printf '{"base":"%s","repeat":%d,"items":[%s]}\n' "$BASE" "$REPEAT" \
    "$(printf '%s' "$JSON_ITEMS" | sed 's/,$//')" > "$JSON_OUT"
  echo "JSON 已写：$JSON_OUT"
fi

if [ "$FAILED" -ne 0 ]; then
  echo "有接口超预算或请求失败（退出码 1）。复现明细：scripts/perf-probe.sh --base $BASE" >&2
  exit 1
fi
echo "全部在预算内。"
exit 0
