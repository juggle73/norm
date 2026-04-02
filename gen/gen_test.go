package gen

import (
	"strings"
	"testing"
)

func TestGen(t *testing.T) {
	t.Run("basic struct", func(t *testing.T) {
		cols := []Col{
			{Name: "id", DataType: "bigint", IsPK: true},
			{Name: "name", DataType: "text"},
			{Name: "email", DataType: "character varying", IsNullable: true},
		}
		src := Gen("models", "User", cols)

		if !strings.Contains(src, "package models") {
			t.Error("missing package declaration")
		}
		if !strings.Contains(src, "type User struct {") {
			t.Error("missing struct declaration")
		}
		if !strings.Contains(src, `Id int64`) {
			t.Errorf("missing Id field, got:\n%s", src)
		}
		if !strings.Contains(src, `norm:"pk,notnull"`) {
			t.Errorf("missing pk,notnull tag, got:\n%s", src)
		}
		if !strings.Contains(src, `*string`) {
			t.Errorf("nullable string should be pointer, got:\n%s", src)
		}
	})

	t.Run("time import", func(t *testing.T) {
		cols := []Col{
			{Name: "created_at", DataType: "timestamp with time zone"},
		}
		src := Gen("models", "Event", cols)

		if !strings.Contains(src, `"time"`) {
			t.Error("missing time import")
		}
		if !strings.Contains(src, "time.Time") {
			t.Error("missing time.Time type")
		}
	})

	t.Run("norm tags", func(t *testing.T) {
		cols := []Col{
			{Name: "id", DataType: "integer", IsPK: true},
			{Name: "email", DataType: "text", IsUnique: true},
			{Name: "user_id", DataType: "integer", FK: "users"},
		}
		src := Gen("models", "Order", cols)

		if !strings.Contains(src, `norm:"pk,notnull"`) {
			t.Errorf("missing pk tag, got:\n%s", src)
		}
		if !strings.Contains(src, `norm:"notnull,unique"`) {
			t.Errorf("missing unique tag, got:\n%s", src)
		}
		if !strings.Contains(src, `norm:"notnull,fk=Users"`) {
			t.Errorf("missing fk tag, got:\n%s", src)
		}
	})

	t.Run("json and bytea not pointer", func(t *testing.T) {
		cols := []Col{
			{Name: "data", DataType: "jsonb", IsNullable: true},
			{Name: "content", DataType: "bytea", IsNullable: true},
		}
		src := Gen("models", "Doc", cols)

		if strings.Contains(src, "*map") {
			t.Error("jsonb should not have pointer prefix")
		}
		if strings.Contains(src, "*[]byte") {
			t.Error("bytea should not have pointer prefix")
		}
	})

	t.Run("all numeric types", func(t *testing.T) {
		cols := []Col{
			{Name: "a", DataType: "smallint"},
			{Name: "b", DataType: "integer"},
			{Name: "c", DataType: "bigint"},
			{Name: "d", DataType: "real"},
			{Name: "e", DataType: "double precision"},
			{Name: "f", DataType: "numeric"},
			{Name: "g", DataType: "boolean"},
		}
		src := Gen("models", "Numbers", cols)

		expects := []string{"int16", "int", "int64", "float32", "float64", "float64", "bool"}
		for _, e := range expects {
			if !strings.Contains(src, e) {
				t.Errorf("missing type %s in:\n%s", e, src)
			}
		}
	})

	t.Run("uuid and inet map to string", func(t *testing.T) {
		cols := []Col{
			{Name: "uid", DataType: "uuid"},
			{Name: "ip", DataType: "inet"},
		}
		src := Gen("models", "Net", cols)

		// Both fields should be string
		count := strings.Count(src, "string")
		if count != 2 {
			t.Errorf("expected 2 string fields, got %d in:\n%s", count, src)
		}
	})

	t.Run("unknown type is skipped", func(t *testing.T) {
		cols := []Col{
			{Name: "id", DataType: "integer"},
			{Name: "geo", DataType: "geometry"},
		}
		src := Gen("models", "Place", cols)

		if strings.Contains(src, "Geo") {
			t.Errorf("unknown type should be skipped, got:\n%s", src)
		}
	})

	t.Run("notnull tag for non-nullable", func(t *testing.T) {
		cols := []Col{
			{Name: "name", DataType: "text"},
		}
		src := Gen("models", "Thing", cols)

		if !strings.Contains(src, `norm:"notnull"`) {
			t.Errorf("non-nullable field should have notnull tag, got:\n%s", src)
		}
	})

	t.Run("nullable field has no notnull tag", func(t *testing.T) {
		cols := []Col{
			{Name: "name", DataType: "text", IsNullable: true},
		}
		src := Gen("models", "Thing", cols)

		if strings.Contains(src, `notnull`) {
			t.Errorf("nullable field should not have notnull tag, got:\n%s", src)
		}
	})
}
