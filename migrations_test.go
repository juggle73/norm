package norm

import (
	"strings"
	"testing"
)

type MigrationTestStruct struct {
	Id    int    `norm:"pk,notnull"`
	Name  string `norm:"notnull,unique"`
	Email string `norm:"default=''"`
	Age   int
	Active bool
}

func TestCreateTableSQL(t *testing.T) {
	n := NewNorm(nil)
	m := n.AddModel(&MigrationTestStruct{}, "users")

	sql := m.CreateTableSQL()

	t.Run("contains CREATE TABLE", func(t *testing.T) {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS users") {
			t.Errorf("missing CREATE TABLE, got:\n%s", sql)
		}
	})

	t.Run("contains fields", func(t *testing.T) {
		if !strings.Contains(sql, "id integer NOT NULL") {
			t.Errorf("missing 'id integer NOT NULL' in:\n%s", sql)
		}
		if !strings.Contains(sql, "name text NOT NULL") {
			t.Errorf("missing 'name text NOT NULL' in:\n%s", sql)
		}
		if !strings.Contains(sql, "email text DEFAULT ''") {
			t.Errorf("missing 'email text DEFAULT ''' in:\n%s", sql)
		}
		if !strings.Contains(sql, "age integer") {
			t.Errorf("missing 'age integer' in:\n%s", sql)
		}
		if !strings.Contains(sql, "active boolean") {
			t.Errorf("missing 'active boolean' in:\n%s", sql)
		}
	})

	t.Run("contains primary key", func(t *testing.T) {
		if !strings.Contains(sql, "CONSTRAINT users_pkey PRIMARY KEY(id)") {
			t.Errorf("missing PRIMARY KEY in:\n%s", sql)
		}
	})

	t.Run("contains unique", func(t *testing.T) {
		if !strings.Contains(sql, "CONSTRAINT unique_users_name UNIQUE(name)") {
			t.Errorf("missing UNIQUE in:\n%s", sql)
		}
	})
}

func TestCreateTableSQL_CustomDbType(t *testing.T) {
	type Custom struct {
		Id   int `norm:"pk,dbType=bigint"`
		Data string `norm:"dbType=jsonb"`
	}

	n := NewNorm(nil)
	m := n.AddModel(&Custom{}, "custom")
	sql := m.CreateTableSQL()

	if !strings.Contains(sql, "id bigint") {
		t.Errorf("missing custom dbType for id:\n%s", sql)
	}
	if !strings.Contains(sql, "data jsonb") {
		t.Errorf("missing custom dbType for data:\n%s", sql)
	}
}

func TestMigrate_NewTable(t *testing.T) {
	n := NewNorm(nil)
	m := n.AddModel(&MigrationTestStruct{}, "users")

	stmts := m.Migrate(nil)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement for new table, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "CREATE TABLE") {
		t.Errorf("expected CREATE TABLE, got:\n%s", stmts[0])
	}
}

func TestMigrate_EmptyFieldNames(t *testing.T) {
	n := NewNorm(nil)
	m := n.AddModel(&MigrationTestStruct{}, "users")

	stmts := m.Migrate([]string{})
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "CREATE TABLE") {
		t.Errorf("expected CREATE TABLE, got:\n%s", stmts[0])
	}
}

func TestMigrate_AddMissingFields(t *testing.T) {
	n := NewNorm(nil)
	m := n.AddModel(&MigrationTestStruct{}, "users")

	// Table already has id and name, missing email, age, active
	stmts := m.Migrate([]string{"id", "name"})

	hasAlterEmail := false
	hasAlterAge := false
	hasAlterActive := false
	for _, s := range stmts {
		if strings.Contains(s, "ALTER TABLE users ADD email") {
			hasAlterEmail = true
		}
		if strings.Contains(s, "ALTER TABLE users ADD age") {
			hasAlterAge = true
		}
		if strings.Contains(s, "ALTER TABLE users ADD active") {
			hasAlterActive = true
		}
	}

	if !hasAlterEmail {
		t.Error("missing ALTER TABLE for email")
	}
	if !hasAlterAge {
		t.Error("missing ALTER TABLE for age")
	}
	if !hasAlterActive {
		t.Error("missing ALTER TABLE for active")
	}
}

func TestMigrate_AllFieldsExist(t *testing.T) {
	n := NewNorm(nil)
	m := n.AddModel(&MigrationTestStruct{}, "users")

	stmts := m.Migrate([]string{"id", "name", "email", "age", "active"})

	// Only pk and unique constraints may be added
	for _, s := range stmts {
		if strings.Contains(s, "ADD id") || strings.Contains(s, "ADD name") {
			t.Errorf("should not add existing fields: %s", s)
		}
	}
}

func TestCreateTableSQL_UintTypes(t *testing.T) {
	type WithUints struct {
		Id    int    `norm:"pk"`
		Count uint
		Big   uint64
	}

	n := NewNorm(nil)
	m := n.AddModel(&WithUints{}, "with_uints")
	sql := m.CreateTableSQL()

	if !strings.Contains(sql, "count integer") {
		t.Errorf("missing uint type:\n%s", sql)
	}
	if !strings.Contains(sql, "big bigint") {
		t.Errorf("missing uint64 type:\n%s", sql)
	}
}

func TestCreateTableSQL_FloatTypes(t *testing.T) {
	type WithFloats struct {
		Id    int     `norm:"pk"`
		Score float64
		Rate  float32
	}

	n := NewNorm(nil)
	m := n.AddModel(&WithFloats{}, "with_floats")
	sql := m.CreateTableSQL()

	if !strings.Contains(sql, "score double precision") {
		t.Errorf("missing float64 type:\n%s", sql)
	}
	if !strings.Contains(sql, "rate real") {
		t.Errorf("missing float32 type:\n%s", sql)
	}
}

func TestCreateTableSQL_PointerField(t *testing.T) {
	type WithPointer struct {
		Id   int     `norm:"pk"`
		Name *string
	}

	n := NewNorm(nil)
	m := n.AddModel(&WithPointer{}, "with_pointer")
	sql := m.CreateTableSQL()

	if !strings.Contains(sql, "name text") {
		t.Errorf("pointer field should resolve to base type:\n%s", sql)
	}
}
