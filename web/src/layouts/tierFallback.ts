/** PC 专属页面的中文名：错档落点提示要用（契约 C-04 / C-05 边界）。 */
const SECTION_LABELS: Record<string, string> = {
  "/home": "首页",
  "/series": "系列",
  "/history": "阅读历史",
  "/dashboard": "驾驶舱",
  "/authors": "作者",
  "/admin": "书籍管理",
  "/import": "导入管理",
};

/** /series/长夜 也归到「系列」这一段。 */
function labelOf(path: string): string | undefined {
  const key = Object.keys(SECTION_LABELS).find((prefix) => path === prefix || path.startsWith(`${prefix}/`));
  return key ? SECTION_LABELS[key] : undefined;
}

/**
 * 错档落点提示文案（契约 C-04 / C-05 边界）：落点是重定向，要让用户知道为什么被换了页。
 * 未知路径返回 null——不硬造提示。
 */
export function tierFallbackText(from: string | null | undefined): string | null {
  const label = from ? labelOf(from) : undefined;
  if (!label) return null;
  if (from === "/home") return `「${label}」只在窄屏提供，已回到书架。`;
  return `「${label}」在窄屏不提供，已回到首页。`;
}
