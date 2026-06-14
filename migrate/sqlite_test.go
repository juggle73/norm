package migrate

import (
	"strings"
	"testing"

	"github.com/juggle73/norm/v4"
)

func newSQLiteMigrate(objs ...any) *Migrate {
	n := norm.NewNorm(&norm.Config{Dialect: norm.SQLite})
	for _, obj := range objs {
		n.M(obj)
	}
	return New(nil, n)
}

func fieldByName(t *testing.T, mig *Migrate, table, column string) *norm.Field {
	t.Helper()
	for _, f := range mig.norm.FieldsByTable(table) {
		if f.DbName() == column {
			return f
		}
	}
	t.Fatalf("field %s.%s not found", table, column)
	return nil
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected %q NOT to contain %q", s, substr)
	}
}

func TestSQLiteCreateTableSQL(t *testing.T) {
	mig := newSQLiteMigrate(&User{})
	sql := mig.CreateTableSQL("user")

	assertContains(t, sql, "CREATE TABLE IF NOT EXISTS user")
	assertContains(t, sql, "id INTEGER NOT NULL")
	assertContains(t, sql, "name TEXT NOT NULL")
	assertContains(t, sql, "email TEXT UNIQUE")
	assertContains(t, sql, "age INTEGER")
	assertContains(t, sql, "PRIMARY KEY (id)")
}

func TestSQLiteCreateTableSQL_FK(t *testing.T) {
	mig := newSQLiteMigrate(&User{}, &Order{})
	sql := mig.CreateTableSQL("order")

	assertContains(t, sql, "user_id INTEGER NOT NULL")
	assertContains(t, sql, "FOREIGN KEY (user_id) REFERENCES user(id)")
	assertContains(t, sql, "PRIMARY KEY (id)")
}

func TestSQLiteCreateTableSQL_AllTypes(t *testing.T) {
	mig := newSQLiteMigrate(&Product{})
	sql := mig.CreateTableSQL("product")

	assertContains(t, sql, "id INTEGER NOT NULL")
	assertContains(t, sql, "name TEXT NOT NULL")
	assertContains(t, sql, "price REAL NOT NULL")
	assertContains(t, sql, "description TEXT") // nullable *string
	assertContains(t, sql, "is_active BOOLEAN NOT NULL DEFAULT true")
	assertContains(t, sql, "created_at TIMESTAMP NOT NULL")
	assertContains(t, sql, "metadata TEXT") // map → JSON → TEXT
}

func TestSQLiteCreateTableSQL_JSONStruct(t *testing.T) {
	mig := newSQLiteMigrate(&UserWithAddress{})
	sql := mig.CreateTableSQL("user_with_address")

	assertContains(t, sql, "address TEXT")
}

func TestSQLiteColumnType(t *testing.T) {
	mig := newSQLiteMigrate(&Product{}, &UserWithAddress{})

	tests := []struct {
		table, column, expected string
	}{
		{"product", "id", "INTEGER"},
		{"product", "name", "TEXT"},
		{"product", "price", "REAL"},
		{"product", "description", "TEXT"},
		{"product", "is_active", "BOOLEAN"},
		{"product", "created_at", "TIMESTAMP"},
		{"product", "metadata", "TEXT"},
		{"user_with_address", "address", "TEXT"},
	}
	for _, tt := range tests {
		got := mig.columnType(fieldByName(t, mig, tt.table, tt.column))
		if got != tt.expected {
			t.Errorf("columnType(%s.%s) = %q, want %q", tt.table, tt.column, got, tt.expected)
		}
	}
}

func TestSQLiteAddColumnSQL(t *testing.T) {
	mig := newSQLiteMigrate(&User{}, &Order{})

	// email: UNIQUE + nullable → SQLite has no IF NOT EXISTS and cannot add a
	// UNIQUE column, so UNIQUE is dropped.
	emailSQL := mig.addColumnSQL("user", fieldByName(t, mig, "user", "email"))
	assertContains(t, emailSQL, "ALTER TABLE user ADD COLUMN email TEXT;")
	assertNotContains(t, emailSQL, "IF NOT EXISTS")
	assertNotContains(t, emailSQL, "UNIQUE")

	// user_id: NOT NULL without a default → SQLite cannot add NOT NULL on an
	// existing table, so NOT NULL is dropped; REFERENCES is kept.
	uidSQL := mig.addColumnSQL("order", fieldByName(t, mig, "order", "user_id"))
	assertContains(t, uidSQL, "ALTER TABLE order ADD COLUMN user_id INTEGER REFERENCES user(id);")
	assertNotContains(t, uidSQL, "NOT NULL")
}

func TestSQLiteNormalizeType(t *testing.T) {
	s := sqliteSchema{}
	cases := map[string]string{
		"INTEGER":          "INTEGER",
		"int":              "INTEGER",
		"bigint":           "INTEGER",
		"varchar(255)":     "TEXT",
		"TEXT":             "TEXT",
		"clob":             "TEXT",
		"REAL":             "REAL",
		"double precision": "REAL",
		"float":            "REAL",
		"BOOLEAN":          "BOOLEAN",
		"bool":             "BOOLEAN",
		"timestamp":        "TIMESTAMP",
		"datetime":         "TIMESTAMP",
		"blob":             "BLOB",
		"":                 "BLOB",
	}
	for in, want := range cases {
		if got := s.normalizeType(in); got != want {
			t.Errorf("normalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}
