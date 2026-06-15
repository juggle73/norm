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
