/** 简介/来源名可能夹带原站残留标签：按纯文本处理，不渲染 HTML。 */
export function stripTags(text: string): string {
  return text.replace(/<[^>]*>/g, "").replace(/\s+/g, " ").trim();
}
