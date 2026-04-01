package norm

import (
	"testing"
)

func TestParseWhere(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		w := parseWhere("")
		if w != nil {
			t.Error("expected nil for empty where")
		}
	})

	t.Run("no placeholders", func(t *testing.T) {
		w := parseWhere("id=1")
		s, next := w.Build(1)
		if s != "id=1" {
			t.Errorf("got %q", s)
		}
		if next != 1 {
			t.Errorf("expected next=1, got %d", next)
		}
	})

	t.Run("single placeholder from 1", func(t *testing.T) {
		w := parseWhere("name = ?", "John")
		s, next := w.Build(1)
		if s != "name = $1" {
			t.Errorf("got %q", s)
		}
		if next != 2 {
			t.Errorf("expected next=2, got %d", next)
		}
		if len(w.Args) != 1 || w.Args[0] != "John" {
			t.Errorf("unexpected args: %v", w.Args)
		}
	})

	t.Run("multiple placeholders", func(t *testing.T) {
		w := parseWhere("age > ? AND name = ?", 18, "John")
		s, next := w.Build(1)
		if s != "age > $1 AND name = $2" {
			t.Errorf("got %q", s)
		}
		if next != 3 {
			t.Errorf("expected next=3, got %d", next)
		}
	})

	t.Run("start from offset", func(t *testing.T) {
		w := parseWhere("name = ? AND age > ?", "John", 18)
		s, next := w.Build(4)
		if s != "name = $4 AND age > $5" {
			t.Errorf("got %q", s)
		}
		if next != 6 {
			t.Errorf("expected next=6, got %d", next)
		}
	})
}

func TestComposeOptions(t *testing.T) {
	t.Run("empty options", func(t *testing.T) {
		co := ComposeOptions()
		if co.Exclude != nil || co.Fields != nil || co.Prefix != "" {
			t.Error("expected zero values")
		}
	})

	t.Run("exclude", func(t *testing.T) {
		co := ComposeOptions(Exclude("id,name"))
		if len(co.Exclude) != 2 {
			t.Fatalf("expected 2 excludes, got %d", len(co.Exclude))
		}
		if co.Exclude[0] != "id" || co.Exclude[1] != "name" {
			t.Errorf("unexpected excludes: %v", co.Exclude)
		}
	})

	t.Run("fields", func(t *testing.T) {
		co := ComposeOptions(Fields("id,name"))
		if len(co.Fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(co.Fields))
		}
	})

	t.Run("prefix", func(t *testing.T) {
		co := ComposeOptions(Prefix("u."))
		if co.Prefix != "u." {
			t.Errorf("expected prefix 'u.', got %q", co.Prefix)
		}
	})

	t.Run("where", func(t *testing.T) {
		co := ComposeOptions(Where("id = ?", 1))
		if co.Where == nil {
			t.Fatal("expected where to be set")
		}
		s, _ := co.Where.Build(1)
		if s != "id = $1" {
			t.Errorf("got %q", s)
		}
	})

	t.Run("offset and limit", func(t *testing.T) {
		co := ComposeOptions(Offset(10), Limit(20))
		if co.Offset != 10 {
			t.Errorf("expected offset 10, got %d", co.Offset)
		}
		if co.Limit != 20 {
			t.Errorf("expected limit 20, got %d", co.Limit)
		}
	})

	t.Run("add targets", func(t *testing.T) {
		var x, y int
		co := ComposeOptions(AddTargets(&x, &y))
		if len(co.AddTargets) != 2 {
			t.Errorf("expected 2 targets, got %d", len(co.AddTargets))
		}
	})

	t.Run("returning", func(t *testing.T) {
		co := ComposeOptions(Returning("id,name"))
		if len(co.Returning) != 2 {
			t.Fatalf("expected 2 returning fields, got %d", len(co.Returning))
		}
	})

	t.Run("combined options", func(t *testing.T) {
		co := ComposeOptions(
			Exclude("age"),
			Prefix("u."),
			Limit(10),
			Offset(5),
		)
		if len(co.Exclude) != 1 || co.Exclude[0] != "age" {
			t.Error("exclude not set correctly")
		}
		if co.Prefix != "u." {
			t.Error("prefix not set correctly")
		}
		if co.Limit != 10 || co.Offset != 5 {
			t.Error("limit/offset not set correctly")
		}
	})
}

func TestOptionTypes(t *testing.T) {
	tests := []struct {
		name     string
		opt      Option
		wantType OptionType
	}{
		{"exclude", Exclude("id"), ExcludeOption},
		{"fields", Fields("id"), FieldsOption},
		{"returning", Returning("id"), ReturningOption},
		{"prefix", Prefix("u."), PrefixOption},
		{"where", Where("id=1"), WhereOption},
		{"addTargets", AddTargets(new(int)), AddTargetsOption},
		{"offset", Offset(10), OffsetOption},
		{"limit", Limit(5), LimitOption},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opt.Type() != tt.wantType {
				t.Errorf("got type %d, want %d", tt.opt.Type(), tt.wantType)
			}
		})
	}
}
