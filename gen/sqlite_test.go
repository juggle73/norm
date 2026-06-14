package gen

import (
	"strings"
	"testing"

	"github.com/juggle73/norm/v4"
)

func TestGeneratorSQLite(t *testing.T) {
	g := NewGenerator(norm.SQLite)

	t.Run("type mapping", func(t *testing.T) {
		cols := []Col{
			{Name: "id", DataType: "INTEGER", IsPK: true},
			{Name: "name", DataType: "TEXT"},
			{Name: "price", DataType: "REAL"},
			{Name: "active", DataType: "BOOLEAN"},
			{Name: "created_at", DataType: "TIMESTAMP"},
			{Name: "data", DataType: "BLOB"},
			{Name: "nick", DataType: "VARCHAR(255)", IsNullable: true},
		}
		src := g.Gen("models", "Account", cols)

		for _, want := range []string{
			"package models",
			"type Account struct {",
			"Id int64",
			"Name string",
			"Price float64",
			"Active bool",
			"CreatedAt time.Time",
			"Data []byte",
			"*string", // nullable VARCHAR → pointer string
			`"time"`,  // time import present
		} {
			if !strings.Contains(src, want) {
				t.Errorf("expected source to contain %q, got:\n%s", want, src)
			}
		}
	})

	t.Run("blob is not pointer when nullable", func(t *testing.T) {
		cols := []Col{{Name: "blob", DataType: "BLOB", IsNullable: true}}
		src := g.Gen("models", "B", cols)
		if strings.Contains(src, "*[]byte") {
			t.Errorf("[]byte should not be a pointer, got:\n%s", src)
		}
		if !strings.Contains(src, "Blob []byte") {
			t.Errorf("missing Blob []byte, got:\n%s", src)
		}
	})

	t.Run("norm tags", func(t *testing.T) {
		cols := []Col{
			{Name: "id", DataType: "INTEGER", IsPK: true},
			{Name: "email", DataType: "TEXT", IsUnique: true},
			{Name: "user_id", DataType: "INTEGER", FK: "users"},
		}
		src := g.Gen("models", "Order", cols)
		for _, want := range []string{`norm:"pk,notnull"`, `norm:"notnull,unique"`, `fk=Users`} {
			if !strings.Contains(src, want) {
				t.Errorf("expected %q, got:\n%s", want, src)
			}
		}
	})
}

// TestPackageGenStillPostgres confirms the package-level Gen keeps PostgreSQL
// type mapping (backward compatibility).
func TestPackageGenStillPostgres(t *testing.T) {
	cols := []Col{{Name: "id", DataType: "bigint", IsPK: true}}
	src := Gen("models", "User", cols)
	if !strings.Contains(src, "Id int64") {
		t.Errorf("package Gen should map bigint→int64, got:\n%s", src)
	}
}
