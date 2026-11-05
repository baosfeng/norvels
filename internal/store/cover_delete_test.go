package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestDeleteCover 删封面：真的从库里删掉（删后 GetCover 报 ErrNotFound）；重复删幂等
// （管理页「删除封面」点两次不该报错）；书不存在报 ErrNotFound；非法 id 报 ErrInvalidArgument。
func TestDeleteCover(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "删封面之书", "/path/to/del.txt", strings.Repeat("e", 64))
	if err := st.SaveCover(ctx, bookID, "image/png", fakePNG); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	if err := st.DeleteCover(ctx, bookID); err != nil {
		t.Fatalf("DeleteCover: %v", err)
	}
	if _, err := st.GetCover(ctx, bookID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删封面后 GetCover err = %v，want ErrNotFound", err)
	}
	if err := st.DeleteCover(ctx, bookID); err != nil {
		t.Fatalf("重复 DeleteCover 应幂等，err = %v", err)
	}
	if err := st.DeleteCover(ctx, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("DeleteCover(0) err = %v，want ErrInvalidArgument", err)
	}
	if err := st.DeleteCover(ctx, bookID+1000); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteCover(不存在的书) err = %v，want ErrNotFound", err)
	}
}
