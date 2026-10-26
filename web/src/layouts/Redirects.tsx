import { Navigate, useNavigate } from "react-router-dom";
import { useTier } from "../lib/tier";
import { EmptyPanel } from "../components/StatePanel";
import { showToast } from "../lib/toastStore";

/** 首屏落点随档位不同：H5 是「继续阅读」，PC 是书架。 */
export function TierHomeRedirect() {
  const tier = useTier();
  return <Navigate to={tier === "h5" ? "/home" : "/shelf"} replace />;
}

export function LandingRedirect() {
  return <Navigate to="/shelf" replace />;
}

/** 404 出口：两档都回书架第 1 页，浮出结果提示，焦点交给书架页标题。 */
export function NotFoundPage() {
  const navigate = useNavigate();
  function backToShelf() {
    showToast("已回到书架。");
    window.scrollTo(0, 0);
    navigate("/shelf", { state: { focusTitle: true } });
  }
  return (
    <EmptyPanel
      title="这个页面不存在"
      description="链接可能过期了，回到书架重新找一本。"
      actions={
        <button className="btn btn-primary" type="button" data-od-id="C-18" onClick={backToShelf}>
          回书架
        </button>
      }
    />
  );
}
