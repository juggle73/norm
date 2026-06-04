package norm

import (
	"reflect"
	"testing"
)

// upsertUser mirrors the README "users" model for UPSERT tests.
type upsertUser struct {
	Id        int    `norm:"pk"`
	Name      string `norm:"notnull"`
	Email     string `norm:"unique,notnull"`
	UpdatedAt string
}

// upsertExternal exercises composite conflict targets.
type upsertExternal struct {
	Id         int    `norm:"pk"`
	Provider   string `norm:"notnull"`
	ExternalId string `norm:"notnull"`
	Payload    string
}

func newUpsertUser(t *testing.T) (*Model, *upsertUser) {
	t.Helper()
	n := NewNorm(nil)
	u := &upsertUser{}
	n.AddModel(u, "users")
	mm, err := n.M(u)
	if err != nil {
		t.Fatalf("M: %v", err)
	}
	return mm, u
}

func TestInsertOnConflictDoNothing(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com"}

	sql, args, err := m.Insert(
		Exclude("id,updated_at"),
		OnConflict("email").DoNothing(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "INSERT INTO users (name, email) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING"
	if sql != want {
		t.Errorf("sql mismatch:\n got: %s\nwant: %s", sql, want)
	}
	if !reflect.DeepEqual(args, []any{"John", "john@example.com"}) {
		t.Errorf("args mismatch: %#v", args)
	}
}

func TestInsertOnConflictDoUpdate(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com", UpdatedAt: "now"}

	sql, args, err := m.Insert(
		Exclude("id"),
		OnConflict("email").DoUpdate("name", "updated_at"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "INSERT INTO users (name, email, updated_at) VALUES ($1, $2, $3) " +
		"ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = EXCLUDED.updated_at"
	if sql != want {
		t.Errorf("sql mismatch:\n got: %s\nwant: %s", sql, want)
	}
	if !reflect.DeepEqual(args, []any{"John", "john@example.com", "now"}) {
		t.Errorf("args mismatch: %#v", args)
	}
}

func TestInsertOnConflictComposite(t *testing.T) {
	n := NewNorm(nil)
	e := &upsertExternal{Provider: "github", ExternalId: "42", Payload: "{}"}
	n.AddModel(e, "external_entities")
	m, err := n.M(e)
	if err != nil {
		t.Fatalf("M: %v", err)
	}

	sql, _, err := m.Insert(
		Exclude("id"),
		OnConflict("provider", "external_id").DoUpdate("payload"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "INSERT INTO external_entities (provider, external_id, payload) VALUES ($1, $2, $3) " +
		"ON CONFLICT (provider, external_id) DO UPDATE SET payload = EXCLUDED.payload"
	if sql != want {
		t.Errorf("sql mismatch:\n got: %s\nwant: %s", sql, want)
	}
}

func TestInsertOnConflictWithReturning(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com"}

	sql, args, err := m.Insert(
		Exclude("id,updated_at"),
		OnConflict("email").DoUpdate("name"),
		Returning("id"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "INSERT INTO users (name, email) VALUES ($1, $2) " +
		"ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name RETURNING id"
	if sql != want {
		t.Errorf("sql mismatch:\n got: %s\nwant: %s", sql, want)
	}
	if !reflect.DeepEqual(args, []any{"John", "john@example.com"}) {
		t.Errorf("args mismatch: %#v", args)
	}
}

func TestInsertOnConflictWithExclude(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com", UpdatedAt: "now"}

	// Exclude updated_at from the insert; it must not appear in the column list.
	sql, args, err := m.Insert(
		Exclude("id,updated_at"),
		OnConflict("email").DoNothing(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "INSERT INTO users (name, email) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING"
	if sql != want {
		t.Errorf("sql mismatch:\n got: %s\nwant: %s", sql, want)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %#v", args)
	}
}

func TestInsertOnConflictOrderIndependent(t *testing.T) {
	m1, u1 := newUpsertUser(t)
	*u1 = upsertUser{Name: "John", Email: "john@example.com"}
	sqlA, _, err := m1.Insert(
		Exclude("id,updated_at"),
		OnConflict("email").DoUpdate("name"),
		Returning("id"),
	)
	if err != nil {
		t.Fatalf("A: %v", err)
	}

	m2, u2 := newUpsertUser(t)
	*u2 = upsertUser{Name: "John", Email: "john@example.com"}
	sqlB, _, err := m2.Insert(
		Returning("id"),
		OnConflict("email").DoUpdate("name"),
		Exclude("id,updated_at"),
	)
	if err != nil {
		t.Fatalf("B: %v", err)
	}

	if sqlA != sqlB {
		t.Errorf("option order changed SQL:\n A: %s\n B: %s", sqlA, sqlB)
	}
}

func TestInsertOnConflictEmptyColumns(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com"}

	_, _, err := m.Insert(
		Exclude("id,updated_at"),
		OnConflict().DoNothing(),
	)
	if err == nil {
		t.Fatal("expected error for empty conflict columns, got nil")
	}
}

func TestInsertOnConflictEmptyUpdateColumns(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com"}

	_, _, err := m.Insert(
		Exclude("id,updated_at"),
		OnConflict("email").DoUpdate(),
	)
	if err == nil {
		t.Fatal("expected error for empty update columns, got nil")
	}
}

func TestInsertOnConflictDuplicateOption(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com"}

	_, _, err := m.Insert(
		Exclude("id,updated_at"),
		OnConflict("email").DoNothing(),
		OnConflict("name").DoNothing(),
	)
	if err == nil {
		t.Fatal("expected error for multiple OnConflict options, got nil")
	}
}

func TestInsertOnConflictUnknownColumn(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com"}

	t.Run("unknown conflict column", func(t *testing.T) {
		_, _, err := m.Insert(
			Exclude("id,updated_at"),
			OnConflict("nope").DoNothing(),
		)
		if err == nil {
			t.Fatal("expected error for unknown conflict column, got nil")
		}
	})

	t.Run("unknown update column", func(t *testing.T) {
		_, _, err := m.Insert(
			Exclude("id,updated_at"),
			OnConflict("email").DoUpdate("nope"),
		)
		if err == nil {
			t.Fatal("expected error for unknown update column, got nil")
		}
	})
}

func TestInsertOnConflictAcceptsAnyNameFormat(t *testing.T) {
	m, u := newUpsertUser(t)
	*u = upsertUser{Name: "John", Email: "john@example.com", UpdatedAt: "now"}

	// "Email" (struct field), "UpdatedAt" (struct field) resolve to db names.
	sql, _, err := m.Insert(
		Exclude("id"),
		OnConflict("Email").DoUpdate("Name", "UpdatedAt"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "INSERT INTO users (name, email, updated_at) VALUES ($1, $2, $3) " +
		"ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = EXCLUDED.updated_at"
	if sql != want {
		t.Errorf("sql mismatch:\n got: %s\nwant: %s", sql, want)
	}
}
