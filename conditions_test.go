package norm

import (
	"testing"
)

type CondTestStruct struct {
	Id   int    `norm:"pk"`
	Name string `norm:"notnull"`
	Age  int
}

func newCondTestModel() *Model {
	n := NewNorm(nil)
	return n.M(&CondTestStruct{})
}

func TestBuildConditions_StringEquality(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(map[string]any{
		"name": "John",
	}, "")

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	if conds[0] != "name=$1" {
		t.Errorf("got %q", conds[0])
	}
	if len(vals) != 1 || vals[0] != "John" {
		t.Errorf("unexpected vals: %v", vals)
	}
}

func TestBuildConditions_IntEquality(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(map[string]any{
		"age": 25,
	}, "")

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	if conds[0] != "age=$1" {
		t.Errorf("got %q", conds[0])
	}
	if len(vals) != 1 || vals[0] != 25 {
		t.Errorf("unexpected vals: %v", vals)
	}
}

func TestBuildConditions_WithPrefix(t *testing.T) {
	m := newCondTestModel()
	conds, _ := m.BuildConditions(map[string]any{
		"name": "John",
	}, "u.")

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	if conds[0] != "u.name=$1" {
		t.Errorf("got %q", conds[0])
	}
}

func TestBuildConditions_StringLike(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(map[string]any{
		"name": map[string]any{"like": "%John%"},
	}, "")

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	if conds[0] != "name LIKE $1" {
		t.Errorf("got %q", conds[0])
	}
	if vals[0] != "%John%" {
		t.Errorf("unexpected val: %v", vals[0])
	}
}

func TestBuildConditions_StringIsNull(t *testing.T) {
	m := newCondTestModel()

	t.Run("is null", func(t *testing.T) {
		conds, _ := m.BuildConditions(map[string]any{
			"name": map[string]any{"isNull": true},
		}, "")
		if len(conds) != 1 || conds[0] != "name IS NULL" {
			t.Errorf("got %v", conds)
		}
	})

	t.Run("is not null", func(t *testing.T) {
		conds, _ := m.BuildConditions(map[string]any{
			"name": map[string]any{"isNull": false},
		}, "")
		if len(conds) != 1 || conds[0] != "name IS NOT NULL" {
			t.Errorf("got %v", conds)
		}
	})
}

func TestBuildConditions_StringIn(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(map[string]any{
		"name": []any{"John", "Jane"},
	}, "")

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	if conds[0] != "name IN ($1, $2)" {
		t.Errorf("got %q", conds[0])
	}
	if len(vals) != 2 {
		t.Errorf("expected 2 vals, got %d", len(vals))
	}
}

func TestBuildConditions_IntComparison(t *testing.T) {
	m := newCondTestModel()

	tests := []struct {
		name string
		op   string
		want string
	}{
		{"greater than", "gt", ">"},
		{"greater or equal", "gte", ">="},
		{"less than", "lt", "<"},
		{"less or equal", "lte", "<="},
		{"not equal", "ne", "!="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conds, vals := m.BuildConditions(map[string]any{
				"age": map[string]any{tt.op: 18},
			}, "")
			if len(conds) != 1 {
				t.Fatalf("expected 1 condition, got %d", len(conds))
			}
			expected := "age " + tt.want + " $1"
			if conds[0] != expected {
				t.Errorf("got %q, want %q", conds[0], expected)
			}
			if vals[0] != 18 {
				t.Errorf("expected val 18, got %v", vals[0])
			}
		})
	}
}

func TestBuildConditions_IntIn(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(map[string]any{
		"age": []any{18, 21, 25},
	}, "")

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	if conds[0] != "age IN ($1, $2, $3)" {
		t.Errorf("got %q", conds[0])
	}
	if len(vals) != 3 {
		t.Errorf("expected 3 vals, got %d", len(vals))
	}
}

func TestBuildConditions_UnknownField(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(map[string]any{
		"nonexistent": "value",
	}, "")

	if len(conds) != 0 {
		t.Errorf("expected 0 conditions for unknown field, got %d", len(conds))
	}
	if len(vals) != 0 {
		t.Errorf("expected 0 vals, got %d", len(vals))
	}
}

func TestBuildConditions_IntIsNull(t *testing.T) {
	m := newCondTestModel()

	conds, _ := m.BuildConditions(map[string]any{
		"age": map[string]any{"isNull": true},
	}, "")
	if len(conds) != 1 || conds[0] != "age IS NULL" {
		t.Errorf("got %v", conds)
	}
}
