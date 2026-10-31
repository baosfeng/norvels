#!/usr/bin/env bash
# 一条命令起本地预览：后端 :8080 + vite HMR :5173。端口已在跑就复用，绝不 kill 别人的进程。
#
# 为什么要有它：改完想「看一眼」时，后端、前端、代理三件事每次都要现搭；
# 而 web/vite.config.ts 早就配好 HMR 与 /api → 127.0.0.1:8080 代理。
# 本脚本只做「缺什么起什么」，并把可点的 URL 打出来。
#
# 用法：
#   scripts/preview.sh            # 前台：缺什么起什么，Ctrl-C 只收掉**本脚本起的**进程
#   scripts/preview.sh --detach   # 后台：打印 URL 后返回，PID 记在 .preview/pids
#   scripts/preview.sh --stop     # 停掉 --detach / 前台残留的本脚本进程（复用的不动）
#   端口可用 API_PORT / WEB_PORT 覆盖（默认 8080 / 5173），临时验证不抢别人的端口。
#
# 退出码：0 = 预览可用；1 = 启动失败（日志在 .preview/）；2 = 用法错误。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

# 端口可用 API_PORT / WEB_PORT 覆盖（默认 8080 / 5173）；换端口时 vite 的 /api 代理跟着走，
# 临时验证不必去抢别人已经在用的 8080/5173。
API_PORT=${API_PORT:-8080}
WEB_PORT=${WEB_PORT:-5173}
STATE_DIR="$ROOT_DIR/.preview"
PID_FILE="$STATE_DIR/pids"
MODE=foreground

case "${1:-}" in
  "") ;;
  --detach) MODE=detach ;;
  --stop)   MODE=stop ;;
  -h|--help) sed -n '2,16p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

port_listening() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }
http_ok() { curl -fsS -o /dev/null --max-time 2 "$1" >/dev/null 2>&1; }

# wait_http <url> <秒数>：轮询到可访问或超时
wait_http() {
  local url=$1 limit=$2 i=0
  while [ "$i" -lt "$limit" ]; do
    http_ok "$url" && return 0
    sleep 0.25
    i=$((i + 1))
  done
  return 1
}

listener_pid() { lsof -nP -iTCP:"$1" -sTCP:LISTEN -t 2>/dev/null | head -1; }

stop_own() {
  [ -f "$PID_FILE" ] || { printf '没有本脚本启动的进程（%s 不存在）\n' "$PID_FILE"; return 0; }
  local key pid cmd
  while IFS='=' read -r key pid; do
    [ -n "${pid:-}" ] || continue
    kill -0 "$pid" 2>/dev/null || continue
    cmd=$(ps -o command= -p "$pid" 2>/dev/null)
    case "$cmd" in
      *norvels*serve*|*vite*)
        kill -TERM "$pid" 2>/dev/null && printf '已停止 %-4s pid %s\n' "$key" "$pid"
        ;;
      *)
        printf '[跳过] pid %s 已被其他进程复用（%s），不动它\n' "$pid" "$cmd"
        ;;
    esac
  done < "$PID_FILE"
  rm -f "$PID_FILE"
  return 0
}

if [ "$MODE" = "stop" ]; then
  stop_own
  exit 0
fi

mkdir -p "$STATE_DIR"
OWN_PIDS=""
API_OWNED=0
WEB_OWNED=0

cleanup() {
  local pid
  for pid in $OWN_PIDS; do
    kill -TERM "$pid" 2>/dev/null
  done
  [ -n "$OWN_PIDS" ] && printf '\n已收掉本脚本起的进程（复用来的不动）\n'
  return 0
}

# ---------- 后端 :8080 ----------
if port_listening "$API_PORT"; then
  if http_ok "http://127.0.0.1:$API_PORT/api/health"; then
    printf '==> 复用已在运行的后端 :%s（pid %s，不由本脚本管理）\n' "$API_PORT" "$(listener_pid "$API_PORT")"
  else
    printf '==> 复用已在运行的后端 :%s（pid %s，健康检查未通过——可能正在启动）\n' \
      "$API_PORT" "$(listener_pid "$API_PORT")"
  fi
else
  if [ ! -x ./bin/norvels ]; then
    printf '==> 未找到 bin/norvels，先编译后端（make build-server）\n'
    make build-server || { printf '[错误] 编译失败\n' >&2; exit 1; }
  fi
  printf '==> 启动后端 ./bin/norvels serve --addr :%s（日志 .preview/api.log）\n' "$API_PORT"
  ./bin/norvels serve --addr ":$API_PORT" --db ./data/norvels.db > "$STATE_DIR/api.log" 2>&1 &
  OWN_PIDS="$OWN_PIDS $!"
  API_OWNED=1
  if ! wait_http "http://127.0.0.1:$API_PORT/api/health" 120; then
    printf '[错误] 后端 30s 内未就绪，日志末尾：\n' >&2
    tail -15 "$STATE_DIR/api.log" >&2
    cleanup
    exit 1
  fi
fi

# ---------- 前端 :5173 ----------
if port_listening "$WEB_PORT"; then
  printf '==> 复用已在运行的前端 :%s（pid %s，HMR 生效，不由本脚本管理）\n' \
    "$WEB_PORT" "$(listener_pid "$WEB_PORT")"
else
  if [ ! -x web/node_modules/.bin/vite ]; then
    printf '==> 安装前端依赖（仅首次，cd web && pnpm install）\n'
    ( cd web && command -v pnpm >/dev/null && pnpm install --prefer-offline || npm install ) \
      || { printf '[错误] 前端依赖安装失败\n' >&2; cleanup; exit 1; }
  fi
  printf '==> 启动前端 vite dev :%s（日志 .preview/web.log）\n' "$WEB_PORT"
  ( cd web && NORVELS_API="http://127.0.0.1:$API_PORT" \
      exec ./node_modules/.bin/vite --port "$WEB_PORT" --host 127.0.0.1 ) > "$STATE_DIR/web.log" 2>&1 &
  OWN_PIDS="$OWN_PIDS $!"
  WEB_OWNED=1
  if ! wait_http "http://127.0.0.1:$WEB_PORT/" 160; then
    printf '[错误] 前端 40s 内未就绪，日志末尾：\n' >&2
    tail -15 "$STATE_DIR/web.log" >&2
    cleanup
    exit 1
  fi
fi

printf '\n✅ 预览就绪\n'
printf '   前端（HMR）：http://127.0.0.1:%s\n' "$WEB_PORT"
printf '   后端 / 接口：http://127.0.0.1:%s/api/health\n' "$API_PORT"
printf '   /api 由 vite 代理到后端，改前端代码即时热更；改后端需重启后端\n'

if [ "$MODE" = "detach" ]; then
  : > "$PID_FILE"
  [ "$API_OWNED" -eq 1 ] && printf 'api=%s\n' "$(echo "$OWN_PIDS" | awk '{print $1}')" >> "$PID_FILE"
  [ "$WEB_OWNED" -eq 1 ] && printf 'web=%s\n' "$(echo "$OWN_PIDS" | awk '{print $NF}')" >> "$PID_FILE"
  printf '\n后台模式：PID 记在 %s；停止用 scripts/preview.sh --stop\n' "$PID_FILE"
  exit 0
fi

if [ -z "$OWN_PIDS" ]; then
  printf '\n两个端口都已在运行（全部复用，不由本脚本管理），直接访问上面的 URL 即可\n'
  exit 0
fi

trap 'cleanup; exit 0' INT TERM
printf '\n前台运行中，Ctrl-C 收掉本脚本起的进程…\n'
wait
cleanup
exit 0
