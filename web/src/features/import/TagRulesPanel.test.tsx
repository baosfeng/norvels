import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Capabilities, TagRule } from "../../api/types";

const rulesState = {
  data: { rules: [] as TagRule[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

/** 后端默认能力清单：词表上限 50 / 100 / 20 / 20（测试夹具，不是前端副本）。 */
const defaultCaps: Capabilities = {
  splitters: ["di-unit"],
  codecs: ["utf-8"],
  filters: [{ name: "bom", priority: 10 }],
  metas: [{ name: "filename", weight: 80 }],
  extensions: ["txt", "epub", "zip"],
  limits: {
    chapterRules: { maxCount: 20, maxPatternRunes: 200, maxNameRunes: 60 },
    tagRules: { maxCount: 50, maxTagRunes: 20, maxKeywords: 100, maxKeywordRunes: 20 },
    sourceProfiles: { maxCount: 20, maxRulesPerProfile: 30, maxPatternRunes: 200, maxNameRunes: 60 },
    upload: { maxFileBytes: 64 * 1024 * 1024, maxTotalBytes: 256 * 1024 * 1024, maxFiles: 20 },
    import: { maxWorkers: 32, defaultMaxChapterChars: 30000, defaultFallbackChunkChars: 8000 },
    dryRun: { maxFiles: 200, samples: 20, errors: 50 },
  },
};

const capsState = {
  data: defaultCaps as Capabilities | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const putTagRules = vi.fn<(rules: TagRule[]) => Promise<void>>();

vi.mock("../../api/hooks", () => ({ useTagRules: () => rulesState, useCapabilities: () => capsState }));

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
    capsState.data = defaultCaps;
    capsState.error = null;
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

  it("词表上限读 capabilities：下发 1 条就只让加 1 条", () => {
    capsState.data = {
      ...defaultCaps,
      limits: { ...defaultCaps.limits, tagRules: { maxCount: 1, maxTagRunes: 20, maxKeywords: 100, maxKeywordRunes: 20 } },
    };
    render(<TagRulesPanel />);

    fireEvent.click(screen.getByTestId("tag-rule-add"));
    expect(screen.getAllByTestId("tag-rule-row")).toHaveLength(1);
    expect(screen.getByTestId("tag-rules-full").textContent).toContain("最多 1 条");
    expect((screen.getByTestId("tag-rule-add") as HTMLButtonElement).disabled).toBe(true);
  });

  it("能力清单读不到时不拦条数，并提示服务端仍会校验", () => {
    capsState.error = new ApiError("http", "boom", 500);
    render(<TagRulesPanel />);

    expect(screen.getByTestId("tag-rules-limit-unknown").textContent).toContain("服务端仍会校验");
    fireEvent.click(screen.getByTestId("tag-rule-add"));
    fireEvent.click(screen.getByTestId("tag-rule-add"));
    expect(screen.getAllByTestId("tag-rule-row")).toHaveLength(2);
    expect(screen.queryByTestId("tag-rules-full")).toBeNull();
  });

  it("后端 400 时把原因说成人话，规则留在编辑区可改", async () => {
    putTagRules.mockRejectedValueOnce(new ApiError("http", "invalid_param", 400, "invalid_param"));
    render(<TagRulesPanel />);

    fireEvent.click(screen.getByTestId("tag-rule-add"));
    typeInto("规则 1 · 标签名", "奇幻");
    typeInto("规则 1 · 关键词（逗号分隔）", "魔法");
    fireEvent.click(screen.getByTestId("tag-rules-save"));

    await waitFor(() =>
      expect(screen.getByTestId("tag-rules-error-text").textContent).toContain("词表没通过校验"),
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
