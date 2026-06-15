package migrate

import (
	"testing"

	"github.com/juggle73/norm/v4"
)

type ArrayRow struct {
	Id    int      `norm:"pk"`
	Tags  []string `norm:"notnull"`
	Nums  []int
	Big   []int64
	Flags []bool
	Blob  []byte
	Meta  map[string]any
}

func arrayMigrate(d norm.Dialect) *Migrate {
	n := norm.NewNorm(&norm.Config{Dialect: d})
	n.M(&ArrayRow{})
	return New(nil, n)
}

// TestPostgresSliceArrays verifies slices map to native PostgreSQL array types,
// while []byte stays bytea and maps stay jsonb.
func TestPostgresSliceArrays(t *testing.T) {
	mig := arrayMigrate(norm.PostgreSQL)
	cases := map[string]string{
		"tags":  "text[]",
		"nums":  "integer[]",
		"big":   "bigint[]",
		"flags": "boolean[]",
		"blob":  "bytea",
		"meta":  "jsonb",
	}
	for col, want := range cases {
		if got := mig.columnType(fieldByName(t, mig, "array_row", col)); got != want {
			t.Errorf("PostgreSQL columnType(%s) = %q, want %q", col, got, want)
		}
	}
}

// TestNonArrayDialectsSliceJSON verifies that dialects without native arrays
// store slices as JSON (and []byte as BLOB).
func TestNonArrayDialectsSliceJSON(t *testing.T) {
	for _, tc := range []struct {
		d          norm.Dialect
		json, blob string
	}{
		{norm.SQLite, "TEXT", "BLOB"},
		{norm.MySQL, "json", "BLOB"},
	} {
		mig := arrayMigrate(tc.d)
		for _, col := range []string{"tags", "nums", "big", "flags", "meta"} {
			if got := mig.columnType(fieldByName(t, mig, "array_row", col)); got != tc.json {
				t.Errorf("%T columnType(%s) = %q, want %q", tc.d, col, got, tc.json)
			}
		}
		if got := mig.columnType(fieldByName(t, mig, "array_row", "blob")); got != tc.blob {
			t.Errorf("%T columnType(blob) = %q, want %q", tc.d, got, tc.blob)
		}
	}
}
