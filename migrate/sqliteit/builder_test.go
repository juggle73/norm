package sqliteit

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

// Account exercises the full type range through the live driver: int, string,
// bool, float, a JSON struct field, and time.Time.
type Account struct {
	Id        int       `norm:"pk"`
	Name      string    `norm:"notnull"`
	Active    bool      `norm:"notnull"`
	Score     float64   `norm:"notnull"`
	Prefs     Prefs     `norm:"notnull"`
	CreatedAt time.Time `norm:"notnull"`
}

// Prefs is a struct field, which norm serializes as JSON.
type Prefs struct {
	Role string `json:"role"`
}

func setupAccounts(t *testing.T) (*norm.Norm, *sql.DB) {
	t.Helper()
	db := openDB(t)
	n := newNorm(&Account{})
	if err := migrate.New(db, n).Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return n, db
}

// TestBuilderInsertSelectRoundTrip runs norm's own Insert and Select output
// against the live database and scans the row back through m.Pointers(),
// verifying bool, JSON and time.Time survive the round-trip.
func TestBuilderInsertSelectRoundTrip(t *testing.T) {
	n, db := setupAccounts(t)
	now := time.Now().UTC().Truncate(time.Second)

	in := Account{Id: 1, Name: "Ann", Active: true, Score: 9.5,
		Prefs: Prefs{Role: "admin"}, CreatedAt: now}
	mIn, _ := n.M(&in)
	sql, vals, err := mIn.Insert()
	if err != nil {
		t.Fatalf("build insert: %v", err)
	}
	if _, err := db.Exec(sql, vals...); err != nil {
		t.Fatalf("exec insert: %v\nsql: %s", err, sql)
	}

	var got Account
	mOut, _ := n.M(&got)
	sql, args, err := mOut.Select(norm.Where("id = ?", 1))
	if err != nil {
		t.Fatalf("build select: %v", err)
	}
	if err := db.QueryRow(sql, args...).Scan(mOut.Pointers()...); err != nil {
		t.Fatalf("scan: %v\nsql: %s", err, sql)
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

// TestBuilderUpdate verifies m.Update output mutates the live row.
func TestBuilderUpdate(t *testing.T) {
	n, db := setupAccounts(t)
	seed(t, n, db, Account{Id: 1, Name: "Ann", Active: true, Score: 1,
		Prefs: Prefs{}, CreatedAt: time.Now()})

	upd := Account{Id: 1, Name: "Ann2", Active: false, Score: 2,
		Prefs: Prefs{}, CreatedAt: time.Now()}
	m, _ := n.M(&upd)
	sql, vals, err := m.Update(norm.Exclude("id"), norm.Where("id = ?", 1))
	if err != nil {
		t.Fatalf("build update: %v", err)
	}
	if _, err := db.Exec(sql, vals...); err != nil {
		t.Fatalf("exec update: %v", err)
	}

	var name string
	var active bool
	if err := db.QueryRow("SELECT name, active FROM account WHERE id=1").Scan(&name, &active); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if name != "Ann2" || active {
		t.Errorf("update not applied: name=%q active=%v", name, active)
	}
}

// TestBuilderUpsert verifies OnConflict DoUpdate against the live database.
func TestBuilderUpsert(t *testing.T) {
	n, db := setupAccounts(t)
	seed(t, n, db, Account{Id: 1, Name: "Ann", Active: true, Score: 1,
		Prefs: Prefs{}, CreatedAt: time.Now()})

	conflict := Account{Id: 1, Name: "Updated", Active: true, Score: 1,
		Prefs: Prefs{}, CreatedAt: time.Now()}
	m, _ := n.M(&conflict)
	sql, vals, err := m.Insert(norm.OnConflict("id").DoUpdate("name"))
	if err != nil {
		t.Fatalf("build upsert: %v", err)
	}
	if _, err := db.Exec(sql, vals...); err != nil {
		t.Fatalf("exec upsert: %v\nsql: %s", err, sql)
	}

	var name string
	if err := db.QueryRow("SELECT name FROM account WHERE id=1").Scan(&name); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if name != "Updated" {
		t.Errorf("upsert did not update name, got %q", name)
	}
}

// TestBuilderUpsertDoNothing verifies OnConflict DoNothing leaves the row
// unchanged on a conflict.
func TestBuilderUpsertDoNothing(t *testing.T) {
	n, db := setupAccounts(t)
	seed(t, n, db, Account{Id: 1, Name: "Ann", Active: true, Score: 1,
		Prefs: Prefs{}, CreatedAt: time.Now()})

	conflict := Account{Id: 1, Name: "Ignored", Active: true, Score: 1,
		Prefs: Prefs{}, CreatedAt: time.Now()}
	m, _ := n.M(&conflict)
	sql, vals, err := m.Insert(norm.OnConflict("id").DoNothing())
	if err != nil {
		t.Fatalf("build upsert: %v", err)
	}
	if _, err := db.Exec(sql, vals...); err != nil {
		t.Fatalf("exec upsert: %v\nsql: %s", err, sql)
	}

	var name string
	if err := db.QueryRow("SELECT name FROM account WHERE id=1").Scan(&name); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if name != "Ann" {
		t.Errorf("DoNothing changed the row, got %q", name)
	}
}

// TestBuilderDelete verifies m.Delete output removes the live row.
func TestBuilderDelete(t *testing.T) {
	n, db := setupAccounts(t)
	seed(t, n, db, Account{Id: 1, Name: "Ann", Active: true, Score: 1,
		Prefs: Prefs{}, CreatedAt: time.Now()})

	m, _ := n.M(&Account{})
	sql, args, err := m.Delete(norm.Where("id = ?", 1))
	if err != nil {
		t.Fatalf("build delete: %v", err)
	}
	if _, err := db.Exec(sql, args...); err != nil {
		t.Fatalf("exec delete: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM account").Scan(&count); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
}

// TestBuilderReturning verifies a RETURNING clause executes on SQLite.
func TestBuilderReturning(t *testing.T) {
	n, db := setupAccounts(t)

	in := Account{Id: 7, Name: "Ret", Active: true, Score: 1,
		Prefs: Prefs{}, CreatedAt: time.Now()}
	m, _ := n.M(&in)
	sql, vals, err := m.Insert(norm.Returning("Id, Name"))
	if err != nil {
		t.Fatalf("build insert: %v", err)
	}
	var id int
	var name string
	if err := db.QueryRow(sql, vals...).Scan(&id, &name); err != nil {
		t.Fatalf("returning scan: %v\nsql: %s", err, sql)
	}
	if id != 7 || name != "Ret" {
		t.Errorf("RETURNING mismatch: id=%d name=%q", id, name)
	}
}

// seed inserts an account row using norm's builder.
func seed(t *testing.T, n *norm.Norm, db *sql.DB, a Account) {
	t.Helper()
	m, _ := n.M(&a)
	sql, vals, err := m.Insert()
	if err != nil {
		t.Fatalf("seed build: %v", err)
	}
	if _, err := db.Exec(sql, vals...); err != nil {
		t.Fatalf("seed exec: %v", err)
	}
}
