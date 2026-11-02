import { Link } from "react-router-dom";
import { useVersion } from "../api/hooks";
import { versionLabel } from "./versionText";

/**
 * 侧栏页脚的版本位（PC 常驻 / H5 抽屉两处共用同一份 DOM）。
 *
 * 为什么必须在这里：用户升级到新版本后，除了翻更新日志没有任何地方能确认
 * 「我现在跑的是哪个版本」——验收时全站含版本号的叶子节点是 0。
 *
 * 三条口径：
 * - **只读既有 `useVersion`**（GET /api/version），不为一个页脚新增接口；
 * - 点它跳 `/changelog`：版本号是「这版改了什么」的入口，不是一行死文字；
 * - 加载中与失败**不渲染**（失败时给不出一行可信的版本号）。`dev` 构建照实说「开发构建」，
 *   不补成 v0.0.0——宁可说清楚也不编一个版本号。
 */
export function SideVersion({ onNavigate }: { onNavigate?: () => void }) {
  const version = useVersion();
  const label = versionLabel(version.data?.version, version.loading, version.error !== null);
  if (label === null) return null;

  return (
    <Link
      to="/changelog"
      className="side-version meta"
      onClick={onNavigate}
      data-testid="side-version"
      title="查看更新日志"
    >
      {label}
    </Link>
  );
}
