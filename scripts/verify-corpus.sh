#!/usr/bin/env bash
# 真实语料导入验收：把 docs/data-profile.md 里的「验收基线」变成可重复执行的脚本。
#
# 用法：
#   scripts/verify-corpus.sh --dir /path/to/txt [--db /tmp/verify.db] [--bin ./bin/norvels]
#
# 产出：书数 / 章数 / 字数 + 7 条质检指标，任一不达标 exit 1。
# 语料与库都不入库（.gitignore 覆盖），脚本本身不含任何真实书名。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

DIR=""
DB="/tmp/norvels-verify.db"
BIN="$ROOT_DIR/bin/norvels"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) DIR=${2:?"--dir 需要参数"}; shift 2 ;;
    --db)  DB=${2:?"--db 需要参数"}; shift 2 ;;
    --bin) BIN=${2:?"--bin 需要参数"}; shift 2 ;;
    -h|--help) sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[[ -n "$DIR" ]] || { printf '[错误] 必须用 --dir 指定语料目录\n' >&2; exit 2; }
[[ -d "$DIR" ]] || { printf '[错误] 目录不存在: %s\n' "$DIR" >&2; exit 2; }
[[ -x "$BIN" ]] || { printf '[错误] 二进制不存在: %s（先 make build-server）\n' "$BIN" >&2; exit 2; }
command -v sqlite3 >/dev/null || { printf '[错误] 需要 sqlite3\n' >&2; exit 2; }

fails=0
check() {
  local label=$1 got=$2 rule=$3
  local verdict="✅"
  case "$rule" in
    "==0") [[ "$got" == "0" ]] || { verdict="❌"; fails=$((fails+1)); } ;;
    "<=15%")
      local pct
      pct=$(awk -v a="$got" 'BEGIN{printf "%.1f", a}')
      awk -v v="$pct" 'BEGIN{exit !(v<=15)}' || { verdict="❌"; fails=$((fails+1)); } ;;
  esac
  printf '  %s %-34s %s\n' "$verdict" "$label" "$got"
}

printf '==> 导入 %s\n' "$DIR"
rm -f "$DB" "$DB-wal" "$DB-shm"
"$BIN" import --dir "$DIR" --db "$DB" 2>&1 | tail -2 || { printf '[错误] 导入失败\n' >&2; exit 1; }

printf '\n==> 质检指标（%s）\n' "$DB"
q() { sqlite3 "$DB" "$1" 2>/dev/null; }

books=$(q "SELECT count(*) FROM books;")
chapters=$(q "SELECT sum(chapter_count) FROM books;")
words=$(q "SELECT sum(word_count) FROM books;")
printf '  书籍 %s 本 / 章节 %s / 字数 %s\n\n' "$books" "$chapters" "$words"

check "章节数 ≤1 的书"          "$(q "SELECT count(*) FROM books WHERE chapter_count<=1;")" "==0"
# 正文已按块压缩存储（0007），content 列不存在；用 block_len（该章在块里的字节数）判定。
check "空章节"                  "$(q "SELECT count(*) FROM chapters WHERE block_len=0;")" "==0"
check "超长单章（>80KB）"        "$(q "SELECT count(*) FROM chapters WHERE block_len>80000;")" "==0"
check "书名残留装饰（【】》搜书吧）" "$(q "SELECT count(*) FROM books WHERE title LIKE '%【%' OR title LIKE '%】%' OR title LIKE '%》%' OR title LIKE '%搜书吧%';")" "==0"
check "作者缺失占比（%）"        "$(q "SELECT printf('%.1f', 100.0*sum(author_id=(SELECT id FROM authors WHERE name='佚名'))/count(*)) FROM books;")" "<=15%"
# 检索已不含正文（0006 起正文不建索引），因此这里不再做 FTS 完整性检查。
# 取而代之：确认分词镜像列与 FTS 表确实不存在，避免老库升级漏删把库撑大。
seg_out=$(q "SELECT count(*) FROM pragma_table_info('chapters') WHERE name IN ('title_seg','content_seg');")
check "分词镜像列已移除"        "$seg_out" "==0"
fts_tbl=$(q "SELECT count(*) FROM sqlite_master WHERE name LIKE 'chapters_fts%';")
check "FTS 索引表已移除"        "$fts_tbl" "==0"

printf '\n'
if [[ $fails -eq 0 ]]; then
  printf '✅ 全部指标达标\n'
  exit 0
fi
printf '❌ %d 项未达标\n' "$fails"
exit 1
