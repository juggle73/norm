// Package mysqlit holds integration tests that run the norm builder, migrate
// and gen packages against a live MySQL database started with testcontainers
// (Docker). It lives in its own Go module so the MySQL driver and the
// testcontainers stack are not imposed on consumers of
// github.com/juggle73/norm/v4.
//
// The tests skip automatically when Docker is unavailable. Run with:
//
//	cd migrate/mysqlit && go test ./...
package mysqlit

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/gen"
	"github.com/juggle73/norm/v4/migrate"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

// User uses an AUTO_INCREMENT primary key (declared via dbType) so the
// LastInsertId path can be exercised.
type User struct {
	Id    int    `norm:"pk,dbType=INT AUTO_INCREMENT"`
	Name  string `norm:"notnull"`
	Email string `norm:"unique"`
	Age   int
}

// Account exercises bool, float, a JSON struct field and time.Time.
type Account struct {
	Id        int       `norm:"pk,dbType=INT AUTO_INCREMENT"`
	Name      string    `norm:"notnull"`
	Active    bool      `norm:"notnull"`
	Score     float64   `norm:"notnull"`
	Prefs     Prefs     `norm:"notnull"`
	CreatedAt time.Time `norm:"notnull"`
}

type Prefs struct {
	Role string `json:"role"`
}

// openMySQL starts a MySQL container and returns a connected *sql.DB. The test
// is skipped when Docker is not available.
func openMySQL(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("testdb"),
		tcmysql.WithUsername("norm"),
		tcmysql.WithPassword("normpass"),
	)
	if err != nil {
		t.Skipf("cannot start MySQL container (is Docker running?): %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// The module waits for readiness, but ping once more to be safe.
	for i := 0; i < 10; i++ {
		if err = db.PingContext(ctx); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	return db
}

func newNorm(objs ...any) *norm.Norm {
	n := norm.NewNorm(&norm.Config{Dialect: norm.MySQL})
	for _, obj := range objs {
		n.M(obj)
	}
	return n
}

// TestSyncAndLastInsertId verifies Sync creates the table on a live MySQL
// server and that an Insert built by norm yields a usable LastInsertId — the
// supported way to read a generated key on MySQL (no RETURNING).
func TestSyncAndLastInsertId(t *testing.T) {
	ctx := context.Background()
	db := openMySQL(t)
	n := newNorm(&User{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	u := User{Name: "Ann", Email: "ann@x.io", Age: 30}
	m, _ := n.M(&u)
	sqlStr, vals, err := m.Insert(norm.Exclude("id"))
	if err != nil {
		t.Fatalf("build insert: %v", err)
	}
	res, err := db.ExecContext(ctx, sqlStr, vals...)
	if err != nil {
		t.Fatalf("exec insert: %v\nsql: %s", err, sqlStr)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive LastInsertId, got %d", id)
	}

	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM user WHERE id=?", id).Scan(&name); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if name != "Ann" {
		t.Errorf("got name %q, want Ann", name)
	}
}

// TestBuilderRoundTrip runs the builder's Insert/Select against live MySQL and
// verifies bool, JSON and time.Time survive a scan through m.Pointers().
func TestBuilderRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openMySQL(t)
	n := newNorm(&Account{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	in := Account{Name: "Ann", Active: true, Score: 9.5,
		Prefs: Prefs{Role: "admin"}, CreatedAt: now}
	mIn, _ := n.M(&in)
	sqlStr, vals, err := mIn.Insert(norm.Exclude("id"))
	if err != nil {
		t.Fatalf("build insert: %v", err)
	}
	res, err := db.ExecContext(ctx, sqlStr, vals...)
	if err != nil {
		t.Fatalf("exec insert: %v\nsql: %s", err, sqlStr)
	}
	id, _ := res.LastInsertId()

	var got Account
	mOut, _ := n.M(&got)
	sqlStr, args, err := mOut.Select(norm.Where("id = ?", id))
	if err != nil {
		t.Fatalf("build select: %v", err)
	}
	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(mOut.Pointers()...); err != nil {
		t.Fatalf("scan: %v\nsql: %s", err, sqlStr)
	}

	if got.Name != "Ann" || !got.Active || got.Score != 9.5 {
		t.Errorf("scalar round-trip mismatch: %+v", got)
	}
	if got.Prefs.Role != "admin" {
		t.Errorf("JSON round-trip mismatch: %v", got.Prefs)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("time round-trip mismatch: got %v want %v", got.CreatedAt, now)
	}
}

// TestOnDuplicateKey verifies the MySQL upsert path (ON DUPLICATE KEY UPDATE)
// for both DoUpdate and DoNothing against a unique-key conflict.
func TestOnDuplicateKey(t *testing.T) {
	ctx := context.Background()
	db := openMySQL(t)
	n := newNorm(&User{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	insert := func(u User, opt norm.Option) {
		m, _ := n.M(&u)
		sqlStr, vals, err := m.Insert(norm.Exclude("id"), opt)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if _, err := db.ExecContext(ctx, sqlStr, vals...); err != nil {
			t.Fatalf("exec: %v\nsql: %s", err, sqlStr)
		}
	}

	// Seed, then a conflicting email with DoUpdate("name") → name overwritten.
	insert(User{Name: "Ann", Email: "dup@x.io", Age: 1}, norm.OnConflict("email").DoUpdate("name"))
	insert(User{Name: "Updated", Email: "dup@x.io", Age: 1}, norm.OnConflict("email").DoUpdate("name"))

	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM user WHERE email='dup@x.io'").Scan(&name); err != nil {
		t.Fatalf("verify update: %v", err)
	}
	if name != "Updated" {
		t.Errorf("DoUpdate did not overwrite name, got %q", name)
	}

	// DoNothing on the same conflict leaves the row unchanged.
	insert(User{Name: "Ignored", Email: "dup@x.io", Age: 1}, norm.OnConflict("email").DoNothing())
	if err := db.QueryRowContext(ctx, "SELECT name FROM user WHERE email='dup@x.io'").Scan(&name); err != nil {
		t.Fatalf("verify nothing: %v", err)
	}
	if name != "Updated" {
		t.Errorf("DoNothing changed the row, got %q", name)
	}
}

// TestGenFromDB verifies gen.FromDB introspects the live MySQL schema —
// tables, columns, the UNIQUE key and the foreign key.
func TestGenFromDB(t *testing.T) {
	ctx := context.Background()
	db := openMySQL(t)

	type Org struct {
		Id   int    `norm:"pk,dbType=INT AUTO_INCREMENT"`
		Name string `norm:"notnull"`
	}
	type Member struct {
		Id    int `norm:"pk,dbType=INT AUTO_INCREMENT"`
		OrgId int `norm:"fk=Org,notnull"`
		Email string `norm:"unique"`
	}
	n := newNorm(&Org{}, &Member{})
	if err := migrate.New(db, n).Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, err := gen.NewGenerator(norm.MySQL).FromDB(ctx, db, "models", "")
	if err != nil {
		t.Fatalf("FromDB: %v", err)
	}

	memberSrc, ok := out["member"]
	if !ok {
		t.Fatalf("no source for member; got %v", keys(out))
	}
	for _, want := range []string{
		"type Member struct {",
		"Id int",
		`norm:"pk,notnull"`,
		"Email *string", // unique but nullable ⇒ pointer
		`norm:"unique"`,
		"fk=Org", // foreign key picked up
	} {
		if !strings.Contains(memberSrc, want) {
			t.Errorf("member source missing %q, got:\n%s", want, memberSrc)
		}
	}
}

// TestDiff verifies Diff against live MySQL: empty when matching, DROP COLUMN
// for an extra column, and MODIFY COLUMN for a type change.
func TestDiff(t *testing.T) {
	ctx := context.Background()
	db := openMySQL(t)
	n := newNorm(&User{})
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

	if _, err := db.ExecContext(ctx, "ALTER TABLE user ADD COLUMN legacy VARCHAR(20)"); err != nil {
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

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
