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
		model := NewModel(defaultConfig)
		err := model.Parse(42, "test")
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
	m := newTestModel()
	obj := &ModelTestStruct{Id: 1, Name: "John", Email: "john@test.com", Age: 30}

	t.Run("returns correct count", func(t *testing.T) {
		ptrs := m.Pointers(obj)
		if len(ptrs) != 4 {
			t.Fatalf("expected 4 pointers, got %d", len(ptrs))
		}
	})

	t.Run("pointers point to correct fields", func(t *testing.T) {
		ptrs := m.Pointers(obj)
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
		ptrs := m.Pointers(obj, Exclude("id"))
		if len(ptrs) != 3 {
			t.Errorf("expected 3 pointers, got %d", len(ptrs))
		}
	})

	t.Run("with add targets", func(t *testing.T) {
		var extra int
		ptrs := m.Pointers(obj, AddTargets(&extra))
		if len(ptrs) != 5 {
			t.Errorf("expected 5 pointers (4+1), got %d", len(ptrs))
		}
	})

	t.Run("panics on non-pointer", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic")
			}
		}()
		m.Pointers(ModelTestStruct{})
	})
}

func TestValues(t *testing.T) {
	m := newTestModel()
	obj := &ModelTestStruct{Id: 1, Name: "John", Email: "john@test.com", Age: 30}

	t.Run("returns correct values", func(t *testing.T) {
		vals := m.Values(obj)
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
		vals := m.Values(obj, Exclude("id"))
		if len(vals) != 3 {
			t.Fatalf("expected 3 values, got %d", len(vals))
		}
		if vals[0] != "John" {
			t.Errorf("expected first value to be Name, got %v", vals[0])
		}
	})

	t.Run("with fields filter", func(t *testing.T) {
		vals := m.Values(obj, Fields("id,name"))
		if len(vals) != 2 {
			t.Fatalf("expected 2 values, got %d", len(vals))
		}
		if vals[0] != 1 || vals[1] != "John" {
			t.Errorf("unexpected values: %v", vals)
		}
	})

	t.Run("panics on non-pointer", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic")
			}
		}()
		m.Values(ModelTestStruct{})
	})
}

func TestPointer(t *testing.T) {
	m := newTestModel()
	obj := &ModelTestStruct{Id: 42, Name: "Test"}

	t.Run("returns correct pointer", func(t *testing.T) {
		p := m.Pointer(obj, "Id")
		if *(p.(*int)) != 42 {
			t.Errorf("expected 42, got %v", *(p.(*int)))
		}
	})

	t.Run("panics on non-pointer", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic")
			}
		}()
		m.Pointer(ModelTestStruct{}, "Id")
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
		// "Name" -> lowerCamel -> "name"
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

	t.Run("pointers work with embedded fields", func(t *testing.T) {
		u := &User{BaseModel: BaseModel{Id: 42, CreatedAt: "2024-01-01"}, Name: "John", Email: "j@t.com"}
		vals := m.Values(u)
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
