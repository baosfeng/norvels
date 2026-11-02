import { Link } from "react-router-dom";
import { useRecommendations } from "../../api/hooks";
import type { RecommendItem, RecommendTab } from "../../api/types";
import { Cover } from "../../components/Cover";
import { track } from "../../lib/telemetry";
import { sceneCopy, type RecommendScene } from "./sceneCopy";

/** 一次给几张：与详情页相似书同一档（横向一排 6 张，窄屏块内横向滚动）。 */
export const SCENE_RECOMMEND_LIMIT = 6;

/**
 * 每个场景走哪个推荐来源（GET /api/recommend?tab=…）：
 * - 首页 / 书架：拿到的是「书」而不是「继续读」，所以走为你推荐（for_you）；
 * - 详情页：相似书有自己的口径（similar=<bookId>），由 SimilarBooks 负责，这里不接；
 * - 搜索空态：同为你推荐（没有关键词可喂给推荐器，不假装按搜索词挑）。
 */
const SCENE_TAB: Record<Exclude<RecommendScene, "book">, RecommendTab> = {
  home: "for_you",
  shelf: "for_you",
  "search-empty": "for_you",
};

/**
 * 场景推荐位：首页 / 书架 / 搜索空态共用的一块「带场景说明的推荐」。
 *
 * 三态口径：
 * - 加载：给两行骨架（与真卡同形，回来时不跳）；
 * - 空：**整块不渲染**——没数据还占一屏标题，是把首屏最贵的位置浪费在一个空框上；
 * - 失败：**静默降级**，同样整块不渲染。推荐是页面的旁路，不该由它决定页面是否可用
 *   （首页的「最近入库」、书架的列表都在这块之外，各自有自己的错误态）。
 *
 * 场景说明（「推荐来源：…」）在标题右侧，与标题同一行：用户要能看出这块为什么推给他，
 * 而不是把「推荐」当成页面本身的固定内容。每个条目自己的理由（后端 reason）显示在卡上。
 */
export function SceneRecommendBlock({ scene, limit = SCENE_RECOMMEND_LIMIT }: {
  scene: RecommendScene;
  limit?: number;
}) {
  // 详情页的相似书由 SimilarBooks 负责（口径不同、接口参数不同），这里不重复请求
  const enabled = scene !== "book";
  const tab: RecommendTab = enabled ? SCENE_TAB[scene as Exclude<RecommendScene, "book">] : "for_you";
  const rec = useRecommendations(limit, tab, { enabled });
  const copy = sceneCopy(scene);

  if (!enabled) return null;
  if (rec.loading) {
    return (
      <section className="scene-rec" data-testid={`scene-rec:${scene}:loading`}>
        <SceneHead scene={scene} copy={copy} />
        <RowSkeletonScene />
      </section>
    );
  }
  // 失败静默降级：不渲染错误面板、不打断页面（推荐是旁路信息）
  if (rec.error) return null;
  const items = rec.data?.items ?? [];
  // 没有推荐不占位：空列表与失败走同一条路——整块不渲染
  if (items.length === 0) return null;

  return (
    <section className="scene-rec" data-testid={`scene-rec:${scene}`} data-scene={scene}>
      <SceneHead scene={scene} copy={copy} />
      <div className="scene-rec-row">
        {items.map((item) => (
          <SceneCard key={item.bookId} item={item} scene={scene} />
        ))}
      </div>
    </section>
  );
}

/** 标题 + 来源说明。来源说明带上 data-testid，测试与接入方都能确认「场景写出来了」。 */
function SceneHead({ scene, copy }: { scene: RecommendScene; copy: { title: string; note: string } }) {
  return (
    <div className="scene-rec-head">
      <h2 className="scene-rec-title">{copy.title}</h2>
      <span className="meta scene-rec-note" data-testid={`scene-rec:${scene}:note`}>
        {copy.note}
      </span>
    </div>
  );
}

/** 骨架：与真卡同宽同高，取回数据时不推动下方内容。 */
function RowSkeletonScene() {
  return (
    <div className="scene-rec-row" aria-hidden="true" data-testid="scene-rec-skeleton">
      {Array.from({ length: 2 }, (_, i) => (
        <div className="scene-rec-card" key={i}>
          <span className="sk sk-cover" />
          <span className="sk sk-line w80" />
          <span className="sk sk-line w40" />
        </div>
      ))}
    </div>
  );
}

/**
 * 一张推荐卡：复用书架的书卡结构，正文列多一行「推荐理由」（后端给，前端不拼装）。
 * 点击进详情并记一次 click（extra 带理由 code）——没有这条信号就无法回答
 * 「这个场景的推荐位到底有没有用」。
 */
function SceneCard({ item, scene }: { item: RecommendItem; scene: RecommendScene }) {
  const signal = {
    bookId: item.bookId,
    categoryL1: item.categoryL1,
    categoryL2: item.categoryL2,
    // 埋点带 code 而不是文案：文案改了不该把看板上的理由分布拆成两行
    extra: item.reasonCode || item.reason,
  };

  return (
    <article className="scene-rec-card book-card" data-testid={`scene-rec-item-${item.bookId}`}>
      <Link
        className="book-link"
        to={`/book/${item.bookId}`}
        state={{ from: scene, reason: item.reason }}
        onClick={() => track("click", signal)}
      >
        <Cover title={item.title} bookId={item.bookId} hasCover={item.hasCover} showLabel={false} />
        <div className="book-card-body">
          <h3 className="book-title">{item.title}</h3>
          <p className="book-author">{item.authorName || "佚名"}</p>
          <p className="scene-rec-reason">{item.reason}</p>
        </div>
      </Link>
    </article>
  );
}
