package norm

import (
	"testing"
)

type ModelTestStruct struct {
	Id    int    `norm:"pk"`
	Name  string `norm:"notnull"`
	Email string
	Age   int
	Skip  string `norm:"-"`
}

func newTestModel() *Model {
	n := NewNorm(nil)
	m, err := n.M(&ModelTestStruct{})
	if err != nil {
		panic(err)
	}
	return m
}

func TestParse(t *testing.T) {
	m := newTestModel()

	t.Run("correct table name", func(t *testing.T) {
		if m.Table() != "model_test_struct" {
			t.Errorf("expected 'model_test_struct', got %q", m.Table())
		}
	})

	t.Run("correct field count (skip excluded)", func(t *testing.T) {
		fields := m.FieldDescriptions()
		if len(fields) != 4 {
			t.Errorf("expected 4 fields, got %d", len(fields))
		}
	})

	t.Run("field db names are snake_case", func(t *testing.T) {
		fields := m.FieldDescriptions()
		expected := []string{"id", "name", "email", "age"}
		for i, f := range fields {
			if f.DbName() != expected[i] {
				t.Errorf("field %d: expected dbName %q, got %q", i, expected[i], f.DbName())
			}
		}
	})

	t.Run("parse non-struct returns error", func(t *testing.T) {
		meta := newModelMeta(defaultConfig)
		err := meta.Parse(42, "test")
		if err == nil {
			t.Error("expected error for non-struct")
		}
	})
}

func TestFields(t *testing.T) {
	m := newTestModel()

	t.Run("all fields", func(t *testing.T) {
		got := m.Fields()
		want := "id, name, email, age"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("with prefix", func(t *testing.T) {
		got := m.Fields(Prefix("u."))
		want := "u.id, u.name, u.email, u.age"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("with exclude", func(t *testing.T) {
		got := m.Fields(Exclude("id,age"))
		want := "name, email"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("with fields filter", func(t *testing.T) {
		got := m.Fields(Fields("id,name"))
		want := "id, name"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestUpdateFields(t *testing.T) {
	m := newTestModel()

	t.Run("all fields", func(t *testing.T) {
		got, nextBind := m.UpdateFields()
		want := "id=$1, name=$2, email=$3, age=$4"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if nextBind != 5 {
			t.Errorf("expected nextBind=5, got %d", nextBind)
		}
	})

	t.Run("with exclude", func(t *testing.T) {
		got, nextBind := m.UpdateFields(Exclude("id"))
		want := "name=$1, email=$2, age=$3"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if nextBind != 4 {
			t.Errorf("expected nextBind=4, got %d", nextBind)
		}
	})

	t.Run("with fields filter", func(t *testing.T) {
		got, nextBind := m.UpdateFields(Fields("name,email"))
		want := "name=$1, email=$2"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if nextBind != 3 {
			t.Errorf("expected nextBind=3, got %d", nextBind)
		}
	})
}

func TestModelBinds(t *testing.T) {
	m := newTestModel()

	t.Run("all fields", func(t *testing.T) {
		got := m.Binds()
		want := "$1, $2, $3, $4"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("with exclude", func(t *testing.T) {
		got := m.Binds(Exclude("id"))
		want := "$1, $2, $3"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestPointers(t *testing.T) {
	n := NewNorm(nil)
	obj := &ModelTestStruct{Id: 1, Name: "John", Email: "john@test.com", Age: 30}
	m, _ := n.M(obj)

	t.Run("returns correct count", func(t *testing.T) {
		ptrs := m.Pointers()
		if len(ptrs) != 4 {
			t.Fatalf("expected 4 pointers, got %d", len(ptrs))
		}
	})

	t.Run("pointers point to correct fields", func(t *testing.T) {
		ptrs := m.Pointers()
		// Writing through pointers should modify the original
		*(ptrs[0].(*int)) = 99
		if obj.Id != 99 {
			t.Error("pointer[0] should point to Id")
		}
		*(ptrs[1].(*string)) = "Jane"
		if obj.Name != "Jane" {
			t.Error("pointer[1] should point to Name")
		}
	})

	t.Run("with exclude", func(t *testing.T) {
		ptrs := m.Pointers(Exclude("id"))
		if len(ptrs) != 3 {
			t.Errorf("expected 3 pointers, got %d", len(ptrs))
		}
	})

	t.Run("with add targets", func(t *testing.T) {
		var extra int
		ptrs := m.Pointers(AddTargets(&extra))
		if len(ptrs) != 5 {
			t.Errorf("expected 5 pointers (4+1), got %d", len(ptrs))
		}
	})
}

func TestValues(t *testing.T) {
	n := NewNorm(nil)
	obj := &ModelTestStruct{Id: 1, Name: "John", Email: "john@test.com", Age: 30}
	m, _ := n.M(obj)

	t.Run("returns correct values", func(t *testing.T) {
		vals := m.Values()
		if len(vals) != 4 {
			t.Fatalf("expected 4 values, got %d", len(vals))
		}
		if vals[0] != 1 {
			t.Errorf("expected Id=1, got %v", vals[0])
		}
		if vals[1] != "John" {
			t.Errorf("expected Name=John, got %v", vals[1])
		}
		if vals[2] != "john@test.com" {
			t.Errorf("expected Email=john@test.com, got %v", vals[2])
		}
		if vals[3] != 30 {
			t.Errorf("expected Age=30, got %v", vals[3])
		}
	})

	t.Run("with exclude", func(t *testing.T) {
		vals := m.Values(Exclude("id"))
		if len(vals) != 3 {
			t.Fatalf("expected 3 values, got %d", len(vals))
		}
		if vals[0] != "John" {
			t.Errorf("expected first value to be Name, got %v", vals[0])
		}
	})

	t.Run("with fields filter", func(t *testing.T) {
		vals := m.Values(Fields("id,name"))
		if len(vals) != 2 {
			t.Fatalf("expected 2 values, got %d", len(vals))
		}
		if vals[0] != 1 || vals[1] != "John" {
			t.Errorf("unexpected values: %v", vals)
		}
	})
}

func TestPointer(t *testing.T) {
	n := NewNorm(nil)
	obj := &ModelTestStruct{Id: 42, Name: "Test"}
	m, _ := n.M(obj)

	t.Run("returns correct pointer", func(t *testing.T) {
		p := m.Pointer("Id")
		if *(p.(*int)) != 42 {
			t.Errorf("expected 42, got %v", *(p.(*int)))
		}
	})
}

func TestFieldByName(t *testing.T) {
	m := newTestModel()

	t.Run("by struct name", func(t *testing.T) {
		f, ok := m.FieldByName("Name")
		if !ok {
			t.Fatal("expected to find field")
		}
		if f.DbName() != "name" {
			t.Errorf("expected dbName 'name', got %q", f.DbName())
		}
	})

	t.Run("by db name", func(t *testing.T) {
		f, ok := m.FieldByName("email")
		if !ok {
			t.Fatal("expected to find field")
		}
		if f.Name() != "Email" {
			t.Errorf("expected name 'Email', got %q", f.Name())
		}
	})

	t.Run("by camelCase", func(t *testing.T) {
		f, ok := m.FieldByName("name")
		if !ok {
			t.Fatal("expected to find field")
		}
		if f.Name() != "Name" {
			t.Errorf("expected 'Name', got %q", f.Name())
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := m.FieldByName("nonexistent")
		if ok {
			t.Error("expected not found")
		}
	})
}

func TestReturning(t *testing.T) {
	m := newTestModel()

	t.Run("empty string", func(t *testing.T) {
		got := m.Returning("")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("single field", func(t *testing.T) {
		got := m.Returning("Id")
		if got != "RETURNING id" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("multiple fields", func(t *testing.T) {
		got := m.Returning("Id,Name")
		if got != "RETURNING id, name" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("by db name", func(t *testing.T) {
		got := m.Returning("email")
		if got != "RETURNING email" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("panics on unknown field", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic")
			}
		}()
		m.Returning("nonexistent")
	})
}

func TestLimitOffset(t *testing.T) {
	m := newTestModel()

	t.Run("no options", func(t *testing.T) {
		got := m.LimitOffset()
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("limit only", func(t *testing.T) {
		got := m.LimitOffset(Limit(10))
		if got != "LIMIT 10" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("offset only", func(t *testing.T) {
		got := m.LimitOffset(Offset(5))
		if got != "OFFSET 5" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("both", func(t *testing.T) {
		got := m.LimitOffset(Limit(10), Offset(20))
		if got != "LIMIT 10 OFFSET 20" {
			t.Errorf("got %q", got)
		}
	})
}

func TestOrderBy(t *testing.T) {
	m := newTestModel()

	t.Run("single field default ASC", func(t *testing.T) {
		got := m.OrderBy("Name")
		if got != "name ASC" {
			t.Errorf("got %q, want %q", got, "name ASC")
		}
	})

	t.Run("single field explicit DESC", func(t *testing.T) {
		got := m.OrderBy("Name DESC")
		if got != "name DESC" {
			t.Errorf("got %q, want %q", got, "name DESC")
		}
	})

	t.Run("multiple fields", func(t *testing.T) {
		got := m.OrderBy("Name ASC, Age DESC")
		if got != "name ASC, age DESC" {
			t.Errorf("got %q, want %q", got, "name ASC, age DESC")
		}
	})

	t.Run("by db name", func(t *testing.T) {
		got := m.OrderBy("email DESC")
		if got != "email DESC" {
			t.Errorf("got %q, want %q", got, "email DESC")
		}
	})

	t.Run("case insensitive direction", func(t *testing.T) {
		got := m.OrderBy("Name asc")
		if got != "name ASC" {
			t.Errorf("got %q, want %q", got, "name ASC")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		got := m.OrderBy("")
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("panics on unknown field", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for unknown field")
			}
		}()
		m.OrderBy("nonexistent ASC")
	})

	t.Run("panics on invalid direction", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for invalid direction")
			}
		}()
		m.OrderBy("Name SIDEWAYS")
	})

	t.Run("panics on invalid format", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for invalid format")
			}
		}()
		m.OrderBy("Name ASC extra")
	})
}

func TestEmbeddedStruct(t *testing.T) {
	type BaseModel struct {
		Id        int `norm:"pk"`
		CreatedAt string
	}
	type User struct {
		BaseModel
		Name  string
		Email string `norm:"unique"`
	}

	n := NewNorm(nil)
	m, err := n.M(&User{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("includes embedded fields", func(t *testing.T) {
		fields := m.FieldDescriptions()
		if len(fields) != 4 {
			names := make([]string, len(fields))
			for i, f := range fields {
				names[i] = f.Name()
			}
			t.Fatalf("expected 4 fields (Id, CreatedAt, Name, Email), got %d: %v", len(fields), names)
		}
	})

	t.Run("fields query includes embedded", func(t *testing.T) {
		got := m.Fields()
		want := "id, created_at, name, email"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("pk from embedded struct", func(t *testing.T) {
		sql := m.CreateTableSQL()
		if !contains(sql, "PRIMARY KEY(id)") {
			t.Errorf("missing pk from embedded struct in:\n%s", sql)
		}
	})

	t.Run("unique from outer struct", func(t *testing.T) {
		sql := m.CreateTableSQL()
		if !contains(sql, "UNIQUE(email)") {
			t.Errorf("missing unique from outer struct in:\n%s", sql)
		}
	})

	t.Run("values work with embedded fields", func(t *testing.T) {
		u := &User{BaseModel: BaseModel{Id: 42, CreatedAt: "2024-01-01"}, Name: "John", Email: "j@t.com"}
		m2, _ := n.M(u)
		vals := m2.Values()
		if vals[0] != 42 {
			t.Errorf("expected Id=42, got %v", vals[0])
		}
		if vals[1] != "2024-01-01" {
			t.Errorf("expected CreatedAt, got %v", vals[1])
		}
	})
}

func TestEmbeddedPointerStruct(t *testing.T) {
	type Base struct {
		Id int `norm:"pk"`
	}
	type Child struct {
		*Base
		Name string
	}

	n := NewNorm(nil)
	m, err := n.M(&Child{})
	if err != nil {
		t.Fatal(err)
	}

	fields := m.FieldDescriptions()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Name() != "Id" || fields[1].Name() != "Name" {
		t.Errorf("unexpected fields: %s, %s", fields[0].Name(), fields[1].Name())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNewInstance(t *testing.T) {
	m := newTestModel()
	inst := m.NewInstance()

	_, ok := inst.(*ModelTestStruct)
	if !ok {
		t.Errorf("expected *ModelTestStruct, got %T", inst)
	}
}

func TestFieldDescriptions(t *testing.T) {
	m := newTestModel()
	fields := m.FieldDescriptions()
	if len(fields) != 4 {
		t.Errorf("expected 4 fields, got %d", len(fields))
	}
}

func TestModelBoundToInstance(t *testing.T) {
	n := NewNorm(nil)
	user1 := &ModelTestStruct{Id: 1, Name: "Alice"}
	user2 := &ModelTestStruct{Id: 2, Name: "Bob"}

	m1, _ := n.M(user1)
	m2, _ := n.M(user2)

	t.Run("different models return different values", func(t *testing.T) {
		v1 := m1.Values()
		v2 := m2.Values()
		if v1[0] != 1 || v2[0] != 2 {
			t.Errorf("expected different Ids: got %v and %v", v1[0], v2[0])
		}
		if v1[1] != "Alice" || v2[1] != "Bob" {
			t.Errorf("expected different Names: got %v and %v", v1[1], v2[1])
		}
	})

	t.Run("modifying original struct reflects in model", func(t *testing.T) {
		user1.Name = "Charlie"
		vals := m1.Values()
		if vals[1] != "Charlie" {
			t.Errorf("expected Charlie, got %v", vals[1])
		}
	})
}
