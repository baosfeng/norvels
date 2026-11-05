package store

import (
	"errors"
	"testing"
	"time"
)

// TestVolumeTitleConflictMapping 撞 0013 唯一索引翻成 ErrVolumeNameTaken（api 映射 409）；
// 其它写错误返回 nil，交给调用方按原样包装，保留底层错误链。
func TestVolumeTitleConflictMapping(t *testing.T) {
	conflict := errors.New("UNIQUE constraint failed: " + volumeTitleUniqueIndex)
	got := volumeTitleConflict(conflict, "第一卷")
	if !errors.Is(got, ErrVolumeNameTaken) {
		t.Fatalf("撞唯一索引 → %v，want ErrVolumeNameTaken", got)
	}
	if other := volumeTitleConflict(errors.New("disk I/O error"), "第一卷"); other != nil {
		t.Fatalf("非唯一冲突应返回 nil，实际 %v", other)
	}
	if nilErr := volumeTitleConflict(nil, "第一卷"); nilErr != nil {
		t.Fatalf("nil 错误应返回 nil，实际 %v", nilErr)
	}
}

// TestWrapUserNameErrorMapping 撞唯一索引翻成 ErrConflict；其余错误保留底层链
// （并发建号的分支与普通写失败要能被上层区分）。
func TestWrapUserNameErrorMapping(t *testing.T) {
	conflict := wrapUserNameError(errors.New("UNIQUE constraint failed: users.name"), "读者甲")
	if !errors.Is(conflict, ErrConflict) {
		t.Fatalf("撞唯一索引 → %v，want ErrConflict", conflict)
	}
	boom := errors.New("disk I/O error")
	plain := wrapUserNameError(boom, "读者甲")
	if errors.Is(plain, ErrConflict) || !errors.Is(plain, boom) {
		t.Fatalf("普通写错误应保留底层链且不是 ErrConflict：%v", plain)
	}
}

// TestIsTagNameConflict 只认 tags.name 的唯一索引冲突，别把别的 UNIQUE 当标签重名。
func TestIsTagNameConflict(t *testing.T) {
	if !isTagNameConflict(errors.New("UNIQUE constraint failed: tags.name")) {
		t.Fatal("tags.name 冲突应判为标签重名")
	}
	if isTagNameConflict(errors.New("UNIQUE constraint failed: books.sha256")) {
		t.Fatal("别的唯一索引不该判成标签重名")
	}
	if isTagNameConflict(nil) {
		t.Fatal("nil 不是冲突")
	}
}

// TestRuneLenCountsRunes 字数按字符（rune）计，与 SQLite length(TEXT) 同口径，不是字节数。
func TestRuneLenCountsRunes(t *testing.T) {
	if got := runeLen("第一卷第"); got != 4 {
		t.Fatalf("runeLen(中文四字) = %d，want 4", got)
	}
	if got := runeLen("abc"); got != 3 {
		t.Fatalf("runeLen(abc) = %d，want 3", got)
	}
	if got := runeLen(""); got != 0 {
		t.Fatalf("runeLen(空) = %d，want 0", got)
	}
}

// TestBookQualityView 零值质量分是「没算过」而不是满分：展示形状必须带 computed 标志，
// 否则「零乱码零噪声」看起来像一份完美质量分。
func TestBookQualityView(t *testing.T) {
	zero := BookQuality{}.View()
	if zero.Computed {
		t.Fatal("零值质量分不该标成 computed")
	}
	// 半截状态（有章数没时间戳）同样按未计算处理，宁可走保守路径。
	half := BookQuality{Chapters: 3}.View()
	if half.Computed {
		t.Fatal("只有章数没有时间戳是半截状态，应判未计算")
	}
	saved := BookQuality{Chapters: 3, ComputedAt: time.Now()}.View()
	if !saved.Computed {
		t.Fatal("章数与时间戳齐备应标 computed")
	}
}

// TestCompareBookQualityExportedEntry 审核列表用的导出入口与导入判定同一份口径：
// 任一侧未计算都要留原因并拒绝判定（不拿半份分数做版本择优）。
func TestCompareBookQualityExportedEntry(t *testing.T) {
	computed := BookQuality{Chapters: 2, ComputedAt: time.Now()}
	if got := CompareBookQuality(BookQuality{}, computed); got.Upgrade || got.Note != VersionNoteOldUnknown {
		t.Fatalf("旧版本未计算 → upgrade=%v note=%q，want 保守路径", got.Upgrade, got.Note)
	}
	if got := CompareBookQuality(computed, BookQuality{}); got.Upgrade || got.Note != VersionNoteNewUnknown {
		t.Fatalf("新版本未计算 → upgrade=%v note=%q，want 保守路径", got.Upgrade, got.Note)
	}
	if got := CompareBookQuality(computed, computed); !got.OldComputed || !got.NewComputed || got.Upgrade {
		t.Fatalf("两侧相等不应判升级：%+v", got)
	}
}
