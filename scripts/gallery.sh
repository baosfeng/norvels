#!/usr/bin/env bash
# 重新生成三版原型的并排对比页 + 截图。
#
# 为什么要脚本：odops gallery 会把 iframe 的 src 写成 file:// + 本机绝对路径，
# 直接入库会带出用户名。这里生成后统一改回相对路径，并跑一次隐私扫描确认。
#
# 用法：scripts/gallery.sh [--cols 3] [--frame 400x700]
set -euo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR"

OUT_HTML=".open-design/03-compare.html"
OUT_SHOT=".open-design/03-compare.jpg"

odops gallery .open-design/03-layouts \
  --out "$OUT_HTML" \
  --notes .open-design/03-layouts/variants.md \
  --cols 3 --frame 400x700 \
  --shot "$OUT_SHOT" \
  --yes "$@"

# file:///abs/path/03-layouts/a.html → 03-layouts/a.html
perl -0pi -e 's{src="file://[^"]*?/03-layouts/([abc])\.html"}{src="03-layouts/$1.html"}g' "$OUT_HTML"

printf '\n==> 已相对化 iframe 路径，复查敏感内容\n'
scripts/privacy-scan.sh --all
