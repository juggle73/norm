package norm

import (
	"strings"
	"testing"
)

// TestCompatDialectsBehaveLikeBase verifies the compatible dialects inherit the
// behavior of their base dialect across the full Dialect interface.
func TestCompatDialectsBehaveLikeBase(t *testing.T) {
	type want struct {
		placeholder string
		returning   bool
		quote       string
		composites  bool
		mysqlFamily bool
	}
	cases := []struct {
		name string
		d    Dialect
		want want
	}{
		{"MariaDB~MySQL", MariaDB, want{"?", false, "`x`", false, true}},
		{"CockroachDB~Postgres", CockroachDB, want{"$1", true, `"x"`, true, false}},
		{"YugabyteDB~Postgres", YugabyteDB, want{"$1", true, `"x"`, true, false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.Placeholder(1); got != c.want.placeholder {
				t.Errorf("Placeholder = %q, want %q", got, c.want.placeholder)
			}
			if got := c.d.SupportsReturning(); got != c.want.returning {
				t.Errorf("SupportsReturning = %v, want %v", got, c.want.returning)
			}
			if got := c.d.QuoteIdentifier("x"); got != c.want.quote {
				t.Errorf("QuoteIdentifier = %q, want %q", got, c.want.quote)
			}
			if got := c.d.NativeComposites(); got != c.want.composites {
				t.Errorf("NativeComposites = %v, want %v", got, c.want.composites)
			}
			if got := IsMySQLFamily(c.d); got != c.want.mysqlFamily {
				t.Errorf("IsMySQLFamily = %v, want %v", got, c.want.mysqlFamily)
			}
		})
	}
}

// TestCompatDialectsAreDistinct verifies the compatible dialects are distinct
// values from their base, so dialect equality checks are not surprising.
func TestCompatDialectsAreDistinct(t *testing.T) {
	if MariaDB == MySQL {
		t.Error("MariaDB should be a distinct value from MySQL")
	}
	if CockroachDB == PostgreSQL || YugabyteDB == PostgreSQL {
		t.Error("CockroachDB/YugabyteDB should be distinct values from PostgreSQL")
	}
}

// TestCompatDialectUpsert verifies MariaDB renders the MySQL upsert and
// CockroachDB the PostgreSQL one.
func TestCompatDialectUpsert(t *testing.T) {
	maria, _ := NewNorm(&Config{Dialect: MariaDB}).M(&ModelTestStruct{})
	sql, _, err := maria.Insert(Exclude("id"), OnConflict("email").DoUpdate("name"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "ON DUPLICATE KEY UPDATE name = VALUES(name)"; !strings.Contains(sql, want) {
		t.Errorf("MariaDB upsert = %q, want substring %q", sql, want)
	}

	cockroach, _ := NewNorm(&Config{Dialect: CockroachDB}).M(&ModelTestStruct{})
	sql, _, err = cockroach.Insert(Exclude("id"), OnConflict("email").DoUpdate("name"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name"; !strings.Contains(sql, want) {
		t.Errorf("CockroachDB upsert = %q, want substring %q", sql, want)
	}
}
