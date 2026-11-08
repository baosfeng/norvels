import { useEffect, useState } from "react";
import {
  readAiReadingConfig,
  writeAiReadingConfig,
  requestAiReading,
  type AiReadingConfig,
  type AiReadingResult,
} from "../../lib/aiReading";

export interface ReaderAiPanelProps {
  open: boolean;
  onClose: () => void;
  chapterTitle?: string;
  chapterText: string;
  selection?: string;
  config?: AiReadingConfig;
  onConfigChange?: (config: AiReadingConfig) => void;
}

type Tab = "summary" | "explain" | "qa";

export function ReaderAiPanel(props: ReaderAiPanelProps) {
  const { open, onClose, chapterTitle, chapterText, selection, config: configProp, onConfigChange } = props;
  const [config, setConfig] = useState<AiReadingConfig>(() =>
    configProp ? configProp : readAiReadingConfig(),
  );
  const [tab, setTab] = useState<Tab>("summary");
  const [question, setQuestion] = useState("");
  const [explainText, setExplainText] = useState(selection || "");
  const [result, setResult] = useState<AiReadingResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [showConfig, setShowConfig] = useState(false);

  useEffect(() => {
    if (configProp) setConfig(configProp);
  }, [configProp]);

  useEffect(() => {
    if (open) {
      setResult(null);
      setExplainText(selection || "");
    }
  }, [open, selection]);

  function patchConfig(patch: Partial<AiReadingConfig>) {
    const next = { ...config, ...patch };
    setConfig(next);
    writeAiReadingConfig(next);
    if (onConfigChange) onConfigChange(next);
  }

  async function run(task: "summary" | "explain" | "qa") {
    setLoading(true);
    setResult(null);
    try {
      let res: AiReadingResult;
      if (task === "summary") {
        res = await requestAiReading(config, { task: "summary", chapterTitle, chapterText });
      } else if (task === "explain") {
        const sel = explainText.trim();
        if (!sel) {
          setResult({ ok: false, text: "", error: "请先选中一段正文" });
          setLoading(false);
          return;
        }
        res = await requestAiReading(config, { task: "explain", chapterTitle, chapterText, selection: sel });
      } else {
        const q = question.trim();
        if (!q) {
          setResult({ ok: false, text: "", error: "请输入问题" });
          setLoading(false);
          return;
        }
        res = await requestAiReading(config, { task: "qa", chapterTitle, chapterText, question: q });
      }
      setResult(res);
    } catch (e) {
      setResult({ ok: false, text: "", error: "网络请求失败，请检查地址与网络" });
    } finally {
      setLoading(false);
    }
  }

  if (!open) return null;

  return (
    <aside className="ai-reading-panel" role="dialog" aria-label="AI 辅读" data-testid="reader-ai-panel">
      <div className="ai-reading-head">
        <strong>AI 辅读</strong>
        <button className="btn btn-ghost" type="button" onClick={onClose} aria-label="关闭 AI 辅读">
          关闭
        </button>
      </div>

      <p className="settings-note">
        隐私提示：仅在你点击按钮时，章节内容会发送到你配置的 OpenAI-compatible 端点。默认关闭。
      </p>

      <div className="settings-row">
        <button type="button" className={tab === "summary" ? "is-active" : ""} onClick={() => setTab("summary")}>章节摘要</button>
        <button type="button" className={tab === "explain" ? "is-active" : ""} onClick={() => setTab("explain")}>段落解释</button>
        <button type="button" className={tab === "qa" ? "is-active" : ""} onClick={() => setTab("qa")}>本章问答</button>
      </div>

      {tab === "summary" && (
        <div>
          <button type="button" className="btn" disabled={loading} onClick={() => run("summary")}>
            {loading ? "生成中…" : "生成本章摘要"}
          </button>
        </div>
      )}

      {tab === "explain" && (
        <div>
          <textarea
            aria-label="选中段落"
            value={explainText}
            onChange={(e) => setExplainText(e.target.value)}
            placeholder="选中正文后自动填入，也可手动粘贴"
            rows={4}
            style={{ width: "100%" }}
          />
          <button type="button" className="btn" disabled={loading} onClick={() => run("explain")}>
            {loading ? "解释中…" : "解释选中段落"}
          </button>
        </div>
      )}

      {tab === "qa" && (
        <div>
          <input
            type="text"
            aria-label="问题"
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            placeholder="输入关于本章的问题"
          />
          <button type="button" className="btn" disabled={loading} onClick={() => run("qa")}>
            {loading ? "回答中…" : "提问"}
          </button>
        </div>
      )}

      {result && (
        <div className={result.ok ? "ai-reading-result" : "ai-reading-error"} data-testid={result.ok ? "ai-reading-result" : "ai-reading-error"}>
          {result.ok ? result.text : result.error}
        </div>
      )}

      <button type="button" className="btn btn-ghost" onClick={() => setShowConfig((v) => !v)}>
        {showConfig ? "收起配置" : "配置端点"}
      </button>

      {showConfig && (
        <div className="ai-reading-config">
          <label>
            启用辅读
            <input
              type="checkbox"
              checked={config.enabled}
              onChange={(e) => patchConfig({ enabled: e.target.checked })}
            />
          </label>
          <label>
            Base URL
            <input
              type="text"
              value={config.baseUrl}
              onChange={(e) => patchConfig({ baseUrl: e.target.value })}
              placeholder="https://api.openai.com/v1"
            />
          </label>
          <label>
            模型
            <input
              type="text"
              value={config.model}
              onChange={(e) => patchConfig({ model: e.target.value })}
            />
          </label>
          <label>
            API Key
            <input
              type="password"
              value={config.apiKey}
              onChange={(e) => patchConfig({ apiKey: e.target.value })}
              placeholder="本地存储，仅浏览器"
            />
          </label>
        </div>
      )}
    </aside>
  );
}

export { ReaderAiPanel as AiReadingPanel };
export default ReaderAiPanel;
