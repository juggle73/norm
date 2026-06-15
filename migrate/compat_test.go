package migrate

import (
	"testing"

	"github.com/juggle73/norm/v4"
)

// TestCompatSchemaSelection verifies that MariaDB reuses the MySQL schema and
// CockroachDB/YugabyteDB reuse the PostgreSQL schema in migrate.
func TestCompatSchemaSelection(t *testing.T) {
	mysqlT := newMySQLMigrate(&Product{}).columnType
	pgT := newMigrate(&Product{}).columnType

	maria := newCompat(norm.MariaDB, &Product{})
	cockroach := newCompat(norm.CockroachDB, &Product{})
	yugabyte := newCompat(norm.YugabyteDB, &Product{})

	for _, col := range []string{"id", "name", "price", "is_active", "created_at", "metadata"} {
		// MariaDB matches MySQL.
		if got, want := maria.columnType(fieldByName(t, maria, "product", col)), mysqlT(fieldByName(t, maria, "product", col)); got != want {
			t.Errorf("MariaDB columnType(%s) = %q, want MySQL %q", col, got, want)
		}
		// CockroachDB and YugabyteDB match PostgreSQL.
		if got, want := cockroach.columnType(fieldByName(t, cockroach, "product", col)), pgT(fieldByName(t, cockroach, "product", col)); got != want {
			t.Errorf("CockroachDB columnType(%s) = %q, want Postgres %q", col, got, want)
		}
		if got, want := yugabyte.columnType(fieldByName(t, yugabyte, "product", col)), pgT(fieldByName(t, yugabyte, "product", col)); got != want {
			t.Errorf("YugabyteDB columnType(%s) = %q, want Postgres %q", col, got, want)
		}
	}
}

func newCompat(d norm.Dialect, objs ...any) *Migrate {
	n := norm.NewNorm(&norm.Config{Dialect: d})
	for _, obj := range objs {
		n.M(obj)
	}
	return New(nil, n)
}
