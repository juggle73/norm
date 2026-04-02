package migrate

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/juggle73/norm/v4"
)

type User struct {
	Id    int    `norm:"pk"`
	Name  string `norm:"notnull"`
	Email string `norm:"unique"`
	Age   int
}

type Order struct {
	Id     int `norm:"pk"`
	UserId int `norm:"fk=User,notnull"`
	Total  int
}

type Product struct {
	Id          int      `norm:"pk"`
	Name        string   `norm:"notnull"`
	Price       float64  `norm:"notnull"`
	Description *string
	IsActive    bool      `norm:"notnull,default=true"`
	CreatedAt   time.Time `norm:"notnull"`
	Metadata    map[string]any
}

type Address struct {
	City   string `json:"city"`
	Street string `json:"street"`
}

type UserWithAddress struct {
	Id      int     `norm:"pk"`
	Name    string  `norm:"notnull"`
	Address Address // JSON struct field
}

func newMigrate(objs ...any) *Migrate {
	n := norm.NewNorm(nil)
	for _, obj := range objs {
		n.M(obj)
	}
	return New(nil, n)
}

func TestCreateTableSQL(t *testing.T) {
	mig := newMigrate(&User{})
	sql := mig.CreateTableSQL("user")

	assertContains(t, sql, "CREATE TABLE IF NOT EXISTS user")
	assertContains(t, sql, "id integer NOT NULL")
	assertContains(t, sql, "name text NOT NULL")
	assertContains(t, sql, "email text UNIQUE")
	assertContains(t, sql, "age integer")
	assertContains(t, sql, "PRIMARY KEY (id)")
}

func TestCreateTableSQL_FK(t *testing.T) {
	mig := newMigrate(&User{}, &Order{})
	sql := mig.CreateTableSQL("order")

	assertContains(t, sql, "user_id integer NOT NULL")
	assertContains(t, sql, "FOREIGN KEY (user_id) REFERENCES user(id)")
	assertContains(t, sql, "PRIMARY KEY (id)")
}

func TestCreateTableSQL_AllTypes(t *testing.T) {
	mig := newMigrate(&Product{})
	sql := mig.CreateTableSQL("product")

	assertContains(t, sql, "id integer NOT NULL")
	assertContains(t, sql, "name text NOT NULL")
	assertContains(t, sql, "price double precision NOT NULL")
	assertContains(t, sql, "description text")   // nullable, no NOT NULL
	assertContains(t, sql, "is_active boolean NOT NULL DEFAULT true")
	assertContains(t, sql, "created_at timestamptz NOT NULL")
	assertContains(t, sql, "metadata jsonb")
}

func TestCreateTableSQL_JSONStruct(t *testing.T) {
	mig := newMigrate(&UserWithAddress{})
	sql := mig.CreateTableSQL("user_with_address")

	assertContains(t, sql, "address jsonb")
}

func TestCreateTableSQL_CustomDefaultString(t *testing.T) {
	n := norm.NewNorm(&norm.Config{DefaultString: "varchar"})
	n.M(&User{})
	mig := New(nil, n)
	sql := mig.CreateTableSQL("user")

	assertContains(t, sql, "name varchar NOT NULL")
	assertContains(t, sql, "email varchar UNIQUE")
}

func TestCreateTableSQL_CustomDefaultTime(t *testing.T) {
	n := norm.NewNorm(&norm.Config{DefaultTime: "timestamp"})
	n.M(&Product{})
	mig := New(nil, n)
	sql := mig.CreateTableSQL("product")

	assertContains(t, sql, "created_at timestamp NOT NULL")
}

func TestCreateTableSQL_CustomDefaultJSON(t *testing.T) {
	n := norm.NewNorm(&norm.Config{DefaultJSON: "json"})
	n.M(&UserWithAddress{})
	mig := New(nil, n)
	sql := mig.CreateTableSQL("user_with_address")

	assertContains(t, sql, "address json")
}

func TestCreateTableSQL_UnknownTable(t *testing.T) {
	mig := newMigrate(&User{})
	sql := mig.CreateTableSQL("nonexistent")
	if sql != "" {
		t.Errorf("expected empty string, got %q", sql)
	}
}

func TestAddColumnSQL(t *testing.T) {
	mig := newMigrate(&User{}, &Order{})

	fields := mig.norm.FieldsByTable("user")
	var emailField *norm.Field
	for _, f := range fields {
		if f.DbName() == "email" {
			emailField = f
			break
		}
	}
	if emailField == nil {
		t.Fatal("email field not found")
	}

	sql := mig.addColumnSQL("user", emailField)
	assertContains(t, sql, "ALTER TABLE user ADD COLUMN IF NOT EXISTS email text UNIQUE;")

	// FK column
	orderFields := mig.norm.FieldsByTable("order")
	var userIdField *norm.Field
	for _, f := range orderFields {
		if f.DbName() == "user_id" {
			userIdField = f
			break
		}
	}
	if userIdField == nil {
		t.Fatal("user_id field not found")
	}

	sql = mig.addColumnSQL("order", userIdField)
	assertContains(t, sql, "ALTER TABLE order ADD COLUMN IF NOT EXISTS user_id integer NOT NULL REFERENCES user(id);")
}

func TestPgType(t *testing.T) {
	mig := newMigrate(&Product{}, &UserWithAddress{})

	tests := []struct {
		table    string
		column   string
		expected string
	}{
		{"product", "id", "integer"},
		{"product", "name", "text"},
		{"product", "price", "double precision"},
		{"product", "description", "text"},
		{"product", "is_active", "boolean"},
		{"product", "created_at", "timestamptz"},
		{"product", "metadata", "jsonb"},
		{"user_with_address", "address", "jsonb"},
	}

	for _, tt := range tests {
		fields := mig.norm.FieldsByTable(tt.table)
		var field *norm.Field
		for _, f := range fields {
			if f.DbName() == tt.column {
				field = f
				break
			}
		}
		if field == nil {
			t.Errorf("field %s.%s not found", tt.table, tt.column)
			continue
		}
		got := mig.pgType(field)
		if got != tt.expected {
			t.Errorf("pgType(%s.%s) = %q, want %q", tt.table, tt.column, got, tt.expected)
		}
	}
}

func TestNormalizeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"integer", "integer"},
		{"int4", "integer"},
		{"int", "integer"},
		{"bigint", "bigint"},
		{"int8", "bigint"},
		{"smallint", "smallint"},
		{"int2", "smallint"},
		{"timestamp with time zone", "timestamptz"},
		{"timestamptz", "timestamptz"},
		{"timestamp without time zone", "timestamp"},
		{"character varying", "varchar"},
		{"varchar", "varchar"},
		{"double precision", "double precision"},
		{"float8", "double precision"},
		{"boolean", "boolean"},
		{"bool", "boolean"},
		{"jsonb", "jsonb"},
		{"text", "text"},
	}

	for _, tt := range tests {
		got := normalizeType(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeType(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestPgType_PointerField(t *testing.T) {
	// *string should map to text, not panic
	mig := newMigrate(&Product{})
	fields := mig.norm.FieldsByTable("product")

	var descField *norm.Field
	for _, f := range fields {
		if f.DbName() == "description" {
			descField = f
			break
		}
	}

	if descField == nil {
		t.Fatal("description field not found")
	}

	if descField.Type().Kind() != reflect.Pointer {
		t.Fatal("expected pointer type for description")
	}

	got := mig.pgType(descField)
	if got != "text" {
		t.Errorf("pgType(*string) = %q, want %q", got, "text")
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
