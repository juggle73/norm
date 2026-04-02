package norm

import (
	"testing"
)

type JoinUser struct {
	Id    int    `norm:"pk"`
	Name  string
	Email string
}

type JoinOrder struct {
	Id     int `norm:"pk"`
	UserId int
	Total  int
}

type JoinOrderItem struct {
	Id      int `norm:"pk"`
	OrderId int
	Product string
}

func setupJoinModels(t *testing.T) (*Model, *Model, *Model) {
	t.Helper()
	n := NewNorm(nil)

	user := &JoinUser{Id: 1, Name: "Alice", Email: "alice@test.com"}
	order := &JoinOrder{Id: 10, UserId: 1, Total: 100}
	item := &JoinOrderItem{Id: 100, OrderId: 10, Product: "Widget"}

	mUser, err := n.M(user)
	if err != nil {
		t.Fatal(err)
	}
	mOrder, err := n.M(order)
	if err != nil {
		t.Fatal(err)
	}
	mItem, err := n.M(item)
	if err != nil {
		t.Fatal(err)
	}

	return mUser, mOrder, mItem
}

func TestJoinInner(t *testing.T) {
	mUser, mOrder, _ := setupJoinModels(t)

	t.Run("basic inner join", func(t *testing.T) {
		sql, args, err := NewJoin(mUser).
			Inner(mOrder, "join_order.user_id = join_user.id").
			Select()
		if err != nil {
			t.Fatal(err)
		}
		want := "SELECT join_user.id, join_user.name, join_user.email, join_order.id, join_order.user_id, join_order.total FROM join_user INNER JOIN join_order ON join_order.user_id = join_user.id"
		if sql != want {
			t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
		}
		if len(args) != 0 {
			t.Errorf("expected no args, got %v", args)
		}
	})

	t.Run("with where", func(t *testing.T) {
		sql, args, err := NewJoin(mUser).
			Inner(mOrder, "join_order.user_id = join_user.id").
			Where("join_user.id = ?", 1).
			Select()
		if err != nil {
			t.Fatal(err)
		}
		want := "SELECT join_user.id, join_user.name, join_user.email, join_order.id, join_order.user_id, join_order.total FROM join_user INNER JOIN join_order ON join_order.user_id = join_user.id WHERE join_user.id = $1"
		if sql != want {
			t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
		}
		if len(args) != 1 || args[0] != 1 {
			t.Errorf("unexpected args: %v", args)
		}
	})

	t.Run("with order limit offset", func(t *testing.T) {
		sql, _, err := NewJoin(mUser).
			Inner(mOrder, "join_order.user_id = join_user.id").
			Order("join_user.name DESC").
			Limit(10).
			Offset(20).
			Select()
		if err != nil {
			t.Fatal(err)
		}
		want := "SELECT join_user.id, join_user.name, join_user.email, join_order.id, join_order.user_id, join_order.total FROM join_user INNER JOIN join_order ON join_order.user_id = join_user.id ORDER BY join_user.name DESC LIMIT 10 OFFSET 20"
		if sql != want {
			t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
		}
	})
}

func TestJoinLeft(t *testing.T) {
	mUser, mOrder, _ := setupJoinModels(t)

	sql, _, err := NewJoin(mUser).
		Left(mOrder, "join_order.user_id = join_user.id").
		Select()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT join_user.id, join_user.name, join_user.email, join_order.id, join_order.user_id, join_order.total FROM join_user LEFT JOIN join_order ON join_order.user_id = join_user.id"
	if sql != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
	}
}

func TestJoinRight(t *testing.T) {
	mUser, mOrder, _ := setupJoinModels(t)

	sql, _, err := NewJoin(mUser).
		Right(mOrder, "join_order.user_id = join_user.id").
		Select()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT join_user.id, join_user.name, join_user.email, join_order.id, join_order.user_id, join_order.total FROM join_user RIGHT JOIN join_order ON join_order.user_id = join_user.id"
	if sql != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
	}
}

func TestJoinMultiple(t *testing.T) {
	mUser, mOrder, mItem := setupJoinModels(t)

	sql, args, err := NewJoin(mUser).
		Inner(mOrder, "join_order.user_id = join_user.id").
		Left(mItem, "join_order_item.order_id = join_order.id").
		Where("join_user.id = ?", 1).
		Select()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT join_user.id, join_user.name, join_user.email, join_order.id, join_order.user_id, join_order.total, join_order_item.id, join_order_item.order_id, join_order_item.product FROM join_user INNER JOIN join_order ON join_order.user_id = join_user.id LEFT JOIN join_order_item ON join_order_item.order_id = join_order.id WHERE join_user.id = $1"
	if sql != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
	}
	if len(args) != 1 || args[0] != 1 {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestJoinPointers(t *testing.T) {
	mUser, mOrder, _ := setupJoinModels(t)

	j := NewJoin(mUser).
		Inner(mOrder, "join_order.user_id = join_user.id")

	ptrs := j.Pointers()

	// 3 fields from user + 3 fields from order = 6
	if len(ptrs) != 6 {
		t.Fatalf("expected 6 pointers, got %d", len(ptrs))
	}
}

func TestJoinPointersMultiple(t *testing.T) {
	mUser, mOrder, mItem := setupJoinModels(t)

	j := NewJoin(mUser).
		Inner(mOrder, "join_order.user_id = join_user.id").
		Left(mItem, "join_order_item.order_id = join_order.id")

	ptrs := j.Pointers()

	// 3 + 3 + 3 = 9
	if len(ptrs) != 9 {
		t.Fatalf("expected 9 pointers, got %d", len(ptrs))
	}
}

func TestJoinNoJoins(t *testing.T) {
	mUser, _, _ := setupJoinModels(t)

	sql, _, err := NewJoin(mUser).Select()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT join_user.id, join_user.name, join_user.email FROM join_user"
	if sql != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
	}
}

func TestJoinWhereMultipleArgs(t *testing.T) {
	mUser, mOrder, _ := setupJoinModels(t)

	sql, args, err := NewJoin(mUser).
		Inner(mOrder, "join_order.user_id = join_user.id").
		Where("join_user.name = ? AND join_order.total > ?", "Alice", 50).
		Select()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT join_user.id, join_user.name, join_user.email, join_order.id, join_order.user_id, join_order.total FROM join_user INNER JOIN join_order ON join_order.user_id = join_user.id WHERE join_user.name = $1 AND join_order.total > $2"
	if sql != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", sql, want)
	}
	if len(args) != 2 || args[0] != "Alice" || args[1] != 50 {
		t.Errorf("unexpected args: %v", args)
	}
}
