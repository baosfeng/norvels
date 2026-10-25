import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ImportJob, UploadResult } from "../../api/types";
import { TierProvider } from "../../lib/tier";
import { TierRoute } from "../../layouts/TierRoute";

const jobState = {
  data: { items: [] as ImportJob[] },
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const uploadImport = vi.fn<(files: File[], opts?: { force?: boolean }) => Promise<UploadResult>>();
const startImport = vi.fn(async () => ({ jobId: 1 }));

vi.mock("../../api/hooks", () => ({ useImportJobs: () => jobState }));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { uploadImport, startImport } };
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
    expect(screen.getByText("支持 TXT、EPUB，可一次选多个")).toBeTruthy();
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
    expect(opts).toEqual({ force: false });

    expect(screen.getByTestId("upload-accepted").textContent).toContain("任务 #7 已排队");
    expect(screen.getByTestId("upload-rejected").textContent).toContain("暂不支持 .pdf，目前支持 TXT 与 EPUB");
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
    expect(uploadImport.mock.calls[0][1]).toEqual({ force: true });
  });

  it("整批都不支持：不发请求，直接说明目前支持什么", () => {
    render(<ImportPage />);

    pickFiles(sample("示例书简.pdf"));
    clickUpload();

    expect(uploadImport).not.toHaveBeenCalled();
    expect(screen.getByTestId("upload-error").textContent).toContain("目前支持 TXT 与 EPUB");
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
