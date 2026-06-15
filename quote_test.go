package norm

import (
	"strings"
	"testing"
)

// quotedModel builds a Model for ModelTestStruct with identifier quoting on.
func quotedModel(t *testing.T, d Dialect) *Model {
	t.Helper()
	n := NewNorm(&Config{Dialect: d, QuoteIdentifiers: true})
	m, err := n.M(&ModelTestStruct{})
	if err != nil {
		t.Fatalf("M: %v", err)
	}
	return m
}

func TestQuoteSelect(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		// WHERE template text is raw (user-supplied) and is NOT quoted.
		{"postgres", PostgreSQL, `SELECT "id", "name", "email", "age" FROM "model_test_struct" WHERE name = $1`},
		{"mysql", MySQL, "SELECT `id`, `name`, `email`, `age` FROM `model_test_struct` WHERE name = ?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := quotedModel(t, c.d)
			sql, _, err := m.Select(Where("name = ?", "John"))
			if err != nil {
				t.Fatal(err)
			}
			if sql != c.want {
				t.Errorf("got  %q\nwant %q", sql, c.want)
			}
		})
	}
}

func TestQuoteInsert(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"postgres", PostgreSQL, `INSERT INTO "model_test_struct" ("name", "email", "age") VALUES ($1, $2, $3)`},
		{"mysql", MySQL, "INSERT INTO `model_test_struct` (`name`, `email`, `age`) VALUES (?, ?, ?)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := quotedModel(t, c.d)
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

func TestQuoteUpdate(t *testing.T) {
	m := quotedModel(t, PostgreSQL)
	sql, _, err := m.Update(Exclude("id"), Where("id = ?", 42))
	if err != nil {
		t.Fatal(err)
	}
	want := `UPDATE "model_test_struct" SET "name"=$1, "email"=$2, "age"=$3 WHERE id = $4`
	if sql != want {
		t.Errorf("got  %q\nwant %q", sql, want)
	}
}

func TestQuoteUpsert(t *testing.T) {
	cases := []struct {
		name string
		d    Dialect
		opt  Option
		want string
	}{
		{
			"postgres do update", PostgreSQL, OnConflict("email").DoUpdate("name"),
			`INSERT INTO "model_test_struct" ("name", "email", "age") VALUES ($1, $2, $3) ON CONFLICT ("email") DO UPDATE SET "name" = EXCLUDED."name"`,
		},
		{
			"mysql do update", MySQL, OnConflict("email").DoUpdate("name", "age"),
			"INSERT INTO `model_test_struct` (`name`, `email`, `age`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `age` = VALUES(`age`)",
		},
		{
			"mysql do nothing", MySQL, OnConflict("email").DoNothing(),
			"INSERT INTO `model_test_struct` (`name`, `email`, `age`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `email`=`email`",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := quotedModel(t, c.d)
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

func TestQuoteReturning(t *testing.T) {
	m := quotedModel(t, PostgreSQL)
	sql, _, err := m.Insert(Exclude("id"), Returning("Id"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sql, `RETURNING "id"`) {
		t.Errorf("expected quoted RETURNING, got %q", sql)
	}
}

func TestQuoteConditions(t *testing.T) {
	m := quotedModel(t, PostgreSQL)
	conds, _ := m.BuildConditions(
		Eq("name", "John"),
		In("id", 1, 2, 3),
		IsNull("email", true),
	)
	want := []string{`"name"=$1`, `"id" IN ($2, $3, $4)`, `"email" IS NULL`}
	if strings.Join(conds, ", ") != strings.Join(want, ", ") {
		t.Errorf("got  %v\nwant %v", conds, want)
	}
}

// TestQuoteConditionsPrefix verifies the dot prefix stays raw while the column
// is quoted.
func TestQuoteConditionsPrefix(t *testing.T) {
	m := quotedModel(t, PostgreSQL)
	conds, _ := m.BuildConditions(Eq("u.name", "John"))
	want := `u."name"=$1`
	if len(conds) != 1 || conds[0] != want {
		t.Errorf("got %v, want %q", conds, want)
	}
}

func TestQuoteOrderBy(t *testing.T) {
	m := quotedModel(t, MySQL)
	got := m.OrderBy("Name DESC, Email")
	want := "`name` DESC, `email` ASC"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestQuoteJoin verifies that table and column names are quoted in a JOIN
// while the user-supplied ON clause stays raw.
func TestQuoteJoin(t *testing.T) {
	n := NewNorm(&Config{Dialect: PostgreSQL, QuoteIdentifiers: true})
	base, _ := n.M(&ModelTestStruct{})
	other, _ := n.M(&QuoteJoinOrder{})

	sql, _, err := NewJoin(base).
		Inner(other, "quote_join_order.owner = model_test_struct.id").
		Select()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`FROM "model_test_struct"`,
		`INNER JOIN "quote_join_order"`,
		`"model_test_struct"."id"`,
		`"quote_join_order"."owner"`,
		// raw ON text is preserved verbatim
		"ON quote_join_order.owner = model_test_struct.id",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("join SQL missing %q, got:\n%s", want, sql)
		}
	}
}

type QuoteJoinOrder struct {
	Id    int `norm:"pk"`
	Owner int
}

// TestQuoteOff confirms the default (flag off) output is unchanged.
func TestQuoteOff(t *testing.T) {
	m := modelFor(t, PostgreSQL)
	sql, _, err := m.Insert(Exclude("id"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, `"`) {
		t.Errorf("expected no quotes when flag off, got %q", sql)
	}
}
