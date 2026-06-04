// Example migration_sync drives norm's migrate package against a live database.
//
// It shows the three building blocks:
//   - CreateTableSQL — preview the CREATE TABLE for a model
//   - Diff           — the full SQL needed to bring the DB in line with the structs
//   - Sync           — apply the safe subset (create tables, add missing columns)
//
// To make the "add missing column" behaviour visible, the example deliberately
// drops a column after the first Sync and shows that the next Diff/Sync restores it.
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
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

type Account struct {
	Id        int64     `norm:"pk"`
	Email     string    `norm:"unique,notnull"`
	Plan      string    `norm:"notnull,default='free'"`
	CreatedAt time.Time `norm:"notnull"`
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
	orm.AddModel(&Account{}, "accounts")

	db, err := sql.Open("pgx", dsn())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Clean slate so the example is repeatable.
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS accounts"); err != nil {
		log.Fatalf("drop: %v", err)
	}

	mig := migrate.New(db, orm)

	fmt.Println("# CreateTableSQL preview")
	fmt.Println(mig.CreateTableSQL("accounts"))

	fmt.Println("\n# Diff before sync (empty database)")
	printDiff(ctx, mig)

	fmt.Println("\n# Sync — applying changes")
	if err := mig.Sync(ctx); err != nil {
		log.Fatalf("sync: %v", err)
	}
	fmt.Println("done")

	fmt.Println("\n# Diff after sync (schema matches structs)")
	printDiff(ctx, mig)

	// Simulate schema drift: a column present in the struct is missing in the DB.
	fmt.Println("\n# Simulating drift: DROP COLUMN plan")
	if _, err := db.ExecContext(ctx, "ALTER TABLE accounts DROP COLUMN plan"); err != nil {
		log.Fatalf("drop column: %v", err)
	}

	fmt.Println("\n# Diff now shows the missing column")
	printDiff(ctx, mig)

	fmt.Println("\n# Sync restores it")
	if err := mig.Sync(ctx); err != nil {
		log.Fatalf("sync: %v", err)
	}
	printDiff(ctx, mig)
}

func printDiff(ctx context.Context, mig *migrate.Migrate) {
	diff, err := mig.Diff(ctx)
	if err != nil {
		log.Fatalf("diff: %v", err)
	}
	if diff == "" {
		fmt.Println("  (no changes — in sync)")
		return
	}
	fmt.Println(diff)
}
