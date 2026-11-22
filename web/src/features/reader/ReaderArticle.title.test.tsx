import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { ChapterBrief } from "../../api/types";
import { chapterPosition } from "../shelf/progressText";
import type { ChapterStream, StreamSegment } from "./useChapterStream";
import { ReaderArticle } from "./ReaderArticle";

/**
 * U20 组件层判据：正文 h2 与顶栏「第 N 章 / 共 M 章」不能互相对不上。
 *
 * 顶栏章号是库内序号 + 1（progressText.chapterPosition），是全站导航真值；
 * 正文标题里印的章号与之冲突时，只留标题文字。
 */

/** 105 章的目录：只用到长度（顶栏「共 M 章」）与实际章节的 index。 */
const chapters: ChapterBrief[] = Array.from({ length: 105 }, (_, i) => ({
  id: 1000 + i,
  idx: i,
  title: `第${i + 1}章 占位`,
  volumeId: 1,
  wordCount: 2000,
}));

function streamOf(segment: StreamSegment): ChapterStream {
  return {
    segments: [segment],
    activeId: segment.id,
    activeIndex: segment.index,
    activeParagraphs: segment.paragraphs,
    chapterData: null,
    loading: false,
    error: null,
    reload: () => {},
    chapters,
    volumes: [],
    prev: null,
    next: null,
    percent: 0.5,
    flipTo: () => {},
    tailLoading: false,
    tailError: null,
    tailReload: () => {},
    consumeAdvance: () => false,
  };
}

function renderAt(segment: StreamSegment) {
  render(
    <MemoryRouter>
      <ReaderArticle
        stream={streamOf(segment)}
        hitsByPara={new Map()}
        currentHit={-1}
        searching={false}
        bookId={7}
        onPrev={() => {}}
        onJumpChapter={() => {}}
        onShelf={() => {}}
        reducedMotion
      />
    </MemoryRouter>,
  );
  return screen.getByRole("heading", { level: 2 });
}

describe("U20 正文标题不与顶栏章号打架", () => {
  it("标题印的章号 ≠ 库内序号：h2 只留标题文字，不重复那个错的章号", () => {
    // 第 18 章（index 0 起 17）却写着「第十九章」
    expect(chapterPosition(17, chapters.length)).toBe("第 18 章 / 共 105 章");
    const h2 = renderAt({
      id: 1017,
      index: 17,
      title: "第十九章 武道要旨",
      wordCount: 2000,
      paragraphs: ["正文。"],
    });
    expect(h2.textContent).toBe("武道要旨");
    expect(h2.textContent).not.toContain("第十九章");
  });

  it("标题印的章号与库内序号一致：原文原样，一个字都不动", () => {
    const h2 = renderAt({
      id: 1018,
      index: 18,
      title: "第十九章 空谷仙影",
      wordCount: 2000,
      paragraphs: ["正文。"],
    });
    expect(h2.textContent).toBe("第十九章 空谷仙影");
  });
});
