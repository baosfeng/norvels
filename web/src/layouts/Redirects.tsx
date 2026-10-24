import { Navigate } from "react-router-dom";
import { useTier } from "../lib/tier";
import { EmptyPanel } from "../components/StatePanel";

/** 首屏落点随档位不同：H5 是「继续阅读」，PC 是书架。 */
export function TierHomeRedirect() {
  const tier = useTier();
  return <Navigate to={tier === "h5" ? "/home" : "/shelf"} replace />;
}

export function LandingRedirect() {
  return <Navigate to="/shelf" replace />;
}

export function NotFoundPage() {
  return (
    <EmptyPanel
      title="这个页面不存在"
      description="链接可能过期了，回到书架重新找一本。"
      actions={
        <a className="btn btn-primary" href="/shelf">
          回书架
        </a>
      }
    />
  );
}
