/** 作者输入的分隔符：逗号 / 顿号 / 分号 / 换行，全角半角都认。 */
const SEPARATORS = /[,，、;；\n\r]+/;

/** 批量设作者用：按分隔符拆开、去空白、按输入顺序去重（后端要求作者不重复）。 */
export function parseAuthorList(value: string): string[] {
  const seen = new Set<string>();
  for (const raw of value.split(SEPARATORS)) {
    const name = raw.trim();
    if (name) seen.add(name);
  }
  return [...seen];
}
