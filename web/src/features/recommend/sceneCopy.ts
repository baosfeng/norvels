/**
 * 推荐位的场景文案（单一事实源）。
 *
 * 需求 9 要求「凡是展示书籍的地方都有推荐位，并且注明场景」——用户看到的每一块推荐
 * 都要能回答「这是从哪儿推给我的」。所以文案按 **场景** 组织，组件只认 scene 这个键，
 * 不各自拼标题：同一个场景在哪个页面都长一样，改文案只改这里一处。
 *
 * 与后端 `reasonCode` 的分工：这里是「这个位子是干什么的」（场景级），
 * 后端给的 `reason` 是「为什么是这一本」（条目级，按 reasonCode 生成），
 * 两者都展示，但都不在前端硬编码理由文案——理由的 code → 文案映射只在 internal/recommend。
 */
export type RecommendScene = "home" | "shelf" | "book" | "search-empty";

export interface SceneCopy {
  /** 区块标题：这个位子推荐的是什么 */
  title: string;
  /** 来源说明：为什么这些书会出现在这里（用户要能看出推荐来源） */
  note: string;
}

/** 每个场景的标题与来源说明。note 一律以「推荐来源：」开头，便于用户一眼扫到出处。 */
export const SCENE_COPY: Record<RecommendScene, SceneCopy> = {
  home: {
    title: "今晚还可以看这些",
    note: "推荐来源：按你的阅读、评分与收藏挑的",
  },
  shelf: {
    title: "书架之外，可能也合你口味",
    note: "推荐来源：按你书架上的书挑的",
  },
  book: {
    title: "你可能也喜欢",
    note: "推荐来源：按分类、作者与共读关系挑的",
  },
  "search-empty": {
    title: "没有完全匹配，试试这些",
    note: "推荐来源：按你的阅读偏好挑的",
  },
};

/**
 * 场景未知时的兜底文案（例如后端 / 路由以后新增了场景，前端还没更新）。
 * 不抛错、也不复用别的场景的标题——宁可说「推荐」这一个泛称，也不冒充具体口径。
 */
const FALLBACK: SceneCopy = {
  title: "为你推荐",
  note: "推荐来源：按你的阅读记录挑的",
};

/** 取某个场景的文案；不认识的场景给泛化文案（fail-open 到「不崩」，不是猜口径）。 */
export function sceneCopy(scene: RecommendScene): SceneCopy {
  return SCENE_COPY[scene] ?? FALLBACK;
}

/** 场景清单：测试与接入方用它遍历，避免各处再抄一份字面量。 */
export const RECOMMEND_SCENES: RecommendScene[] = ["home", "shelf", "book", "search-empty"];
