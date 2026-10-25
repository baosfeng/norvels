#!/usr/bin/env bash
# norvels 一键更新：拉新镜像 → 预跑 schema 迁移 → 重启容器 → 等健康。
#
# 用法（必须有 docker 权限，飞牛上就是 root）：
#   sudo ./update.sh            # 按 .env 里现有的 NORVELS_VERSION 更新
#   sudo ./update.sh 0.3.0      # 切到指定版本，并写回 .env
#   sudo ./update.sh --check    # 只看当前状态，不动任何东西
#
# 为什么先单独跑迁移：大库迁移可能几十分钟，期间进程不监听端口，
# 容器自带的 healthcheck 会判 unhealthy 并重启，导致迁移回滚重来。
# 这里用一次性容器把迁移跑完，再 up -d，启动时就是"0 个迁移可应用"。
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
ENV_FILE=".env"

die() { echo "错误：$*" >&2; exit 1; }

# ---------- 参数 ----------
CHECK_ONLY=0
NEW_VERSION=""
for arg in "$@"; do
  case "$arg" in
    --check) CHECK_ONLY=1 ;;
    -* ) die "未知参数 $arg" ;;
    *  ) NEW_VERSION="$arg" ;;
  esac
done

command -v docker >/dev/null || die "没有 docker"
docker compose version >/dev/null 2>&1 || die "没有 docker compose 插件"
[ -f "$ENV_FILE" ] || die "缺少 .env（先 cp .env.example .env 并按机器改）"
[ -f compose.yml ] || die "缺少 compose.yml（应与本脚本同目录）"

# shellcheck disable=SC1090
set -a; . "./$ENV_FILE"; set +a
DATA_DIR="${NORVELS_DATA_DIR:?请在 .env 里设置 NORVELS_DATA_DIR}"
[ -d "$DATA_DIR" ] || die "数据目录不存在：$DATA_DIR"

echo "== 当前状态 =="
docker compose ps
echo "当前版本：${NORVELS_VERSION:-latest}   数据目录：$DATA_DIR"

if [ "$CHECK_ONLY" = "1" ]; then
  echo "（--check 模式，到此为止）"
  exit 0
fi

# ---------- 切换版本 ----------
if [ -n "$NEW_VERSION" ]; then
  if grep -q '^NORVELS_VERSION=' "$ENV_FILE"; then
    sed -i.bak "s|^NORVELS_VERSION=.*|NORVELS_VERSION=$NEW_VERSION|" "$ENV_FILE"
    rm -f "$ENV_FILE.bak"
  else
    printf 'NORVELS_VERSION=%s\n' "$NEW_VERSION" >> "$ENV_FILE"
  fi
  echo "已把 .env 中的版本改为 $NEW_VERSION"
  set -a; . "./$ENV_FILE"; set +a
fi
VERSION="${NORVELS_VERSION:-latest}"
IMAGE="bsfeng/norvels:$VERSION"

# ---------- 拉镜像 ----------
echo "== 拉取 $IMAGE =="
docker pull "$IMAGE"

# ---------- 预跑迁移 ----------
echo "== 检查待应用迁移 =="
docker run --rm --user 65532:65532 -v "$DATA_DIR:/data" "$IMAGE" \
  migrate --db /data/norvels.db --dry-run

PENDING=$(docker run --rm --user 65532:65532 -v "$DATA_DIR:/data" "$IMAGE" \
  migrate --db /data/norvels.db --dry-run | grep -c '^  0' || true)

if [ "${PENDING:-0}" -gt 0 ]; then
  echo "== 检测到 $PENDING 个待应用迁移，先单独执行（大库可能需要几十分钟）=="
  echo "   期间不要中断；完成后推荐再跑一次 VACUUM 回收空间（见 README）。"
  docker run --rm --user 65532:65532 -v "$DATA_DIR:/data" "$IMAGE" \
    migrate --db /data/norvels.db
else
  echo "无待应用迁移，直接重启容器"
fi

# ---------- 重启并等待健康 ----------
echo "== 启动容器 =="
docker compose up -d

echo "== 等待健康检查（最多 120 秒）=="
for i in $(seq 1 24); do
  STATE=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' norvels 2>/dev/null || echo missing)
  echo "  [$((i*5))s] $STATE"
  case "$STATE" in
    healthy) echo "== 更新完成：$IMAGE =="; docker compose ps; exit 0 ;;
    unhealthy) die "容器进入 unhealthy，用 docker compose logs --tail 50 norvels 查看" ;;
  esac
  sleep 5
done

die "120 秒内未变 healthy：docker compose logs --tail 50 norvels"
