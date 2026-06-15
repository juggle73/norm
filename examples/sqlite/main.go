// Example sqlite shows norm targeting a non-PostgreSQL dialect.
//
// Unlike the other examples it needs no database server: it uses the pure-Go
// SQLite driver (modernc.org/sqlite), so `go run .` works on its own.
//
// It demonstrates the multi-dialect features:
//   - Config.Dialect = norm.SQLite — "?" placeholders, SQLite DDL
//   - migrate.Sync against SQLite (sqlite_master + PRAGMA introspection)
//   - the builder CRUD cycle (Insert with RETURNING, Select, UPSERT)
//   - Config.QuoteIdentifiers — using the reserved word "order" as a table
//
// Run it with:
//
//	go run .
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"

	_ "modernc.org/sqlite"
)

type Account struct {
	Id    int64  `norm:"pk,dbType=INTEGER"`
	Email string `norm:"unique,notnull"`
	Plan  string `norm:"notnull,default='free'"`
}

func main() {
	ctx := context.Background()

	// A throwaway file database so the example is self-contained.
	dbPath := filepath.Join(mustTempDir(), "example.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Select the SQLite dialect. Everything norm generates — placeholders,
	// DDL, introspection — follows from this.
	orm := norm.NewNorm(&norm.Config{Dialect: norm.SQLite})
	orm.AddModel(&Account{}, "account")

	mig := migrate.New(db, orm)

	fmt.Println("# CreateTableSQL (SQLite types)")
	fmt.Println(mig.CreateTableSQL("account"))

	fmt.Println("\n# Sync")
	if err := mig.Sync(ctx); err != nil {
		log.Fatalf("sync: %v", err)
	}
	fmt.Println("done")

	// INSERT ... RETURNING (SQLite ≥ 3.35 supports RETURNING).
	acc := Account{Email: "ann@example.com", Plan: "pro"}
	m, _ := orm.M(&acc)
	insertSQL, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
	fmt.Printf("\n# INSERT (note the ? placeholders)\n%s\n", insertSQL)
	if err := db.QueryRowContext(ctx, insertSQL, vals...).Scan(&acc.Id); err != nil {
		log.Fatalf("insert: %v", err)
	}
	fmt.Printf("inserted id=%d\n", acc.Id)

	// UPSERT — INSERT ... ON CONFLICT ... DO UPDATE (shared with PostgreSQL).
	dup := Account{Email: "ann@example.com", Plan: "enterprise"}
	m, _ = orm.M(&dup)
	upsertSQL, vals, _ := m.Insert(norm.Exclude("id"), norm.OnConflict("email").DoUpdate("plan"))
	fmt.Printf("\n# UPSERT\n%s\n", upsertSQL)
	if _, err := db.ExecContext(ctx, upsertSQL, vals...); err != nil {
		log.Fatalf("upsert: %v", err)
	}

	// SELECT it back through the builder's scan pointers.
	var got Account
	m, _ = orm.M(&got)
	selectSQL, args, _ := m.Select(norm.Where("email = ?", "ann@example.com"))
	if err := db.QueryRowContext(ctx, selectSQL, args...).Scan(m.Pointers()...); err != nil {
		log.Fatalf("select: %v", err)
	}
	fmt.Printf("\n# SELECT\n%s\n→ %+v\n", selectSQL, got)

	quotingDemo(ctx)
}

// quotingDemo shows Config.QuoteIdentifiers, which lets a reserved word like
// "order" be used as a table name.
func quotingDemo(ctx context.Context) {
	dbPath := filepath.Join(mustTempDir(), "quoting.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	type Order struct {
		Id    int64 `norm:"pk,dbType=INTEGER"`
		Total int   `norm:"notnull"`
	}

	orm := norm.NewNorm(&norm.Config{Dialect: norm.SQLite, QuoteIdentifiers: true})
	orm.AddModel(&Order{}, "order") // "order" is a reserved word
	mig := migrate.New(db, orm)

	fmt.Println("\n# QuoteIdentifiers — reserved word table \"order\"")
	fmt.Println(mig.CreateTableSQL("order"))
	if err := mig.Sync(ctx); err != nil {
		log.Fatalf("sync: %v", err)
	}

	o := Order{Total: 100}
	m, _ := orm.M(&o)
	insertSQL, vals, _ := m.Insert(norm.Exclude("id"))
	fmt.Println(insertSQL)
	if _, err := db.ExecContext(ctx, insertSQL, vals...); err != nil {
		log.Fatalf("insert into order: %v", err)
	}
	fmt.Println("inserted into \"order\" successfully")
}

func mustTempDir() string {
	dir, err := os.MkdirTemp("", "norm-sqlite-example")
	if err != nil {
		log.Fatalf("temp dir: %v", err)
	}
	return dir
}
