#!/usr/bin/env bash
# 构建一致性检查：确保容器里能构建出和本地一致的前端产物。
#
# 为什么需要：Dockerfile 里的 pnpm/node 版本可能与本机不同，
# 而 web/pnpm-workspace.yaml、package.json 里的字段是有版本下限的。
# 本地跑通 ≠ 容器里跑通（实测踩过：pnpm 9 读 allowBuilds 直接报
# "packages field missing or empty"，本机 pnpm 11 完全正常）。
#
# 用法：scripts/check-build-parity.sh
# 退出码：0 一致；1 不一致（列出具体差异）
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

fails=0
fail() { printf '❌ %s\n' "$1" >&2; fails=$((fails+1)); }
ok()   { printf '✅ %s\n' "$1"; }

echo "==> 校验 Dockerfile 与 web/ 的构建工具版本一致性"

# 1) pnpm 主版本：Dockerfile 的 ARG 必须 >= web/pnpm-workspace.yaml 要求的下限
docker_pnpm=$(grep -E '^ARG PNPM_VERSION=' deploy/Dockerfile | head -1 | cut -d= -f2)
[[ -n "$docker_pnpm" ]] || fail "deploy/Dockerfile 里找不到 ARG PNPM_VERSION"

ws="web/pnpm-workspace.yaml"
if [[ -f "$ws" ]]; then
  # allowBuilds / onlyBuiltDependencies 之类的字段需要 pnpm >= 10
  if grep -qE '^(allowBuilds|onlyBuiltDependencies|neverBuiltDependencies):' "$ws"; then
    if [[ "${docker_pnpm:-0}" -lt 10 ]]; then
      fail "web/pnpm-workspace.yaml 用了 pnpm 10+ 才有的字段，但 Dockerfile 的 PNPM_VERSION=${docker_pnpm}"
    else
      ok "pnpm 版本匹配：workspace 需要 >=10，Dockerfile 用 ${docker_pnpm}"
    fi
  else
    ok "workspace 文件没有高版本专属字段（Dockerfile PNPM_VERSION=${docker_pnpm}）"
  fi
fi

# 2) Node 主版本：package.json engines 若声明，Dockerfile 必须满足
node_from_docker=$(grep -oE 'node:[0-9]+' deploy/Dockerfile | head -1 | cut -d: -f2)
if [[ -f web/package.json ]] && command -v python3 >/dev/null; then
  need=$(python3 -c "
import json,sys
try:
    d=json.load(open('web/package.json'))
    e=d.get('engines',{}).get('node','')
    import re
    m=re.search(r'(\d+)', e)
    print(m.group(1) if m else '')
except Exception:
    print('')
")
  if [[ -n "$need" && -n "$node_from_docker" && "$node_from_docker" -lt "$need" ]]; then
    fail "package.json engines.node 要求 >=$need，但 Dockerfile 用 node:$node_from_docker"
  else
    ok "node 版本满足要求（Dockerfile node:${node_from_docker:-未指定}，engines ${need:-未声明}）"
  fi
fi

# 3) lockfile 存在性：没有 lockfile 时 Dockerfile 会退化成非 frozen 安装，构建不可复现
if [[ -f web/pnpm-lock.yaml ]]; then
  ok "web/pnpm-lock.yaml 存在，容器内可走 --frozen-lockfile"
else
  fail "web/pnpm-lock.yaml 缺失：容器构建会退化为非 frozen 安装（不可复现）"
fi

# 4) Dockerfile 必须在 pnpm install 之前复制 pnpm-workspace.yaml
#    它承载 allowBuilds（放行 esbuild 等构建脚本）。缺了它 pnpm 以
#    [ERR_PNPM_IGNORED_BUILDS] 失败——本地有该文件，干净容器里才会暴露。
if [[ -f "$ws" ]]; then
  install_line=$(grep -n 'pnpm install' deploy/Dockerfile | head -1 | cut -d: -f1)
  ws_copy_line=$(grep -n 'pnpm-workspace.yaml' deploy/Dockerfile | head -1 | cut -d: -f1)
  if [[ -z "$ws_copy_line" ]]; then
    fail "Dockerfile 没有复制 web/pnpm-workspace.yaml（allowBuilds 会失效，容器构建报 ERR_PNPM_IGNORED_BUILDS）"
  elif [[ -n "$install_line" && "$ws_copy_line" -gt "$install_line" ]]; then
    fail "Dockerfile 里 pnpm-workspace.yaml 的复制在 pnpm install 之后（第 $ws_copy_line 行 vs 第 $install_line 行），allowBuilds 不生效"
  else
    ok "Dockerfile 在 install 前复制了 pnpm-workspace.yaml（第 $ws_copy_line 行）"
  fi
fi

# 5) .dockerignore 不能把构建必需的文件排除掉
if [[ -f .dockerignore ]]; then
  for need in "web/package.json" "web/pnpm-workspace.yaml" "web/src"; do
    base=$(basename "$need")
    if grep -qxE "${base}|web" .dockerignore 2>/dev/null; then
      fail ".dockerignore 排除了构建必需的 $need"
    fi
  done
  ok ".dockerignore 未排除 web/ 构建输入"
fi

echo
if [[ $fails -eq 0 ]]; then
  echo "✅ 构建一致性检查通过"
  exit 0
fi
echo "❌ $fails 项不一致，修完再提交"
exit 1
