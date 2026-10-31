import { describe, expect, it } from "vitest";
import { ApiError } from "../../api/client";
import type { UploadLimits } from "./limits";
import { formatOf, rejectionHeadline, rejectionText, screenFiles, uploadErrorText } from "./uploadScreen";

/** 小上限：证明面板跟着下发值走，而不是内置的大值。 */
const small: UploadLimits = { extensions: ["txt"], maxFileBytes: 1024, maxTotalBytes: 2048, maxFiles: 2 };

/** 能力清单读不到：上限全部 null、扩展名空 = 不本地预检。 */
const unknown: UploadLimits = { extensions: [], maxFileBytes: null, maxTotalBytes: null, maxFiles: null };

function fileOf(name: string, size: number): File {
  const file = new File(["x"], name);
  Object.defineProperty(file, "size", { value: size });
  return file;
}

describe("上传预检：上限全部来自 capabilities", () => {
  it("格式不在下发清单里就拒绝，并说清支持哪些", () => {
    const { accepted, rejected } = screenFiles([fileOf("示例书简.txt", 10), fileOf("示例书简.pdf", 10)], small);
    expect(accepted.map((f) => f.name)).toEqual(["示例书简.txt"]);
    expect(rejected[0].code).toBe("unsupported_format");
    expect(rejected[0].message).toBe("暂不支持 .pdf，目前支持 TXT");
  });

  it("单本超过下发上限时按下发值说上限（1 KB，不是 64 MB）", () => {
    const { rejected } = screenFiles([fileOf("示例书简.txt", 2048)], small);
    expect(rejected[0].message).toBe("「示例书简.txt」超过 1 KB 上限");
  });

  it("一批合计超过下发上限时提示分批", () => {
    // 每个文件都在单本上限内（900 ≤ 1024），但三个加起来超过 2 KB 合计上限
    const files = [fileOf("甲.txt", 900), fileOf("乙.txt", 900), fileOf("丙.txt", 900)];
    const { accepted, rejected } = screenFiles(files, small);
    expect(accepted.map((f) => f.name)).toEqual(["甲.txt", "乙.txt"]);
    expect(rejected[0].name).toBe("丙.txt");
    expect(rejected[0].message).toBe("这一批文件超过 2 KB 上限，分批再传");
  });

  it("能力清单读不到时不做本地预检：不支持的格式与超大文件都放行，交给服务端", () => {
    const { accepted, rejected } = screenFiles([fileOf("示例书简.pdf", 999 * 1024 * 1024)], unknown);
    expect(rejected).toEqual([]);
    expect(accepted).toHaveLength(1);
  });

  it("上传失败的文案用下发上限；读不到时不编数字", () => {
    const tooLarge = new ApiError("http", "too large", 413, "payload_too_large");
    expect(uploadErrorText(tooLarge, small)).toBe("文件太大：单本 1 KB、一批 2 KB 以内");
    expect(uploadErrorText(tooLarge, unknown)).toBe("文件太大，超出服务端上限");
    expect(uploadErrorText(new ApiError("timeout", "timeout"), small)).toBe("上传超时了，检查网络后再试");
    expect(uploadErrorText(new Error("boom"), small)).toBe("上传没能完成，稍后再试");
  });

  it("拒绝原因兜底文案同样读下发值", () => {
    expect(rejectionText({ name: "a.txt", code: "too_many_files", message: "" }, small)).toBe("一次最多 2 个文件");
    expect(rejectionText({ name: "a.txt", code: "too_many_files", message: "" }, unknown)).toBe(
      "文件数量超过服务端上限",
    );
    expect(rejectionText({ name: "a.txt", code: "payload_too_large", message: "" }, small)).toBe("文件超过 1 KB 上限");
    expect(rejectionText({ name: "a.txt", code: "unsupported_format", message: "" }, small)).toBe(
      "暂不支持这个文件，目前支持 TXT",
    );
  });

  it("整批都进不去时用第一条原因，没有原因才给兜底文案", () => {
    const rejected = [{ name: "示例书简.pdf", code: "unsupported_format", message: "暂不支持 .pdf，目前支持 TXT" }];
    expect(rejectionHeadline(rejected, small)).toBe("暂不支持 .pdf，目前支持 TXT");
    expect(rejectionHeadline([], small)).toBe("这批文件都不支持，目前支持 TXT");
  });

  it("扩展名判定只看最后一个点之后，且大小写无关", () => {
    expect(formatOf("示例书简.TXT")).toBe("txt");
    expect(formatOf("无扩展名")).toBe("");
  });
});
