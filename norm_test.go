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
}

func TestM(t *testing.T) {
	n := NewNorm(nil)

	t.Run("auto table name", func(t *testing.T) {
		m, err := n.M(&TestUser{})
		if err != nil {
			t.Fatal(err)
		}
		if m.Table() != "test_user" {
			t.Errorf("expected table 'test_user', got %q", m.Table())
		}
	})

	t.Run("metadata cached on second call", func(t *testing.T) {
		m1, _ := n.M(&TestUser{})
		m2, _ := n.M(&TestUser{})
		// Different Model wrappers but same underlying metadata
		if m1.modelMeta != m2.modelMeta {
			t.Error("expected same modelMeta from cache")
		}
	})

	t.Run("error on non-pointer", func(t *testing.T) {
		_, err := n.M(TestUser{})
		if err == nil {
			t.Error("expected error for non-pointer")
		}
	})
}

func TestAddModel_InvalidTableName(t *testing.T) {
	n := NewNorm(nil)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid table name")
		}
	}()
	n.AddModel(&TestUser{}, "users; DROP TABLE")
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
