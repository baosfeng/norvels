import { createContext, useContext, useEffect, useState } from "react";

/** H5/PC 分档：≤768 是轻量阅读器，≥1024 是全功能工作台（768–1024 归 PC，保留管理能力）。 */
export type Tier = "pc" | "h5";

export const H5_QUERY = "(max-width: 768px)";

export function tierOf(matches: boolean): Tier {
  return matches ? "h5" : "pc";
}

/** 该路由是否属于管理能力：H5 下不渲染，也直接不承认这个路由。 */
export const MANAGEMENT_ROUTES = ["import", "authors", "dashboard"] as const;

export function isManagementRoute(segment: string): boolean {
  return (MANAGEMENT_ROUTES as readonly string[]).includes(segment);
}

const TierContext = createContext<Tier>("pc");

export function useTierMedia(): Tier {
  const [tier, setTier] = useState<Tier>(() => {
    if (typeof window === "undefined" || !window.matchMedia) return "pc";
    return tierOf(window.matchMedia(H5_QUERY).matches);
  });

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mq = window.matchMedia(H5_QUERY);
    const onChange = (event: MediaQueryListEvent) => setTier(tierOf(event.matches));
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  return tier;
}

export const TierProvider = TierContext.Provider;

/** 唯一的分档读取入口：条件渲染一律走它，不用 CSS 隐藏。 */
export function useTier(): Tier {
  return useContext(TierContext);
}

export function useIsH5(): boolean {
  return useTier() === "h5";
}