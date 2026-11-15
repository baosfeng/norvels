import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AI_READING_KEYS, readAiReadingConfig, type AiReadingConfig } from "../../lib/aiReading";
import { ReaderAiPanel } from "./ReaderAiPanel";

vi.mock("../../lib/aiReading", async () => {
  const actual = await vi.importActual<typeof import("../../lib/aiReading")>("../../lib/aiReading");
  return { ...actual, requestAiReading: vi.fn() };
});

const { requestAiReading } = await import("../../lib/aiReading");
const requestMock = vi.mocked(requestAiReading);

const config: AiReadingConfig = { enabled: true, baseUrl: "https://ai.example/v1", model: "m", apiKey: "k" };

function open(props: Partial<React.ComponentProps<typeof ReaderAiPanel>> = {}) {
  const onClose = vi.fn();
  const onConfigChange = vi.fn();
  const view = render(
    <ReaderAiPanel
      open
      onClose={onClose}
      chapterTitle="第一章"
      chapterText="正文第一段"
      selection=""
      config={config}
      onConfigChange={onConfigChange}
      {...props}
    />,
  );
  return { ...view, onClose, onConfigChange };
}

beforeEach(() => {
  localStorage.clear();
  requestMock.mockReset();
});

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("ReaderAiPanel 基础渲染", () => {
  it("关闭时不渲染任何内容", () => {
    render(<ReaderAiPanel open={false} onClose={() => {}} chapterText="正文" config={config} />);
    expect(screen.queryByTestId("reader-ai-panel")).toBeNull();
  });

  it("打开时展示隐私说明与三个页签，关闭按钮回调父级", () => {
    const { onClose } = open();
    expect(screen.getByTestId("reader-ai-panel")).toBeTruthy();
    expect(screen.getByText(/隐私提示/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "关闭 AI 辅读" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("ReaderAiPanel 三个任务", () => {
  it("摘要：成功后展示模型返回的正文", async () => {
    requestMock.mockResolvedValue({ ok: true, text: "这是摘要" });
    open();
    fireEvent.click(screen.getByRole("button", { name: "生成本章摘要" }));
    await waitFor(() => expect(screen.getByTestId("ai-reading-result").textContent).toBe("这是摘要"));
    expect(requestMock).toHaveBeenCalledWith(config, expect.objectContaining({ task: "summary", chapterText: "正文第一段" }));
  });

  it("段落解释：没填内容时给出提示且不发请求", async () => {
    open();
    fireEvent.click(screen.getByRole("button", { name: "段落解释" }));
    fireEvent.click(screen.getByRole("button", { name: "解释选中段落" }));
    await waitFor(() => expect(screen.getByTestId("ai-reading-error").textContent).toContain("请先选中一段正文"));
    expect(requestMock).not.toHaveBeenCalled();
  });

  it("段落解释：手动输入后带上选段请求", async () => {
    requestMock.mockResolvedValue({ ok: true, text: "解释结果" });
    open();
    fireEvent.click(screen.getByRole("button", { name: "段落解释" }));
    fireEvent.change(screen.getByLabelText("选中段落"), { target: { value: "  某段  " } });
    fireEvent.click(screen.getByRole("button", { name: "解释选中段落" }));
    await waitFor(() => expect(screen.getByTestId("ai-reading-result")).toBeTruthy());
    expect(requestMock).toHaveBeenCalledWith(config, expect.objectContaining({ task: "explain", selection: "某段" }));
  });

  it("本章问答：空问题给提示，填了才请求", async () => {
    open();
    fireEvent.click(screen.getByRole("button", { name: "本章问答" }));
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() => expect(screen.getByTestId("ai-reading-error").textContent).toContain("请输入问题"));

    requestMock.mockResolvedValue({ ok: true, text: "答案" });
    fireEvent.change(screen.getByLabelText("问题"), { target: { value: "他想干什么" } });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    await waitFor(() => expect(screen.getByTestId("ai-reading-result").textContent).toBe("答案"));
    expect(requestMock).toHaveBeenLastCalledWith(config, expect.objectContaining({ task: "qa", question: "他想干什么" }));
  });

  it("请求抛异常时展示网络失败文案", async () => {
    requestMock.mockRejectedValue(new Error("boom"));
    open();
    fireEvent.click(screen.getByRole("button", { name: "生成本章摘要" }));
    await waitFor(() => expect(screen.getByTestId("ai-reading-error").textContent).toContain("网络请求失败"));
  });
});

describe("ReaderAiPanel 配置", () => {
  it("受控配置：改动回调给父级并写入本地存储", () => {
    const { onConfigChange } = open();
    fireEvent.click(screen.getByRole("button", { name: "配置端点" }));
    fireEvent.change(screen.getByDisplayValue("https://ai.example/v1"), { target: { value: "https://new.example/v1/" } });

    expect(onConfigChange).toHaveBeenCalledWith(expect.objectContaining({ baseUrl: "https://new.example/v1/" }));
    expect(localStorage.getItem(AI_READING_KEYS.baseUrl)).toBe("https://new.example/v1");
  });

  it("非受控：没给 config 时自己从本地存储读，并自己回写", () => {
    localStorage.setItem(AI_READING_KEYS.baseUrl, "https://local.example/v1");
    localStorage.setItem(AI_READING_KEYS.model, "local-model");
    render(<ReaderAiPanel open onClose={() => {}} chapterText="正文" />);
    fireEvent.click(screen.getByRole("button", { name: "配置端点" }));
    expect(screen.getByDisplayValue("https://local.example/v1")).toBeTruthy();

    fireEvent.change(screen.getByDisplayValue("local-model"), { target: { value: "next-model" } });
    expect(readAiReadingConfig().model).toBe("next-model");
  });

  it("启用开关按勾选状态写回", () => {
    const { onConfigChange } = open();
    fireEvent.click(screen.getByRole("button", { name: "配置端点" }));
    fireEvent.click(screen.getByRole("checkbox"));
    expect(onConfigChange).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }));
  });
});

describe("ReaderAiPanel 打开时的状态重置", () => {
  it("重新打开会清空上一轮结果，并把新的选中段落填进输入框", async () => {
    requestMock.mockResolvedValue({ ok: true, text: "旧结果" });
    const view = open();
    fireEvent.click(screen.getByRole("button", { name: "生成本章摘要" }));
    await waitFor(() => expect(screen.getByTestId("ai-reading-result")).toBeTruthy());

    act(() => {
      view.rerender(
        <ReaderAiPanel open={false} onClose={() => {}} chapterText="正文" selection="" config={config} />,
      );
    });
    act(() => {
      view.rerender(
        <ReaderAiPanel open onClose={() => {}} chapterText="正文" selection="新选段" config={config} />,
      );
    });

    expect(screen.queryByTestId("ai-reading-result")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "段落解释" }));
    expect((screen.getByLabelText("选中段落") as HTMLTextAreaElement).value).toBe("新选段");
  });
});
