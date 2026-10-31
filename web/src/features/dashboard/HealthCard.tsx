import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import type { ApiError } from "../../api/client";
import type { HealthIssueKind, HealthReport } from "../../api/types";
import { ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { formatCount, formatDate } from "../../lib/format";
import { HealthDrawer } from "./HealthDrawer";
import { HEALTH_ISSUE_META } from "./healthIssues";

/**
 * 库健康状况卡：四项人话指标 + 每项一个「查看明细」入口。
 *
 * 数字来自 /api/health/report，明细来自 /api/health/issues（判定口径同源，
 * 见 internal/store/health_issues.go），所以「卡片上是几本」与「抽屉里列几条」恒等。
 * 没有问题的项入口置灰并说明「没有这类问题」，不让用户点进一个空抽屉。
 */
export function HealthCard({
  report,
  loading,
  error,
  onRetry,
  lastImportAt,
}: {
  report: HealthReport | null;
  loading: boolean;
  error: ApiError | null;
  onRetry: () => void;
  lastImportAt: string | null;
}) {
  const [openKind, setOpenKind] = useState<HealthIssueKind | null>(null);
  /** 打开抽屉的那个入口按钮：关闭后焦点要回到它（modal 语义，键盘用户不迷路）。 */
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const headingRef = useRef<HTMLHeadingElement | null>(null);
  const wasOpen = useRef(false);

  useEffect(() => {
    if (openKind) {
      wasOpen.current = true;
      return;
    }
    if (!wasOpen.current) return;
    wasOpen.current = false;
    const trigger = triggerRef.current;
    // 入口可能因为问题清零而变成 disabled：这时退回卡片标题，焦点仍在健康卡里
    if (trigger && !trigger.disabled) trigger.focus();
    else headingRef.current?.focus();
  }, [openKind]);

  function openDrawer(kind: HealthIssueKind, trigger: HTMLButtonElement) {
    triggerRef.current = trigger;
    setOpenKind(kind);
  }

  return (
    <section className="dash-block" data-od-id="dashboard:health-card">
      <div className="dash-block-head">
        <h2 tabIndex={-1} ref={headingRef}>
          库健康状况
        </h2>
      </div>
      {loading ? (
        <LoadingPanel title="正在检查书库" description="稍等一下。" />
      ) : error ? (
        <ErrorPanel error={error} title="健康状况没能读取" onRetry={onRetry} />
      ) : report ? (
        <>
          <div className="health-item">
            <div className="health-top">
              <b>章节完整</b>
              <span className={`pill${report.thinBooks > 0 ? " is-warn" : " is-live"}`}>
                {report.thinBooks > 0 ? "需处理" : "良好"}
              </span>
            </div>
            <p className="health-num num">
              {formatCount(report.totalBooks - report.thinBooks)} / {formatCount(report.totalBooks)} 本
            </p>
            <p className="meta">
              {report.thinBooks > 0
                ? `${report.thinBooks} 本书的章节数偏少，可能是原文缺少章节标记。`
                : "所有书都拆出了完整目录。"}
            </p>
            <HealthEntry kind="thin_book" count={report.thinBooks} onOpen={openDrawer} />
          </div>

          <div className={`health-item${report.missingAuthor + report.missingIntro > 0 ? " is-warn" : ""}`}>
            <div className="health-top">
              <b>待补信息</b>
              <span className={`pill${report.missingAuthor + report.missingIntro > 0 ? " is-warn" : " is-live"}`}>
                {report.missingAuthor + report.missingIntro > 0 ? "需处理" : "已齐全"}
              </span>
            </div>
            <p className="health-num num">
              {formatCount(report.missingAuthor)} 本缺作者 · {formatCount(report.missingIntro)} 本缺简介
            </p>
            <p className="meta">补全后，在书架和搜索结果里更容易分辨。</p>
            <HealthEntry kind="missing_author" count={report.missingAuthor} onOpen={openDrawer} />
            <HealthEntry kind="missing_intro" count={report.missingIntro} onOpen={openDrawer} />
          </div>

          <div className="health-item">
            <div className="health-top">
              <b>超长章节</b>
              <span className={`pill${report.hugeChapters > 0 ? " is-warn" : " is-live"}`}>
                {report.hugeChapters > 0 ? "留意" : "正常"}
              </span>
            </div>
            <p className="health-num num">{formatCount(report.hugeChapters)} 个</p>
            <p className="meta">单章超过 3 万字，可能是章节分界没识别到。</p>
            <HealthEntry kind="huge_chapter" count={report.hugeChapters} onOpen={openDrawer} />
          </div>

          <div className="health-item">
            <div className="health-top">
              <b>最近更新</b>
            </div>
            <p className="health-num num">{formatDate(lastImportAt)}</p>
            <p className="meta">库里最新一次导入的时间。</p>
          </div>

          {/* C-27 / C-28：健康卡给出下一步入口，按指出的问题去补 */}
          <div className="health-actions">
            <Link className="btn btn-sm" to="/admin" data-od-id="C-27">
              去书籍管理
            </Link>
            <Link className="btn btn-sm btn-ghost" to="/import" data-od-id="C-28">
              去导入
            </Link>
          </div>
        </>
      ) : null}

      {openKind ? <HealthDrawer kind={openKind} onClose={() => setOpenKind(null)} /> : null}
    </section>
  );
}

/**
 * 一项问题的明细入口：有 N 条就是「查看 N 本/个」，0 条置灰并说明没有这类问题。
 *
 * 置灰用 disabled 按钮而不是普通文本：入口位置固定，用户不会在点击后才发现是空的。
 */
function HealthEntry({
  kind,
  count,
  onOpen,
}: {
  kind: HealthIssueKind;
  count: number;
  onOpen: (kind: HealthIssueKind, trigger: HTMLButtonElement) => void;
}) {
  const meta = HEALTH_ISSUE_META[kind];
  if (count <= 0) {
    return (
      <button className="health-entry is-empty" type="button" disabled data-testid={`health-entry-${kind}`}>
        {meta.empty}
      </button>
    );
  }
  return (
    <button
      className="health-entry"
      type="button"
      data-testid={`health-entry-${kind}`}
      title={`打开${meta.title}明细`}
      onClick={(event) => onOpen(kind, event.currentTarget)}
    >
      {meta.entry} · 查看 {formatCount(count)} {meta.unit}
    </button>
  );
}