package norm

import (
	"testing"
)

type TestUser struct {
	Id   int    `norm:"pk"`
	Name string `norm:"notnull"`
	Age  int
}

func TestNewNorm(t *testing.T) {
	t.Run("nil config uses default", func(t *testing.T) {
		n := NewNorm(nil)
		if n.config.DefaultString != "text" {
			t.Errorf("expected default string 'text', got %q", n.config.DefaultString)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		n := NewNorm(&Config{DefaultString: "varchar"})
		if n.config.DefaultString != "varchar" {
			t.Errorf("expected 'varchar', got %q", n.config.DefaultString)
		}
	})
}

func TestAddModel(t *testing.T) {
	n := NewNorm(nil)
	m := n.AddModel(&TestUser{}, "users")

	if m.Table() != "users" {
		t.Errorf("expected table 'users', got %q", m.Table())
	}

	// Should be cached
	m2 := n.T("users")
	if m2 != m {
		t.Error("expected same model from cache")
	}
}

func TestM(t *testing.T) {
	n := NewNorm(nil)

	t.Run("auto table name", func(t *testing.T) {
		m := n.M(&TestUser{})
		if m.Table() != "test_user" {
			t.Errorf("expected table 'test_user', got %q", m.Table())
		}
	})

	t.Run("cached on second call", func(t *testing.T) {
		m1 := n.M(&TestUser{})
		m2 := n.M(&TestUser{})
		if m1 != m2 {
			t.Error("expected same model pointer from cache")
		}
	})

	t.Run("panics on non-pointer", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for non-pointer")
			}
		}()
		n.M(TestUser{})
	})
}

func TestT(t *testing.T) {
	n := NewNorm(nil)

	t.Run("not found returns nil", func(t *testing.T) {
		if n.T("nonexistent") != nil {
			t.Error("expected nil for unknown table")
		}
	})

	t.Run("found after add", func(t *testing.T) {
		n.AddModel(&TestUser{}, "my_users")
		m := n.T("my_users")
		if m == nil {
			t.Fatal("expected model, got nil")
		}
		if m.Table() != "my_users" {
			t.Errorf("expected 'my_users', got %q", m.Table())
		}
	})
}

func TestTables(t *testing.T) {
	n := NewNorm(nil)

	type Product struct {
		Id   int `norm:"pk"`
		Name string
	}

	n.AddModel(&TestUser{}, "users")
	n.AddModel(&Product{}, "products")

	tables := n.Tables()
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}

	found := make(map[string]bool)
	for _, tbl := range tables {
		found[tbl] = true
	}
	if !found["users"] || !found["products"] {
		t.Errorf("expected users and products, got %v", tables)
	}
}
