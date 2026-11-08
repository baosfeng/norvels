import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { installTelemetry } from "./lib/telemetry";
import { registerServiceWorker } from "./lib/pwa";
import "./styles/globals.css";

// 埋点的 pagehide / 隐藏监听只在这里挂一次：放在 App 之外，因为「关标签」时
// React 树已经不保证还能跑到卸载钩子，能救回来的只有 window 级监听 + sendBeacon。
installTelemetry();
registerServiceWorker();

const container = document.getElementById("root");
if (!container) throw new Error("缺少挂载节点 #root");

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
