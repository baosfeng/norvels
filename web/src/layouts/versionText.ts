/**
 * 侧栏页脚版本位的文案口径（纯函数：页面与测试共用）。
 *
 * 与组件分开成两个文件，是为了让 react-refresh 的「一个文件只导组件」保持干净——
 * 同目录已有 Sidebar / SideAccount 这类「组件 + 纯函数」混排的历史文件，不必再添一个。
 */

/**
 * 版本位要显示什么：加载中、失败、拿不到版本号时返回 null（调用方整行不渲染）。
 *
 * `dev` 构建单独说明：它没有真实版本号，写 `vdev` 会被当成一个版本号。
 */
export function versionLabel(version: string | undefined, loading: boolean, failed: boolean): string | null {
  if (loading || failed || !version) return null;
  return version === "dev" ? "开发构建" : `v${version}`;
}
