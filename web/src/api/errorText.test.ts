import { describe, expect, it } from "vitest";
import { ApiError } from "./client";
import { codeToText, effectiveCode, levelOf } from "./errorText";

/** 单章正文损坏的测试夹具：状态码 503，但故障范围只有这一章。 */
const corrupted = new ApiError("http", "本章正文读取失败", 503, "content_corrupted");
/** 推荐库未启用：503，但只影响当前区块。 */
const unavailable = new ApiError("http", "推荐功能未启用（推荐库不可用）", 503, "unavailable");
/** 服务端预检超时：503 + timeout，提示缩小范围。 */
const timeout503 = new ApiError("http", "预检超过 60 秒，请缩小目录范围后重试", 503, "timeout");

describe("levelOf：错误该在哪儿说", () => {
  it("单章正文损坏只说当前章，不许整页也不许指向数据库", () => {
    expect(levelOf("content_corrupted")).toBe("chapter");
  });

  it("能力未启用与预检超时都不该占整页", () => {
    expect(levelOf("unavailable")).toBe("section");
    expect(levelOf("timeout")).toBe("inline");
  });

  it("身份失效走整页（去登录），不是重试", () => {
    expect(levelOf("unauthorized")).toBe("page");
  });

  it("表单 / 文件卡片 / 搜索框这类提示都留在行内", () => {
    for (const code of ["invalid_id", "invalid_param", "not_appendable", "review_state", "query_too_short"]) {
      expect(levelOf(code)).toBe("inline");
    }
  });

  it("前端 bug 类（方法不允许 / 接口不存在）只做行内提示，不整页报错", () => {
    expect(levelOf("method_not_allowed")).toBe("inline");
    expect(levelOf("not_found")).toBe("inline");
  });

  it("未知 code 与空值都有层级兜底，不返回 undefined", () => {
    expect(levelOf("no_such_code")).toBe("section");
    expect(levelOf(undefined)).toBe("section");
    expect(levelOf(null)).toBe("section");
  });
});

describe("codeToText：说什么", () => {
  it("content_corrupted 只描述这一章，且不提数据库", () => {
    const text = codeToText("content_corrupted", 503);
    expect(text).toContain("这一章");
    expect(text).toContain("下一章");
    expect(text).not.toContain("数据库");
  });

  it("503 但 code 为 content_corrupted 时不能落到「服务不可用」口径", () => {
    const byCode = codeToText("content_corrupted", 503);
    const byStatusOnly = codeToText(undefined, 503);
    expect(byCode).not.toBe(byStatusOnly);
    expect(byCode).not.toContain("暂时不可用");
    expect(byStatusOnly).not.toContain("数据库");
  });

  it("unavailable 与 timeout 不再同文案（同是 503 也要分开说）", () => {
    expect(unavailable.status).toBe(503);
    expect(timeout503.status).toBe(503);
    const unavailableText = codeToText("unavailable", 503);
    const timeoutText = codeToText("timeout", 503);
    expect(unavailableText).not.toBe(timeoutText);
    expect(unavailableText).toContain("未启用");
    expect(timeoutText).toContain("超时");
  });

  it("unauthorized 走整页口径：提示重新登录", () => {
    expect(codeToText("unauthorized", 401)).toContain("重新登录");
  });

  it("每个登记过层级的 code 都有文案，真值且不重复", () => {
    const codes = [
      "content_corrupted",
      "unavailable",
      "timeout",
      "unauthorized",
      "method_not_allowed",
      "invalid_id",
      "invalid_param",
      "not_found",
      "not_appendable",
      "review_state",
      "query_too_short",
      "internal",
      "network",
      "aborted",
      "parse",
    ];
    const texts = codes.map((code) => codeToText(code, 500));
    for (const text of texts) expect(text.trim().length).toBeGreaterThan(0);
    expect(new Set(texts).size).toBe(codes.length);
  });

  it("未知 code 与完全没线索时都有兜底文案，不出现 undefined / 空白", () => {
    const unknown = codeToText("no_such_code", 500);
    expect(unknown.length).toBeGreaterThan(0);
    expect(unknown).not.toContain("undefined");
    expect(unknown).not.toContain("数据库");

    const nothing = codeToText(undefined, undefined);
    expect(nothing.length).toBeGreaterThan(0);
    expect(nothing).not.toContain("undefined");

    expect(codeToText(null, null)).toBe(nothing);
  });

  it("状态码只在没有 code 时兜底：401 / 503 各给一句人话", () => {
    expect(codeToText(undefined, 401)).toContain("重新登录");
    expect(codeToText(undefined, 503)).not.toContain("数据库");
    expect(codeToText(undefined, 500)).toBe(codeToText(undefined, undefined));
  });
});

describe("effectiveCode：kind 与 code 共用一个映射空间", () => {
  it("code 优先", () => {
    expect(effectiveCode(corrupted)).toBe("content_corrupted");
  });

  it("没有 code 的客户端失败用 kind 顶替（超时 / 断网 / 解析）", () => {
    expect(effectiveCode(new ApiError("timeout", "请求超时"))).toBe("timeout");
    expect(effectiveCode(new ApiError("network", "连不上"))).toBe("network");
    expect(effectiveCode(new ApiError("parse", "返回内容无法识别"))).toBe("parse");
  });

  it("http 但没有 code 时留空，交给状态码兜底", () => {
    expect(effectiveCode(new ApiError("http", "请求失败（503）", 503))).toBeUndefined();
  });

  it("没有错误对象时返回 undefined，不抛异常", () => {
    expect(effectiveCode(null)).toBeUndefined();
    expect(effectiveCode(undefined)).toBeUndefined();
  });
});
