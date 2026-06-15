package sqliteit

import (
	"context"
	"testing"

	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

// CompositeRow has a map and a slice field, which norm JSON-marshals on SQLite.
type CompositeRow struct {
	Id   int            `norm:"pk"`
	Tags map[string]any `norm:"notnull"`
	List []string       `norm:"notnull"`
	Blob []byte
}

// TestCompositeRoundTrip proves that map and slice fields survive a full
// Insert/Select round-trip through the live SQLite driver (the asymmetry fix),
// while a []byte field stays raw binary.
func TestCompositeRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := newNorm(&CompositeRow{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	in := CompositeRow{
		Id:   1,
		Tags: map[string]any{"role": "admin", "level": float64(5)},
		List: []string{"x", "y", "z"},
		Blob: []byte{0x01, 0x02, 0x03},
	}
	mIn, _ := n.M(&in)
	sql, vals, err := mIn.Insert()
	if err != nil {
		t.Fatalf("build insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, sql, vals...); err != nil {
		t.Fatalf("exec insert: %v\nsql: %s", err, sql)
	}

	var got CompositeRow
	mOut, _ := n.M(&got)
	sql, args, err := mOut.Select(norm.Where("id = ?", 1))
	if err != nil {
		t.Fatalf("build select: %v", err)
	}
	if err := db.QueryRowContext(ctx, sql, args...).Scan(mOut.Pointers()...); err != nil {
		t.Fatalf("scan: %v\nsql: %s", err, sql)
	}

	if got.Tags["role"] != "admin" || got.Tags["level"] != float64(5) {
		t.Errorf("map round-trip mismatch: %v", got.Tags)
	}
	if len(got.List) != 3 || got.List[0] != "x" || got.List[2] != "z" {
		t.Errorf("slice round-trip mismatch: %v", got.List)
	}
	if len(got.Blob) != 3 || got.Blob[0] != 0x01 || got.Blob[2] != 0x03 {
		t.Errorf("blob round-trip mismatch: %v", got.Blob)
	}
}
