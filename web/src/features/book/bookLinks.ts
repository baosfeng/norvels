/**
 * 详情页里「点作者 / 点分类」的落地地址。
 *
 * - 作者页（/authors/:authorId，AuthorWorksPage）是 PC 专属：H5 点过去会被 TierRoute 弹回首页，
 *   所以窄屏落到「按该作者筛选的书架」（?author= 口径见 shelf/shelfQuery.ts）。
 * - 分类没有详情页，大 / 小分类统一落到书架的 l1 / l2 筛选；小分类依附大分类，只给 l2 是无主值。
 */
export function authorHref(authorId: number, isH5: boolean): string {
  return isH5 ? `/shelf?author=${authorId}` : `/authors/${authorId}`;
}

export function categoryHref(l1: string, l2?: string | null): string {
  const params = new URLSearchParams({ l1 });
  if (l2) params.set("l2", l2);
  return `/shelf?${params.toString()}`;
}
