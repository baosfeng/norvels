import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ImportReview, ImportReviews, ReviewQuality } from "../../api/types";

const reviewState = {
  data: { items: [] as ImportReview[], aiEnabled: true } as ImportReviews,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const decideImportReview = vi.fn<(id: number, action: "replace" | "keep") => Promise<ImportReview>>();
const rollbackImportReview = vi.fn<(id: number) => Promise<ImportReview>>();

vi.mock("../../api/hooks", () => ({ useImportReviews: () => reviewState }));
vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { decideImportReview, rollbackImportReview } };
});

const { ReviewPanel } = await import("./ReviewPanel");
const { ApiError } = await import("../../api/client");

/** 一份算过的质量分；测试里改字段就在这上面覆盖。 */
function quality(overrides: Partial<ReviewQuality> = {}): ReviewQuality {
  return {
    garbageRatio: 0.0002,
    noiseRatio: 0.0399,
    structureScore: 0.4281,
    duplicateRatio: 0.9463,
    chapters: 24,
    chars: 25686,
    computed: true,
    ...overrides,
  };
}

const cleanQuality = quality({ garbageRatio: 0, noiseRatio: 0, structureScore: 0.428, chars: 25680 });

/** 契约文档里的那条 ready 记录（书名与取值都是虚构夹具）。 */
function makeReview(overrides: Partial<ImportReview> = {}): ImportReview {
  return {
    id: 3,
    jobId: 2,
    status: "ready",
    hitRatio: 0.958,
    oldBook: { id: 41, title: "示例旧版", chapterCount: 24, wordCount: 25680, supersededBy: 0, quality: quality() },
    newBook: { id: 57, title: "示例新版", chapterCount: 24, wordCount: 25680, supersededBy: 41, quality: cleanQuality },
    quality: {
      old: quality(),
      new: cleanQuality,
      better: [
        { key: "noise", old: 0.0399, new: 0 },
        { key: "garbage", old: 0.0002, new: 0 },
      ],
      worse: [],
    },
    ai: { winner: "new", confidence: 0.82, reasons: ["错字更少", "广告已清除"] },
    ruleReasons: ["噪声率 4.0%→0.0%", "乱码率 0.016%→0.000%"],
    createdAt: "2026-11-01T05:25:53.786Z",
    decidedAt: null,
    decidedBy: "",
    ...overrides,
  };
}

/** 展开面板：收起是默认态，每个用例都要先点一下。 */
function expand() {
  fireEvent.click(screen.getByTestId("review-toggle"));
}

describe("升级版审核", () => {
  beforeEach(() => {
    reviewState.data = { items: [], aiEnabled: true };
    reviewState.loading = false;
    reviewState.error = null;
    reviewState.reload.mockReset();
    decideImportReview.mockReset();
    rollbackImportReview.mockReset();
  });

  it("入口收起时只露待办数：pending + ready 都算，卡片不在 DOM 里", () => {
    reviewState.data = {
      items: [makeReview({ id: 1, status: "pending" }), makeReview({ id: 2, status: "ready" }), makeReview({ id: 3, status: "confirmed_keep" })],
      aiEnabled: true,
    };
    render(<ReviewPanel />);

    expect(screen.getByTestId("review-todo-count").textContent).toBe("2 条待确认");
    expect(screen.queryByTestId("review-card-1")).toBeNull();
    expect(screen.getByTestId("review-toggle").textContent).toBe("展开");
  });

  it("展开后没有记录时给空态，不是一片空白", () => {
    render(<ReviewPanel />);
    expand();

    expect(screen.getByTestId("review-empty").textContent).toContain("没有待确认的升级版");
  });

  it("ready 卡片：状态、命中率、两版书名与章数字数、质量对比与更优项、AI 理由都在", () => {
    reviewState.data = { items: [makeReview()], aiEnabled: true };
    render(<ReviewPanel />);
    expand();

    const card = screen.getByTestId("review-card-3");
    expect(screen.getByTestId("review-status-3").textContent).toBe("AI 已给结论");
    expect(screen.getByTestId("review-hit-3").textContent).toContain("95.8%");
    expect(screen.getByTestId("review-old-3").textContent).toContain("示例旧版");
    expect(screen.getByTestId("review-old-3").textContent).toContain("24 章");
    expect(screen.getByTestId("review-old-3").textContent).toContain("25,680 字");
    expect(screen.getByTestId("review-new-3").textContent).toContain("示例新版");
    // 两版都在库里：旧版是在用的当前版本，新版入库即隐藏
    expect(screen.getByTestId("review-old-3").textContent).toContain("在用");
    expect(screen.getByTestId("review-new-3").textContent).toContain("隐藏中");

    // 质量对比：噪声率 3.99% → 0（一位小数），标「更优」；结构分按分数展示
    const noiseRow = screen.getByTestId("review-dim-3-noise");
    expect(noiseRow.textContent).toContain("4%");
    expect(noiseRow.textContent).toContain("0%");
    expect(noiseRow.textContent).toContain("更优");
    expect(screen.getByTestId("review-dim-3-garbage").textContent).toContain("更优");
    expect(screen.getByTestId("review-dim-3-structure").textContent).toContain("0.4281");

    const ai = screen.getByTestId("review-ai-3");
    expect(ai.textContent).toContain("建议换成这一版");
    expect(ai.textContent).toContain("82%");
    expect(ai.textContent).toContain("错字更少");
    expect(ai.textContent).toContain("广告已清除");
    expect(card.textContent).toContain("创建于");

    // ready 有 AI 理由时不重复展示规则理由
    expect(screen.queryByTestId("review-rule-reasons-3")).toBeNull();
  });

  it("质量分没算过时显示「未计算」，不当成 0 分", () => {
    const review = makeReview();
    reviewState.data = {
      items: [makeReview({ quality: { ...review.quality, old: quality({ computed: false }), note: "旧版没有质量分，按保守口径处理" } })],
      aiEnabled: true,
    };
    render(<ReviewPanel />);
    expand();

    expect(screen.getByTestId("review-quality-3").textContent).toContain("未计算");
    expect(screen.getByTestId("review-quality-note-3").textContent).toContain("保守口径");
  });

  it("pending 不可操作：按 AI 开关分别说「等待 AI」与「等待人工」，没有替换 / 保留按钮", () => {
    reviewState.data = { items: [makeReview({ id: 4, status: "pending" })], aiEnabled: true };
    const first = render(<ReviewPanel />);
    expand();

    expect(screen.getByTestId("review-status-4").textContent).toBe("等待 AI");
    expect(screen.getByTestId("review-waiting-4").textContent).toContain("等待 AI 裁决");
    expect(screen.queryByTestId("review-replace-4")).toBeNull();
    expect(screen.queryByTestId("review-keep-4")).toBeNull();
    first.unmount();

    reviewState.data = { items: [makeReview({ id: 4, status: "pending" })], aiEnabled: false };
    render(<ReviewPanel />);
    expand();

    expect(screen.getByTestId("review-waiting-4").textContent).toContain("等待人工处理");
    expect(screen.getByTestId("review-ai-off").textContent).toContain("AI 未启用");
  });

  it("needs_manual 显示规则理由，并给人工决策的两个按钮", () => {
    reviewState.data = {
      items: [
        makeReview({
          id: 5,
          status: "needs_manual",
          ai: { winner: "", confidence: 0, reasons: ["两次裁决结论不一致"] },
        }),
      ],
      aiEnabled: true,
    };
    render(<ReviewPanel />);
    expand();

    expect(screen.getByTestId("review-status-5").textContent).toBe("需要你判断");
    expect(screen.getByTestId("review-ai-5").textContent).toContain("需要你判断");
    expect(screen.getByTestId("review-ai-5").textContent).toContain("两次裁决结论不一致");
    expect(screen.getByTestId("review-rule-reasons-5").textContent).toContain("噪声率 4.0%→0.0%");
    expect(screen.getByTestId("review-replace-5")).toBeTruthy();
    expect(screen.getByTestId("review-keep-5")).toBeTruthy();
  });

  it("替换要二次确认：确认后调 decide，卡片就地变成「已替换」并给出回滚", async () => {
    reviewState.data = { items: [makeReview()], aiEnabled: true };
    decideImportReview.mockResolvedValueOnce({
      ...makeReview(),
      status: "confirmed_replace",
      oldBook: { ...makeReview().oldBook, supersededBy: 57 },
      newBook: { ...makeReview().newBook, supersededBy: 0 },
      decidedAt: "2026-11-01T06:00:00.000Z",
      decidedBy: "user:1",
    });
    render(<ReviewPanel />);
    expand();

    fireEvent.click(screen.getByTestId("review-replace-3"));
    // 决策前先弹确认，不直接发请求
    expect(screen.getByTestId("review-confirm")).toBeTruthy();
    expect(decideImportReview).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("review-confirm-submit"));

    await waitFor(() => expect(decideImportReview).toHaveBeenCalledWith(3, "replace"));
    await waitFor(() => expect(screen.getByTestId("review-status-3").textContent).toBe("已替换"));
    expect(screen.getByTestId("review-note").textContent).toContain("已替换为「示例新版」");
    expect(screen.getByTestId("review-rollback-3")).toBeTruthy();
    expect(screen.queryByTestId("review-replace-3")).toBeNull();
    expect(screen.getByTestId("review-new-3").textContent).toContain("在用");
  });

  it("保留当前版本：动作是 keep，状态变成已保留且没有回滚入口", async () => {
    reviewState.data = { items: [makeReview()], aiEnabled: true };
    decideImportReview.mockResolvedValueOnce({ ...makeReview(), status: "confirmed_keep", decidedAt: "2026-11-01T06:00:00.000Z" });
    render(<ReviewPanel />);
    expand();

    fireEvent.click(screen.getByTestId("review-keep-3"));
    expect(screen.getByTestId("review-confirm-text").textContent).toContain("最终决定");
    fireEvent.click(screen.getByTestId("review-confirm-submit"));

    await waitFor(() => expect(decideImportReview).toHaveBeenCalledWith(3, "keep"));
    await waitFor(() => expect(screen.getByTestId("review-status-3").textContent).toBe("已保留"));
    expect(screen.queryByTestId("review-rollback-3")).toBeNull();
    expect(screen.queryByTestId("review-replace-3")).toBeNull();
  });

  it("回滚只在 confirmed_replace 上出现；确认后调 rollback，状态变已回滚", async () => {
    const replaced = makeReview({
      id: 6,
      status: "confirmed_replace",
      oldBook: { ...makeReview().oldBook, supersededBy: 57 },
      newBook: { ...makeReview().newBook, supersededBy: 0 },
      decidedAt: "2026-11-01T06:00:00.000Z",
    });
    reviewState.data = {
      items: [replaced, makeReview({ id: 7, status: "ready" }), makeReview({ id: 8, status: "confirmed_keep" })],
      aiEnabled: true,
    };
    rollbackImportReview.mockResolvedValueOnce({ ...replaced, status: "rolled_back", oldBook: { ...replaced.oldBook, supersededBy: 0 } });
    render(<ReviewPanel />);
    expand();

    expect(screen.getByTestId("review-rollback-6")).toBeTruthy();
    expect(screen.queryByTestId("review-rollback-7")).toBeNull();
    expect(screen.queryByTestId("review-rollback-8")).toBeNull();

    fireEvent.click(screen.getByTestId("review-rollback-6"));
    expect(screen.getByTestId("review-confirm-title").textContent).toContain("回滚到");
    fireEvent.click(screen.getByTestId("review-confirm-submit"));

    await waitFor(() => expect(rollbackImportReview).toHaveBeenCalledWith(6));
    await waitFor(() => expect(screen.getByTestId("review-status-6").textContent).toBe("已回滚"));
    expect(screen.getByTestId("review-note").textContent).toContain("已回滚");
  });

  it("决策被状态机拒绝（409）时把冲突翻成人话并重新拉取列表", async () => {
    reviewState.data = { items: [makeReview()], aiEnabled: true };
    decideImportReview.mockRejectedValueOnce(new ApiError("http", "review_state", 409, "review_state"));
    render(<ReviewPanel />);
    expand();

    fireEvent.click(screen.getByTestId("review-replace-3"));
    fireEvent.click(screen.getByTestId("review-confirm-submit"));

    await waitFor(() => expect(screen.getByTestId("review-note").textContent).toContain("刚刚已经变过"));
    expect(reviewState.reload).toHaveBeenCalled();
    expect(screen.queryByTestId("review-confirm")).toBeNull();
  });

  it("取消确认不发请求，卡片保持原状", () => {
    reviewState.data = { items: [makeReview()], aiEnabled: true };
    render(<ReviewPanel />);
    expand();

    fireEvent.click(screen.getByTestId("review-replace-3"));
    fireEvent.click(screen.getByTestId("review-confirm-cancel"));

    expect(screen.queryByTestId("review-confirm")).toBeNull();
    expect(decideImportReview).not.toHaveBeenCalled();
    expect(screen.getByTestId("review-status-3").textContent).toBe("AI 已给结论");
  });

  it("加载态与错误态各有面板，错误态可重试", () => {
    reviewState.loading = true;
    const first = render(<ReviewPanel />);
    expand();
    expect(screen.getByTestId("review-loading")).toBeTruthy();
    first.unmount();

    reviewState.loading = false;
    reviewState.error = new ApiError("network", "boom");
    render(<ReviewPanel />);
    expand();

    expect(screen.getByTestId("review-error")).toBeTruthy();
    fireEvent.click(screen.getByText("重试"));
    expect(reviewState.reload).toHaveBeenCalled();
  });
});
