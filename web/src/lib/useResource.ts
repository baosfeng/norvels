import { useCallback, useEffect, useState } from "react";
import { ApiError } from "../api/client";

/** 每个数据区四种状态：加载 / 空 / 错误 / 有数据。空态由调用方按 data 判，这里只给 loading/error。 */
export type ResourceState<T> = {
  data: T | null;
  loading: boolean;
  error: ApiError | null;
  reload: () => void;
};

type Snapshot<T> = {
  key: string;
  data: T | null;
  loading: boolean;
  error: ApiError | null;
};

/**
 * 通用取数：自动取消过期请求（卸载或依赖变化时 abort），
 * 只有最新一次请求的结果会被采纳（过期判断用请求序号，不靠时间戳）。
 *
 * 状态重置发生在渲染期（React 官方的「派生 state」写法），不在 effect 体内同步 setState ——
 * 避免级联渲染，也让 lint 的 react-hooks/set-state-in-effect 保持干净。
 */
export function useResource<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
  deps: readonly unknown[],
  options: { enabled?: boolean } = {},
): ResourceState<T> {
  const enabled = options.enabled ?? true;
  const key = JSON.stringify([deps, enabled]);
  const [nonce, setNonce] = useState(0);
  const [snapshot, setSnapshot] = useState<Snapshot<T>>(() => ({
    key: "",
    data: null,
    loading: enabled,
    error: null,
  }));

  // 依赖变化 → 在本次渲染内重置为加载态（不触发额外渲染轮次）
  if (snapshot.key !== key) {
    setSnapshot({ key, data: null, loading: enabled, error: null });
  }

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    let active = true;

    fetcher(controller.signal)
      .then((value) => {
        if (!active) return;
        setSnapshot({ key, data: value, loading: false, error: null });
      })
      .catch((err: unknown) => {
        if (!active) return;
        if (err instanceof ApiError && err.kind === "aborted") return;
        setSnapshot({
          key,
          data: null,
          loading: false,
          error:
            err instanceof ApiError
              ? err
              : new ApiError("network", err instanceof Error ? err.message : "请求失败"),
        });
      });

    return () => {
      active = false;
      controller.abort();
    };
    // fetcher 的身份由 key（= deps 的序列化）决定：key 不变则语义相同，无需进依赖
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, enabled, nonce]);

  const current = snapshot.key === key ? snapshot : { key, data: null, loading: enabled, error: null };
  return { data: current.data, loading: current.loading, error: current.error, reload };
}