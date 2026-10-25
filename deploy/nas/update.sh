#!/usr/bin/env bash
# norvels 一键更新：备份 → 拉新镜像 → 预跑 schema 迁移 → 重启 → 等健康；失败自动回滚镜像版本。
#
# 用法（必须有 docker 权限，飞牛上就是 root）：
#   sudo ./update.sh              # 按 .env 里现有的 NORVELS_VERSION 更新
#   sudo ./update.sh --latest     # 自动升级到 Docker Hub 上的最新正式版
#   sudo ./update.sh 0.3.1        # 切到指定版本，并写回 .env
#   sudo ./update.sh --check      # 只看当前状态，不动任何东西
#   sudo ./update.sh --check-updates  # 问一下上游有没有新版本，不升级
#
# 环境变量：
#   NORVELS_BACKUP_KEEP  保留几份数据库备份，默认 3
#   NORVELS_BACKUP_DIR   备份目录，默认 <数据目录的父目录>/backup
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
ENV_FILE=".env"

die() { echo "错误：$*" >&2; exit 1; }

# ---------- 参数 ----------
CHECK_ONLY=0
CHECK_UPDATES=0
USE_LATEST=0
NEW_VERSION=""
for arg in "$@"; do
  case "$arg" in
    --check) CHECK_ONLY=1 ;;
    --check-updates) CHECK_UPDATES=1 ;;
    --latest) USE_LATEST=1 ;;
    -* ) die "未知参数 $arg" ;;
    *  ) NEW_VERSION="$arg" ;;
  esac
done

# 对比「容器正在用的镜像」与「上游 latest」。
#
# 为什么不查 Docker Hub API 或 manifest：这两种都要直连 registry-1.docker.io，
# 在国内常常超时。而 docker pull 走的是本机配置的镜像加速器——也就是部署时
# 唯一必须打通的通道，用它判断最可靠（镜像 17 MB，层已缓存时约 2 秒）。
pull_latest_version() {
  docker pull -q bsfeng/norvels:latest >/dev/null 2>&1 || return 1
  docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' bsfeng/norvels:latest 2>/dev/null || true
}

check_updates() {
  local cur_id cur_ver latest_id latest_ver
  if ! cur_id=$(docker inspect --format '{{.Image}}' norvels 2>/dev/null); then
    die "找不到运行中的容器 norvels（先 docker compose up -d）"
  fi
  cur_ver=$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' norvels 2>/dev/null || true)
  echo "当前运行：${cur_ver:-unknown}"

  echo "查询最新正式版（经本机 docker 镜像加速器）…"
  latest_ver=$(pull_latest_version) || { echo "拉取失败：本机访问不了镜像仓库。检查 docker info 里的 Registry Mirrors。" >&2; return 1; }
  [ -n "$latest_ver" ] || { echo "读取不到 latest 的版本号" >&2; return 1; }
  latest_id=$(docker inspect --format '{{.Id}}' bsfeng/norvels:latest)

  if [ "$cur_id" = "$latest_id" ]; then
    echo "已是最新（$latest_ver）"
    return 0
  fi
  echo "有新版本：$latest_ver（当前 ${cur_ver:-unknown}）"
  echo "  升级命令：sudo ./update.sh --latest"
}

command -v docker >/dev/null || die "没有 docker"
docker compose version >/dev/null 2>&1 || die "没有 docker compose 插件"
[ -f "$ENV_FILE" ] || die "缺少 .env（先 cp .env.example .env 并按机器改）"
[ -f compose.yml ] || die "缺少 compose.yml（应与本脚本同目录）"

# shellcheck disable=SC1090
set -a; . "./$ENV_FILE"; set +a
DATA_DIR="${NORVELS_DATA_DIR:?请在 .env 里设置 NORVELS_DATA_DIR}"
[ -d "$DATA_DIR" ] || die "数据目录不存在：$DATA_DIR"

echo "== 当前状态 =="
docker compose ps || true
echo "当前版本：${NORVELS_VERSION:-latest}   数据目录：$DATA_DIR"

if [ "$CHECK_UPDATES" = "1" ]; then
  check_updates
  exit $?
fi

if [ "$CHECK_ONLY" = "1" ]; then
  echo "（--check 模式，到此为止）"
  exit 0
fi

OLD_VERSION="${NORVELS_VERSION:-latest}"
APPLIED=0

# 失败自动回滚镜像版本（数据库不回滚：schema 只前进，恢复备份按 README）。
on_exit() {
  local code=$?
  if [ "$code" -ne 0 ] && [ "$APPLIED" = "1" ]; then
    echo "!! 升级失败（退出码 $code），回滚镜像版本到 $OLD_VERSION" >&2
    sed -i.bak "s|^NORVELS_VERSION=.*|NORVELS_VERSION=$OLD_VERSION|" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
    docker compose up -d >/dev/null 2>&1 || true
    echo "!! 数据库未回滚——若新库确认不可用，按 README「备份」一节恢复。" >&2
  fi
  exit $code
}
trap on_exit EXIT

# ---------- 解析最新版本 ----------
if [ "$USE_LATEST" = "1" ]; then
  echo "== 查询最新正式版 =="
  LATEST_VER=$(pull_latest_version) || die "拉取 latest 失败（检查镜像加速器）"
  [ -n "$LATEST_VER" ] || die "读取不到 latest 的版本号"
  echo "最新正式版：$LATEST_VER"
  NEW_VERSION="$LATEST_VER"
fi

# ---------- 备份 ----------
BACKUP_DIR="${NORVELS_BACKUP_DIR:-$(dirname -- "$DATA_DIR")/backup}"
KEEP="${NORVELS_BACKUP_KEEP:-3}"
case "$KEEP" in (''|*[!0-9]*) die "NORVELS_BACKUP_KEEP 必须是数字" ;; esac

if [ -f "$DATA_DIR/norvels.db" ]; then
  mkdir -p "$BACKUP_DIR"
  echo "== 备份数据库（保留最近 $KEEP 份）=="
  docker compose stop
  BK="$BACKUP_DIR/norvels-$OLD_VERSION-$(date +%Y%m%d-%H%M).db"
  cp -a "$DATA_DIR/norvels.db" "$BK"
  SRC=$(stat -c %s "$DATA_DIR/norvels.db")
  DST=$(stat -c %s "$BK")
  [ "$SRC" = "$DST" ] || die "备份大小不一致（$SRC vs $DST），已中止"
  echo "备份完成：$BK（$DST 字节）"
  ls -1t "$BACKUP_DIR"/norvels-*.db 2>/dev/null | tail -n +$((KEEP + 1)) | while read -r old; do
    rm -f "$old" && echo "  清理旧备份：$old"
  done
fi

# ---------- 切换版本 ----------
if [ -n "$NEW_VERSION" ]; then
  if grep -q '^NORVELS_VERSION=' "$ENV_FILE"; then
    sed -i.bak "s|^NORVELS_VERSION=.*|NORVELS_VERSION=$NEW_VERSION|" "$ENV_FILE"
    rm -f "$ENV_FILE.bak"
  else
    printf 'NORVELS_VERSION=%s\n' "$NEW_VERSION" >> "$ENV_FILE"
  fi
  APPLIED=1
  echo "已把 .env 中的版本改为 $NEW_VERSION"
  set -a; . "./$ENV_FILE"; set +a
fi
VERSION="${NORVELS_VERSION:-latest}"
IMAGE="bsfeng/norvels:$VERSION"

# ---------- 拉镜像 ----------
echo "== 拉取 $IMAGE =="
docker pull "$IMAGE"

# ---------- 预跑迁移 ----------
# --entrypoint /norvels 不能少：镜像 ENTRYPOINT 是 "norvels serve"，不覆盖就会
# 变成 "serve migrate --dry-run"，静默起一个服务、永远看不到迁移计划。
echo "== 检查待应用迁移 =="
DRY_OUT=$(docker run --rm --user 65532:65532 --entrypoint /norvels \
  -v "$DATA_DIR:/data" "$IMAGE" migrate --db /data/norvels.db --dry-run 2>&1)
echo "$DRY_OUT"

# 输出里必须出现"待应用"。没有就说明 migrate 根本没执行（例如 ENTRYPOINT 覆盖失效），
# 这时绝不能当成"0 个迁移"继续往下走。
case "$DRY_OUT" in
  *待应用*) ;;
  *) die "migrate --dry-run 输出异常，未识别到迁移计划（检查 --entrypoint 是否生效）" ;;
esac

PENDING=$(printf '%s\n' "$DRY_OUT" | grep -c '^  0' || true)

if [ "${PENDING:-0}" -gt 0 ]; then
  echo "== 检测到 $PENDING 个待应用迁移，先单独执行（大库可能需要几十分钟）=="
  echo "   期间不要中断；完成后推荐再跑一次 VACUUM 回收空间（见 README）。"
  docker run --rm --user 65532:65532 --entrypoint /norvels \
    -v "$DATA_DIR:/data" "$IMAGE" migrate --db /data/norvels.db
else
  echo "无待应用迁移，直接重启容器"
fi

# ---------- 重启并等待健康 ----------
echo "== 启动容器 =="
docker compose up -d

echo "== 等待健康检查（最多 120 秒）=="
for i in $(seq 1 24); do
  STATE=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' norvels 2>/dev/null || echo missing)
  echo "  [$((i * 5))s] $STATE"
  case "$STATE" in
    healthy) echo "== 更新完成：$IMAGE =="; docker compose ps; exit 0 ;;
    unhealthy) die "容器进入 unhealthy，用 docker compose logs --tail 50 norvels 查看" ;;
  esac
  sleep 5
done

die "120 秒内未变 healthy：docker compose logs --tail 50 norvels"
