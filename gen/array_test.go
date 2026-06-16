package gen

import (
	"strings"
	"testing"
)

// TestPostgresArrayColumn guards against the regression where information_schema
// reports array columns with data_type "ARRAY" (uppercase) but the type map was
// keyed lowercase, silently dropping the column from the generated struct.
func TestPostgresArrayColumn(t *testing.T) {
	cols := []Col{
		{Name: "id", DataType: "integer", IsPK: true},
		{Name: "tags", DataType: "ARRAY", IsNullable: false},
	}
	src := Gen("models", "Row", cols)
	if !strings.Contains(src, "Tags []string") {
		t.Errorf("expected array column to map to []string, got:\n%s", src)
	}
}

// TestPostgresArrayElementTypes checks that an array udt_name (as supplied by
// the live introspection) resolves to the precise Go slice element type.
func TestPostgresArrayElementTypes(t *testing.T) {
	cols := []Col{
		{Name: "tags", DataType: "_text"},
		{Name: "nums", DataType: "_int4"},
		{Name: "big", DataType: "_int8"},
		{Name: "flags", DataType: "_bool"},
		{Name: "scores", DataType: "_float8"},
		{Name: "stamps", DataType: "_timestamptz"},
		{Name: "weird", DataType: "_unknownelem"}, // unknown → []string
	}
	src := Gen("models", "Row", cols)
	for _, want := range []string{
		"Tags []string",
		"Nums []int",
		"Big []int64",
		"Flags []bool",
		"Scores []float64",
		"Stamps []time.Time",
		"Weird []string",
		`"time"`, // time import present for []time.Time
	} {
		if !strings.Contains(src, want) {
			t.Errorf("expected %q, got:\n%s", want, src)
		}
	}
}
