// Example upsert demonstrates PostgreSQL INSERT ... ON CONFLICT via norm.
//
// norm only builds the SQL and args; you run it. The conflict target must be
// backed by a unique constraint/index in the schema (norm's migrate creates
// one for `unique` columns; composite indexes you create yourself).
//
//	docker compose up -d
//	go run .
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

// Setting is upserted by its unique key.
type Setting struct {
	Id    int64  `norm:"pk,dbType=bigserial"`
	Key   string `norm:"unique,notnull"`
	Value string `norm:"notnull"`
}

// ExternalEntity is upserted by a composite (provider, external_id) key.
type ExternalEntity struct {
	Id         int64  `norm:"pk,dbType=bigserial"`
	Provider   string `norm:"notnull"`
	ExternalId string `norm:"notnull"`
	Payload    string `norm:"notnull"`
}

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://norm:norm@localhost:5432/norm?sslmode=disable"
}

func main() {
	ctx := context.Background()

	orm := norm.NewNorm(nil)
	orm.AddModel(&Setting{}, "settings")
	orm.AddModel(&ExternalEntity{}, "external_entities")

	db, err := sql.Open("pgx", dsn())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := migrate.New(db, orm).Sync(ctx); err != nil {
		log.Fatalf("sync schema: %v", err)
	}
	// norm's migrate doesn't generate composite unique indexes — create it
	// ourselves so the (provider, external_id) conflict target is valid.
	if _, err := db.ExecContext(ctx,
		`CREATE UNIQUE INDEX IF NOT EXISTS external_entities_provider_external_id_key
		 ON external_entities (provider, external_id)`); err != nil {
		log.Fatalf("create composite index: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn())
	if err != nil {
		log.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, "TRUNCATE settings, external_entities RESTART IDENTITY"); err != nil {
		log.Fatalf("truncate: %v", err)
	}

	// 1. First insert of key="theme" — row is created.
	fmt.Println("# DO UPDATE on a single unique key")
	upsertSetting(ctx, pool, orm, "theme", "dark")
	// 2. Same key again, new value — DO UPDATE overwrites it (no duplicate row).
	upsertSetting(ctx, pool, orm, "theme", "light")
	dumpSettings(ctx, pool, orm)

	// 3. DO NOTHING — conflicting insert is silently skipped.
	fmt.Println("\n# DO NOTHING")
	s := Setting{Key: "theme", Value: "ignored"}
	m, _ := orm.M(&s)
	query, vals, err := m.Insert(norm.Exclude("id"), norm.OnConflict("key").DoNothing())
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	fmt.Println("  SQL:", query)
	tag, err := pool.Exec(ctx, query, vals...)
	if err != nil {
		log.Fatalf("exec: %v", err)
	}
	fmt.Printf("  rows affected: %d (0 = existing row left untouched)\n", tag.RowsAffected())

	// 4. Composite conflict target + RETURNING.
	fmt.Println("\n# Composite conflict target with RETURNING")
	upsertExternal(ctx, pool, orm, "github", "42", `{"stars":1}`)
	upsertExternal(ctx, pool, orm, "github", "42", `{"stars":2}`) // same key → update
	dumpExternal(ctx, pool, orm)
}

// upsertSetting inserts or updates a setting by its unique key, returning the id.
func upsertSetting(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm, key, value string) {
	s := Setting{Key: key, Value: value}
	m, _ := orm.M(&s)
	query, vals, err := m.Insert(
		norm.Exclude("id"),
		norm.OnConflict("key").DoUpdate("value"),
		norm.Returning("id"),
	)
	if err != nil {
		log.Fatalf("build setting upsert: %v", err)
	}
	fmt.Println("  SQL:", query)
	if err := pool.QueryRow(ctx, query, vals...).Scan(m.Pointer("Id")); err != nil {
		log.Fatalf("upsert setting: %v", err)
	}
	fmt.Printf("  upserted %q=%q (id=%d)\n", key, value, s.Id)
}

func upsertExternal(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm, provider, externalId, payload string) {
	e := ExternalEntity{Provider: provider, ExternalId: externalId, Payload: payload}
	m, _ := orm.M(&e)
	query, vals, err := m.Insert(
		norm.Exclude("id"),
		norm.OnConflict("provider", "external_id").DoUpdate("payload"),
		norm.Returning("id"),
	)
	if err != nil {
		log.Fatalf("build external upsert: %v", err)
	}
	fmt.Println("  SQL:", query)
	if err := pool.QueryRow(ctx, query, vals...).Scan(m.Pointer("Id")); err != nil {
		log.Fatalf("upsert external: %v", err)
	}
	fmt.Printf("  upserted %s/%s payload=%s (id=%d)\n", provider, externalId, payload, e.Id)
}

func dumpSettings(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm) {
	var s Setting
	m, _ := orm.M(&s)
	rows, err := pool.Query(ctx, fmt.Sprintf("SELECT %s FROM %s ORDER BY id", m.Fields(), m.Table()))
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(m.Pointers()...); err != nil {
			log.Fatalf("scan: %v", err)
		}
		fmt.Printf("  row: id=%d key=%q value=%q\n", s.Id, s.Key, s.Value)
	}
}

func dumpExternal(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm) {
	var e ExternalEntity
	m, _ := orm.M(&e)
	rows, err := pool.Query(ctx, fmt.Sprintf("SELECT %s FROM %s ORDER BY id", m.Fields(), m.Table()))
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(m.Pointers()...); err != nil {
			log.Fatalf("scan: %v", err)
		}
		fmt.Printf("  row: id=%d %s/%s payload=%s\n", e.Id, e.Provider, e.ExternalId, e.Payload)
	}
}
