import type { ReaderHit } from "../../../lib/readerSearch";

/**
 * 把一章正文拼成 DOM 片段：分页引擎的排版源与页层都从它克隆。
 *
 * 走 DOM 构造而不是 innerHTML 字符串：正文来自解析器，可能带 <>& 之类的字符，
 * 拼字符串迟早会遇到一处没转义的注入点；节点 API 天然免疫。
 * 段落一律带 data-para（锚点测量按它找段），id 只在「常驻测量源」那一份上挂——
 * 页层是克隆，多份同 id 会让 getElementById 拿到错的那一份。
 */
export interface FragmentInput {
  chapterId: number;
  title: string;
  paragraphs: string[];
  /** 搜索命中：段号 → 命中区间。 */
  hitsByPara?: Map<number, ReaderHit[]>;
  currentHit?: number;
  searching?: boolean;
  /** 是否挂 id（para-{chapterId}-{i}）；只有常驻测量源需要。 */
  withIds?: boolean;
  /** 是否渲染章标题（跨章拼接时只有首章带标题）。 */
  withTitle?: boolean;
}

export function paraDomId(chapterId: number, paraIndex: number): string {
  return `para-${chapterId}-${paraIndex}`;
}

export function buildChapterFragment(input: FragmentInput): DocumentFragment {
  const frag = document.createDocumentFragment();
  if (input.withTitle !== false) frag.appendChild(titleNode(input.title));
  input.paragraphs.forEach((text, i) => {
    if (!text) return;
    frag.appendChild(paraNode(text, i, input));
  });
  return frag;
}

function titleNode(title: string): HTMLElement {
  const h2 = document.createElement("h2");
  h2.textContent = title;
  // 标题不算段落：锚点只认 p，给它一个不参与换算的标记便于取证
  h2.dataset.paraTitle = "";
  return h2;
}

function paraNode(text: string, i: number, input: FragmentInput): HTMLElement {
  const p = document.createElement("p");
  p.dataset.para = String(i);
  if (input.withIds) p.id = paraDomId(input.chapterId, i);
  const hits = input.searching ? input.hitsByPara?.get(i) ?? [] : [];
  if (hits.length === 0) {
    p.textContent = text;
    return p;
  }
  appendHitNodes(p, text, hits, input.currentHit ?? 0);
  return p;
}

/** 命中包 <mark>：与连续流同一套标记（data-hit / is-current），搜索条能按索引对齐。 */
function appendHitNodes(p: HTMLElement, text: string, hits: ReaderHit[], currentHit: number) {
  let from = 0;
  for (const hit of hits) {
    if (hit.start > from) p.appendChild(document.createTextNode(text.slice(from, hit.start)));
    const mark = document.createElement("mark");
    mark.dataset.hit = String(hit.index);
    if (hit.index === currentHit) mark.className = "is-current";
    mark.textContent = text.slice(hit.start, hit.end);
    p.appendChild(mark);
    from = hit.end;
  }
  if (from < text.length) p.appendChild(document.createTextNode(text.slice(from)));
}
