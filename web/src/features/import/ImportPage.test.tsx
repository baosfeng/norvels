import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChapterRule, DryRunReport, ImportJob, ImportJobFile, UploadResult } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { TierRoute } from "../../layouts/TierRoute";

/** 页里有「去书籍管理」这类站内链接，渲染需要路由上下文（产品由 App 提供）。 */
function renderPage() {
  return render(
    <MemoryRouter>
      <ImportPage />
    </MemoryRouter>,
  );
}

const jobState = {
  data: { items: [] as ImportJob[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const rulesState = {
  data: { rules: [] as ChapterRule[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const tagRulesState = {
  data: { rules: [] as ChapterRule[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const failedFilesState = {
  data: { files: [] as ImportJobFile[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const uploadImport = vi.fn<(files: File[], opts?: { force?: boolean; update?: boolean }) => Promise<UploadResult>>();
const startImport = vi.fn(async () => ({ jobId: 1 }));
const cancelImport = vi.fn<(id: number) => Promise<{ status: string }>>();
const importJob = vi.fn<(id: number) => Promise<ImportJob>>();
const putChapterRules = vi.fn<(rules: ChapterRule[]) => Promise<void>>();
const dryRunImport = vi.fn<(body: { dir: string }) => Promise<DryRunReport>>();
const retryImport = vi.fn<(jobId: number, sources?: string[]) => Promise<{ jobId: number }>>();

vi.mock("../../api/hooks", () => ({
  useImportJobs: () => jobState,
  useChapterRules: () => rulesState,
  useTagRules: () => tagRulesState,
  useFailedFiles: () => failedFilesState,
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: { uploadImport, startImport, cancelImport, importJob, putChapterRules, dryRunImport, retryImport },
  };
});

const { ImportPage } = await import("./ImportPage");
const { ApiError } = await import("../../api/client");

function pickFiles(...files: File[]) {
  fireEvent.change(screen.getByTestId("upload-input"), { target: { files } });
}

function clickUpload() {
  fireEvent.click(screen.getByTestId("upload-submit"));
}

function sample(name: string, content = "正文") {
  return new File([content], name);
}

const acceptedTxt: UploadResult = {
  jobId: 7,
  accepted: [{ name: "示例书简.txt", format: "txt" }],
  rejected: [],
};

describe("导入页：文件上传与逐文件友好报错", () => {
  beforeEach(() => {
    jobState.data = { items: [] };
    jobState.loading = false;
    jobState.error = null;
    jobState.reload.mockReset();
    failedFilesState.data = { files: [] };
    failedFilesState.loading = false;
    failedFilesState.error = null;
    uploadImport.mockReset();
    startImport.mockReset();
    dryRunImport.mockReset();
    retryImport.mockReset();
  });

  it("空态：没选文件时说清支持什么，上传按钮先不可用", () => {
    renderPage();

    expect(screen.getByTestId("upload-panel")).toBeTruthy();
    expect(screen.getByText("支持 TXT、EPUB、ZIP，可一次选多个")).toBeTruthy();
    expect(screen.getByTestId("upload-empty").textContent).toContain("还没有选文件");
    expect((screen.getByTestId("upload-submit") as HTMLButtonElement).disabled).toBe(true);
    // 没有进行中的任务时给空态，而不是一片空白
    expect(screen.getByTestId("state:empty")).toBeTruthy();
  });

  it("混批选择：合法文件照常上传，pdf 逐个说明原因且不阻断", async () => {
    uploadImport.mockResolvedValueOnce(acceptedTxt);
    renderPage();

    pickFiles(sample("示例书简.txt"), sample("示例书简.pdf"));
    expect(screen.getByTestId("upload-picked").textContent).toContain("示例书简.txt");
    expect((screen.getByTestId("upload-submit") as HTMLButtonElement).textContent).toContain("上传文件（1）");

    clickUpload();

    await waitFor(() => expect(screen.getByTestId("upload-accepted")).toBeTruthy());
    expect(uploadImport).toHaveBeenCalledTimes(1);
    const [files, opts] = uploadImport.mock.calls[0];
    expect(files).toHaveLength(1);
    expect(files[0].name).toBe("示例书简.txt");
    expect(opts).toEqual({ force: false, update: false });

    expect(screen.getByTestId("upload-accepted").textContent).toContain("任务 #7 已排队");
    expect(screen.getByTestId("upload-rejected").textContent).toContain("暂不支持 .pdf，目前支持 TXT、EPUB、ZIP");
    expect(jobState.reload).toHaveBeenCalled();
  });

  it("拖入文件与点选等效，勾选重导后 force 一并提交", async () => {
    uploadImport.mockResolvedValueOnce({ jobId: 8, accepted: [{ name: "示例书简.epub", format: "epub" }], rejected: [] });
    renderPage();

    fireEvent.drop(screen.getByTestId("upload-dropzone"), {
      dataTransfer: { files: [sample("示例书简.epub")] },
    });
    expect(screen.getByTestId("upload-picked").textContent).toContain("示例书简.epub");

    fireEvent.click(screen.getByLabelText("已入库的书也重新导入"));
    clickUpload();

    await waitFor(() => expect(uploadImport).toHaveBeenCalledTimes(1));
    expect(uploadImport.mock.calls[0][1]).toEqual({ force: true, update: false });
  });

  it("zip 预检放行并提示包内逐条导入，结果表按「包名.zip:内部路径」列出条目", async () => {
    uploadImport.mockResolvedValueOnce({
      jobId: 9,
      accepted: [
        { name: "示例合集.zip:第一本.txt", format: "txt" },
        { name: "示例合集.zip:第二本.epub", format: "epub" },
      ],
      rejected: [
        { name: "示例合集.zip:说明.pdf", code: "unsupported_format", message: "暂不支持 .pdf，目前支持 TXT、EPUB、ZIP" },
      ],
    });
    renderPage();

    pickFiles(sample("示例合集.zip"));
    const hint = screen.getByTestId("upload-zip-hint").textContent ?? "";
    expect(hint).toContain("包内 txt/epub 各成一条");
    expect(hint).toContain("包名.zip:内部路径");
    expect((screen.getByTestId("upload-submit") as HTMLButtonElement).textContent).toContain("上传文件（1）");
    expect(screen.queryByTestId("upload-rejected")).toBeNull();

    clickUpload();

    await waitFor(() => expect(screen.getByTestId("upload-accepted")).toBeTruthy());
    const [files] = uploadImport.mock.calls[0];
    expect(files.map((file) => file.name)).toEqual(["示例合集.zip"]);
    const acceptedText = screen.getByTestId("upload-accepted-table").textContent ?? "";
    expect(acceptedText).toContain("示例合集.zip:第一本.txt");
    expect(acceptedText).toContain("示例合集.zip:第二本.epub");
    expect(screen.getByTestId("upload-rejected").textContent).toContain("示例合集.zip:说明.pdf");
  });

  it("勾选追更后 update 随上传提交，不误带 force", async () => {
    uploadImport.mockResolvedValueOnce(acceptedTxt);
    renderPage();

    pickFiles(sample("示例书简.txt"));
    fireEvent.click(screen.getByLabelText("追更（只追加新章节）"));
    expect(screen.getByTestId("upload-update-hint").textContent).toContain("只追加新章节");
    clickUpload();

    await waitFor(() => expect(uploadImport).toHaveBeenCalledTimes(1));
    expect(uploadImport.mock.calls[0][1]).toEqual({ force: false, update: true });
  });

  it("整批都不支持：不发请求，直接说明目前支持什么", () => {
    renderPage();

    pickFiles(sample("示例书简.pdf"));
    clickUpload();

    expect(uploadImport).not.toHaveBeenCalled();
    expect(screen.getByTestId("upload-error").textContent).toContain("目前支持 TXT、EPUB、ZIP");
    expect(screen.getByTestId("upload-rejected").textContent).toContain("暂不支持 .pdf");
  });

  it("单本超过 64 MB 时在上传前挡下并说清上限", () => {
    renderPage();
    const huge = sample("示例书简.epub");
    Object.defineProperty(huge, "size", { value: 65 * 1024 * 1024 });

    pickFiles(huge);
    clickUpload();

    expect(uploadImport).not.toHaveBeenCalled();
    expect(screen.getByTestId("upload-rejected").textContent).toContain("超过 64 MB 上限");
  });

  it("已有任务在跑时把冲突翻成人话", async () => {
    uploadImport.mockRejectedValueOnce(new ApiError("http", "import job already running", 409, "conflict"));
    renderPage();

    pickFiles(sample("示例书简.txt"));
    clickUpload();

    await waitFor(() => expect(screen.getByTestId("upload-error")).toBeTruthy());
    expect(screen.getByTestId("upload-error").textContent).toContain("已经有一个导入任务在跑");
  });

  it("任务列表的加载态与错误态照常呈现，不被上传区吞掉", () => {
    jobState.loading = true;
    const first = renderPage();
    expect(screen.getByTestId("state:loading")).toBeTruthy();
    first.unmount();

    jobState.loading = false;
    jobState.error = new ApiError("http", "boom", 500);
    renderPage();
    expect(screen.getByTestId("state:error")).toBeTruthy();
  });

  it("H5 下导入页不渲染（沿用 TierRoute 口径）", () => {
    render(
      <TierProvider value="h5">
        <MemoryRouter initialEntries={["/import"]}>
          <Routes>
            <Route
              path="/import"
              element={
                <TierRoute>
                  <ImportPage />
                </TierRoute>
              }
            />
            <Route path="/home" element={<p>轻量入口</p>} />
          </Routes>
        </MemoryRouter>
      </TierProvider>,
    );

    expect(screen.queryByTestId("view:import")).toBeNull();
    expect(screen.queryByTestId("upload-panel")).toBeNull();
  });
});

/** 运行中的任务：取消按钮只在它身上出现。 */
const runningJob: ImportJob = {
  id: 42,
  sourceDir: "/path/to/txt",
  status: "running",
  total: 10,
  imported: 3,
  skipped: 1,
  failed: 0,
  startedAt: "2026-10-25T02:00:00.000Z",
  finishedAt: null,
  message: "",
};

describe("导入页：取消进行中的任务", () => {
  beforeEach(() => {
    jobState.data = { items: [runningJob] };
    jobState.loading = false;
    jobState.error = null;
    jobState.reload.mockReset();
    rulesState.data = { rules: [] };
    rulesState.loading = false;
    rulesState.error = null;
    cancelImport.mockReset();
    importJob.mockReset();
  });

  it("运行中任务给出取消入口，点击调取消接口并轮询到 canceled 后给出提示", async () => {
    cancelImport.mockResolvedValueOnce({ status: "canceling" });
    importJob.mockResolvedValueOnce({ ...runningJob, status: "canceled", imported: 3 });
    renderPage();

    // 取消要二次确认：先点「取消任务」，再点弹层里的「确认取消」（契约 C-22）
    fireEvent.click(screen.getByTestId("import-cancel"));
    fireEvent.click(screen.getByTestId("import-cancel-confirm-btn"));

    await waitFor(() => expect(cancelImport).toHaveBeenCalledWith(42));
    // 轮询拿到 canceled 才停：提示里带上是哪一单、保留了多少本
    await waitFor(() => expect(screen.getByTestId("import-cancel-note").textContent).toContain("已取消"));
    expect(importJob).toHaveBeenCalledWith(42);
    expect(screen.getByTestId("import-cancel-note").textContent).toContain("3 本书");
  });

  it("取消失败把冲突翻成人话，不把后端错误甩给用户", async () => {
    cancelImport.mockRejectedValueOnce(new ApiError("http", "conflict", 409, "conflict"));
    renderPage();

    fireEvent.click(screen.getByTestId("import-cancel"));
    fireEvent.click(screen.getByTestId("import-cancel-confirm-btn"));

    await waitFor(() => expect(screen.getByTestId("import-cancel-note").textContent).toContain("已经结束"));
  });

  it("没有运行中的任务时不给取消入口", () => {
    jobState.data = { items: [] };
    renderPage();

    expect(screen.queryByTestId("import-cancel")).toBeNull();
    expect(screen.getByTestId("state:empty")).toBeTruthy();
  });
});

describe("导入页：章节识别规则", () => {
  beforeEach(() => {
    jobState.data = { items: [] };
    jobState.loading = false;
    jobState.error = null;
    jobState.reload.mockReset();
    rulesState.data = { rules: [{ name: "分节符", pattern: "^=== 第\\d+ 节 ===$" }] };
    rulesState.loading = false;
    rulesState.error = null;
    rulesState.reload.mockReset();
    putChapterRules.mockReset();
  });

  it("回填已有规则，编辑后按全量 PUT 提交并说明生效时机", async () => {
    putChapterRules.mockResolvedValueOnce(undefined);
    renderPage();

    const name = screen.getByLabelText("规则 1 · 名称") as HTMLInputElement;
    expect(name.value).toBe("分节符");
    fireEvent.change(name, { target: { value: "分节" } });
    fireEvent.click(screen.getByTestId("rules-save"));

    await waitFor(() => expect(putChapterRules).toHaveBeenCalledTimes(1));
    expect(putChapterRules.mock.calls[0][0]).toEqual([{ name: "分节", pattern: "^=== 第\\d+ 节 ===$" }]);
    await waitFor(() => expect(screen.getByTestId("rules-saved").textContent).toContain("下次导入生效"));
  });

  it("新增与删除都是本地编辑，点保存才一次性提交全量", async () => {
    putChapterRules.mockResolvedValueOnce(undefined);
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "新增一条规则" }));
    const names = screen.getAllByLabelText(/规则 \d+ · 名称/);
    expect(names).toHaveLength(2);
    fireEvent.change(names[1], { target: { value: "卷首" } });
    fireEvent.change(screen.getAllByLabelText("正则")[1], { target: { value: "^卷[一二三]" } });
    expect(putChapterRules).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("rules-save"));

    await waitFor(() => expect(putChapterRules).toHaveBeenCalledTimes(1));
    expect(putChapterRules.mock.calls[0][0]).toEqual([
      { name: "分节符", pattern: "^=== 第\\d+ 节 ===$" },
      { name: "卷首", pattern: "^卷[一二三]" },
    ]);
  });

  it("C-13 新增一条规则：末尾立刻多一行，焦点进名称框（满 20 条另禁用）", () => {
    renderPage();
    expect(screen.getAllByTestId("rule-row")).toHaveLength(1);
    fireEvent.click(screen.getByText("新增一条规则"));
    expect(screen.getAllByTestId("rule-row")).toHaveLength(2);
    expect(document.activeElement).toBe(screen.getByLabelText("规则 2 · 名称"));
  });

  it("C-12 删除这条规则：行数减一、焦点回「新增一条规则」，最后一条也能删", () => {
    renderPage();
    fireEvent.click(screen.getByLabelText("删除第 1 条规则"));
    expect(screen.queryAllByTestId("rule-row")).toHaveLength(0);
    expect(screen.getByTestId("rules-empty").textContent).toContain("还没有自定义规则，导入一律走内置切章");
    expect(document.activeElement).toBe(screen.getByText("新增一条规则"));
  });

  it("非法正则在本地就拦下：括号没闭合与写法不对分开说，都不发请求", () => {
    renderPage();

    // 括号没闭合：契约 C-11 要求单独一句「没闭合」，不要混成「写得不对」
    fireEvent.change(screen.getByLabelText("正则"), { target: { value: "([ " } });
    fireEvent.click(screen.getByTestId("rules-save"));
    expect(screen.getByTestId("rules-error-text").textContent).toContain("正则没闭合");

    // 括号闭合但写法非法：仍走「写得不对」
    fireEvent.change(screen.getByLabelText("正则"), { target: { value: "[z-a]" } });
    fireEvent.click(screen.getByTestId("rules-save"));
    expect(screen.getByTestId("rules-error-text").textContent).toContain("第 1 条规则的正则写得不对");
    expect(putChapterRules).not.toHaveBeenCalled();
  });

  it("试切预览按命中行数报章数，不碰后端", () => {
    rulesState.data = { rules: [{ name: "分节符", pattern: "^第\\d+章" }] };
    renderPage();

    fireEvent.change(screen.getByLabelText("试切预览 · 粘一段正文"), {
      target: { value: "第1章 开端\n正文\n第2章 承接\n第3章 结尾" },
    });
    fireEvent.click(screen.getByTestId("rules-preview-run"));

    expect(screen.getByTestId("rules-preview").textContent).toContain("3 章");
    expect(putChapterRules).not.toHaveBeenCalled();
  });

  it("一条规则都没命中时如实说明会退回内置切章", () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("试切预览 · 粘一段正文"), { target: { value: "一整段没有分节符的正文" } });
    fireEvent.click(screen.getByTestId("rules-preview-run"));

    expect(screen.getByTestId("rules-preview").textContent).toContain("退回内置切章");
  });

  it("规则区的加载 / 错误 / 空态各有可读状态", () => {
    rulesState.loading = true;
    const first = renderPage();
    expect(screen.getByTestId("rules-loading")).toBeTruthy();
    first.unmount();

    rulesState.loading = false;
    rulesState.error = new ApiError("http", "boom", 500);
    const second = renderPage();
    expect(screen.getByTestId("rules-error")).toBeTruthy();
    second.unmount();

    rulesState.error = null;
    rulesState.data = { rules: [] };
    renderPage();
    expect(screen.getByTestId("rules-empty")).toBeTruthy();
  });
});

describe("导入页：只预检（dry-run）", () => {
  beforeEach(() => {
    jobState.data = { items: [] };
    jobState.loading = false;
    jobState.error = null;
    failedFilesState.data = { files: [] };
    failedFilesState.loading = false;
    failedFilesState.error = null;
    dryRunImport.mockReset();
    startImport.mockReset();
  });

  it("点「只预检」同步拿回报告：四类计数 + 样例表 + 错误清单，且不建任务", async () => {
    dryRunImport.mockResolvedValueOnce({
      truncated: false,
      files: { total: 12, willImport: 9, duplicate: 2, unsupported: 1, failed: 0 },
      books: { new: 8, duplicate: 2, split: 1 },
      sample: [
        {
          source: "/path/to/txt/示例书简.txt",
          title: "示例书简",
          chapters: 12,
          words: 34567,
          duplicate: false,
          reason: "",
        },
      ],
      errors: [{ source: "/path/to/x.pdf", reason: "unsupported_format" }],
    });
    renderPage();

    fireEvent.change(screen.getByLabelText("目录或单个文件路径"), { target: { value: "/path/to/txt" } });
    fireEvent.click(screen.getByTestId("import-dry-run"));

    await waitFor(() => expect(screen.getByTestId("dry-run-report")).toBeTruthy());
    expect(dryRunImport.mock.calls[0][0]).toMatchObject({ dir: "/path/to/txt", recursive: true });
    expect(screen.getByTestId("dry-run-will-import").textContent).toBe("9");
    expect(screen.getByTestId("dry-run-duplicate").textContent).toBe("2");
    expect(screen.getByTestId("dry-run-unsupported").textContent).toBe("1");
    expect(screen.getByTestId("dry-run-failed").textContent).toBe("0");
    expect(screen.getByTestId("dry-run-total").textContent).toBe("12");
    expect(screen.getByTestId("dry-run-sample").textContent).toContain("示例书简");
    // 告警默认收起（契约 C-20）：点折叠条才展开明细
    fireEvent.click(screen.getByTestId("dry-run-warnings").querySelector("button")!);
    expect(screen.getByTestId("dry-run-errors").textContent).toContain("unsupported_format");
    // 预检不建任务
    expect(startImport).not.toHaveBeenCalled();
  });

  it("没填路径时先拦下，不发预检请求", () => {
    renderPage();

    fireEvent.click(screen.getByTestId("import-dry-run"));

    expect(dryRunImport).not.toHaveBeenCalled();
    expect(screen.getByText(/先填一个目录或 txt 文件路径，再预检/)).toBeTruthy();
  });

  it("预检失败给错误面板，不把失败当成功", async () => {
    dryRunImport.mockRejectedValueOnce(new ApiError("http", "timeout", 503, "timeout"));
    renderPage();

    fireEvent.change(screen.getByLabelText("目录或单个文件路径"), { target: { value: "/path/to/txt" } });
    fireEvent.click(screen.getByTestId("import-dry-run"));

    await waitFor(() => expect(screen.getByTestId("dry-run-error")).toBeTruthy());
    expect(screen.queryByTestId("dry-run-report")).toBeNull();
  });

  it("目录条目过多时说明只预检了前 200 个文件", async () => {
    dryRunImport.mockResolvedValueOnce({
      truncated: true,
      files: { total: 500, willImport: 480, duplicate: 20, unsupported: 0, failed: 0 },
      books: { new: 470, duplicate: 20, split: 10 },
      sample: [],
      errors: [],
    });
    renderPage();

    fireEvent.change(screen.getByLabelText("目录或单个文件路径"), { target: { value: "/path/to/txt" } });
    fireEvent.click(screen.getByTestId("import-dry-run"));

    await waitFor(() => expect(screen.getByTestId("dry-run-truncated")).toBeTruthy());
    expect(screen.getByTestId("dry-run-truncated").textContent).toContain("前 200 个文件");
  });
});

describe("导入页：失败清单与重试", () => {
  const failedJob: ImportJob = {
    id: 7,
    sourceDir: "/path/to/txt",
    status: "completed_with_errors",
    total: 3,
    imported: 0,
    skipped: 0,
    failed: 3,
    failedCount: 3,
    startedAt: "2026-10-25T10:00:00.000Z",
    finishedAt: "2026-10-25T10:01:00.000Z",
    message: "",
  };

  const failedFiles: ImportJobFile[] = [
    {
      id: 1,
      source: "/path/to/txt/a.txt",
      displayPath: "a.txt",
      status: "failed",
      reason: "parse_failed",
      bookId: 0,
      updatedAt: "2026-10-25T10:01:00.000Z",
    },
    {
      id: 2,
      source: "/path/to/txt/b.txt",
      displayPath: "b.txt",
      status: "failed",
      reason: "parse_failed",
      bookId: 0,
      updatedAt: "2026-10-25T10:01:00.000Z",
    },
  ];

  beforeEach(() => {
    jobState.data = { items: [failedJob] };
    jobState.loading = false;
    jobState.error = null;
    failedFilesState.data = { files: failedFiles };
    failedFilesState.loading = false;
    failedFilesState.error = null;
    retryImport.mockReset();
  });

  it("每行可单独重试，顶部可一键重试全部失败，成功后提示新任务号", async () => {
    retryImport.mockResolvedValue({ jobId: 9 });
    renderPage();

    expect(screen.getByTestId("failed-files")).toBeTruthy();

    fireEvent.click(screen.getByLabelText("重试 a.txt"));
    await waitFor(() => expect(retryImport.mock.calls[0]).toEqual([7, ["/path/to/txt/a.txt"]]));

    fireEvent.click(screen.getByTestId("retry-all-failed"));
    await waitFor(() => expect(retryImport.mock.calls[1]?.[0]).toBe(7));
    // 全部重试不带 sources（后端据此重试该任务全部 failed）
    expect(retryImport.mock.calls[1][1]).toBeUndefined();
    await waitFor(() => expect(screen.getByTestId("retry-note").textContent).toContain("#9"));
  });

  it("任务还在跑时重试被拒，把 409 说成人话", async () => {
    retryImport.mockRejectedValueOnce(new ApiError("http", "conflict", 409, "conflict"));
    renderPage();

    fireEvent.click(screen.getByTestId("retry-all-failed"));

    await waitFor(() => expect(screen.getByTestId("retry-note").textContent).toContain("任务还在跑"));
  });

  it("completed_with_errors 显示「完成（部分失败）」，不再落到排队中", () => {
    failedFilesState.data = { files: [] };
    renderPage();

    expect(screen.getByText("完成（部分失败）")).toBeTruthy();
    expect(screen.queryByText("排队中")).toBeNull();
  });
});
describe("导入页：运行中任务的进度轮询", () => {
  beforeEach(() => {
    jobState.data = { items: [runningJob] };
    jobState.loading = false;
    jobState.error = null;
    jobState.reload.mockReset();
    importJob.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  /** 把首帧那次 tick（effect 里立刻拉一次）落定。 */
  async function settle() {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
  }

  it("进页面先拉一次，之后每 2 秒一次：进度条与计数跟着动", async () => {
    vi.useFakeTimers();
    importJob.mockResolvedValue({ ...runningJob, imported: 6, skipped: 2 });
    renderPage();
    // 列表里那份是 3 + 1 = 4 个文件；轮询回来后变 6 + 2 = 8
    expect(screen.getByTestId("import-progress-done").textContent).toBe("4");
    await settle();

    expect(importJob).toHaveBeenCalledWith(42);
    expect(screen.getByTestId("import-progress-done").textContent).toBe("8");
    expect((screen.getByTestId("import-progress-bar") as HTMLElement).style.width).toBe("80%");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(importJob).toHaveBeenCalledTimes(2);
  });

  it("到终态就停：completed 后不再轮询，并刷新一次任务列表", async () => {
    vi.useFakeTimers();
    importJob.mockResolvedValueOnce({ ...runningJob, status: "completed", imported: 10, skipped: 0, failed: 0 });
    renderPage();
    await settle();

    expect(jobState.reload).toHaveBeenCalledTimes(1);
    importJob.mockClear();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(importJob).not.toHaveBeenCalled();
  });

  it("卸载后定时器清掉：不再发轮询请求", async () => {
    vi.useFakeTimers();
    importJob.mockResolvedValue({ ...runningJob });
    const view = renderPage();
    await settle();
    view.unmount();
    importJob.mockClear();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(importJob).not.toHaveBeenCalled();
  });

  it("单次轮询失败不中断，下一轮继续把进度补上", async () => {
    vi.useFakeTimers();
    importJob
      .mockRejectedValueOnce(new ApiError("network", "boom"))
      .mockResolvedValue({ ...runningJob, imported: 8, skipped: 1, failed: 0 });
    renderPage();
    await settle();
    // 这一轮失败：仍显示进页面时列表里的 3 + 1 = 4，不报错也不清空
    expect(screen.getByTestId("import-progress-done").textContent).toBe("4");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(importJob).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId("import-progress-done").textContent).toBe("9");
  });

  it("没有运行中的任务时不轮询", async () => {
    vi.useFakeTimers();
    jobState.data = { items: [{ ...runningJob, status: "completed" }] };
    renderPage();
    await settle();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000);
    });
    expect(importJob).not.toHaveBeenCalled();
  });
});
