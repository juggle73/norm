// Package pgit holds integration tests that run the norm builder, migrate and
// gen packages against a live PostgreSQL database started with testcontainers
// (Docker). PostgreSQL is norm's default dialect; these tests in particular
// cover native array and jsonb binding through the pgx driver.
//
// The builder round-trip uses pgxpool (pgx's native interface, as in norm's
// docs), which decodes Postgres arrays and jsonb into Go slices and maps.
// migrate and gen use a database/sql handle for schema work.
//
// It lives in its own Go module so pgx and the testcontainers stack are not
// imposed on consumers of github.com/juggle73/norm/v4. The tests skip
// automatically when Docker is unavailable. Run with:
//
//	cd migrate/pgit && go test ./...
package pgit

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/gen"
	"github.com/juggle73/norm/v4/migrate"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ArrayRow exercises native PostgreSQL composite binding: a text[] array, a
// bigint[] array, a jsonb map and a jsonb struct, plus a bytea []byte.
type ArrayRow struct {
	Id   int      `norm:"pk,dbType=serial"`
	Tags []string `norm:"notnull"`
	Nums []int64  `norm:"notnull"`
	Meta Prefs    `norm:"notnull"`
	Attr map[string]any
	Blob []byte
}

type Prefs struct {
	Role string `json:"role"`
}

// openPG starts a PostgreSQL container and returns a database/sql handle (for
// migrate/gen) and a pgxpool (for the builder round-trip).
func openPG(t *testing.T) (*sql.DB, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("norm"),
		tcpostgres.WithPassword("normpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Skipf("cannot start PostgreSQL container (is Docker running?): %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open sql: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for i := 0; i < 10; i++ {
		if err = db.PingContext(ctx); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatalf("ping: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	return db, pool
}

func newNorm(objs ...any) *norm.Norm {
	n := norm.NewNorm(nil) // default: PostgreSQL
	for _, obj := range objs {
		n.M(obj)
	}
	return n
}

// TestArrayRoundTrip proves the fix for slice columns: migrate now types a
// []string as text[] (not jsonb), matching how pgx binds and scans Go slices.
// It also covers a jsonb map, a jsonb struct and a bytea []byte.
func TestArrayRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, pool := openPG(t)
	n := newNorm(&ArrayRow{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	in := ArrayRow{
		Tags: []string{"a", "b", "c"},
		Nums: []int64{10, 20},
		Meta: Prefs{Role: "admin"},
		Attr: map[string]any{"k": "v"},
		Blob: []byte{0x01, 0x02},
	}
	mIn, _ := n.M(&in)
	sqlStr, vals, err := mIn.Insert(norm.Exclude("id"), norm.Returning("Id"))
	if err != nil {
		t.Fatalf("build insert: %v", err)
	}
	var id int
	if err := pool.QueryRow(ctx, sqlStr, vals...).Scan(&id); err != nil {
		t.Fatalf("insert returning: %v\nsql: %s", err, sqlStr)
	}

	var got ArrayRow
	mOut, _ := n.M(&got)
	sqlStr, args, err := mOut.Select(norm.Where("id = ?", id))
	if err != nil {
		t.Fatalf("build select: %v", err)
	}
	if err := pool.QueryRow(ctx, sqlStr, args...).Scan(mOut.Pointers()...); err != nil {
		t.Fatalf("scan: %v\nsql: %s", err, sqlStr)
	}

	if len(got.Tags) != 3 || got.Tags[0] != "a" || got.Tags[2] != "c" {
		t.Errorf("text[] round-trip mismatch: %v", got.Tags)
	}
	if len(got.Nums) != 2 || got.Nums[1] != 20 {
		t.Errorf("bigint[] round-trip mismatch: %v", got.Nums)
	}
	if got.Meta.Role != "admin" {
		t.Errorf("jsonb struct round-trip mismatch: %v", got.Meta)
	}
	if got.Attr["k"] != "v" {
		t.Errorf("jsonb map round-trip mismatch: %v", got.Attr)
	}
	if len(got.Blob) != 2 || got.Blob[0] != 0x01 {
		t.Errorf("bytea round-trip mismatch: %v", got.Blob)
	}
}

// TestColumnTypes verifies the generated column types on a live server: text[]
// for the slice, jsonb for map/struct, bytea for []byte.
func TestColumnTypes(t *testing.T) {
	ctx := context.Background()
	db, _ := openPG(t)
	n := newNorm(&ArrayRow{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	types := map[string]string{}
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, data_type FROM information_schema.columns
		 WHERE table_name='array_row'`)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, dt string
		if err := rows.Scan(&name, &dt); err != nil {
			t.Fatal(err)
		}
		types[name] = dt
	}

	want := map[string]string{
		"tags": "ARRAY", "nums": "ARRAY",
		"meta": "jsonb", "attr": "jsonb", "blob": "bytea",
	}
	for col, w := range want {
		if types[col] != w {
			t.Errorf("column %s: data_type = %q, want %q", col, types[col], w)
		}
	}
}

// TestGenFromDB verifies gen.FromDB introspects a live PostgreSQL schema,
// including the array column type.
func TestGenFromDB(t *testing.T) {
	ctx := context.Background()
	db, _ := openPG(t)
	n := newNorm(&ArrayRow{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, err := gen.FromDB(ctx, db, "models", "public")
	if err != nil {
		t.Fatalf("FromDB: %v", err)
	}
	src, ok := out["array_row"]
	if !ok {
		t.Fatalf("no source for array_row")
	}
	for _, want := range []string{
		"type ArrayRow struct {",
		"Tags []string", // text[] → []string
		"Nums []int64",  // bigint[] → []int64 (precise element type)
		`norm:"pk,notnull"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("source missing %q, got:\n%s", want, src)
		}
	}
}

type Member struct {
	Id    int    `norm:"pk,dbType=serial"`
	Email string `norm:"unique,notnull"`
	Plan  string `norm:"notnull"`
}

// TestUpsert verifies the PostgreSQL ON CONFLICT path (DoUpdate and DoNothing)
// against a live database via pgxpool.
func TestUpsert(t *testing.T) {
	ctx := context.Background()
	db, pool := openPG(t)
	n := newNorm(&Member{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	insert := func(m Member, opt norm.Option) {
		mod, _ := n.M(&m)
		sqlStr, vals, err := mod.Insert(norm.Exclude("id"), opt)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if _, err := pool.Exec(ctx, sqlStr, vals...); err != nil {
			t.Fatalf("exec: %v\nsql: %s", err, sqlStr)
		}
	}

	insert(Member{Email: "a@x.io", Plan: "free"}, norm.OnConflict("email").DoUpdate("plan"))
	insert(Member{Email: "a@x.io", Plan: "pro"}, norm.OnConflict("email").DoUpdate("plan"))

	var plan string
	if err := pool.QueryRow(ctx, "SELECT plan FROM member WHERE email='a@x.io'").Scan(&plan); err != nil {
		t.Fatalf("verify update: %v", err)
	}
	if plan != "pro" {
		t.Errorf("DoUpdate did not overwrite plan, got %q", plan)
	}

	insert(Member{Email: "a@x.io", Plan: "ignored"}, norm.OnConflict("email").DoNothing())
	if err := pool.QueryRow(ctx, "SELECT plan FROM member WHERE email='a@x.io'").Scan(&plan); err != nil {
		t.Fatalf("verify nothing: %v", err)
	}
	if plan != "pro" {
		t.Errorf("DoNothing changed the row, got %q", plan)
	}
}

// TestDiff verifies Diff against a live PostgreSQL: empty when the schema
// matches, and DROP COLUMN for an extra column.
func TestDiff(t *testing.T) {
	ctx := context.Background()
	db, _ := openPG(t)
	n := newNorm(&Member{})
	mig := migrate.New(db, n)
	if err := mig.Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	diff, err := mig.Diff(ctx)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if strings.TrimSpace(diff) != "" {
		t.Errorf("expected empty diff for matching schema, got:\n%s", diff)
	}

	if _, err := db.ExecContext(ctx, "ALTER TABLE member ADD COLUMN legacy text"); err != nil {
		t.Fatalf("add legacy: %v", err)
	}
	diff, err = mig.Diff(ctx)
	if err != nil {
		t.Fatalf("diff after add: %v", err)
	}
	if !strings.Contains(diff, "DROP COLUMN legacy;") {
		t.Errorf("expected DROP COLUMN legacy, got:\n%s", diff)
	}
}
