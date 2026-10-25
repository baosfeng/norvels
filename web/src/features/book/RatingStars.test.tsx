import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, BookMetaPatch } from "../../api/types";
import { ToastHost } from "../../lib/toast";

const updateBook = vi.fn<(id: number, patch: BookMetaPatch) => Promise<BookDetail>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook } };
});

const { RatingStars, BookRating } = await import("./RatingStars");

const book = {
  id: 7,
  title: "示例书",
  authorId: 1,
  authorName: "示例作者",
  chapterCount: 10,
  wordCount: 1000,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
  authors: ["示例作者"],
  volumes: [],
} as unknown as BookDetail;

beforeEach(() => {
  updateBook.mockReset();
});

describe("评分控件", () => {
  it("按星级点亮，未评时给「未评分」", () => {
    render(<RatingStars value={3} onRate={() => {}} />);
    expect(screen.getByTestId("rating-stars-star-3").textContent).toBe("★");
    expect(screen.getByTestId("rating-stars-star-4").textContent).toBe("☆");
    expect(screen.getByTestId("rating-stars-star-3").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("rating-stars-label").textContent).toBe("3 星");
  });

  it("点其它星级提交新值，点当前星级取消评分", () => {
    const onRate = vi.fn();
    render(<RatingStars value={3} onRate={onRate} />);
    fireEvent.click(screen.getByTestId("rating-stars-star-5"));
    expect(onRate).toHaveBeenCalledWith(5);
    fireEvent.click(screen.getByTestId("rating-stars-star-3"));
    expect(onRate).toHaveBeenCalledWith(0);
  });

  it("越界与非法值不会渲染出 6 星", () => {
    render(<RatingStars value={Number.NaN} onRate={() => {}} />);
    expect(screen.getByTestId("rating-stars-label").textContent).toBe("未评分");
    render(<RatingStars value={9} onRate={() => {}} />);
    expect(screen.getAllByTestId("rating-stars-star-5").length).toBeGreaterThan(0);
  });

  it("详情页评分：乐观更新只下发 rating 字段", async () => {
    updateBook.mockResolvedValue(book);
    const onChanged = vi.fn();
    render(<BookRating book={{ ...book, rating: 2 }} onChanged={onChanged} />);
    expect(screen.getByTestId("book-rating-label").textContent).toBe("2 星");

    fireEvent.click(screen.getByTestId("book-rating-star-4"));
    expect(screen.getByTestId("book-rating-label").textContent).toBe("4 星");

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0]).toEqual([7, { rating: 4 }]);
    await waitFor(() => expect(onChanged).toHaveBeenCalledWith(4));
  });

  it("保存失败退回原值并提示", async () => {
    updateBook.mockRejectedValueOnce(new Error("离线"));
    render(
      <>
        <BookRating book={{ ...book, rating: 1 }} />
        <ToastHost />
      </>,
    );

    fireEvent.click(screen.getByTestId("book-rating-star-5"));
    expect(screen.getByTestId("book-rating-label").textContent).toBe("5 星");

    await waitFor(() => expect(screen.getByTestId("toast").textContent).toContain("评分没能保存"));
    expect(screen.getByTestId("book-rating-label").textContent).toBe("1 星");
  });
});
