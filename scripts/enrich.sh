#!/usr/bin/env bash
# norvels 元数据补全：按在线书源补作者/简介/分类，并把超长章节切分。
#
# 本地：  make build-server && scripts/enrich.sh --plan
# Docker：scripts/enrich.sh --mode docker --apply --limit 100
#
# 设计要点：
#   - **默认 dry-run**：不加 --apply 只打印「会改什么」，绝不写库；
#   - 每次 --apply 生成一个 batch id，回滚用 --rollback <batchID> --apply；
#   - 并发与限流在二进制里（--workers / --qps），脚本只负责把参数透传；
#   - SQLite 是单写者：--apply 期间请停 serve，或至少不要同时跑导入。
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd -- "$SCRIPT_DIR/.." && pwd)

MODE=${NORVELS_ENRICH_MODE:-local}
DB=${NORVELS_DB:-"$ROOT_DIR/data/norvels.db"}
BIN=${NORVELS_BIN:-"$ROOT_DIR/bin/norvels"}
IMAGE=${NORVELS_IMAGE:-"bsfeng/norvels:${NORVELS_VERSION:-latest}"}
DATA_DIR=${NORVELS_DATA_DIR:-"$ROOT_DIR/data"}
ARGS=()

usage() {
  cat <<'EOF'
norvels 元数据补全脚本（默认 dry-run）

用法:
  scripts/enrich.sh [选项] [-- <norvels enrich 的参数>]

选项:
  --mode <local|docker>  运行方式，默认 local
  --db <路径>            数据库路径，默认 <仓库>/data/norvels.db
  --bin <路径>           本地模式的 norvels 二进制，默认 <仓库>/bin/norvels
  --image <标签>         docker 模式的镜像，默认 bsfeng/norvels:latest
  --data-dir <路径>      docker 模式挂到容器 /data 的宿主目录，默认 <仓库>/data
  -h, --help             显示本帮助

常用参数（原样透传给 norvels enrich）:
  --plan                 只列待补清单与规模，不联网
  --apply                真正写库（默认只 dry-run）
  --limit N              每类最多处理多少本
  --workers 4            并发检索数
  --qps 1                单个书源每秒请求数
  --max-sources 3        一本书最多问几个书源
  --budget 30m           墙钟预算
  --max-requests 20000   全局请求上限
  --split-long-chapters  只做超长章节切分
  --rollback <batchID>   回滚某批次
  --fields author,intro,category
  --min-title-sim 0.86
  --resume --resume-batch <batchID>

示例:
  # 1) 先看规模（不联网、不写库）
  scripts/enrich.sh -- --plan

  # 2) 小批量试跑（dry-run，打印将改什么）
  scripts/enrich.sh -- --limit 50

  # 3) 小批量真写（生成可回滚的 batch）
  scripts/enrich.sh -- --limit 50 --apply

  # 4) 超长章节切分（先 dry-run 看切点）
  scripts/enrich.sh -- --split-long-chapters

  # 5) 回滚
  scripts/enrich.sh -- --rollback 20260101-120000-abcdef --apply

  # Docker 模式（NAS 上）
  scripts/enrich.sh --mode docker --image bsfeng/norvels:0.13.2 -- --plan
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --mode) MODE="$2"; shift 2 ;;
    --db) DB="$2"; shift 2 ;;
    --bin) BIN="$2"; shift 2 ;;
    --image) IMAGE="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --) shift; ARGS=("$@"); break ;;
    *) printf '[错误] 未知参数: %s（norvels enrich 的参数请写在 -- 之后）\n' "$1" >&2; exit 2 ;;
  esac
done

run_local() {
  if [ ! -x "$BIN" ]; then
    printf '[错误] 找不到可执行文件 %s（先跑 make build-server）\n' "$BIN" >&2
    exit 2
  fi
  "$BIN" enrich --db "$DB" "${ARGS[@]}"
}

run_docker() {
  if ! command -v docker >/dev/null; then
    printf '[错误] 未安装 docker\n' >&2
    exit 2
  fi
  mkdir -p "$DATA_DIR"
  # 容器内路径固定 /data/norvels.db；宿主目录挂到 /data。
  docker run --rm --user 65532:65532 --entrypoint /norvels \
    -v "$DATA_DIR:/data" "$IMAGE" enrich --db /data/norvels.db "${ARGS[@]}"
}

case "$MODE" in
  local) run_local ;;
  docker) run_docker ;;
  *) printf '[错误] --mode 只能是 local 或 docker（收到 %s）\n' "$MODE" >&2; exit 2 ;;
esac
