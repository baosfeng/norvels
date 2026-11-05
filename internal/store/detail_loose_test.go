package store

import (
	"context"
	"strings"
	"testing"
)

// TestGetBookLooseChaptersAndEmptyTags 没有分卷的书，散章要显式归进一个「未分卷」虚拟卷
// （ID=0 / Idx=-1），前端才能按统一结构渲染；无标签时 Tags 返回空切片而不是 nil，
// 前端直接 map 不会炸。两张表都为空时也不该把 Volumes 留成 nil。
func TestGetBookLooseChaptersAndEmptyTags(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res, err := st.SaveBook(ctx, sampleBook("散章之书", "示例作者", "/path/to/loose.txt", strings.Repeat("d", 64)), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	d, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if d.Tags == nil {
		t.Fatal("无标签时 Tags 应是空切片而不是 nil")
	}
	var loose *VolumeNode
	for i := range d.Volumes {
		if d.Volumes[i].ID == 0 && d.Volumes[i].Idx == -1 {
			loose = &d.Volumes[i]
			break
		}
	}
	if loose == nil {
		t.Fatalf("散章应归入「未分卷」虚拟卷（id 0 / idx -1），实际卷：%+v", d.Volumes)
	}
	if len(loose.Chapters) == 0 {
		t.Fatal("未分卷虚拟卷应包含散章")
	}
	if loose.Title == "" {
		t.Fatal("未分卷虚拟卷要有标题，前端按标题渲染分组")
	}
}
