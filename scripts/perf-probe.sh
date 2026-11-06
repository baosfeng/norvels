#!/usr/bin/env bash
# 接口耗时探针：对**同一个部署**把关键 GET 接口各打 N 次，输出状态码与耗时分布（min/avg/p95/max）。
#
# 为什么要有它：/api/health/report 在生产要 20 秒级，但此前没有任何可复现的量化手段——
# 只能靠肉眼觉得"慢"、靠翻容器 stdout 日志猜。这个脚本给"改前 → 改后"一份同一口径的数字表，
# 不依赖登录态、不依赖外部压测工具（只用 curl）。
#
# 用法：
#   scripts/perf-probe.sh                          # 打本机 :8080，每接口 5 次
#   scripts/perf-probe.sh --base http://nas:8080   # 指定部署
#   scripts/perf-probe.sh --repeat 20 --json out.json
#   scripts/perf-probe.sh --only health            # 只跑名字含该子串的项
#
#   --base URL      部署根地址（默认 http://127.0.0.1:8080）
#   --repeat N      每接口重复次数（默认 5；首列"首次"单独给出，便于看冷缓存）
#   --timeout SEC   单请求超时（默认 60）
#   --json FILE     同时写出机器可读结果（做前后对比用）
#   --only STR      只跑名字含该子串的条目
#
# 退出码：0 = 没有 5xx/超时；1 = 出现 5xx/超时/连接失败；2 = 用法或环境问题。
# 兼容 macOS 自带 bash 3.2（不用关联数组 / mapfile / gawk-asort）。
set -uo pipefail

BASE=http://127.0.0.1:8080
REPEAT=5
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
    -h|--help) sed -n '2,26p' "$0"; exit 0 ;;
    *) echo "未知参数：$1（--help 看用法）" >&2; exit 2 ;;
  esac
done

command -v curl >/dev/null || { echo "需要 curl" >&2; exit 2; }
BASE=$(printf '%s' "$BASE" | sed 's:/*$::')

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# 自动发现一个真实 book id：列表接口第一本书，供详情/章节接口使用。
BOOK_ID=$(curl -fsS --max-time "$TIMEOUT" "$BASE/api/books?limit=1" 2>/dev/null \
  | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
[ -n "$BOOK_ID" ] || BOOK_ID=1
CHAPTER_ID=$(curl -fsS --max-time "$TIMEOUT" "$BASE/api/books/$BOOK_ID/chapters" 2>/dev/null \
  | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
[ -n "$CHAPTER_ID" ] || CHAPTER_ID=1

ENTRIES="stats	/api/stats
books	/api/books?limit=50
books-sort-title	/api/books?limit=20&sort=title
search	/api/search?q=%E4%BF%AE%E4%BB%99
health-report	/api/health/report
health-issues	/api/health/issues?kind=huge_chapter&size=20
import-trend	/api/stats/import-trend
reading-stats	/api/stats/reading
categories	/api/categories
authors	/api/authors
series	/api/series
recommend	/api/recommend?tab=for_you
random	/api/books/random
book-detail	/api/books/$BOOK_ID
book-chapters	/api/books/$BOOK_ID/chapters
chapter-content	/api/books/$BOOK_ID/chapters/$CHAPTER_ID"

FAILED=0
JSON_ITEMS=
printf '%-16s %6s %8s %8s %8s %8s %8s\n' 接口 次数 首次ms 平均 最快 最慢 p95
printf '%s\n' "---------------------------------------------------------------------------"

while IFS='	' read -r name path; do
  [ -n "$name" ] || continue
  if [ -n "$ONLY" ]; then
    case "$name" in *"$ONLY"*) ;; *) continue ;; esac
  fi
  f="$TMP/$name.txt"
  : > "$f"
  codes=
  i=1
  while [ "$i" -le "$REPEAT" ]; do
    out=$(curl -s -o /dev/null --max-time "$TIMEOUT" -w '%{http_code} %{time_total}' --connect-timeout 5 "$BASE$path" 2>/dev/null) \
      || out="000 $TIMEOUT"
    code=$(printf '%s' "$out" | awk '{print $1}')
    secs=$(printf '%s' "$out" | awk '{print $2}')
    awk -v s="$secs" 'BEGIN{printf "%.1f\n", s*1000}' >> "$f"
    codes="$codes $code"
    case "$code" in 5*|000) FAILED=1 ;; esac
    i=$((i + 1))
  done
  first=$(head -1 "$f")
  avg=$(awk '{s+=$1} END{printf "%.1f", s/NR}' "$f")
  min=$(sort -n "$f" | head -1)
  max=$(sort -n "$f" | tail -1)
  idx=$(awk -v n="$REPEAT" 'BEGIN{i=int(n*0.95+0.999); if(i<1)i=1; if(i>n)i=n; print i}')
  p95=$(sort -n "$f" | sed -n "$idx"p)
  printf '%-16s %6s %8s %8s %8s %8s %8s\n' "$name" "$REPEAT" "$first" "$avg" "$min" "$max" "$p95"
  JSON_ITEMS="$JSON_ITEMS{\"name\":\"$name\",\"path\":\"$path\",\"firstMs\":$first,\"avgMs\":$avg,\"maxMs\":$max,\"p95Ms\":$p95,\"codes\":\"$(printf '%s' "$codes" | sed 's/^ //')\"},"
done <<EOF
$ENTRIES
EOF

if [ -n "$JSON_OUT" ]; then
  printf '{"base":"%s","repeat":%d,"items":[%s]}\n' "$BASE" "$REPEAT" "$(printf '%s' "$JSON_ITEMS" | sed 's/,$//')" > "$JSON_OUT"
  echo "JSON 已写：$JSON_OUT"
fi

exit "$FAILED"
