/**
 * 章节正文的纯计算：字数、段数、改动检测。抽出来的理由是这三件事都有口径争议，
 * 埋在组件里只能靠手点验证。
 *
 * 字数口径 = **非空白字符数**（与原型一致）：汉字、字母、标点都各算 1，空白不算。
 * 与后端 countWords（internal/parser/words.go）的差异，写清楚免得以后当 bug 查：
 *   - 后端只数 CJK 与「词」，标点不计；连续的拉丁字母/数字串算 1 个字。
 *     所以 `"你好，世界"` 后端得 4、这里得 5；`"hello world"` 后端得 2、这里得 10。
 *   - 中文正文里两者随手改几个字就同步增长，做「实时字数」够用；它是**本地估算**。
 * 唯一真相是服务端：保存成功后重拉章节目录，目录里的 wordCount 由后端重算，
 * 面板上的估算值不会回写任何地方。
 */

/** 非空白字符数：`"你好，世界"` → 5，`"  a b  "` → 2。 */
export function countBodyWords(text: string): number {
  let n = 0;
  // 按码点遍历（for…of）而不是 length：扩展 B 区汉字在 UTF-16 里占两个码元，
  // 后端按 rune 计数，用 length 会多数一次。
  for (const ch of text) {
    if (!/\s/.test(ch)) n += 1;
  }
  return n;
}

/**
 * 段落数：按空行分段（正文用空行分段的书写习惯），纯空白段不计。
 *
 * 与书签的段落口径**不同**（书签按单个 \n 切段，见 api/types 的 paraIndex），
 * 这里只回答「这一章看起来有多少段」，不参与任何定位计算。
 */
export function countBodyParagraphs(text: string): number {
  return text.split(/\n{2,}/).filter((block) => block.trim() !== "").length;
}

/** 有没有未保存的改动：与上次保存的正文逐字比较（不 trim，删掉的空格也算改动）。 */
export function bodyIsDirty(draft: string, saved: string): boolean {
  return draft !== saved;
}

/** 抽屉副行的状态词。 */
export function bodyStateText(dirty: boolean): string {
  return dirty ? "有未保存的改动" : "未改动";
}
