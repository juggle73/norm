package norm

import (
	"bytes"
	"encoding/json"
	"testing"
)

type compositeModel struct {
	Id   int            `norm:"pk"`
	Tags map[string]any `norm:"notnull"`
	List []string       `norm:"notnull"`
	Blob []byte
}

func compositeFor(t *testing.T, d Dialect, v *compositeModel) *Model {
	t.Helper()
	n := NewNorm(&Config{Dialect: d})
	m, err := n.M(v)
	if err != nil {
		t.Fatalf("M: %v", err)
	}
	return m
}

// TestCompositeMarshalSQLite verifies that on a non-native dialect the builder
// JSON-marshals map and non-[]byte slice fields, while []byte stays raw.
func TestCompositeMarshalSQLite(t *testing.T) {
	v := &compositeModel{
		Id:   1,
		Tags: map[string]any{"role": "admin"},
		List: []string{"a", "b"},
		Blob: []byte{0x01, 0x02, 0x03},
	}
	m := compositeFor(t, SQLite, v)
	vals := m.Values() // order: id, tags, list, blob

	// tags → JSON bytes
	tagsB, ok := vals[1].([]byte)
	if !ok {
		t.Fatalf("tags: expected []byte, got %T", vals[1])
	}
	var tags map[string]any
	if err := json.Unmarshal(tagsB, &tags); err != nil || tags["role"] != "admin" {
		t.Errorf("tags not JSON-marshaled: %s (%v)", tagsB, err)
	}

	// list → JSON bytes
	listB, ok := vals[2].([]byte)
	if !ok {
		t.Fatalf("list: expected []byte, got %T", vals[2])
	}
	var list []string
	if err := json.Unmarshal(listB, &list); err != nil || len(list) != 2 || list[0] != "a" {
		t.Errorf("list not JSON-marshaled: %s (%v)", listB, err)
	}

	// blob → raw bytes, NOT JSON-marshaled
	blobB, ok := vals[3].([]byte)
	if !ok {
		t.Fatalf("blob: expected []byte, got %T", vals[3])
	}
	if !bytes.Equal(blobB, []byte{0x01, 0x02, 0x03}) {
		t.Errorf("blob should stay raw, got %v", blobB)
	}
}

// TestCompositeNativePostgres verifies that PostgreSQL is unchanged: maps and
// slices are passed to the driver natively (not marshaled by norm).
func TestCompositeNativePostgres(t *testing.T) {
	v := &compositeModel{
		Id:   1,
		Tags: map[string]any{"role": "admin"},
		List: []string{"a", "b"},
	}
	m := compositeFor(t, PostgreSQL, v)
	vals := m.Values()

	if _, ok := vals[1].(map[string]any); !ok {
		t.Errorf("tags: expected native map[string]any, got %T", vals[1])
	}
	if _, ok := vals[2].([]string); !ok {
		t.Errorf("list: expected native []string, got %T", vals[2])
	}
}

// TestCompositePointersScanner verifies that Pointers wraps map/slice targets
// in the JSON scanner on non-native dialects but not on PostgreSQL.
func TestCompositePointersScanner(t *testing.T) {
	var v compositeModel
	mSQLite := compositeFor(t, SQLite, &v)
	if _, ok := mSQLite.Pointers()[1].(*jsonScanner); !ok {
		t.Errorf("SQLite: expected *jsonScanner for tags, got %T", mSQLite.Pointers()[1])
	}

	mPG := compositeFor(t, PostgreSQL, &v)
	if _, ok := mPG.Pointers()[1].(*jsonScanner); ok {
		t.Errorf("PostgreSQL: tags should not be wrapped in jsonScanner")
	}
}

func TestDialectNativeComposites(t *testing.T) {
	cases := map[Dialect]bool{PostgreSQL: true, SQLite: false, MySQL: false}
	for d, want := range cases {
		if got := d.NativeComposites(); got != want {
			t.Errorf("%T.NativeComposites() = %v, want %v", d, got, want)
		}
	}
}
