import { useReleaseNotes, useVersion } from "../../api/hooks";
import type { ReleaseEntry } from "../../api/types";
import { EmptyPanel, ErrorPanel, RowSkeleton } from "../../components/StatePanel";
import { useIsH5 } from "../../lib/tier";
import {
  formatReleaseDate,
  isCurrent,
  notesSummary,
  versionAnchor,
  versionLabel,
  visibleSections,
} from "./releaseNotes";

/**
 * 更新日志（两档都开）：把 CHANGELOG 里每个版本讲清楚，用户不必去 GitHub。
 *
 * 数据只有一处：二进制内嵌的 internal/releases/CHANGELOG.md，经 GET /api/releases 返回。
 * 所以「仓库里的说明」与「产品里的说明」不可能各说各话——这也是这页存在的理由：
 * 说明写在仓库、产品里看不到，等于没写。
 *
 * 「正在使用」的高亮只认后端返回的 current（它已和说明文件对齐过）；
 * 当前运行版本再单独从 /api/version 取一次，顶部展示——版本号对不上说明时，
 * 页面照样能打开，只是没有高亮，不会整页报错。
 */
export function ReleaseNotesPage() {
  const notes = useReleaseNotes();
  const version = useVersion();
  const isH5 = useIsH5();
  const items = notes.data?.items ?? [];

  return (
    <section className="view-releases" data-testid="view:releases" data-od-id="releases">
      <div className="page-head">
        <div>
          <p className="eyebrow">
            更新日志 · {notes.loading ? "读取中" : notesSummary(items)}
          </p>
          <h1>更新日志</h1>
          <p className="sub">
            你在用的这一版：<strong data-testid="releases-running-version">{runningVersion(version.data?.version)}</strong>
            {version.data?.builtAt ? <span className="meta"> · 构建于 {formatBuiltAt(version.data.builtAt)}</span> : null}
          </p>
        </div>
      </div>

      <ReleaseNotesBody notes={notes} items={items} current={notes.data?.current ?? ""} isH5={isH5} />
    </section>
  );
}

/** 四种状态（加载 / 出错 / 空 / 有数据）；空态说明「说明文件没随版本发布」，不是「没有更新」。 */
function ReleaseNotesBody({
  notes,
  items,
  current,
  isH5,
}: {
  notes: ReturnType<typeof useReleaseNotes>;
  items: ReleaseEntry[];
  current: string;
  isH5: boolean;
}) {
  if (notes.loading) return <RowSkeleton count={4} testId="releases-skeleton" />;

  if (notes.error) {
    return (
      <ErrorPanel
        error={notes.error}
        title="更新日志没能加载"
        onRetry={notes.reload}
        testId="releases-error"
      />
    );
  }

  if (items.length === 0) {
    return (
      <EmptyPanel
        title="还没有发版说明"
        description="当前版本没有随包附带发版说明，可以先在仓库的 internal/releases/CHANGELOG.md 里补充。"
        testId="releases-empty"
      />
    );
  }

  return (
    <ol className="release-list" data-testid="releases-list">
      {items.map((entry) => (
        <ReleaseCard key={entry.version} entry={entry} current={current} isH5={isH5} />
      ))}
    </ol>
  );
}

/** 单个版本：标题行（版本 / 日期 / 正在使用 / 回溯标记）+ 分组条目。 */
function ReleaseCard({ entry, current, isH5 }: { entry: ReleaseEntry; current: string; isH5: boolean }) {
  const sections = visibleSections(entry);
  const date = formatReleaseDate(entry.date);

  return (
    <li
      className={`release-card${isCurrent(entry, current) ? " is-current" : ""}`}
      id={versionAnchor(entry.version)}
      data-testid={`release:${entry.version}`}
      data-current={isCurrent(entry, current) ? "true" : undefined}
    >
      <header className="release-head">
        <h2>{versionLabel(entry.version)}</h2>
        {isCurrent(entry, current) ? (
          <span className="release-badge" data-testid="releases-current-badge">
            正在使用
          </span>
        ) : null}
        {date ? <span className="meta release-date">{date}</span> : null}
        {entry.retro ? <span className="meta release-retro">据 git 历史回溯</span> : null}
      </header>

      <div className={`release-groups${isH5 ? " is-h5" : ""}`}>
        {sections.map((section) => (
          <div
            key={section.key}
            className={`release-group release-${section.key}`}
            data-testid={`release-group:${section.key}`}
          >
            <h3>{section.label}</h3>
            <ul>
              {section.items.map((item, index) => (
                <li key={index}>{item}</li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </li>
  );
}

/** 顶部「你在用的这一版」：dev 构建没有真实版本号，直接显示 dev。 */
function runningVersion(v: string | undefined): string {
  if (!v) return "读取中…";
  return v === "dev" ? "dev（开发构建）" : versionLabel(v);
}

/** 构建时间只到分钟；解析不出来就原样不展示（不编一个时间）。 */
function formatBuiltAt(builtAt: string): string {
  const parsed = new Date(builtAt);
  if (Number.isNaN(parsed.getTime())) return builtAt;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())} ${pad(parsed.getHours())}:${pad(parsed.getMinutes())}`;
}
