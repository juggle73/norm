package norm

import (
	"strings"
	"testing"
)

// modelFor builds a Model for ModelTestStruct bound to the given dialect.
func modelFor(t *testing.T, d Dialect) *Model {
	t.Helper()
	n := NewNorm(&Config{Dialect: d})
	m, err := n.M(&ModelTestStruct{})
	if err != nil {
		t.Fatalf("M: %v", err)
	}
	return m
}

func TestDialectPlaceholder(t *testing.T) {
	cases := []struct {
		d    Dialect
		n    int
		want string
	}{
		{PostgreSQL, 1, "$1"},
		{PostgreSQL, 7, "$7"},
		{SQLite, 1, "?"},
		{SQLite, 7, "?"},
		{MySQL, 1, "?"},
		{MySQL, 7, "?"},
	}
	for _, c := range cases {
		if got := c.d.Placeholder(c.n); got != c.want {
			t.Errorf("%T.Placeholder(%d) = %q, want %q", c.d, c.n, got, c.want)
		}
	}
}

func TestDialectDefaultIsPostgres(t *testing.T) {
	n := NewNorm(nil)
	if n.GetConfig().Dialect != PostgreSQL {
		t.Fatalf("default dialect = %T, want PostgreSQL", n.GetConfig().Dialect)
	}
}

func TestDialectSelect(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"postgres", PostgreSQL, "SELECT id, name, email, age FROM model_test_struct WHERE name = $1 AND age > $2"},
		{"sqlite", SQLite, "SELECT id, name, email, age FROM model_test_struct WHERE name = ? AND age > ?"},
		{"mysql", MySQL, "SELECT id, name, email, age FROM model_test_struct WHERE name = ? AND age > ?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := modelFor(t, c.d)
			sql, args, err := m.Select(Where("name = ? AND age > ?", "John", 18))
			if err != nil {
				t.Fatal(err)
			}
			if sql != c.want {
				t.Errorf("got  %q\nwant %q", sql, c.want)
			}
			if len(args) != 2 {
				t.Errorf("expected 2 args, got %d", len(args))
			}
		})
	}
}

func TestDialectInsert(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"postgres", PostgreSQL, "INSERT INTO model_test_struct (name, email, age) VALUES ($1, $2, $3)"},
		{"sqlite", SQLite, "INSERT INTO model_test_struct (name, email, age) VALUES (?, ?, ?)"},
		{"mysql", MySQL, "INSERT INTO model_test_struct (name, email, age) VALUES (?, ?, ?)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := modelFor(t, c.d)
			sql, _, err := m.Insert(Exclude("id"))
			if err != nil {
				t.Fatal(err)
			}
			if sql != c.want {
				t.Errorf("got  %q\nwant %q", sql, c.want)
			}
		})
	}
}

func TestDialectUpdate(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"postgres", PostgreSQL, "UPDATE model_test_struct SET name=$1, email=$2, age=$3 WHERE id = $4"},
		{"sqlite", SQLite, "UPDATE model_test_struct SET name=?, email=?, age=? WHERE id = ?"},
		{"mysql", MySQL, "UPDATE model_test_struct SET name=?, email=?, age=? WHERE id = ?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := modelFor(t, c.d)
			sql, _, err := m.Update(Exclude("id"), Where("id = ?", 42))
			if err != nil {
				t.Fatal(err)
			}
			if sql != c.want {
				t.Errorf("got  %q\nwant %q", sql, c.want)
			}
		})
	}
}

func TestDialectConditionsIn(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want []string
	}{
		{"postgres", PostgreSQL, []string{"name=$1", "age > $2", "id IN ($3, $4, $5)"}},
		{"sqlite", SQLite, []string{"name=?", "age > ?", "id IN (?, ?, ?)"}},
		{"mysql", MySQL, []string{"name=?", "age > ?", "id IN (?, ?, ?)"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := modelFor(t, c.d)
			conds, vals := m.BuildConditions(
				Eq("name", "John"),
				Gt("age", 18),
				In("id", 1, 2, 3),
			)
			if strings.Join(conds, ", ") != strings.Join(c.want, ", ") {
				t.Errorf("got  %v\nwant %v", conds, c.want)
			}
			if len(vals) != 5 {
				t.Errorf("expected 5 values, got %d", len(vals))
			}
		})
	}
}

func TestDialectUpsert(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		opt  Option
		want string
	}{
		{
			"postgres do nothing", PostgreSQL, OnConflict("email").DoNothing(),
			"INSERT INTO model_test_struct (name, email, age) VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING",
		},
		{
			"postgres do update", PostgreSQL, OnConflict("email").DoUpdate("name"),
			"INSERT INTO model_test_struct (name, email, age) VALUES ($1, $2, $3) ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name",
		},
		{
			"sqlite do update", SQLite, OnConflict("email").DoUpdate("name"),
			"INSERT INTO model_test_struct (name, email, age) VALUES (?, ?, ?) ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name",
		},
		{
			"mysql do nothing", MySQL, OnConflict("email").DoNothing(),
			"INSERT INTO model_test_struct (name, email, age) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE email=email",
		},
		{
			"mysql do update", MySQL, OnConflict("email").DoUpdate("name", "age"),
			"INSERT INTO model_test_struct (name, email, age) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE name = VALUES(name), age = VALUES(age)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := modelFor(t, c.d)
			sql, _, err := m.Insert(Exclude("id"), c.opt)
			if err != nil {
				t.Fatal(err)
			}
			if sql != c.want {
				t.Errorf("got  %q\nwant %q", sql, c.want)
			}
		})
	}
}

func TestDialectReturningUnsupported(t *testing.T) {
	m := modelFor(t, MySQL)
	_, _, err := m.Insert(Exclude("id"), Returning("Id"))
	if err == nil {
		t.Fatal("expected error for RETURNING on MySQL, got nil")
	}
	if !strings.Contains(err.Error(), "RETURNING") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDialectReturningSupported(t *testing.T) {
	for _, d := range []Dialect{PostgreSQL, SQLite} {
		m := modelFor(t, d)
		sql, _, err := m.Insert(Exclude("id"), Returning("Id"))
		if err != nil {
			t.Fatalf("%T: %v", d, err)
		}
		if !strings.HasSuffix(sql, "RETURNING id") {
			t.Errorf("%T: expected RETURNING suffix, got %q", d, sql)
		}
	}
}

func TestDialectPlaceholdersMethod(t *testing.T) {
	cases := []struct {
		d     Dialect
		count int
		want  string
	}{
		{PostgreSQL, 3, "$1, $2, $3"},
		{PostgreSQL, 0, ""},
		{SQLite, 3, "?, ?, ?"},
		{MySQL, 3, "?, ?, ?"},
		{MySQL, 0, ""},
	}
	for _, c := range cases {
		m := modelFor(t, c.d)
		if got := m.Placeholders(c.count); got != c.want {
			t.Errorf("%T.Placeholders(%d) = %q, want %q", c.d, c.count, got, c.want)
		}
	}
}

func TestDialectBuildWhereMethod(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"postgres", PostgreSQL, "name = $2 AND age > $3"},
		{"sqlite", SQLite, "name = ? AND age > ?"},
		{"mysql", MySQL, "name = ? AND age > ?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := modelFor(t, c.d)
			s, args := m.BuildWhere(2, "name = ? AND age > ?", "John", 18)
			if s != c.want {
				t.Errorf("got %q, want %q", s, c.want)
			}
			if len(args) != 2 {
				t.Errorf("expected 2 args, got %d", len(args))
			}
		})
	}
}

func TestDialectReturningMethodPanicsOnMySQL(t *testing.T) {
	m := modelFor(t, MySQL)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Returning on MySQL")
		}
	}()
	m.Returning("Id")
}

func TestDialectQuoteIdentifier(t *testing.T) {
	cases := []struct {
		d    Dialect
		in   string
		want string
	}{
		{PostgreSQL, "name", `"name"`},
		{SQLite, "name", `"name"`},
		{MySQL, "name", "`name`"},
	}
	for _, c := range cases {
		if got := c.d.QuoteIdentifier(c.in); got != c.want {
			t.Errorf("%T.QuoteIdentifier(%q) = %q, want %q", c.d, c.in, got, c.want)
		}
	}
}
