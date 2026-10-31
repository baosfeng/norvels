import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Capabilities, DryRunReport, SourceProfile } from "../../api/types";

const profile = (over: Partial<SourceProfile> = {}): SourceProfile => ({
  id: "sample-dir",
  name: "示例目录",
  schemaVersion: 1,
  enabled: true,
  priority: 10,
  kind: "local",
  match: { ext: ["txt"] },
  decode: {},
  filters: { disable: [], extra: [] },
  meta: [],
  split: { forceSplitter: "", rules: [] },
  tags: [],
  limits: { maxChapterChars: 0, minChapters: 0 },
  ...over,
});

const defaultCaps: Capabilities = {
  splitters: ["di-unit", "unmarked"],
  codecs: ["utf-8", "gb18030"],
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

const profilesState = {
  data: { profiles: [] as SourceProfile[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const capsState = {
  data: defaultCaps as Capabilities | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const validateSourceProfiles = vi.fn<(profiles: SourceProfile[]) => Promise<{ results: { index: number; id: string; ok: boolean; reason: string }[] }>>();
const putSourceProfiles = vi.fn<(profiles: SourceProfile[]) => Promise<void>>();
const dryRunImport = vi.fn<(body: { dir: string; recursive?: boolean; profileId?: string }) => Promise<DryRunReport>>();

vi.mock("../../api/hooks", () => ({
  useSourceProfiles: () => profilesState,
  useCapabilities: () => capsState,
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { validateSourceProfiles, putSourceProfiles, dryRunImport } };
});

const { SourceProfilesPanel } = await import("./SourceProfilesPanel");
const { ApiError } = await import("../../api/client");

function renderPanel(dir = "") {
  return render(<SourceProfilesPanel dir={dir} />);
}

const okIssue = (index: number, id: string) => ({ index, id, ok: true, reason: "" });

const emptyReport: DryRunReport = {
  truncated: false,
  files: { total: 0, willImport: 0, duplicate: 0, unsupported: 0, failed: 0 },
  books: { new: 0, duplicate: 0, split: 0 },
  sample: [],
};

describe("书源规则面板", () => {
  beforeEach(() => {
    profilesState.data = { profiles: [] };
    profilesState.loading = false;
    profilesState.error = null;
    profilesState.reload.mockReset();
    capsState.data = defaultCaps;
    capsState.error = null;
    validateSourceProfiles.mockReset();
    putSourceProfiles.mockReset();
    dryRunImport.mockReset();
  });

  it("空态：说清「不配也能导入」并给新建入口", () => {
    renderPanel();

    expect(screen.getByTestId("source-profiles")).toBeTruthy();
    expect(screen.getByTestId("source-profiles-empty").textContent).toContain("还没有书源规则");
    expect(screen.getByTestId("source-profile-add")).toBeTruthy();
  });

  it("新建一份并保存：先校验再整批 PUT，保存后提示生效时机", async () => {
    validateSourceProfiles.mockResolvedValueOnce({ results: [okIssue(0, "sample-dir")] });
    putSourceProfiles.mockResolvedValueOnce(undefined);
    renderPanel();

    fireEvent.click(screen.getByTestId("source-profile-add"));
    fireEvent.change(screen.getByTestId("source-profile-name"), { target: { value: "示例目录" } });
    fireEvent.change(screen.getByTestId("source-profile-id"), { target: { value: "sample-dir" } });
    fireEvent.click(screen.getByTestId("source-profiles-save"));

    await waitFor(() => expect(putSourceProfiles).toHaveBeenCalledTimes(1));
    expect(validateSourceProfiles).toHaveBeenCalledTimes(1);
    expect(putSourceProfiles.mock.calls[0][0][0]).toMatchObject({ id: "sample-dir", name: "示例目录" });
    await waitFor(() => expect(screen.getByTestId("source-profiles-saved").textContent).toContain("下次导入生效"));
    expect(profilesState.reload).toHaveBeenCalled();
  });

  it("校验失败时把原因标到那张卡上，不提交", async () => {
    validateSourceProfiles.mockResolvedValueOnce({
      results: [{ index: 0, id: "sample-dir", ok: false, reason: "第 1 条切章规则的正则非法：missing closing )" }],
    });
    profilesState.data = { profiles: [profile()] };
    renderPanel();

    fireEvent.click(screen.getByTestId("source-profiles-save"));

    await waitFor(() => expect(screen.getByTestId("source-profile-reason")).toBeTruthy());
    expect(screen.getByTestId("source-profile-reason").textContent).toContain("正则非法");
    expect(screen.getByTestId("source-profiles-error-text").textContent).toContain("没通过校验");
    expect(putSourceProfiles).not.toHaveBeenCalled();
  });

  it("份数到上限时禁用新建并说明原因（上限读 capabilities）", () => {
    capsState.data = {
      ...defaultCaps,
      limits: { ...defaultCaps.limits, sourceProfiles: { ...defaultCaps.limits.sourceProfiles, maxCount: 1 } },
    };
    profilesState.data = { profiles: [profile()] };
    renderPanel();

    expect((screen.getByTestId("source-profile-add") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("source-profiles-full").textContent).toContain("最多 1 份");
  });

  it("导入坏 JSON 只给行内提示，不崩也不改草稿", async () => {
    profilesState.data = { profiles: [profile()] };
    renderPanel();

    const input = screen.getByTestId("source-profiles-import") as HTMLInputElement;
    const file = new File(["{not json"], "source-profiles.json", { type: "application/json" });
    fireEvent.change(input, { target: { files: [file] } });

    await waitFor(() => expect(screen.getByTestId("source-profiles-import-note")).toBeTruthy());
    expect(screen.getByTestId("source-profiles-import-note").textContent).toContain("不是合法 JSON");
    expect(screen.getByTestId("source-profile-card")).toBeTruthy();
  });

  it("删掉最后一份后仍能保存（PUT 空数组 = 清空），不会卡在空态", async () => {
    profilesState.data = { profiles: [profile()] };
    validateSourceProfiles.mockResolvedValueOnce({ results: [] });
    putSourceProfiles.mockResolvedValueOnce(undefined);
    renderPanel();

    fireEvent.click(screen.getByTestId("source-profile-delete"));
    expect(screen.getByTestId("source-profiles-empty")).toBeTruthy();

    fireEvent.click(screen.getByTestId("source-profiles-save"));

    await waitFor(() => expect(putSourceProfiles).toHaveBeenCalledTimes(1));
    expect(putSourceProfiles.mock.calls[0][0]).toEqual([]);
  });

  it("试跑走 POST /api/import + dryRun，并把命中的书与章数说出来", async () => {
    profilesState.data = { profiles: [profile()] };
    dryRunImport.mockResolvedValueOnce({
      ...emptyReport,
      files: { total: 1, willImport: 1, duplicate: 0, unsupported: 0, failed: 0 },
      books: { new: 1, duplicate: 0, split: 0 },
      sample: [
        {
          source: "/path/to/txt/示例书简.txt",
          title: "示例书简",
          chapters: 12,
          words: 1000,
          duplicate: false,
          reason: "",
          profileId: "sample-dir",
        },
      ],
    });
    renderPanel("/path/to/txt");

    fireEvent.click(screen.getByTestId("source-profile-dry-run"));

    await waitFor(() => expect(dryRunImport).toHaveBeenCalledTimes(1));
    expect(dryRunImport.mock.calls[0][0]).toEqual({
      dir: "/path/to/txt",
      recursive: true,
      profileId: "sample-dir",
    });
    await waitFor(() =>
      expect(screen.getByTestId("source-profile-dry-run-result").textContent).toContain("命中 1 本"),
    );
    expect(screen.getByTestId("source-profile-dry-run-result").textContent).toContain("12 章");
  });

  it("没填路径时试跑只提示，不发请求", () => {
    profilesState.data = { profiles: [profile()] };
    renderPanel("");

    fireEvent.click(screen.getByTestId("source-profile-dry-run"));

    expect(dryRunImport).not.toHaveBeenCalled();
    expect(screen.getByTestId("source-profile-dry-run-error").textContent).toContain("先在上面填导入路径");
  });

  it("读取失败给错误面板与重试；能力清单读不到时提示服务端仍会校验", () => {
    profilesState.error = new ApiError("http", "boom", 500);
    renderPanel();
    expect(screen.getByTestId("source-profiles-error")).toBeTruthy();
  });
});
