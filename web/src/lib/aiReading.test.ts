import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  readAiReadingConfig,
  writeAiReadingConfig,
  isAiReadingConfigured,
  buildChatCompletionsUrl,
  buildAiMessages,
  AI_READING_KEYS,
  normalizeBaseUrl,
} from "./aiReading";

describe("aiReading config", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => localStorage.clear());

  it("defaults disabled and empty", () => {
    const cfg = readAiReadingConfig();
    expect(cfg.enabled).toBe(false);
    expect(cfg.baseUrl).toBe("");
    expect(isAiReadingConfigured(cfg)).toBe(false);
  });

  it("reads endpoint alias", () => {
    localStorage.setItem(AI_READING_KEYS.endpoint, "https://example.com/v1");
    localStorage.setItem(AI_READING_KEYS.model, "m");
    const cfg = readAiReadingConfig();
    expect(cfg.baseUrl).toBe("https://example.com/v1");
    expect(isAiReadingConfigured(cfg)).toBe(true);
  });

  it("writes and reads config", () => {
    writeAiReadingConfig({ enabled: true, baseUrl: "https://x/v1/", model: "m", apiKey: "k" });
    const cfg = readAiReadingConfig();
    expect(cfg.enabled).toBe(true);
    expect(cfg.baseUrl).toBe("https://x/v1");
    expect(localStorage.getItem(AI_READING_KEYS.baseUrl)).toBe("https://x/v1");
    expect(localStorage.getItem(AI_READING_KEYS.endpoint)).toBe("https://x/v1");
  });

  it("builds chat url", () => {
    expect(buildChatCompletionsUrl("https://x/v1")).toBe("https://x/v1/chat/completions");
    expect(buildChatCompletionsUrl("https://x/v1/chat/completions")).toBe("https://x/v1/chat/completions");
  });

  it("normalizes base", () => {
    expect(normalizeBaseUrl(" https://a/v1/// ")).toBe("https://a/v1");
  });

  it("builds messages with selected context", () => {
    const msgs = buildAiMessages({ task: "explain", chapterTitle: "第一章", chapterText: "原文", selection: "选段", question: "" });
    expect(msgs[1].content).toContain("选段");
    const sum = buildAiMessages({ task: "summary", chapterTitle: "第一章", chapterText: "原文", selection: "", question: "" });
    expect(sum[1].content).toContain("要点");
  });
});