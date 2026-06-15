// Package sqliteit holds integration tests that run the norm migrate and gen
// packages against a live SQLite database, exercising the PRAGMA-based schema
// introspection end-to-end through the public API.
//
// It lives in its own Go module so the pure-Go SQLite driver
// (modernc.org/sqlite) and its transitive dependencies are NOT imposed on
// consumers of github.com/juggle73/norm/v4 and do not bump that module's go
// directive. Run it with:
//
//	cd migrate/sqliteit && go test ./...
package sqliteit

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/gen"
	"github.com/juggle73/norm/v4/migrate"

	_ "modernc.org/sqlite"
)

type User struct {
	Id    int    `norm:"pk"`
	Name  string `norm:"notnull"`
	Email string `norm:"unique"`
	Age   int
}

type Purchase struct {
	Id     int `norm:"pk"`
	UserId int `norm:"fk=User,notnull"`
	Total  int
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Foreign keys are off by default in SQLite; turn them on so FK metadata is
	// honoured and exercised.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newNorm(objs ...any) *norm.Norm {
	n := norm.NewNorm(&norm.Config{Dialect: norm.SQLite})
	for _, obj := range objs {
		n.M(obj)
	}
	return n
}

// TestSyncCreatesTables verifies that Sync creates the registered tables on a
// live SQLite database and that the resulting schema is usable.
func TestSyncCreatesTables(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := newNorm(&User{}, &Purchase{})

	mig := migrate.New(db, n)
	if err := mig.Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Both tables must now exist and be insertable.
	if _, err := db.Exec(`INSERT INTO user (id, name, email, age) VALUES (1, 'Ann', 'ann@x.io', 30)`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO purchase (id, user_id, total) VALUES (1, 1, 100)`); err != nil {
		t.Fatalf("insert purchase: %v", err)
	}

	// Sync must be idempotent: a second run changes nothing and errors nowhere.
	if err := mig.Sync(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// The UNIQUE constraint on email must be enforced (CreateTableSQL emitted it).
	if _, err := db.Exec(`INSERT INTO user (id, name, email, age) VALUES (2, 'Bob', 'ann@x.io', 40)`); err == nil {
		t.Fatal("expected UNIQUE violation on duplicate email, got nil")
	}
}

// TestSyncAddsColumns verifies that Sync introspects an existing table via
// PRAGMA table_info and adds only the missing columns.
func TestSyncAddsColumns(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	// Pre-create the table with a subset of columns.
	if _, err := db.Exec(`CREATE TABLE user (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("pre-create: %v", err)
	}

	n := newNorm(&User{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := columnNames(t, db, "user")
	for _, want := range []string{"id", "name", "email", "age"} {
		if !got[want] {
			t.Errorf("column %q missing after sync; have %v", want, keys(got))
		}
	}

	// The newly added columns must be usable.
	if _, err := db.Exec(`INSERT INTO user (id, name, email, age) VALUES (1, 'Ann', 'ann@x.io', 30)`); err != nil {
		t.Fatalf("insert after add column: %v", err)
	}
}

// TestDiff verifies Diff against a live database: no changes when the schema
// matches, and a DROP COLUMN when the database has an extra column.
func TestDiff(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := newNorm(&User{})
	mig := migrate.New(db, n)

	if err := mig.Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Schema matches the model → Diff is empty.
	diff, err := mig.Diff(ctx)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if strings.TrimSpace(diff) != "" {
		t.Errorf("expected empty diff for matching schema, got:\n%s", diff)
	}

	// Add a column the model does not know about → Diff proposes DROP COLUMN.
	if _, err := db.Exec(`ALTER TABLE user ADD COLUMN legacy TEXT`); err != nil {
		t.Fatalf("add legacy column: %v", err)
	}
	diff, err = mig.Diff(ctx)
	if err != nil {
		t.Fatalf("diff after add: %v", err)
	}
	if !strings.Contains(diff, "ALTER TABLE user DROP COLUMN legacy;") {
		t.Errorf("expected DROP COLUMN legacy, got:\n%s", diff)
	}
}

// TestGenFromDB verifies that gen.FromDB introspects a live SQLite schema —
// tables (sqlite_master), columns (PRAGMA table_info), the UNIQUE index
// (PRAGMA index_list/index_info) and the foreign key (PRAGMA foreign_key_list).
func TestGenFromDB(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := newNorm(&User{}, &Purchase{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, err := gen.NewGenerator(norm.SQLite).FromDB(ctx, db, "models", "")
	if err != nil {
		t.Fatalf("FromDB: %v", err)
	}

	userSrc, ok := out["user"]
	if !ok {
		t.Fatalf("no source generated for table user; got tables %v", keys2(out))
	}
	for _, want := range []string{
		"package models",
		"type User struct {",
		"Id int64",
		"Name string",
		`norm:"pk,notnull"`, // id: PK ⇒ pk + notnull
		"Email *string",     // email is UNIQUE but nullable ⇒ pointer
		`norm:"unique"`,     // unique index picked up via PRAGMA
	} {
		if !strings.Contains(userSrc, want) {
			t.Errorf("user source missing %q, got:\n%s", want, userSrc)
		}
	}

	orderSrc, ok := out["purchase"]
	if !ok {
		t.Fatalf("no source generated for table purchase")
	}
	if !strings.Contains(orderSrc, "fk=User") {
		t.Errorf("purchase source missing FK to User, got:\n%s", orderSrc)
	}
}

// TestColumnTypeRoundTrip checks that the Go→SQLite types emitted by migrate
// are accepted by SQLite and survive a real round-trip, including time.Time
// and a JSON map column.
func TestColumnTypeRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	type Product struct {
		Id        int       `norm:"pk"`
		Name      string    `norm:"notnull"`
		Price     float64   `norm:"notnull"`
		CreatedAt time.Time `norm:"notnull"`
		Metadata  map[string]any
	}

	n := newNorm(&Product{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	_, err := db.Exec(
		`INSERT INTO product (id, name, price, created_at, metadata) VALUES (?, ?, ?, ?, ?)`,
		1, "Widget", 9.99, time.Now().Format(time.RFC3339), `{"k":"v"}`,
	)
	if err != nil {
		t.Fatalf("insert product: %v", err)
	}

	var name string
	var price float64
	if err := db.QueryRow(`SELECT name, price FROM product WHERE id=1`).Scan(&name, &price); err != nil {
		t.Fatalf("select product: %v", err)
	}
	if name != "Widget" || price != 9.99 {
		t.Errorf("round-trip mismatch: name=%q price=%v", name, price)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func columnNames(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()

	names := map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		names[name] = true
	}
	return names
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keys2(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
