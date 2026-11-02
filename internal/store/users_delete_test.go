package store

import (
	"context"
	"errors"
	"testing"
)

// 删账号的口径：删账号必须带走**它的**个人数据，且只带走它的；
// 默认账号（id=1）拒绝；不存在返回 ErrNotFound。

// seedUserData 给账号铺满各类个人数据，返回一本书的 id（书库那一侧不该被删）。
func seedUserData(t *testing.T, ctx context.Context, st *Store, userID int64) int64 {
	t.Helper()
	scoped := WithUserID(ctx, userID)
	bookID := seedSeriesBook(t, ctx, st, 1, "示例书甲", "", 0, 30)
	requireOK(t, "加入书架", st.AddToShelf(scoped, bookID))
	saveProgressFor(t, ctx, st, userID, bookID)
	addSessionFor(t, ctx, st, userID, bookID, 120)
	if _, _, err := st.AddBookmark(scoped, bookID, 1, 0, "读到这儿"); err != nil {
		t.Fatalf("建书签: %v", err)
	}
	if _, err := st.CreateCollection(scoped, "想读"); err != nil {
		t.Fatalf("建书单: %v", err)
	}
	if _, err := st.CreateUserSession(ctx, userID); err != nil {
		t.Fatalf("建会话: %v", err)
	}
	return bookID
}

// TestDeleteUserRemovesOwnData 删账号带走它的全部个人数据行。
func TestDeleteUserRemovesOwnData(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u := mustRegister(t, st, "读者甲")
	seedUserData(t, ctx, st, u.ID)

	requireOK(t, "DeleteUser", st.DeleteUser(ctx, u.ID))
	if _, err := st.UserByID(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("删后 UserByID = %v，期望 ErrNotFound", err)
	}
	for _, table := range userDataTables {
		var n int
		requireOK(t, "count "+table, st.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", u.ID).Scan(&n))
		requireEqual(t, table+" 残留行数", n, 0)
	}
}

// TestDeleteUserKeepsBooksAndOtherUsers 书库不动、别的账号的记录也不动。
func TestDeleteUserKeepsBooksAndOtherUsers(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	keeper := mustRegister(t, st, "读者甲")
	goner := mustRegister(t, st, "读者乙")
	bookID := seedUserData(t, ctx, st, keeper.ID)
	seedUserData(t, ctx, st, goner.ID)

	requireOK(t, "DeleteUser", st.DeleteUser(ctx, goner.ID))
	// 书还在（删账号不删书）
	if _, err := st.GetBook(ctx, bookID); err != nil {
		t.Errorf("删账号后书不见了: %v", err)
	}
	// 另一个账号的书架 / 书签 / 书单 / 会话都还在
	ids, err := st.ShelfBookIDs(WithUserID(ctx, keeper.ID))
	requireOK(t, "保留账号的书架", err)
	if len(ids) == 0 {
		t.Error("保留账号的书架被清空了")
	}
	if _, err := st.UserByID(ctx, keeper.ID); err != nil {
		t.Errorf("保留账号不见了: %v", err)
	}
}

// TestDeleteUserRejectsDefault 默认账号不能删（ErrDefaultUser），也不留下半删状态。
func TestDeleteUserRejectsDefault(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.DeleteUser(ctx, DefaultUserID()); !errors.Is(err, ErrDefaultUser) {
		t.Fatalf("删默认账号 = %v，期望 ErrDefaultUser", err)
	}
	if _, err := st.UserByID(ctx, DefaultUserID()); err != nil {
		t.Errorf("默认账号被删掉了: %v", err)
	}
}

// TestDeleteUserUnknownAndInvalid 不存在 404 / id 非法 400。
func TestDeleteUserUnknownAndInvalid(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.DeleteUser(ctx, 99999); !errors.Is(err, ErrNotFound) {
		t.Errorf("删不存在的账号 = %v，期望 ErrNotFound", err)
	}
	for _, id := range []int64{0, -1} {
		if err := st.DeleteUser(ctx, id); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("DeleteUser(%d) = %v，期望 ErrInvalidArgument", id, err)
		}
	}
}

// TestEnterUserCreatesThenLogsIn 单入口：第一次建号（created），第二次直接进（不建新号）。
func TestEnterUserCreatesThenLogsIn(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, created, err := st.EnterUser(ctx, " 读者甲 ")
	requireOK(t, "EnterUser(首次)", err)
	if !created {
		t.Error("首次进入应是新建")
	}
	requireEqual(t, "去空白后的账号名", first.Name, "读者甲")
	if first.LastSeenAt == "" {
		t.Error("进入后 lastSeenAt 仍为空（账号列表会显示「从未登录」）")
	}

	again, created, err := st.EnterUser(ctx, "读者甲")
	requireOK(t, "EnterUser(再次)", err)
	if created {
		t.Error("同名再次进入不应新建账号")
	}
	requireEqual(t, "命中同一账号", again.ID, first.ID)
	// 回显的时间必须是落库的那个值
	read, err := st.UserByID(ctx, first.ID)
	requireOK(t, "UserByID", err)
	requireEqual(t, "落库的 lastSeenAt", read.LastSeenAt, again.LastSeenAt)

	if _, _, err := st.EnterUser(ctx, "  "); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空名进入 = %v，期望 ErrInvalidArgument", err)
	}
}

// TestEnterUserIsNotCaseSensitive 单入口沿用 TRIM + 大小写折叠：alice 不会新建第二个号。
func TestEnterUserIsNotCaseSensitive(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, _, err := st.EnterUser(ctx, "Alice")
	requireOK(t, "EnterUser(Alice)", err)
	again, created, err := st.EnterUser(ctx, " alice ")
	requireOK(t, "EnterUser(alice)", err)
	if created {
		t.Error("大小写折叠后是同一个账号，不该新建")
	}
	requireEqual(t, "命中的 id", again.ID, first.ID)
}

// TestRegisterDoesNotFakeLastSeen 只建号（不建会话）不算一次登录。
//
// 账号列表要能区分「建了号却从没进来过」；API 层的注册会另走一次 TouchUser，
// 那是「注册即登录」的产物，不是建号本身的副作用。
func TestRegisterDoesNotFakeLastSeen(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u, err := st.RegisterUser(ctx, "读者甲")
	requireOK(t, "RegisterUser", err)
	requireEqual(t, "只建号时的 lastSeenAt", u.LastSeenAt, "")

	touched, err := st.TouchUser(ctx, u.ID)
	requireOK(t, "TouchUser", err)
	if touched.LastSeenAt == "" {
		t.Fatal("TouchUser 后 lastSeenAt 仍为空")
	}
	read, err := st.UserByID(ctx, u.ID)
	requireOK(t, "UserByID", err)
	requireEqual(t, "落库的 lastSeenAt", read.LastSeenAt, touched.LastSeenAt)
}
