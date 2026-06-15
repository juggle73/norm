package gen

import (
	"strings"
	"testing"

	"github.com/juggle73/norm/v4"
)

func TestGeneratorMySQL(t *testing.T) {
	g := NewGenerator(norm.MySQL)

	t.Run("type mapping", func(t *testing.T) {
		cols := []Col{
			{Name: "id", DataType: "int(11)", IsPK: true},
			{Name: "name", DataType: "varchar(255)"},
			{Name: "price", DataType: "double"},
			{Name: "active", DataType: "tinyint(1)"},
			{Name: "rating", DataType: "tinyint"},
			{Name: "created_at", DataType: "datetime"},
			{Name: "data", DataType: "longblob"},
			{Name: "meta", DataType: "json"},
			{Name: "nick", DataType: "varchar(50)", IsNullable: true},
		}
		src := g.Gen("models", "Account", cols)

		for _, want := range []string{
			"package models",
			"type Account struct {",
			"Id int",
			"Name string",
			"Price float64",
			"Active bool",      // tinyint(1) → bool
			"Rating int8",      // tinyint → int8
			"CreatedAt time.Time",
			"Data []byte",
			"Meta map[string]any",
			"*string", // nullable varchar → pointer string
			`"time"`,
		} {
			if !strings.Contains(src, want) {
				t.Errorf("expected source to contain %q, got:\n%s", want, src)
			}
		}
	})

	t.Run("norm tags", func(t *testing.T) {
		cols := []Col{
			{Name: "id", DataType: "int", IsPK: true},
			{Name: "email", DataType: "varchar(255)", IsUnique: true},
			{Name: "user_id", DataType: "int", FK: "users"},
		}
		src := g.Gen("models", "Order", cols)
		for _, want := range []string{`norm:"pk,notnull"`, `norm:"notnull,unique"`, `fk=Users`} {
			if !strings.Contains(src, want) {
				t.Errorf("expected %q, got:\n%s", want, src)
			}
		}
	})
}
