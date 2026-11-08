/** AI reading aid module */
export type AiReadingTask = "summary" | "explain" | "qa";
export interface AiReadingConfig { enabled: boolean; baseUrl: string; model: string; apiKey: string; }
export interface AiReadingRequest { task: AiReadingTask; chapterTitle?: string; chapterText: string; selection?: string; question?: string; }
export interface AiReadingResult { ok: boolean; text: string; error?: string; }
export const DEFAULT_AI_READING_CONFIG: AiReadingConfig = { enabled: false, baseUrl: "", model: "gpt-4o-mini", apiKey: "" };
export const AI_READING_KEYS = { enabled: "norvels.ai.enabled", endpoint: "norvels.ai.endpoint", baseUrl: "norvels.ai.baseUrl", base: "norvels.ai.base", model: "norvels.ai.model", apiKey: "norvels.ai.apiKey" } as const;
type StorageLike = Pick<Storage, "getItem" | "setItem"> | undefined;
function raw(storage: StorageLike, key: string): string | null {
  try { return storage ? storage.getItem(key) : null; } catch { return null; }
}
function safeSet(storage: StorageLike, key: string, value: string): void {
  try { storage && storage.setItem(key, value); } catch { /* ignore */ }
}
export function normalizeBaseUrl(input: string): string {
  const value = (input || "").trim();
  if (!value) return "";
  return value.replace(/\/+$/, "");
}
export function readAiReadingConfig(storage: StorageLike = typeof window === "undefined" ? undefined : window.localStorage): AiReadingConfig {
  const enabledRaw = raw(storage, AI_READING_KEYS.enabled);
  const enabled = enabledRaw === "true" || enabledRaw === "1";
  const baseUrl = normalizeBaseUrl(raw(storage, AI_READING_KEYS.baseUrl) || raw(storage, AI_READING_KEYS.endpoint) || raw(storage, AI_READING_KEYS.base) || "");
  const model = (raw(storage, AI_READING_KEYS.model) || "").trim() || DEFAULT_AI_READING_CONFIG.model;
  const apiKey = raw(storage, AI_READING_KEYS.apiKey) || "";
  return { enabled, baseUrl, model, apiKey };
}
export function writeAiReadingConfig(config: AiReadingConfig, storage: StorageLike = typeof window === "undefined" ? undefined : window.localStorage): void {
  if (!storage) return;
  const baseUrl = normalizeBaseUrl(config.baseUrl);
  const model = (config.model || "").trim() || DEFAULT_AI_READING_CONFIG.model;
  safeSet(storage, AI_READING_KEYS.enabled, config.enabled ? "true" : "false");
  safeSet(storage, AI_READING_KEYS.baseUrl, baseUrl);
  safeSet(storage, AI_READING_KEYS.endpoint, baseUrl);
  safeSet(storage, AI_READING_KEYS.base, baseUrl);
  safeSet(storage, AI_READING_KEYS.model, model);
  safeSet(storage, AI_READING_KEYS.apiKey, config.apiKey || "");
}
export function isAiReadingConfigured(config: AiReadingConfig): boolean {
  return Boolean(normalizeBaseUrl(config.baseUrl) && (config.model || "").trim());
}
export function buildChatCompletionsUrl(baseUrl: string): string {
  const base = normalizeBaseUrl(baseUrl);
  if (!base) return "";
  if (/\/chat\/completions$/i.test(base)) return base;
  return base + "/chat/completions";
}
export function buildAiMessages(request: AiReadingRequest): { role: "system" | "user"; content: string }[] {
  const chapterTitle = (request.chapterTitle || "").trim() || "本章";
  const chapterText = (request.chapterText || "").trim().slice(0, 16000);
  const system = "你是中文小说阅读助手。只基于提供的原文回答，不编造事实；回答简洁有条理；如果原文信息不足，明确说“原文未提供”。";
  let user = "";
  if (request.task === "summary") user = "请用 3-5 条要点概括「" + chapterTitle + "」的主要内容与关键情节：\n\n" + chapterText;
  else if (request.task === "explain") user = "请用通俗语言解释下面选中的段落。\n\n选中段落：\n" + (request.selection || "").trim() + "\n\n上下文（同章）：\n" + chapterText;
  else if (request.task === "qa") user = "基于「" + chapterTitle + "」原文回答问题：" + (request.question || "").trim() + "\n\n原文：\n" + chapterText;
  else user = chapterText;
  return [ { role: "system", content: system }, { role: "user", content: user } ];
}
export async function requestAiReading(config: AiReadingConfig, request: AiReadingRequest, opts?: { signal?: AbortSignal; timeoutMs?: number }): Promise<AiReadingResult> {
  if (!isAiReadingConfigured(config)) return { ok: false, text: "", error: "请先在设置中填写 API 地址与模型" };
  const url = buildChatCompletionsUrl(config.baseUrl);
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), opts && opts.timeoutMs ? opts.timeoutMs : 30000);
  const signal = (opts && opts.signal) ? opts.signal : controller.signal;
  try {
    const res = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json", ...(config.apiKey ? { Authorization: "Bearer " + config.apiKey } : {}) }, body: JSON.stringify({ model: config.model, messages: buildAiMessages(request), stream: false, temperature: 0.3 }), signal });
    if (!res.ok) {
      let detail = "";
      try { detail = (await res.text()).slice(0, 200); } catch { detail = ""; }
      return { ok: false, text: "", error: "请求失败 (" + res.status + ")" + (detail ? "：" + detail : "") };
    }
    const data = (await res.json()) as { choices?: { message?: { content?: string } }[] };
    const text = (data && data.choices && data.choices[0] && data.choices[0].message && data.choices[0].message.content ? data.choices[0].message.content : "").trim();
    if (!text) return { ok: false, text: "", error: "服务返回空内容" };
    return { ok: true, text: text };
  } catch (error) {
    const msg = error instanceof Error ? error.message : String(error);
    if (msg.indexOf("aborted") >= 0 || msg.indexOf("AbortError") >= 0) return { ok: false, text: "", error: "请求超时，请稍后重试" };
    return { ok: false, text: "", error: "网络请求失败，请检查地址与网络" };
  } finally { clearTimeout(timeout); }
}
export async function summarizeChapter(config: AiReadingConfig, chapterTitle: string, chapterText: string, opts?: { signal?: AbortSignal; timeoutMs?: number }): Promise<AiReadingResult> {
  return requestAiReading(config, { task: "summary", chapterTitle, chapterText }, opts);
}
export async function explainParagraph(config: AiReadingConfig, selection: string, chapterText: string, opts?: { signal?: AbortSignal; timeoutMs?: number }): Promise<AiReadingResult> {
  return requestAiReading(config, { task: "explain", selection, chapterText }, opts);
}
export async function askChapter(config: AiReadingConfig, question: string, chapterTitle: string, chapterText: string, opts?: { signal?: AbortSignal; timeoutMs?: number }): Promise<AiReadingResult> {
  return requestAiReading(config, { task: "qa", question, chapterTitle, chapterText }, opts);
}