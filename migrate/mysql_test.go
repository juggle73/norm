package migrate

import (
	"testing"

	"github.com/juggle73/norm/v4"
)

func newMySQLMigrate(objs ...any) *Migrate {
	n := norm.NewNorm(&norm.Config{Dialect: norm.MySQL})
	for _, obj := range objs {
		n.M(obj)
	}
	return New(nil, n)
}

func TestMySQLCreateTableSQL(t *testing.T) {
	mig := newMySQLMigrate(&User{})
	sql := mig.CreateTableSQL("user")

	assertContains(t, sql, "CREATE TABLE IF NOT EXISTS user")
	assertContains(t, sql, "id INT NOT NULL")
	assertContains(t, sql, "name varchar(255) NOT NULL")
	assertContains(t, sql, "email varchar(255) UNIQUE")
	assertContains(t, sql, "age INT")
	assertContains(t, sql, "PRIMARY KEY (id)")
}

func TestMySQLCreateTableSQL_FK(t *testing.T) {
	mig := newMySQLMigrate(&User{}, &Order{})
	sql := mig.CreateTableSQL("order")

	assertContains(t, sql, "user_id INT NOT NULL")
	assertContains(t, sql, "FOREIGN KEY (user_id) REFERENCES user(id)")
}

func TestMySQLCreateTableSQL_AllTypes(t *testing.T) {
	mig := newMySQLMigrate(&Product{})
	sql := mig.CreateTableSQL("product")

	assertContains(t, sql, "id INT NOT NULL")
	assertContains(t, sql, "name varchar(255) NOT NULL")
	assertContains(t, sql, "price DOUBLE NOT NULL")
	assertContains(t, sql, "description varchar(255)") // nullable *string
	assertContains(t, sql, "is_active TINYINT(1) NOT NULL DEFAULT true")
	assertContains(t, sql, "created_at datetime NOT NULL")
	assertContains(t, sql, "metadata json") // map → JSON
}

func TestMySQLColumnType(t *testing.T) {
	mig := newMySQLMigrate(&Product{}, &UserWithAddress{})

	tests := []struct {
		table, column, expected string
	}{
		{"product", "id", "INT"},
		{"product", "name", "varchar(255)"},
		{"product", "price", "DOUBLE"},
		{"product", "description", "varchar(255)"},
		{"product", "is_active", "TINYINT(1)"},
		{"product", "created_at", "datetime"},
		{"product", "metadata", "json"},
		{"user_with_address", "address", "json"},
	}
	for _, tt := range tests {
		got := mig.columnType(fieldByName(t, mig, tt.table, tt.column))
		if got != tt.expected {
			t.Errorf("columnType(%s.%s) = %q, want %q", tt.table, tt.column, got, tt.expected)
		}
	}
}

func TestMySQLAddColumnSQL(t *testing.T) {
	mig := newMySQLMigrate(&User{}, &Order{})

	// MySQL ADD COLUMN has no IF NOT EXISTS, but keeps UNIQUE/NOT NULL.
	emailSQL := mig.addColumnSQL("user", fieldByName(t, mig, "user", "email"))
	assertContains(t, emailSQL, "ALTER TABLE user ADD COLUMN email varchar(255) UNIQUE;")
	assertNotContains(t, emailSQL, "IF NOT EXISTS")

	uidSQL := mig.addColumnSQL("order", fieldByName(t, mig, "order", "user_id"))
	assertContains(t, uidSQL, "ALTER TABLE order ADD COLUMN user_id INT NOT NULL REFERENCES user(id);")
}

func TestMySQLNormalizeType(t *testing.T) {
	s := mysqlSchema{}
	cases := map[string]string{
		"INT":          "int",
		"int(11)":      "int",
		"integer":      "int",
		"bigint":       "bigint",
		"tinyint(1)":   "tinyint",
		"tinyint":      "tinyint",
		"bool":         "tinyint",
		"double":       "double",
		"DOUBLE":       "double",
		"float":        "float",
		"varchar(255)": "varchar",
		"char(3)":      "varchar",
		"datetime":     "datetime",
		"timestamp":    "datetime",
		"int unsigned": "int",
		"json":         "json",
		"text":         "text",
	}
	for in, want := range cases {
		if got := s.normalizeType(in); got != want {
			t.Errorf("normalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMySQLAlterColumn verifies MODIFY COLUMN rendering for type/nullability.
func TestMySQLAlterColumn(t *testing.T) {
	s := mysqlSchema{}

	// Type change: existing varchar(100), want bigint.
	got := s.alterColumn("`t`", "`c`", "BIGINT",
		dbColumn{dataType: "varchar(100)", isNullable: true}, false)
	if len(got) != 1 || got[0] != "ALTER TABLE `t` MODIFY COLUMN `c` BIGINT;" {
		t.Errorf("type change: got %v", got)
	}

	// NOT NULL change only (type matches): existing int nullable, want int NOT NULL.
	got = s.alterColumn("`t`", "`c`", "INT",
		dbColumn{dataType: "int(11)", isNullable: true}, true)
	if len(got) != 1 || got[0] != "ALTER TABLE `t` MODIFY COLUMN `c` INT NOT NULL;" {
		t.Errorf("notnull change: got %v", got)
	}

	// No change: same type, same nullability → nil.
	got = s.alterColumn("`t`", "`c`", "INT",
		dbColumn{dataType: "int(11)", isNullable: true}, false)
	if got != nil {
		t.Errorf("expected nil for no change, got %v", got)
	}
}
