import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RecommendItem, RecommendResult } from "../../api/types";

/** 场景推荐位：取数与埋点都换成桩，只断言渲染结果、跳转目标与 track 的 payload。 */
const track = vi.hoisted(() => vi.fn());
vi.mock("../../lib/telemetry", () => ({ track, useImpression: () => undefined }));

const h = vi.hoisted(() => ({
  state: {
    data: null as RecommendResult | null,
    loading: false,
    error: null as unknown,
    reload: vi.fn(),
  },
  limit: 0,
  tab: "",
  enabled: true,
}));

vi.mock("../../api/hooks", () => ({
  useRecommendations: (limit: number, tab: string, options: { enabled?: boolean } = {}) => {
    h.limit = limit;
    h.tab = tab;
    h.enabled = options.enabled ?? true;
    return h.state;
  },
}));

const { SceneRecommendBlock, SCENE_RECOMMEND_LIMIT } = await import("./SceneRecommendBlock");

function item(bookId: number, overrides: Partial<RecommendItem> = {}): RecommendItem {
  return {
    bookId,
    title: `样书${bookId}`,
    authorId: 100 + bookId,
    authorName: `作者${bookId}`,
    categoryL1: "武侠",
    categoryL2: "江湖",
    hasCover: false,
    reason: "因为你常看武侠",
    reasonCode: "interest:武侠",
    score: 1,
    ...overrides,
  };
}

const feed = (items: RecommendItem[] | null, over: Partial<typeof h.state> = {}) => ({
  data: items === null ? null : { items, generatedAt: "2026-11-01T10:00:00Z" },
  loading: over.loading ?? false,
  error: over.error ?? null,
  reload: over.reload ?? vi.fn(),
});

function renderBlock(scene: "home" | "shelf" | "search-empty" | "book" = "home") {
  return render(
    <MemoryRouter>
      <SceneRecommendBlock scene={scene} />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  track.mockReset();
  h.limit = 0;
  h.tab = "";
  h.enabled = true;
  h.state = feed([item(1), item(2), item(3)]);
});

describe("场景推荐位：有数据", () => {
  it("按场景取数（for_you），标题与来源说明都写出来", () => {
    renderBlock("home");
    expect(h.tab).toBe("for_you");
    expect(h.limit).toBe(SCENE_RECOMMEND_LIMIT);
    const block = screen.getByTestId("scene-rec:home");
    expect(block.getAttribute("data-scene")).toBe("home");
    expect(block.textContent).toContain("今晚还可以看这些");
    // 用户要能看出推荐来源：来源说明与每条理由分别出现在 DOM 里
    expect(screen.getByTestId("scene-rec:home:note").textContent).toContain("推荐来源：");
    expect(block.querySelectorAll(".scene-rec-card")).toHaveLength(3);
    expect(screen.getByTestId("scene-rec-item-1").textContent).toContain("因为你常看武侠");
  });

  it("书架场景用自己的文案，且仍写明来源", () => {
    renderBlock("shelf");
    expect(screen.getByTestId("scene-rec:shelf").textContent).toContain("书架之外");
    expect(screen.getByTestId("scene-rec:shelf:note").textContent).toContain("推荐来源：");
  });

  it("点一张卡：进详情并记 click（书 / 作者 / 分类 / 理由 code）", () => {
    renderBlock("home");
    const link = screen.getByTestId("scene-rec-item-2").querySelector("a");
    expect(link?.getAttribute("href")).toBe("/book/2");
    fireEvent.click(link as HTMLAnchorElement);
    expect(track).toHaveBeenCalledWith("click", {
      bookId: 2,
      categoryL1: "武侠",
      categoryL2: "江湖",
      extra: "interest:武侠",
    });
    expect(link?.getAttribute("state")).toBeNull();
  });

  it("limit 可覆盖默认值（搜索空态给得更少时）", () => {
    render(
      <MemoryRouter>
        <SceneRecommendBlock scene="search-empty" limit={3} />
      </MemoryRouter>,
    );
    expect(h.limit).toBe(3);
    expect(screen.getByTestId("scene-rec:search-empty")).toBeTruthy();
  });
});

describe("场景推荐位：空 / 失败 / 加载三态", () => {
  it("没有推荐时整块不渲染（不占位、不留标题）", () => {
    h.state = feed([]);
    const { container } = renderBlock("home");
    expect(container.querySelector('[data-testid="scene-rec:home"]')).toBeNull();
    expect(container.textContent).toBe("");
  });

  it("失败静默降级：不渲染错误面板，页面其余部分不受影响", () => {
    h.state = feed(null, { error: { kind: "network" } });
    const { container } = renderBlock("home");
    expect(container.querySelector('[data-testid="scene-rec:home"]')).toBeNull();
    expect(container.textContent).not.toContain("重试");
    expect(container.textContent).toBe("");
  });

  it("加载中给骨架（形状与真卡一致），标题与来源说明已经就位", () => {
    h.state = feed(null, { loading: true });
    renderBlock("home");
    expect(screen.getByTestId("scene-rec:home:loading")).toBeTruthy();
    expect(screen.getByTestId("scene-rec-skeleton")).toBeTruthy();
    expect(screen.queryByTestId("scene-rec:home")).toBeNull();
  });

  it("book 场景不发请求（详情页相似书有自己的口径与接口参数）", () => {
    const { container } = renderBlock("book");
    expect(h.enabled).toBe(false);
    expect(container.textContent).toBe("");
  });
});
