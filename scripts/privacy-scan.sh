#!/usr/bin/env bash
# 提交前隐私扫描：确保仓库里不出现本机路径、内网地址、真实书名/作者、密钥等不该公开的内容。
#
# 用法：
#   scripts/privacy-scan.sh              # 扫描暂存区（先 git add）
#   scripts/privacy-scan.sh --all        # 扫描工作区全部受版本控制的文件
#
# 三层规则：
#   1) 内置通用规则：绝对路径、IP、真实邮箱、私钥、常见令牌前缀
#   2) 项目黑名单 .privacy-denylist.txt（本地维护真实书名/作者/站点名，不入库）
#   3) 跳过二进制、依赖目录与生成物
#
# 退出码：0 = 干净；1 = 命中；2 = 用法错误。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

case "${1:-}" in
  ""|--staged) MODE=staged ;;
  --all)       MODE=all ;;
  -h|--help)   sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

LIST_FILE=$(mktemp -t privacy-files)
RULES_FILE=$(mktemp -t privacy-rules)
trap 'rm -f "$LIST_FILE" "$RULES_FILE"' EXIT

if [[ "$MODE" == "staged" ]]; then
  git diff --cached --name-only --diff-filter=ACMR > "$LIST_FILE" 2>/dev/null
else
  # --all：已跟踪 + 未跟踪（但遵守 .gitignore）的全部文件，适合首次提交前自查
  { git ls-files 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null; } | sort -u > "$LIST_FILE"
fi

TOTAL=$(wc -l < "$LIST_FILE" | tr -d ' ')
if [[ "$TOTAL" -eq 0 ]]; then
  printf '[提示] 没有待扫描文件（暂存区为空？先 git add，或改用 --all）\n'
  exit 0
fi
printf '==> 隐私扫描（%s），候选文件 %d 个\n' "$MODE" "$TOTAL"

is_text() {
  case "$1" in
    *.jpg|*.jpeg|*.png|*.webp|*.gif|*.pdf|*.ico|*.woff|*.woff2) return 1 ;;
    *.db|*.db-wal|*.db-shm|*.sqlite|*.test|*.out|*.exe|*.a) return 1 ;;
    go.sum) return 1 ;;
    node_modules/*|*/node_modules/*|dist/*|*/dist/*|.git/*) return 1 ;;
    .privacy-denylist.txt) return 1 ;;   # 黑名单自身不入库，避免把真实书名写进仓库
  esac
  return 0
}

hits=0
# scan_pattern <pattern> <label> [allow-regex]
# allow-regex 命中的行会被放行（用于把 127.0.0.1 / 0.0.0.0 / localhost 这类正常回环地址排除掉）。
scan_pattern() {
  local pat=$1 label=$2 allow=${3:-} f out
  local found=""
  while IFS= read -r f; do
    [[ -n "$f" && -f "$f" ]] || continue
    is_text "$f" || continue
    out=$(grep -nIE --binary-files=without-match -- "$pat" "$f" 2>/dev/null)
    if [[ -n "$allow" && -n "$out" ]]; then
      out=$(printf '%s\n' "$out" | grep -vE -- "$allow")
    fi
    out=$(printf '%s\n' "$out" | head -8)
    [[ -n "$out" ]] && found="${found}${f}\n${out}\n"
  done < "$LIST_FILE"
  if [[ -n "$found" ]]; then
    printf '\n[命中] %s\n' "$label"
    printf '%b' "$found" | sed 's/^/  /' | cut -c1-200
    hits=$((hits + 1))
  fi
}

# 内置通用规则（pattern<TAB>label）；用 \t 分隔避免与正则里的 | 冲突。
printf '%s\n' \
  '/Users/[A-Za-z0-9._-]+	本机绝对路径（含用户名）' \
  '/home/[A-Za-z0-9._-]+	本机绝对路径（含用户名）' \
  '(^|[^0-9.])((25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]?))\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[0-9]{1,2})\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[0-9]{1,2})\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]?)([^0-9.]|$)	疑似 IP 地址	127\.0\.0\.1|0\.0\.0\.0|localhost' \
  '[A-Za-z0-9._%+-]+@(qq|163|126|gmail|outlook|foxmail|icloud)\.com	真实邮箱地址' \
  '-----BEGIN (RSA |OPENSSH |EC |PGP )?PRIVATE KEY-----	私钥' \
  '(ghp_|gho_|ghu_|ghs_|github_pat_)[A-Za-z0-9_]{16,}	GitHub 令牌' \
  'AKIA[0-9A-Z]{16}	AWS Access Key' \
  'sk-[A-Za-z0-9]{20,}	API Secret Key' \
  > "$RULES_FILE"

while IFS=$'\t' read -r pat label allow; do
  [[ -n "${pat:-}" && -n "${label:-}" ]] || continue
  scan_pattern "$pat" "$label" "${allow:-}"
done < "$RULES_FILE"

DENY=".privacy-denylist.txt"
if [[ -f "$DENY" ]]; then
  printf '\n==> 项目黑名单 %s\n' "$DENY"
  while IFS= read -r term; do
    [[ -z "$term" ]] && continue
    case "$term" in \#*) continue ;; esac
    esc=$(printf '%s' "$term" | sed 's/[][\\./\*^$()+?{|]/\\&/g')
    scan_pattern "$esc" "黑名单：$term"
  done < "$DENY"
else
  printf '\n[提示] 可选：创建 %s 登记真实书名/作者/站点名\n' "$DENY"
fi

printf '\n'
if [[ $hits -eq 0 ]]; then
  printf '✅ 未发现敏感内容\n'
  exit 0
fi
printf '❌ 命中 %d 类敏感内容，处理后重新扫描\n' "$hits"
exit 1
