#!/usr/bin/env bash
# 提交前隐私扫描：确保仓库里不出现本机路径、内网地址、真实书名/作者、密钥等不该公开的内容。
#
# 用法：
#   scripts/privacy-scan.sh              # 扫描暂存区（先 git add）
#   scripts/privacy-scan.sh --changed    # 只扫相对 HEAD 的改动（含未跟踪），秒级自查
#   scripts/privacy-scan.sh --all        # 扫描工作区全部受版本控制的文件（提交前硬条件）
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
  --changed)   MODE=changed ;;
  --all)       MODE=all ;;
  -h|--help)   sed -n '2,15p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

LIST_FILE=$(mktemp -t privacy-files)
RULES_FILE=$(mktemp -t privacy-rules)
DENY_RULES_FILE=$(mktemp -t privacy-deny-rules)
PAT_FILE=$(mktemp -t privacy-patterns)
CAND_FILE=$(mktemp -t privacy-candidates)
TEXT_LIST=$(mktemp -t privacy-text-files)
WORK_DIR=$(mktemp -d -t privacy-hits)
trap 'rm -f "$LIST_FILE" "$RULES_FILE" "$DENY_RULES_FILE" "$PAT_FILE" "$CAND_FILE" "$TEXT_LIST"; rm -rf "$WORK_DIR"' EXIT

if [[ "$MODE" == "staged" ]]; then
  git diff --cached --name-only --diff-filter=ACMR > "$LIST_FILE" 2>/dev/null
elif [[ "$MODE" == "changed" ]]; then
  # --changed：相对 HEAD 的全部改动（已暂存 + 未暂存 + 未跟踪），删掉已不存在的文件。
  # 是 --all 的子集，用于改完随手自查；提交前仍以 --all 为准（未改动的旧文件也要扫）。
  { git diff --name-only HEAD 2>/dev/null || git diff --cached --name-only 2>/dev/null
    git ls-files --others --exclude-standard 2>/dev/null
  } | sort -u | while IFS= read -r f; do [[ -f "$f" ]] && printf '%s\n' "$f"; done > "$LIST_FILE"
else
  # --all：已跟踪 + 未跟踪（但遵守 .gitignore）的全部文件，适合首次提交前自查
  { git ls-files 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null; } | sort -u > "$LIST_FILE"
fi

TOTAL=$(wc -l < "$LIST_FILE" | tr -d ' ')
if [[ "$TOTAL" -eq 0 ]]; then
  if [[ "$MODE" == "staged" ]]; then
    # 暂存区为空是正常状态（还没 git add）：提示即可，不算失败。
    printf '[提示] 暂存区没有待扫描文件（先 git add，或改用 --all）\n'
    exit 0
  fi
  # --all / --changed 拿不到任何文件 = git 命令失败 / 不在工作区 / 浅克隆异常。
  # 这时 exit 0 是假绿：一个文件都没扫却报「通过」，比红更糟。按用法错误处理。
  printf '[错误] 扫描清单为空（--all/--changed 至少应包含已跟踪文件）：检查是否在 git 工作区、git 是否可用\n' >&2
  exit 2
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

# build_text_list <文件列表> <输出>：过滤掉二进制与依赖/生成物，供后续 grep 批量传参。
# 候选列表只生成一次，N 条规则共用，避免「每条规则 × 每个文件」各 fork 一次。
build_text_list() {
  local src=$1 dst=$2 f
  : > "$dst"
  while IFS= read -r f; do
    [[ -n "$f" && -f "$f" ]] || continue
    is_text "$f" && printf '%s\n' "$f" >> "$dst"
  done < "$src"
}

hits=0
# scan_rule <序号> <pattern<TAB>label[<TAB>allow-regex]>
# allow-regex 命中的行会被放行（用于把 127.0.0.1 / 0.0.0.0 / localhost 这类正常回环地址排除掉）。
# 每条规则独立扫完全部候选文件，结果写进自己的 .hit；回放由 run_rule_set 负责。
scan_rule() {
  local idx=$1 line=$2 pat label allow out
  local found=""
  IFS=$'\t' read -r pat label allow <<<"$line"
  # 一次 grep 扫完全部候选文件（xargs 分批避开 ARG_MAX）：进程数从「规则数 × 文件数」降到「规则数 × 批数」。
  # 多文件输出形如「文件:行号:内容」，这里按文件还原成「文件一行 + 每行『行号:内容』」；
  # allow 放行与每文件 head -8 都按文件应用，与原逐文件实现逐字节等价。
  if [[ -s "$TEXT_LIST" ]]; then
    found=$(tr '\n' '\0' < "$TEXT_LIST" \
      | xargs -0 -n 200 grep -nIE --binary-files=without-match -- "$pat" 2>/dev/null \
      | PRIV_ALLOW="$allow" awk '
          function flush() { if (cur != "" && n > 0) printf "%s\\n%s\\n", cur, buf }
          {
            p = index($0, ":")
            if (p == 0) next
            f = substr($0, 1, p - 1)
            rest = substr($0, p + 1)
            q = index(rest, ":")
            if (q == 0) next
            hit = substr(rest, 1, q - 1) ":" substr(rest, q + 1)
            if (ENVIRON["PRIV_ALLOW"] != "" && hit ~ ENVIRON["PRIV_ALLOW"]) next
            if (f != cur) { flush(); cur = f; n = 0; buf = "" }
            if (n < 8) { buf = buf (n == 0 ? "" : "\n") hit; n++ }
          }
          END { flush() }')
  fi
  if [[ -n "$found" ]]; then
    {
      printf '\n[命中] %s\n' "$label"
      printf '%b' "$found" | sed 's/^/  /' | cut -c1-200
    } > "$WORK_DIR/$idx.hit"
  fi
}

# run_rule_set <规则文件>：规则之间并发执行，再按规则在文件里的顺序回放命中块。
# 输出与串行版本逐字节相同（只有快慢变了），hits 的计数口径也不变：一条规则算一类。
run_rule_set() {
  local file=$1 idx=0 line
  build_text_list "$LIST_FILE" "$TEXT_LIST"   # 本批规则共用的候选文本文件列表
  rm -f "$WORK_DIR"/*.hit   # 两次调用（内置规则 / 黑名单）共用编号空间，先清掉上一批
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    idx=$((idx + 1))
    ( scan_rule "$idx" "$line" ) &
  done < "$file"
  wait
  idx=0
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    idx=$((idx + 1))
    if [[ -f "$WORK_DIR/$idx.hit" ]]; then
      cat "$WORK_DIR/$idx.hit"
      hits=$((hits + 1))
    fi
  done < "$file"
}

# 内置通用规则（pattern<TAB>label）；用 \t 分隔避免与正则里的 | 冲突。
printf '%s\n' \
  '/Users/[A-Za-z0-9._-]+	本机绝对路径（含用户名）' \
  '/home/[A-Za-z0-9._-]+	本机绝对路径（含用户名）' \
  '(^|[^0-9.])((25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]?))\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[0-9]{1,2})\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[0-9]{1,2})\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]?)([^0-9.]|$)	疑似 IP 地址	10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.|169\.254\.|100\.64\.|192\.0\.0\.|198\.18\.|198\.51\.100\.|203\.0\.113\.|192\.0\.2\.|127\.|0\.0\.0\.0|::1|fc00|fe80|::ffff' \
  '[A-Za-z0-9._%+-]+@(qq|163|126|gmail|outlook|foxmail|icloud)\.com	真实邮箱地址' \
  '-----BEGIN (RSA |OPENSSH |EC |PGP )?PRIVATE KEY-----	私钥' \
  '(ghp_|gho_|ghu_|ghs_|github_pat_)[A-Za-z0-9_]{16,}	GitHub 令牌' \
  'AKIA[0-9A-Z]{16}	AWS Access Key' \
  'sk-[A-Za-z0-9]{20,}	API Secret Key' \
  > "$RULES_FILE"

# 项目黑名单规则（先建好，预筛要把它一起算进去）
DENY=".privacy-denylist.txt"
if [[ -f "$DENY" ]]; then
  while IFS= read -r term; do
    [[ -z "$term" ]] && continue
    case "$term" in \#*) continue ;; esac
    esc=$(printf '%s' "$term" | sed 's/[][\\./\*^$()+?{|]/\\&/g')
    printf '%s\t%s\n' "$esc" "黑名单：$term" >> "$DENY_RULES_FILE"
  done < "$DENY"
fi

# 预筛：把所有规则合成一份 pattern 文件，一次 grep 找出「可能有命中」的文件。
# 干净仓库（绝大多数情况）这一步就结束扫描；有嫌疑文件时才逐规则精确扫描，
# 精确扫描仍走原样的 grep -n + allow 放行，命中集合与「全量逐规则扫」一致。
cut -f1 "$RULES_FILE" > "$PAT_FILE"
cut -f1 "$DENY_RULES_FILE" >> "$PAT_FILE"
: > "$CAND_FILE"
if [[ -s "$PAT_FILE" ]]; then
  # 一次 grep 扫完所有文本文件（xargs -0 处理含空格的路径名，不逐个 fork）
  build_text_list "$LIST_FILE" "$TEXT_LIST"
  if [[ -s "$TEXT_LIST" ]]; then
    tr '\n' '\0' < "$TEXT_LIST" \
      | xargs -0 grep -lIE --binary-files=without-match -f "$PAT_FILE" -- > "$CAND_FILE" 2>/dev/null
  fi
fi

if [[ -s "$CAND_FILE" ]]; then
  printf '==> 预筛：%d 个文件可能有命中，逐规则精确扫描\n' "$(wc -l < "$CAND_FILE" | tr -d ' ')"
  ALL_LIST=$LIST_FILE
  LIST_FILE=$CAND_FILE
  run_rule_set "$RULES_FILE"
  LIST_FILE=$ALL_LIST
fi

if [[ -f "$DENY" ]]; then
  printf '\n==> 项目黑名单 %s\n' "$DENY"
  if [[ -s "$CAND_FILE" ]]; then
    ALL_LIST=$LIST_FILE
    LIST_FILE=$CAND_FILE
    run_rule_set "$DENY_RULES_FILE"
    LIST_FILE=$ALL_LIST
  fi
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
