import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TagRule } from "../../api/types";

const rulesState = {
  data: { rules: [] as TagRule[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const putTagRules = vi.fn<(rules: TagRule[]) => Promise<void>>();

vi.mock("../../api/hooks", () => ({ useTagRules: () => rulesState }));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { putTagRules } };
});

const { TagRulesPanel } = await import("./TagRulesPanel");
const { ApiError } = await import("../../api/client");

function typeInto(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

describe("词表规则面板", () => {
  beforeEach(() => {
    rulesState.data = { rules: [] };
    rulesState.loading = false;
    rulesState.error = null;
    rulesState.reload.mockReset();
    putTagRules.mockReset();
  });

  it("没配置时给空态与新增入口，不是一片空白", () => {
    render(<TagRulesPanel />);

    expect(screen.getByTestId("tag-rules-empty")).toBeTruthy();
    expect(screen.getByTestId("tag-rule-add")).toBeTruthy();
    expect(screen.getByTestId("tag-rules-save")).toBeTruthy();
  });

  it("新增一条规则并保存：关键词按逗号拆开、去重后提交", async () => {
    putTagRules.mockResolvedValueOnce(undefined);
    render(<TagRulesPanel />);

    fireEvent.click(screen.getByTestId("tag-rule-add"));
    typeInto("规则 1 · 标签名", "奇幻");
    typeInto("规则 1 · 关键词（逗号分隔）", "魔法，精灵, 魔法");
    fireEvent.click(screen.getByTestId("tag-rules-save"));

    await waitFor(() => expect(putTagRules).toHaveBeenCalledTimes(1));
    expect(putTagRules.mock.calls[0][0]).toEqual([{ tag: "奇幻", keywords: ["魔法", "精灵"] }]);
    expect(rulesState.reload).toHaveBeenCalled();
  });

  it("空标签 / 标签重名 / 关键词超长都在前端拦下，不发请求", () => {
    render(<TagRulesPanel />);

    fireEvent.click(screen.getByTestId("tag-rule-add"));
    fireEvent.click(screen.getByTestId("tag-rules-save"));
    expect(screen.getByTestId("tag-rules-error-text").textContent).toContain("缺少标签名");

    typeInto("规则 1 · 标签名", "奇幻");
    typeInto("规则 1 · 关键词（逗号分隔）", "魔法");
    fireEvent.click(screen.getByTestId("tag-rule-add"));
    typeInto("规则 2 · 标签名", "奇幻");
    typeInto("规则 2 · 关键词（逗号分隔）", "精灵");
    fireEvent.click(screen.getByTestId("tag-rules-save"));
    expect(screen.getByTestId("tag-rules-error-text").textContent).toContain("出现了两次");

    typeInto("规则 2 · 标签名", "玄幻");
    typeInto("规则 2 · 关键词（逗号分隔）", "一二三四五六七八九十一二三四五六七八九十一");
    fireEvent.click(screen.getByTestId("tag-rules-save"));
    expect(screen.getByTestId("tag-rules-error-text").textContent).toContain("超过 20 字");

    expect(putTagRules).not.toHaveBeenCalled();
  });

  it("后端 400 时把原因说成人话，规则留在编辑区可改", async () => {
    putTagRules.mockRejectedValueOnce(new ApiError("http", "invalid_param", 400, "invalid_param"));
    render(<TagRulesPanel />);

    fireEvent.click(screen.getByTestId("tag-rule-add"));
    typeInto("规则 1 · 标签名", "奇幻");
    typeInto("规则 1 · 关键词（逗号分隔）", "魔法");
    fireEvent.click(screen.getByTestId("tag-rules-save"));

    await waitFor(() =>
      expect(screen.getByTestId("tag-rules-error-text").textContent).toContain("没通过后端校验"),
    );
    expect(screen.getByLabelText("规则 1 · 标签名")).toBeTruthy();
  });

  it("已配置时回填；读不到时给错误面板与重试", () => {
    rulesState.data = {
      rules: [{ tag: "奇幻", keywords: ["魔法", "精灵"] }],
    };
    const first = render(<TagRulesPanel />);
    expect((screen.getByLabelText("规则 1 · 标签名") as HTMLInputElement).value).toBe("奇幻");
    expect((screen.getByLabelText("规则 1 · 关键词（逗号分隔）") as HTMLInputElement).value).toBe("魔法，精灵");
    first.unmount();

    rulesState.loading = false;
    rulesState.error = new ApiError("http", "boom", 500);
    rulesState.data = { rules: [] };
    render(<TagRulesPanel />);
    expect(screen.getByTestId("tag-rules-error")).toBeTruthy();
  });
});
