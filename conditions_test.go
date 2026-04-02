package norm

import (
	"testing"
)

type CondTestStruct struct {
	Id     int     `norm:"pk"`
	Name   string  `norm:"notnull"`
	Age    int
	Active bool
	Score  float64
	Count  uint
}

func newCondTestModel() *Model {
	n := NewNorm(nil)
	m, err := n.M(&CondTestStruct{})
	if err != nil {
		panic(err)
	}
	return m
}

func TestBuildConditions_Eq(t *testing.T) {
	m := newCondTestModel()

	t.Run("string", func(t *testing.T) {
		conds, vals := m.BuildConditions(Eq("name", "John"))
		assertCond(t, conds, vals, "name=$1", "John")
	})

	t.Run("int", func(t *testing.T) {
		conds, vals := m.BuildConditions(Eq("age", 25))
		assertCond(t, conds, vals, "age=$1", 25)
	})

	t.Run("bool", func(t *testing.T) {
		conds, vals := m.BuildConditions(Eq("active", true))
		assertCond(t, conds, vals, "active=$1", true)
	})

	t.Run("float", func(t *testing.T) {
		conds, vals := m.BuildConditions(Eq("score", 9.5))
		assertCond(t, conds, vals, "score=$1", 9.5)
	})

	t.Run("uint", func(t *testing.T) {
		conds, vals := m.BuildConditions(Eq("count", uint(42)))
		assertCond(t, conds, vals, "count=$1", uint(42))
	})
}

func TestBuildConditions_Comparison(t *testing.T) {
	m := newCondTestModel()

	tests := []struct {
		name string
		cond Cond
		want string
	}{
		{"gt", Gt("age", 18), "age > $1"},
		{"gte", Gte("age", 18), "age >= $1"},
		{"lt", Lt("age", 65), "age < $1"},
		{"lte", Lte("age", 65), "age <= $1"},
		{"ne", Ne("age", 0), "age != $1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conds, vals := m.BuildConditions(tt.cond)
			if len(conds) != 1 || conds[0] != tt.want {
				t.Errorf("got %v, want %q", conds, tt.want)
			}
			if len(vals) != 1 {
				t.Errorf("expected 1 val, got %d", len(vals))
			}
		})
	}
}

func TestBuildConditions_Like(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(Like("name", "%John%"))
	assertCond(t, conds, vals, "name LIKE $1", "%John%")
}

func TestBuildConditions_IsNull(t *testing.T) {
	m := newCondTestModel()

	t.Run("is null", func(t *testing.T) {
		conds, vals := m.BuildConditions(IsNull("name", true))
		if len(conds) != 1 || conds[0] != "name IS NULL" {
			t.Errorf("got %v", conds)
		}
		if len(vals) != 0 {
			t.Errorf("expected 0 vals, got %d", len(vals))
		}
	})

	t.Run("is not null", func(t *testing.T) {
		conds, vals := m.BuildConditions(IsNull("name", false))
		if len(conds) != 1 || conds[0] != "name IS NOT NULL" {
			t.Errorf("got %v", conds)
		}
		if len(vals) != 0 {
			t.Errorf("expected 0 vals, got %d", len(vals))
		}
	})
}

func TestBuildConditions_In(t *testing.T) {
	m := newCondTestModel()

	t.Run("strings", func(t *testing.T) {
		conds, vals := m.BuildConditions(In("name", "John", "Jane"))
		if len(conds) != 1 || conds[0] != "name IN ($1, $2)" {
			t.Errorf("got %v", conds)
		}
		if len(vals) != 2 {
			t.Errorf("expected 2 vals, got %d", len(vals))
		}
	})

	t.Run("ints", func(t *testing.T) {
		conds, vals := m.BuildConditions(In("age", 18, 21, 25))
		if len(conds) != 1 || conds[0] != "age IN ($1, $2, $3)" {
			t.Errorf("got %v", conds)
		}
		if len(vals) != 3 {
			t.Errorf("expected 3 vals, got %d", len(vals))
		}
	})

	t.Run("floats", func(t *testing.T) {
		conds, vals := m.BuildConditions(In("score", 1.0, 2.5, 3.7))
		if len(conds) != 1 || conds[0] != "score IN ($1, $2, $3)" {
			t.Errorf("got %v", conds)
		}
		if len(vals) != 3 {
			t.Errorf("expected 3 vals, got %d", len(vals))
		}
	})
}

func TestBuildConditions_Prefix(t *testing.T) {
	m := newCondTestModel()
	conds, _ := m.BuildConditions(Eq("name", "John"), Prefix("u."))
	if len(conds) != 1 || conds[0] != "u.name=$1" {
		t.Errorf("got %v", conds)
	}
}

func TestBuildConditions_Multiple(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(
		Eq("name", "John"),
		Gte("age", 18),
		Lt("age", 65),
	)
	if len(conds) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(conds))
	}
	if conds[0] != "name=$1" {
		t.Errorf("cond[0] = %q", conds[0])
	}
	if conds[1] != "age >= $2" {
		t.Errorf("cond[1] = %q", conds[1])
	}
	if conds[2] != "age < $3" {
		t.Errorf("cond[2] = %q", conds[2])
	}
	if len(vals) != 3 {
		t.Errorf("expected 3 vals, got %d", len(vals))
	}
}

func TestBuildConditions_UnknownField(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions(Eq("nonexistent", "value"))
	if len(conds) != 0 {
		t.Errorf("expected 0 conditions, got %d", len(conds))
	}
	if len(vals) != 0 {
		t.Errorf("expected 0 vals, got %d", len(vals))
	}
}

func TestBuildConditions_JSONAccess(t *testing.T) {
	n := NewNorm(nil)

	type Doc struct {
		Id   int            `norm:"pk"`
		Data map[string]any `norm:"dbType=jsonb"`
	}

	m, _ := n.M(&Doc{})
	conds, vals := m.BuildConditions(Eq("data->>key", "value"))
	if len(conds) != 1 || conds[0] != "data->>key=$1" {
		t.Errorf("got %v", conds)
	}
	if len(vals) != 1 || vals[0] != "value" {
		t.Errorf("unexpected vals: %v", vals)
	}
}

func TestBuildConditions_Empty(t *testing.T) {
	m := newCondTestModel()
	conds, vals := m.BuildConditions()
	if len(conds) != 0 || len(vals) != 0 {
		t.Errorf("expected empty, got conds=%v vals=%v", conds, vals)
	}
}

// assertCond checks a single condition + single value result.
func assertCond(t *testing.T, conds []string, vals []any, wantCond string, wantVal any) {
	t.Helper()
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d: %v", len(conds), conds)
	}
	if conds[0] != wantCond {
		t.Errorf("condition = %q, want %q", conds[0], wantCond)
	}
	if len(vals) != 1 || vals[0] != wantVal {
		t.Errorf("vals = %v, want [%v]", vals, wantVal)
	}
}
