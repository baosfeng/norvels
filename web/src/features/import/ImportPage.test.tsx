import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChapterRule, ImportJob, UploadResult } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { TierRoute } from "../../layouts/TierRoute";

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

const uploadImport = vi.fn<(files: File[], opts?: { force?: boolean; update?: boolean }) => Promise<UploadResult>>();
const startImport = vi.fn(async () => ({ jobId: 1 }));
const cancelImport = vi.fn<(id: number) => Promise<{ status: string }>>();
const importJob = vi.fn<(id: number) => Promise<ImportJob>>();
const putChapterRules = vi.fn<(rules: ChapterRule[]) => Promise<void>>();

vi.mock("../../api/hooks", () => ({
  useImportJobs: () => jobState,
  useChapterRules: () => rulesState,
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { uploadImport, startImport, cancelImport, importJob, putChapterRules } };
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
    uploadImport.mockReset();
    startImport.mockReset();
  });

  it("空态：没选文件时说清支持什么，上传按钮先不可用", () => {
    render(<ImportPage />);

    expect(screen.getByTestId("upload-panel")).toBeTruthy();
    expect(screen.getByText("支持 TXT、EPUB、ZIP，可一次选多个")).toBeTruthy();
    expect(screen.getByTestId("upload-empty").textContent).toContain("还没有选文件");
    expect((screen.getByTestId("upload-submit") as HTMLButtonElement).disabled).toBe(true);
    // 没有进行中的任务时给空态，而不是一片空白
    expect(screen.getByTestId("state:empty")).toBeTruthy();
  });

  it("混批选择：合法文件照常上传，pdf 逐个说明原因且不阻断", async () => {
    uploadImport.mockResolvedValueOnce(acceptedTxt);
    render(<ImportPage />);

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
    render(<ImportPage />);

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
    render(<ImportPage />);

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
    render(<ImportPage />);

    pickFiles(sample("示例书简.txt"));
    fireEvent.click(screen.getByLabelText("追更（只追加新章节）"));
    expect(screen.getByTestId("upload-update-hint").textContent).toContain("只追加新章节");
    clickUpload();

    await waitFor(() => expect(uploadImport).toHaveBeenCalledTimes(1));
    expect(uploadImport.mock.calls[0][1]).toEqual({ force: false, update: true });
  });

  it("整批都不支持：不发请求，直接说明目前支持什么", () => {
    render(<ImportPage />);

    pickFiles(sample("示例书简.pdf"));
    clickUpload();

    expect(uploadImport).not.toHaveBeenCalled();
    expect(screen.getByTestId("upload-error").textContent).toContain("目前支持 TXT、EPUB、ZIP");
    expect(screen.getByTestId("upload-rejected").textContent).toContain("暂不支持 .pdf");
  });

  it("单本超过 64 MB 时在上传前挡下并说清上限", () => {
    render(<ImportPage />);
    const huge = sample("示例书简.epub");
    Object.defineProperty(huge, "size", { value: 65 * 1024 * 1024 });

    pickFiles(huge);
    clickUpload();

    expect(uploadImport).not.toHaveBeenCalled();
    expect(screen.getByTestId("upload-rejected").textContent).toContain("超过 64 MB 上限");
  });

  it("已有任务在跑时把冲突翻成人话", async () => {
    uploadImport.mockRejectedValueOnce(new ApiError("http", "import job already running", 409, "conflict"));
    render(<ImportPage />);

    pickFiles(sample("示例书简.txt"));
    clickUpload();

    await waitFor(() => expect(screen.getByTestId("upload-error")).toBeTruthy());
    expect(screen.getByTestId("upload-error").textContent).toContain("已经有一个导入任务在跑");
  });

  it("任务列表的加载态与错误态照常呈现，不被上传区吞掉", () => {
    jobState.loading = true;
    const first = render(<ImportPage />);
    expect(screen.getByTestId("state:loading")).toBeTruthy();
    first.unmount();

    jobState.loading = false;
    jobState.error = new ApiError("http", "boom", 500);
    render(<ImportPage />);
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
    render(<ImportPage />);

    fireEvent.click(screen.getByTestId("import-cancel"));

    await waitFor(() => expect(cancelImport).toHaveBeenCalledWith(42));
    // 轮询拿到 canceled 才停：提示里带上是哪一单、保留了多少本
    await waitFor(() => expect(screen.getByTestId("import-cancel-note").textContent).toContain("已取消"));
    expect(importJob).toHaveBeenCalledWith(42);
    expect(screen.getByTestId("import-cancel-note").textContent).toContain("3 本书");
  });

  it("取消失败把冲突翻成人话，不把后端错误甩给用户", async () => {
    cancelImport.mockRejectedValueOnce(new ApiError("http", "conflict", 409, "conflict"));
    render(<ImportPage />);

    fireEvent.click(screen.getByTestId("import-cancel"));

    await waitFor(() => expect(screen.getByTestId("import-cancel-note").textContent).toContain("已经结束"));
  });

  it("没有运行中的任务时不给取消入口", () => {
    jobState.data = { items: [] };
    render(<ImportPage />);

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
    render(<ImportPage />);

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
    render(<ImportPage />);

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

  it("非法正则在本地就拦下：不发请求，并说清是哪一条", () => {
    render(<ImportPage />);

    fireEvent.change(screen.getByLabelText("正则"), { target: { value: "([ " } });
    fireEvent.click(screen.getByTestId("rules-save"));

    expect(screen.getByTestId("rules-error-text").textContent).toContain("第 1 条规则的正则写得不对");
    expect(putChapterRules).not.toHaveBeenCalled();
  });

  it("试切预览按命中行数报章数，不碰后端", () => {
    rulesState.data = { rules: [{ name: "分节符", pattern: "^第\\d+章" }] };
    render(<ImportPage />);

    fireEvent.change(screen.getByLabelText("试切预览 · 粘一段正文"), {
      target: { value: "第1章 开端\n正文\n第2章 承接\n第3章 结尾" },
    });
    fireEvent.click(screen.getByTestId("rules-preview-run"));

    expect(screen.getByTestId("rules-preview").textContent).toContain("3 章");
    expect(putChapterRules).not.toHaveBeenCalled();
  });

  it("一条规则都没命中时如实说明会退回内置切章", () => {
    render(<ImportPage />);

    fireEvent.change(screen.getByLabelText("试切预览 · 粘一段正文"), { target: { value: "一整段没有分节符的正文" } });
    fireEvent.click(screen.getByTestId("rules-preview-run"));

    expect(screen.getByTestId("rules-preview").textContent).toContain("退回内置切章");
  });

  it("规则区的加载 / 错误 / 空态各有可读状态", () => {
    rulesState.loading = true;
    const first = render(<ImportPage />);
    expect(screen.getByTestId("rules-loading")).toBeTruthy();
    first.unmount();

    rulesState.loading = false;
    rulesState.error = new ApiError("http", "boom", 500);
    const second = render(<ImportPage />);
    expect(screen.getByTestId("rules-error")).toBeTruthy();
    second.unmount();

    rulesState.error = null;
    rulesState.data = { rules: [] };
    render(<ImportPage />);
    expect(screen.getByTestId("rules-empty")).toBeTruthy();
  });
});
