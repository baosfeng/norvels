import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { installTelemetry } from "./lib/telemetry";
import { registerServiceWorker } from "./lib/pwa";
import { ensurePersistentStorage } from "./lib/storagePersist";
import "./styles/globals.css";

// 埋点的 pagehide / 隐藏监听只在这里挂一次：放在 App 之外，因为「关标签」时
// React 树已经不保证还能跑到卸载钩子，能救回来的只有 window 级监听 + sendBeacon。
installTelemetry();
registerServiceWorker();

// 申请持久化存储，避免浏览器把离线缓存当临时数据清掉（清了就离线打不开最近 24 章）。
// 挂在 load 之后而不是渲染期：申请要走一次存储检查，首屏已经画完，抢不到关键路径；
// 放在 main 而不是 pwa.ts 的注册分支里，因为不支持 Service Worker 的浏览器同样值得申请。
window.addEventListener("load", () => {
  void ensurePersistentStorage();
});

const container = document.getElementById("root");
if (!container) throw new Error("缺少挂载节点 #root");

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
