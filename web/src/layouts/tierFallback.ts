/**
 * 错档落点提示的文案（契约 C-04 / C-05 边界）。
 *
 * 只有「被换页」才需要解释，所以这里列的是**真的会跨档重定向**的路径：
 * - PC 专属（H5 访问会被送回首页）：导入 / 作者 / 驾驶舱 / 书籍管理
 * - H5 专属（PC 访问会被送回书架）：首页
 *
 * 系列与阅读历史两档都开，不在名单里——它们不再产生重定向，也就不该有提示。
 */
const SECTION_LABELS: Record<string, string> = {
  "/home": "首页",
  "/dashboard": "驾驶舱",
  "/authors": "作者",
  "/admin": "书籍管理",
  "/import": "导入管理",
};

/** /authors/12 也归到「作者」这一段。 */
function labelOf(path: string): string | undefined {
  const key = Object.keys(SECTION_LABELS).find((prefix) => path === prefix || path.startsWith(`${prefix}/`));
  return key ? SECTION_LABELS[key] : undefined;
}

/** 错档落点提示文案：落点是重定向，要让用户知道为什么被换了页。未知路径返回 null——不硬造提示。 */
export function tierFallbackText(from: string | null | undefined): string | null {
  const label = from ? labelOf(from) : undefined;
  if (!label) return null;
  if (from === "/home") return `「${label}」只在窄屏提供，已回到书架。`;
  return `「${label}」在窄屏不提供，已回到首页。`;
}
