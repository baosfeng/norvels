import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// jsdom 量不到 flex 排版，这里守 CSS 源文件规则（口径同 adminLayout.ix.test.ts）。
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");
const paginationCss = readCss("admin-pagination.css");

/** 取某条规则的选择器命中后的第一段声明体（源文件里规则是扁平的单层写法）。 */
function ruleBody(selector: string) {
  const at = paginationCss.indexOf(selector);
  if (at < 0) return "";
  const open = paginationCss.indexOf("{", at);
  const close = paginationCss.indexOf("}", open);
  return paginationCss.slice(open + 1, close);
}

/**
 * 书籍管理列表分页条的版式口径。
 * 报障：.page-jump 此前没有任何 CSS 规则，.input 的 width:100% 把跳页输入框撑成整行宽，
 * 压住页码与箭头；「跳转」被挤到下一行，「已经到头了」被顶到右侧。
 * 实测（agent-browser /admin，1024/1280/1440 × 900）：输入框 84×34，右边界 ≤「跳转」左边界，
 * 输入框与「跳转」y 差 0，页码组不被输入框覆盖，nav 高 70（三档一致，未换行）。
 */
describe("书籍管理列表：分页条排版", () => {
  it("跳页表单是一组不拉伸的横向 flex（不再被 .input 的 100% 带走）", () => {
    const body = ruleBody(".pagination .page-jump {");
    expect(body).toContain("display: flex;");
    expect(body).toContain("flex: 0 0 auto;");
  });

  it("跳页输入框固定窄宽、自身不拉伸（不能是 100%）", () => {
    const body = ruleBody(".pagination .page-jump input {");
    expect(body).toContain("flex: 0 0 auto;");
    expect(body).toContain("width: 84px;");
    expect(body).not.toContain("width: 100%");
  });

  it("页码组与 nav 都按内容宽排布（宽度不够时换行，不互相挤压重叠）", () => {
    expect(ruleBody(".pagination {")).toContain("flex-wrap: wrap;");
    expect(ruleBody(".pagination > button,")).toContain("flex: 0 0 auto;");
  });

  it("「跳转」与跳页输入框同一基线，不再上下错位", () => {
    expect(ruleBody(".pagination .page-jump .btn-sm {")).toContain("height: 34px;");
  });

  it("「已经到头了」按 .meta 口径常驻在行尾，不落成一颗带框按钮", () => {
    const body = ruleBody(".pagination > .inline-note {");
    expect(body).toContain("border: 0;");
    expect(body).toContain("font-size: 12.5px;");
  });
});
